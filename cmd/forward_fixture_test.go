package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"

	"github.com/iyear/tdl/core/storage"
)

// Optional live fixture, restricted to Saved Messages. Never runs in normal CI.
func TestCreateForwardAlbumFixture(t *testing.T) {
	value := os.Getenv("TDL_FORWARD_FIXTURE_PHOTO_ID")
	if value == "" {
		t.Skip("set TDL_FORWARD_FIXTURE_PHOTO_ID to create a synthetic album in Saved Messages")
	}
	id, err := strconv.Atoi(value)
	if err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("TDL_FORWARD_FIXTURE_OUTPUT")
	if output == "" {
		t.Fatal("TDL_FORWARD_FIXTURE_OUTPUT is required")
	}
	root := New()
	root.AddCommand(&cobra.Command{Use: "fixture", RunE: func(cmd *cobra.Command, _ []string) error {
		return tRun(cmd.Context(), func(ctx context.Context, c *telegram.Client, _ storage.Storage) error {
			self, err := c.Self(ctx)
			if err != nil {
				return err
			}
			result, err := c.API().MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: id}})
			if err != nil {
				return err
			}
			list := result.(interface{ GetMessages() []tg.MessageClass }).GetMessages()
			var photo *tg.Photo
			for _, item := range list {
				if m, ok := item.(*tg.Message); ok && m.ID == id {
					if p, ok := m.PeerID.(*tg.PeerUser); !ok || p.UserID != self.ID {
						return fmt.Errorf("fixture photo must belong to Saved Messages")
					}
					if media, ok := m.Media.(*tg.MessageMediaPhoto); ok {
						photo, _ = media.Photo.(*tg.Photo)
					}
				}
			}
			if photo == nil {
				return fmt.Errorf("fixture photo not found")
			}
			var media []tg.InputSingleMedia
			// Use raw caption strings so each part can be identified after forwarding.
			for _, caption := range []string{"tdl ID 范围测试相册 A", "tdl ID 范围测试相册 B", "tdl ID 范围测试相册 C"} {
				randomID, err := crypto.RandInt64(crypto.DefaultRand())
				if err != nil {
					return err
				}
				media = append(media, tg.InputSingleMedia{Media: &tg.InputMediaPhoto{ID: photo.AsInput()}, RandomID: randomID, Message: caption})
			}
			updates, err := c.API().MessagesSendMultiMedia(ctx, &tg.MessagesSendMultiMediaRequest{Peer: &tg.InputPeerSelf{}, MultiMedia: media})
			if err != nil {
				return err
			}
			var ids []int
			if u, ok := updates.(*tg.Updates); ok {
				for _, update := range u.Updates {
					if n, ok := update.(*tg.UpdateNewMessage); ok {
						ids = append(ids, n.Message.GetID())
					}
				}
			}
			if len(ids) != 3 {
				return fmt.Errorf("expected 3 fixture messages, got %v", ids)
			}
			t.Logf("fixture message IDs: %v", ids)
			data, err := json.Marshal(ids)
			if err != nil {
				return err
			}
			return os.WriteFile(output, data, 0o600)
		})
	}})
	root.SetArgs([]string{"fixture"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
}
