package uploader

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// Hooks allow durable task tracking without changing ordinary uploads.
type Hooks struct {
	RandomID   int64
	Transcoded bool
	BeforeSend func(context.Context, tg.MessageMediaClass) error
	Sent       func(context.Context, int) error
}

func fixedRandomID(id int64) *bytes.Reader {
	var data [8]byte
	binary.LittleEndian.PutUint64(data[:], uint64(id))
	return bytes.NewReader(data[:])
}

func storedMedia(media tg.MessageMediaClass) (tg.InputMediaClass, error) {
	switch m := media.(type) {
	case *tg.MessageMediaDocument:
		if d, ok := m.Document.(*tg.Document); ok {
			return &tg.InputMediaDocument{ID: d.AsInput()}, nil
		}
	case *tg.MessageMediaPhoto:
		if p, ok := m.Photo.(*tg.Photo); ok {
			return &tg.InputMediaPhoto{ID: p.AsInput()}, nil
		}
	}
	return nil, fmt.Errorf("unsupported prepared media %T", media)
}

func sentID(updates tg.UpdatesClass, randomID int64) (int, error) {
	if short, ok := updates.(*tg.UpdateShortSentMessage); ok && short.ID > 0 {
		return short.ID, nil
	}
	var list []tg.UpdateClass
	switch u := updates.(type) {
	case *tg.Updates:
		list = u.Updates
	case *tg.UpdatesCombined:
		list = u.Updates
	case *tg.UpdateShort:
		list = []tg.UpdateClass{u.Update}
	}
	for _, item := range list {
		if m, ok := item.(*tg.UpdateMessageID); ok && m.RandomID == randomID && m.ID > 0 {
			return m.ID, nil
		}
	}
	// A single-media request can also return only its new message update.
	var ids []int
	for _, item := range list {
		switch m := item.(type) {
		case *tg.UpdateNewMessage:
			ids = append(ids, m.Message.GetID())
		case *tg.UpdateNewChannelMessage:
			ids = append(ids, m.Message.GetID())
		}
	}
	if len(ids) == 1 && ids[0] > 0 {
		return ids[0], nil
	}
	return 0, fmt.Errorf("send result has no unambiguous message ID")
}

func taskSender(client *tg.Client, hooks *Hooks) *message.Sender {
	sender := message.NewSender(client)
	if hooks != nil && hooks.RandomID != 0 {
		sender.WithRand(fixedRandomID(hooks.RandomID))
	}
	return sender
}
