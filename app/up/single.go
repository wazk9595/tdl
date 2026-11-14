package up

import (
	"context"
	"errors"
	"html"
	"path/filepath"
	"strconv"

	"github.com/expr-lang/expr"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/uploader"
)

// Single reuses media preparation and upload validation for resumable tasks.
// The caller owns its client/pool and journals the supplied hooks.
func Single(ctx context.Context, client *tg.Client, manager *peers.Manager, path, chat string, topic int, mode, encoder string, threads int, asPhoto bool, hooks *uploader.Hooks, progress func(uploader.ProgressState)) error {
	to, err := expr.Compile(`""`)
	if err != nil {
		return err
	}
	caption, err := expr.Compile(strconv.Quote(html.EscapeString(filepath.Base(path))))
	if err != nil {
		return err
	}
	it := newIter([]*File{{File: path}}, to, caption, chat, topic, asPhoto, false, 0, manager, mode, encoder)
	it.quiet = true
	p := &singleProgress{progress: progress}
	err = uploader.New(uploader.Options{Client: client, Threads: threads, Iter: it, Progress: p, Hooks: hooks}).Upload(ctx, 1)
	return errors.Join(err, p.closeError)
}

type singleProgress struct {
	progress   func(uploader.ProgressState)
	closeError error
}

func (*singleProgress) OnAdd(uploader.Elem) {}
func (p *singleProgress) OnUpload(_ uploader.Elem, s uploader.ProgressState) {
	if p.progress != nil {
		p.progress(s)
	}
}

func (p *singleProgress) OnDone(elem uploader.Elem, _ error) {
	e := elem.(*iterElem)
	p.closeError = e.file.Close()
	if e.thumb != nil {
		p.closeError = errors.Join(p.closeError, e.thumb.Close())
	}
	if e.prepared != nil {
		p.closeError = errors.Join(p.closeError, e.prepared.Close())
	}
}
