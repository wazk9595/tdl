package send

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"golang.org/x/sync/errgroup"

	"github.com/iyear/tdl/core/uploader"
)

var (
	ErrRemoteMissing  = errors.New("对应消息已不存在")
	ErrNeedsTranscode = errors.New("需要转码；明确允许后才会处理")
)

type Backend interface {
	Upload(context.Context, Entry, *uploader.Hooks, func(uploader.ProgressState)) error
	Verify(context.Context, Entry) error
	Find(context.Context, Entry, Task) (int, error)
}

type EngineOptions struct {
	Concurrency    int
	Retries        int
	ResendMissing  bool
	RetryUncertain bool
	Emit           func(string, ...any)
}

func newRandomID() (int64, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return 0, err
	}
	id := int64(binary.LittleEndian.Uint64(bytes[:]) & 0x7fffffffffffffff)
	if id == 0 {
		id = 1
	}
	return id, nil
}

func Ref(media tg.MessageMediaClass) (MediaRef, error) {
	switch m := media.(type) {
	case *tg.MessageMediaDocument:
		if d, ok := m.Document.(*tg.Document); ok {
			r := MediaRef{Kind: kindDocument, ID: d.ID, Size: d.Size}
			for _, item := range d.Attributes {
				if v, ok := item.(*tg.DocumentAttributeVideo); ok {
					r.Kind = kindVideo
					r.Duration = v.Duration
					r.Width = v.W
					r.Height = v.H
				}
			}
			return r, nil
		}
	case *tg.MessageMediaPhoto:
		if p, ok := m.Photo.(*tg.Photo); ok {
			return MediaRef{Kind: kindPhoto, ID: p.ID}, nil
		}
	}
	return MediaRef{}, fmt.Errorf("服务器未返回可记录的媒体")
}

func Execute(ctx context.Context, store *Store, scan Scan, backend Backend, opts EngineOptions) (Summary, error) {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Emit == nil {
		opts.Emit = func(string, ...any) {}
	}
	if err := store.Update(func(t *Task) error {
		for _, file := range scan.Files {
			if old := t.Entries[file.Hash]; old != nil {
				old.File.Path = file.Path
				old.File.Name = file.Name
				continue
			}
			id, err := newRandomID()
			if err != nil {
				return err
			}
			t.Entries[file.Hash] = &Entry{File: file, Status: Pending, RandomID: id, Updated: time.Now()}
		}
		return nil
	}); err != nil {
		return Summary{}, err
	}
	var fatal []error
	var mu sync.Mutex
	wg, wctx := errgroup.WithContext(ctx)
	wg.SetLimit(opts.Concurrency)
	for _, file := range scan.Files {
		if wctx.Err() != nil {
			break
		}
		wg.Go(func() error {
			if err := process(wctx, store, file.Hash, backend, opts); err != nil {
				mu.Lock()
				fatal = append(fatal, err)
				mu.Unlock()
				return err
			}
			return nil
		})
	}
	_ = wg.Wait()
	task := store.Snapshot()
	summary := Summary{TaskID: task.ID, Destination: task.Destination.Name, Scanned: scan.Count, Duplicates: scan.Duplicates, Ignored: scan.Ignored, Expected: len(scan.Files), Issues: append([]string{}, scan.Errors...)}
	for _, file := range scan.Files {
		e := task.Entries[file.Hash]
		if e.Status == Verified {
			summary.Verified++
			switch e.Media.Kind {
			case kindVideo:
				summary.Videos++
			case kindPhoto:
				summary.Photos++
			default:
				summary.Documents++
			}
			if e.Transcoded {
				summary.Transcoded++
			}
		} else {
			summary.Unresolved++
			summary.Issues = append(summary.Issues, fmt.Sprintf("%s：%s (%s)", file.Name, e.Error, e.Status))
		}
	}
	if ctx.Err() != nil {
		fatal = append(fatal, ctx.Err())
	}
	if summary.Unresolved > 0 || len(scan.Errors) > 0 {
		fatal = append(fatal, fmt.Errorf("有 %d 个文件待处理，%d 个扫描问题；成功项已记录，下次运行会先核对再跳过", summary.Unresolved, len(scan.Errors)))
	}
	return summary, errors.Join(fatal...)
}

func process(ctx context.Context, store *Store, hash string, backend Backend, opts EngineOptions) error {
	entry := store.Entry(hash)
	mark := func(status string, err error) error {
		return store.Change(hash, func(e *Entry) {
			e.Status = status
			e.Error = ""
			if err != nil {
				e.Error = err.Error()
			}
		})
	}
	if entry.MessageID > 0 {
		err := backend.Verify(ctx, entry)
		if err == nil {
			opts.Emit("已核对，跳过：%s", entry.File.Name)
			return mark(Verified, nil)
		}
		if !errors.Is(err, ErrRemoteMissing) || !opts.ResendMissing {
			status := Review
			if errors.Is(err, ErrRemoteMissing) {
				status = Missing
			}
			return mark(status, err)
		}
		id, err := newRandomID()
		if err != nil {
			return err
		}
		if err := store.Change(hash, func(e *Entry) { e.RandomID = id; e.MessageID = 0; e.Media = MediaRef{}; e.Status = Pending }); err != nil {
			return err
		}
		entry = store.Entry(hash)
	}
	if entry.Status == Sending || entry.Status == Uncertain {
		id, err := backend.Find(ctx, entry, store.Snapshot())
		if err != nil {
			return mark(Uncertain, err)
		}
		if id > 0 {
			if err := store.Claim(hash, id); err != nil {
				return err
			}
			return process(ctx, store, hash, backend, opts)
		}
		if !opts.RetryUncertain {
			return mark(Uncertain, fmt.Errorf("未找到可确认的消息，暂不重发；请核查后使用 --retry-uncertain"))
		}
	}
	for attempt := 0; attempt <= opts.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := store.Change(hash, func(e *Entry) { e.Status = Uploading; e.Error = ""; e.Attempts++ }); err != nil {
			return err
		}
		entry = store.Entry(hash)
		opts.Emit("正在发送：%s", entry.File.Name)
		lastProgress := time.Now()
		var progressMu sync.Mutex
		hooks := &uploader.Hooks{RandomID: entry.RandomID}
		hooks.BeforeSend = func(ctx context.Context, media tg.MessageMediaClass) error {
			digest, size, err := HashFile(ctx, entry.File.Path)
			if err != nil {
				return err
			}
			if digest != hash || size != entry.File.Size {
				return fmt.Errorf("发送前发现源文件改变，已停止该文件")
			}
			ref, err := Ref(media)
			if err != nil {
				return err
			}
			return store.Change(hash, func(e *Entry) {
				e.Media = ref
				e.Status = Sending
				e.Caption = filepath.Base(entry.File.Path)
				e.Transcoded = hooks.Transcoded
			})
		}
		hooks.Sent = func(_ context.Context, id int) error {
			return store.Claim(hash, id)
		}
		err := backend.Upload(ctx, entry, hooks, func(s uploader.ProgressState) {
			progressMu.Lock()
			defer progressMu.Unlock()
			if time.Since(lastProgress) >= 5*time.Second && s.Total > 0 {
				opts.Emit("上传 %s：%.0f%%", entry.File.Name, float64(s.Uploaded)*100/float64(s.Total))
				lastProgress = time.Now()
			}
		})
		current := store.Entry(hash)
		if current.MessageID > 0 {
			if verifyErr := backend.Verify(ctx, current); verifyErr == nil {
				opts.Emit("核对完成：%s", entry.File.Name)
				return mark(Verified, nil)
			} else {
				return mark(Review, verifyErr)
			}
		}
		if current.Status == Sending {
			if saveErr := mark(Uncertain, err); saveErr != nil {
				return saveErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			id, findErr := backend.Find(ctx, store.Entry(hash), store.Snapshot())
			if findErr != nil {
				return mark(Uncertain, findErr)
			}
			if id > 0 {
				if err := store.Claim(hash, id); err != nil {
					return err
				}
				return process(ctx, store, hash, backend, opts)
			}
			return nil
		}
		if errors.Is(err, ErrNeedsTranscode) {
			return mark(NeedsTranscode, err)
		}
		if err == nil {
			err = fmt.Errorf("发送未返回可核对的消息回执")
		}
		if saveErr := mark(Failed, err); saveErr != nil {
			return saveErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt < opts.Retries {
			opts.Emit("稍后重试：%s (%v)", entry.File.Name, err)
			timer := time.NewTimer(time.Second * time.Duration(1<<attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}
