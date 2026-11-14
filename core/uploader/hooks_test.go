package uploader

import (
	"testing"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/tg"
)

func TestStableMessageRandomID(t *testing.T) {
	const want int64 = 0x1234567890abcd
	for range 2 {
		got, err := crypto.RandInt64(fixedRandomID(want))
		if err != nil || got != want {
			t.Fatalf("random ID = %d, %v", got, err)
		}
	}
}

func TestSendReceiptMustBeUnambiguous(t *testing.T) {
	updates := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateNewMessage{Message: &tg.Message{ID: 99}},
		&tg.UpdateMessageID{ID: 42, RandomID: 7},
	}}
	if id, err := sentID(updates, 7); err != nil || id != 42 {
		t.Fatalf("did not prefer explicit random-ID mapping: %d, %v", id, err)
	}
	updates.Updates = append(updates.Updates[:1], &tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 100}})
	if _, err := sentID(updates, 7); err == nil {
		t.Fatal("accepted ambiguous unrelated new messages")
	}
	if id, err := sentID(&tg.UpdateShortSentMessage{ID: 12}, 7); err != nil || id != 12 {
		t.Fatalf("short receipt: %d, %v", id, err)
	}
}

func TestPreparedMediaPreservesServerReference(t *testing.T) {
	media, err := storedMedia(&tg.MessageMediaDocument{Document: &tg.Document{ID: 42, AccessHash: 7, FileReference: []byte{1, 2}}})
	if err != nil {
		t.Fatal(err)
	}
	doc := media.(*tg.InputMediaDocument).ID.(*tg.InputDocument)
	if doc.ID != 42 || doc.AccessHash != 7 || len(doc.FileReference) != 2 {
		t.Fatal("lost persistent document reference")
	}
	if _, err := storedMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 43}}); err != nil {
		t.Fatal(err)
	}
	if _, err := storedMedia(&tg.MessageMediaEmpty{}); err == nil {
		t.Fatal("accepted empty prepared media")
	}
}
