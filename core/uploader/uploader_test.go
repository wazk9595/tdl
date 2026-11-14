package uploader

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gotd/td/tg"
)

type badFile struct{ *bytes.Reader }

func (f badFile) Name() string { return "bad.mp4" }
func (f badFile) Size() int64  { return f.Reader.Size() }

type unreadableFile struct{ badFile }

func (unreadableFile) Seek(int64, int) (int64, error) { return 0, errors.New("unreadable input") }

type testElem struct{ f File }

func (e testElem) File() File                                 { return e.f }
func (e testElem) Thumb() (File, bool)                        { return nil, false }
func (e testElem) Caption() (string, []tg.MessageEntityClass) { return "", nil }
func (e testElem) To() tg.InputPeerClass                      { return &tg.InputPeerSelf{} }
func (e testElem) Thread() int                                { return 0 }
func (e testElem) AsPhoto() bool                              { return false }

type testIter struct {
	elem Elem
	done bool
}

func (i *testIter) Next(context.Context) bool {
	if i.done {
		return false
	}
	i.done = true
	return true
}
func (i *testIter) Value() Elem { return i.elem }
func (i *testIter) Err() error  { return nil }

type testProgress struct{ doneErr error }

func (*testProgress) OnAdd(Elem)                   {}
func (*testProgress) OnUpload(Elem, ProgressState) {}
func (p *testProgress) OnDone(_ Elem, err error)   { p.doneErr = err }

// Cancellation must reach OnDone; otherwise --rm can delete a failed source.
func TestUploadReportsFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &testProgress{}
	u := New(Options{Iter: &testIter{elem: testElem{badFile{bytes.NewReader(nil)}}}, Progress: p})
	if err := u.Upload(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("returned: %v", err)
	}
	if !errors.Is(p.doneErr, context.Canceled) {
		t.Fatalf("OnDone got: %v", p.doneErr)
	}
}

func TestUploadReturnsOrdinaryError(t *testing.T) {
	p := &testProgress{}
	u := New(Options{Iter: &testIter{elem: testElem{unreadableFile{badFile{bytes.NewReader(nil)}}}}, Progress: p})
	if err := u.Upload(context.Background(), 1); err == nil {
		t.Fatal("failed upload reported success")
	}
	if p.doneErr == nil {
		t.Fatal("failed upload passed nil to OnDone")
	}
}

type thumbnailRPC struct{ small, big atomic.Int32 }

func (r *thumbnailRPC) UploadSaveFilePart(context.Context, *tg.UploadSaveFilePartRequest) (bool, error) {
	r.small.Add(1)
	return true, nil
}
func (r *thumbnailRPC) UploadSaveBigFilePart(context.Context, *tg.UploadSaveBigFilePartRequest) (bool, error) {
	r.big.Add(1)
	return true, nil
}

func TestThumbnailUsesSmallFileAPI(t *testing.T) {
	rpc := &thumbnailRPC{}
	file, err := uploadThumbnail(context.Background(), rpc, badFile{bytes.NewReader([]byte("thumbnail"))})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := file.(*tg.InputFile); !ok {
		t.Fatalf("thumbnail returned %T", file)
	}
	if rpc.small.Load() != 1 || rpc.big.Load() != 0 {
		t.Fatalf("small=%d big=%d", rpc.small.Load(), rpc.big.Load())
	}
}

func TestVideoMediaRequiresThumbnailAndStreaming(t *testing.T) {
	media := &tg.MessageMediaDocument{Video: true, Document: &tg.Document{Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{SupportsStreaming: true}}}}
	if _, err := validateVideoMedia(media, true, true); err == nil {
		t.Fatal("accepted missing thumbnail")
	}
	doc := media.Document.(*tg.Document)
	doc.Thumbs = []tg.PhotoSizeClass{&tg.PhotoSize{}}
	if _, err := validateVideoMedia(media, true, true); err != nil {
		t.Fatal(err)
	}
	doc.Attributes[0].(*tg.DocumentAttributeVideo).SupportsStreaming = false
	if _, err := validateVideoMedia(media, true, true); err == nil {
		t.Fatal("accepted missing streaming")
	}
}
