package forward

import (
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/forwarder"
)

type iterElem struct {
	from         peers.Peer
	msg          *tg.Message
	to           peers.Peer
	thread       int
	modeOverride forwarder.Mode
	opts         iterOptions
}

func (i *iterElem) Mode() forwarder.Mode {
	if i.modeOverride.IsValid() {
		return i.modeOverride
	}
	return i.opts.mode
}

func (i *iterElem) From() peers.Peer { return i.from }

func (i *iterElem) Msg() *tg.Message { return i.msg }

func (i *iterElem) To() peers.Peer { return i.to }

func (i *iterElem) Thread() int { return i.thread }

func (i *iterElem) AsSilent() bool { return i.opts.silent }

func (i *iterElem) AsDryRun() bool { return i.opts.dryRun }

func (i *iterElem) AsGrouped() bool { return i.opts.grouped }

func (i *iterElem) PreparedGroup() ([]*tg.Message, bool) {
	if i.opts.batch == nil {
		return nil, false
	}
	group, ok := i.opts.batch.Groups[i.msg.ID]
	if !ok {
		return nil, false
	}
	copy := append([]*tg.Message(nil), group...)
	for index, msg := range copy {
		if msg.ID == i.msg.ID {
			copy[index] = i.msg
		}
	}
	return copy, true
}
