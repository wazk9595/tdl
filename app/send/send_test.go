package send

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/uploader"
)

func TestTargetLinks(t *testing.T) {
	for _, test := range []struct {
		link, peer string
		topic, msg int
		explicit   bool
	}{
		{"https://t.me/c/123/42", "123", 42, 42, false},
		{"https://t.me/c/123/42/99", "123", 42, 99, true},
		{"https://t.me/c/123/99?single&thread=42", "123", 42, 99, true},
		{"https://t.me/group_name/42/99", "group_name", 42, 99, true},
	} {
		got, err := ParseTarget(test.link)
		if err != nil {
			t.Fatal(err)
		}
		if got.Peer != test.peer || got.Candidate != test.topic || got.Message != test.msg || got.Explicit != test.explicit {
			t.Fatalf("%s: %+v", test.link, got)
		}
	}
	for _, link := range []string{"https://t.me.evil/c/123/42", "https://evil@t.me/c/123/42", "https://t.me/c/123/0", "https://t.me/c/123/42/99?thread=43", "https://t.me/c/abc/42", "https://t.me/c/123/2147483648", "https://t.me/group_name"} {
		if _, err := ParseTarget(link); err == nil {
			t.Fatalf("accepted %s", link)
		}
	}
	if got, err := ParseTarget("me"); err != nil || !got.Self {
		t.Fatal(got, err)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanTest(t *testing.T, root string) Scan {
	t.Helper()
	scan, err := ScanFolder(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Errors) > 0 {
		t.Fatal(scan.Errors)
	}
	return scan
}

func initialTask(scan Scan) Task {
	dest := Destination{PeerID: 123, Topic: 42, Name: "test", FloorID: 5}
	return Task{Version: 1, ID: TaskID(scan.Root, "default", 9, dest), Root: scan.Root, Namespace: "default", AccountID: 9, Destination: dest, Created: time.Now(), Entries: map[string]*Entry{}}
}

func openTestStore(t *testing.T, dir string, scan Scan) *Store {
	t.Helper()
	s, err := OpenStore(dir, initialTask(scan))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestScanHashesNotNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/same.txt", "first")
	writeFile(t, root, "b/same.txt", "second")
	writeFile(t, root, "copy.txt", "first")
	writeFile(t, root, "download.part", "incomplete")
	writeFile(t, root, ".git/config", "ignore")
	scan := scanTest(t, root)
	if scan.Count != 3 || len(scan.Files) != 2 || scan.Duplicates != 1 || scan.Ignored != 2 {
		t.Fatalf("%+v", scan)
	}
	if scan.Files[0].Hash == scan.Files[1].Hash {
		t.Fatal("different content collapsed")
	}
	if TaskID(scan.Root, "default", 9, Destination{PeerID: 1, Topic: 1}) == TaskID(scan.Root, "default", 10, Destination{PeerID: 1, Topic: 1}) {
		t.Fatal("account not isolated")
	}
	if TaskID(scan.Root, "default", 9, Destination{PeerID: 1, Topic: 1}) == TaskID(scan.Root, "default", 9, Destination{PeerID: 1, Topic: 2}) {
		t.Fatal("topic not isolated")
	}
}

type fakeBackend struct {
	mu          sync.Mutex
	remote      map[int]MediaRef
	uploads     int
	loseReceipt bool
	find        bool
	mutate      bool
	needs       bool
}

func newFake() *fakeBackend { return &fakeBackend{remote: map[int]MediaRef{}} }
func (b *fakeBackend) Upload(ctx context.Context, e Entry, h *uploader.Hooks, _ func(uploader.ProgressState)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.needs {
		return ErrNeedsTranscode
	}
	b.uploads++
	id := b.uploads + 100
	media := &tg.MessageMediaDocument{Document: &tg.Document{ID: int64(id), Size: e.File.Size}}
	if b.mutate {
		if err := os.WriteFile(e.File.Path, []byte("changed"), 0o600); err != nil {
			return err
		}
		b.mutate = false
	}
	if err := h.BeforeSend(ctx, media); err != nil {
		return err
	}
	ref, err := Ref(media)
	if err != nil {
		return err
	}
	b.remote[id] = ref
	if b.loseReceipt {
		return errors.New("connection lost after server accepted")
	}
	return h.Sent(ctx, id)
}

func (b *fakeBackend) Verify(_ context.Context, e Entry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.remote[e.MessageID]
	if !ok {
		return ErrRemoteMissing
	}
	if r != e.Media {
		return errors.New("media changed")
	}
	return nil
}

func (b *fakeBackend) Find(_ context.Context, e Entry, _ Task) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.find {
		for id, ref := range b.remote {
			if ref == e.Media {
				return id, nil
			}
		}
	}
	return 0, nil
}

func TestSendResumeAndChangedSameName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/same.txt", "first")
	writeFile(t, root, "b/same.txt", "second")
	writeFile(t, root, "copy.txt", "first")
	scan := scanTest(t, root)
	dir := t.TempDir()
	store := openTestStore(t, dir, scan)
	backend := newFake()
	summary, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 2})
	if err != nil || summary.Verified != 2 || summary.Duplicates != 1 {
		t.Fatal(summary, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, dir, scan)
	defer store.Close()
	summary, err = Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 2})
	if err != nil || summary.Verified != 2 || backend.uploads != 2 {
		t.Fatalf("repeat uploaded again: %+v %v uploads=%d", summary, err, backend.uploads)
	}
	writeFile(t, root, "a/same.txt", "new content")
	scan = scanTest(t, root)
	summary, err = Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 2})
	if err != nil || summary.Verified != 3 || backend.uploads != 3 {
		t.Fatal(summary, err, backend.uploads)
	}
}

func TestLostReceiptRecoveredAfterReopen(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	dir := t.TempDir()
	store := openTestStore(t, dir, scan)
	backend := newFake()
	backend.loseReceipt = true
	summary, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1})
	if err == nil || summary.Unresolved != 1 || store.Entry(scan.Files[0].Hash).Status != Uncertain {
		t.Fatal(summary, err)
	}
	id := store.Entry(scan.Files[0].Hash).RandomID
	_ = store.Close()
	store = openTestStore(t, dir, scan)
	defer store.Close()
	// An unconfirmed send is not automatically repeated, even if retries are set.
	_, _ = Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1, Retries: 2})
	if backend.uploads != 1 {
		t.Fatal("uncertain message blindly repeated")
	}
	backend.find = true
	summary, err = Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1})
	if err != nil || summary.Verified != 1 || backend.uploads != 1 || store.Entry(scan.Files[0].Hash).RandomID != id {
		t.Fatal(summary, err, backend.uploads)
	}
}

func TestDeletedMessageRequiresExplicitResend(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	store := openTestStore(t, t.TempDir(), scan)
	defer store.Close()
	backend := newFake()
	_, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	entry := store.Entry(scan.Files[0].Hash)
	delete(backend.remote, entry.MessageID)
	summary, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1})
	if err == nil || summary.Unresolved != 1 || backend.uploads != 1 {
		t.Fatal(summary, err)
	}
	summary, err = Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1, ResendMissing: true})
	if err != nil || summary.Verified != 1 || backend.uploads != 2 || store.Entry(scan.Files[0].Hash).RandomID == entry.RandomID {
		t.Fatal(summary, err)
	}
}

func TestSourceMutationStopsBeforeSend(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	store := openTestStore(t, t.TempDir(), scan)
	defer store.Close()
	backend := newFake()
	backend.mutate = true
	summary, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1})
	if err == nil || summary.Unresolved != 1 || len(backend.remote) != 0 {
		t.Fatal(summary, err)
	}
}

func TestNeedsTranscodeIsExplicit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	store := openTestStore(t, t.TempDir(), scan)
	defer store.Close()
	backend := newFake()
	backend.needs = true
	_, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1, Retries: 2})
	if err == nil || store.Entry(scan.Files[0].Hash).Status != NeedsTranscode || backend.uploads != 0 {
		t.Fatal(err)
	}
}

func TestJournalLockAndRollback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	dir := t.TempDir()
	store := openTestStore(t, dir, scan)
	if other, err := OpenStore(dir, initialTask(scan)); err == nil {
		other.Close()
		t.Fatal("concurrent task lock not enforced")
	}
	before := store.Snapshot()
	if err := store.Update(func(task *Task) error { task.AccountID = 99; return errors.New("abort") }); err == nil {
		t.Fatal("missing error")
	}
	store.Close()
	store = openTestStore(t, dir, scan)
	defer store.Close()
	if store.Snapshot().AccountID != before.AccountID {
		t.Fatal("failed transaction persisted")
	}
}

func TestValidateVideoReceipt(t *testing.T) {
	ref := MediaRef{Kind: kindVideo, ID: 7, Size: 100, Duration: 2, Width: 640, Height: 360}
	doc := &tg.Document{ID: 7, Size: 100, Thumbs: []tg.PhotoSizeClass{&tg.PhotoSize{}}, Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{Duration: 2, W: 640, H: 360, SupportsStreaming: true}}}
	msg := &tg.Message{ID: 10, PeerID: &tg.PeerChannel{ChannelID: 123}, ReplyTo: &tg.MessageReplyHeader{ForumTopic: true, ReplyToMsgID: 42}, Media: &tg.MessageMediaDocument{Video: true, Document: doc}}
	entry := Entry{Media: ref}
	dest := Destination{PeerID: 123, Topic: 42}
	if err := ValidateMessage(msg, entry, dest); err != nil {
		t.Fatal(err)
	}
	doc.Thumbs = nil
	if err := ValidateMessage(msg, entry, dest); err == nil {
		t.Fatal("accepted video without thumbnail")
	}
	if !errors.Is(ValidateMessage(nil, entry, dest), ErrRemoteMissing) {
		t.Fatal("deleted message not identified")
	}
}

func TestJournalCannotBeUploaded(t *testing.T) {
	root, err := CanonicalRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Join(root, "new", "tasks")} {
		if _, err := taskDirectory(root, dir); err == nil {
			t.Fatal("accepted task journal inside source folder")
		}
	}
	if _, err := taskDirectory(root, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownMessageResponseIsNotDeletion(t *testing.T) {
	if _, err := messageList(&tg.MessagesMessagesNotModified{}); err == nil {
		t.Fatal("unexpected response treated as an empty history")
	}
}

func TestCancelledTaskCanResume(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "one.txt", "one")
	scan := scanTest(t, root)
	store := openTestStore(t, t.TempDir(), scan)
	defer store.Close()
	backend := newFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Execute(ctx, store, scan, backend, EngineOptions{Concurrency: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result: %v", err)
	}
	if backend.uploads != 0 {
		t.Fatal("cancelled task uploaded")
	}
	if summary, err := Execute(context.Background(), store, scan, backend, EngineOptions{Concurrency: 1}); err != nil || summary.Verified != 1 {
		t.Fatal(summary, err)
	}
}
