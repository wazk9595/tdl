package up

import (
	"os"
	"path/filepath"

	"github.com/gotd/td/telegram/message/entity"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/uploader"
	"github.com/iyear/tdl/core/util/mediautil"
)

type iterElem struct {
	file    *uploaderFile
	thumb   *uploaderFile
	to      peers.Peer
	caption *entity.Builder
	thread  int

	asPhoto      bool
	remove       bool
	originalPath string
	prepared     *mediautil.PreparedVideo
	asFile       bool
}

func (e *iterElem) VideoInfo() *mediautil.VideoInfo {
	if e.prepared == nil {
		return nil
	}
	return e.prepared.Info
}

func (e *iterElem) AsFile() bool { return e.asFile }

func (e *iterElem) File() uploader.File {
	return e.file
}

func (e *iterElem) Thumb() (uploader.File, bool) {
	if e.thumb == nil {
		return nil, false
	}
	return e.thumb, true
}

func (e *iterElem) Caption() (string, []tg.MessageEntityClass) {
	return e.caption.Complete()
}

func (e *iterElem) To() tg.InputPeerClass {
	return e.to.InputPeer()
}

func (e *iterElem) Thread() int {
	return e.thread
}

func (e *iterElem) AsPhoto() bool {
	return e.asPhoto
}

type uploaderFile struct {
	*os.File
	size int64
	name string
}

func (u *uploaderFile) Name() string {
	if u.name != "" {
		return u.name
	}
	return filepath.Base(u.File.Name())
}

func (u *uploaderFile) Size() int64 {
	return u.size
}
