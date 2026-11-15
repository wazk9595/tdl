package tutil

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

type groupRPC struct {
	t        *testing.T
	err      error
	messages []tg.MessageClass
}

func (r groupRPC) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req := input.(*tg.MessagesGetHistoryRequest)
	if req.OffsetID != 100 || req.AddOffset >= 0 || req.Limit < 20 {
		r.t.Fatal("album lookup assumes consecutive numeric IDs")
	}
	if r.err != nil {
		return r.err
	}
	output.(*tg.MessagesMessagesBox).Messages = &tg.MessagesMessages{Messages: r.messages}
	return nil
}

func TestGroupedLookupHandlesIDGapsAndKeepsEditedSeed(t *testing.T) {
	seed := &tg.Message{ID: 100, Message: "edited"}
	seed.SetGroupedID(8)
	newer := &tg.Message{ID: 1000}
	newer.SetGroupedID(8)
	older := &tg.Message{ID: 1}
	older.SetGroupedID(8)
	oldSeed := &tg.Message{ID: 100, Message: "old"}
	oldSeed.SetGroupedID(8)
	rpc := groupRPC{t: t, messages: []tg.MessageClass{newer, oldSeed, older}}
	items, err := GetGroupedMessages(context.Background(), tg.NewClient(rpc), &tg.InputPeerSelf{}, seed)
	if err != nil || len(items) != 3 || items[0].ID != 1 || items[1] != seed || items[2].ID != 1000 {
		t.Fatal(items, err)
	}
	rpc.err = errors.New("connection lost")
	if _, err := GetGroupedMessages(context.Background(), tg.NewClient(rpc), &tg.InputPeerSelf{}, seed); err == nil {
		t.Fatal("album fetch error ignored")
	}
}
