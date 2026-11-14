package send

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/app/up"
	"github.com/iyear/tdl/core/uploader"
	"github.com/iyear/tdl/core/util/mediautil"
	"github.com/iyear/tdl/core/util/tutil"
)

type Telegram struct {
	Client         *tg.Client
	Manager        *peers.Manager
	Peer           tg.InputPeerClass
	Channel        tg.InputChannelClass
	Destination    Destination
	AccountID      int64
	AllowTranscode bool
	Encoder        string
	Threads        int
	OriginalImages bool
	Ban            *tg.ChatBannedRights
	// Populated once per run, before uploads start. New messages are read live.
	checked map[int]*tg.Message
}

func Resolve(ctx context.Context, client *tg.Client, manager *peers.Manager, target Target) (*Telegram, error) {
	self, err := manager.Self(ctx)
	if err != nil {
		return nil, err
	}
	b := &Telegram{Client: client, Manager: manager, AccountID: self.ID()}
	if target.Self {
		b.Peer = &tg.InputPeerSelf{}
		b.Destination = Destination{PeerID: self.ID(), Name: "收藏夹", Link: "me"}
		messages, err := b.history(ctx, 0, 0)
		if err != nil {
			return nil, err
		}
		if len(messages) > 0 {
			b.Destination.FloorID = messages[0].GetID()
		}
		return b, nil
	}
	peer, err := tutil.GetInputPeer(ctx, manager, target.Peer)
	if err != nil {
		return nil, err
	}
	channel, ok := peer.(peers.Channel)
	if !ok {
		return nil, fmt.Errorf("目标不是论坛群组，请提供话题链接")
	}
	if err := channel.Sync(ctx); err != nil {
		return nil, err
	}
	raw := channel.Raw()
	if !raw.Forum {
		return nil, fmt.Errorf("目标群组没有开启话题")
	}
	b.Peer = channel.InputPeer()
	b.Channel = channel.InputChannel()
	_, admin := raw.GetAdminRights()
	if !raw.Creator && !admin {
		d, _ := raw.GetDefaultBannedRights()
		u, _ := raw.GetBannedRights()
		b.Ban = &tg.ChatBannedRights{ViewMessages: d.ViewMessages || u.ViewMessages, SendMessages: d.SendMessages || u.SendMessages, SendMedia: d.SendMedia || u.SendMedia, SendPhotos: d.SendPhotos || u.SendPhotos, SendVideos: d.SendVideos || u.SendVideos, SendDocs: d.SendDocs || u.SendDocs}
		if b.Ban != nil && (b.Ban.ViewMessages || b.Ban.SendMessages || b.Ban.SendMedia) {
			return nil, fmt.Errorf("当前账号没有在该群发送媒体的权限")
		}
	}
	getTopic := func(id int) (*tg.ForumTopic, error) {
		result, err := client.MessagesGetForumTopicsByID(ctx, &tg.MessagesGetForumTopicsByIDRequest{Peer: b.Peer, Topics: []int{id}})
		if err != nil {
			return nil, err
		}
		for _, item := range result.Topics {
			if topic, ok := item.(*tg.ForumTopic); ok && topic.ID == id {
				return topic, nil
			}
		}
		return nil, nil
	}
	topic, err := getTopic(target.Candidate)
	if err != nil {
		return nil, err
	}
	if topic == nil && !target.Explicit {
		msg, err := b.message(ctx, target.Message)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			if reply, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && reply.ForumTopic {
				id := reply.ReplyToTopID
				if id == 0 {
					id = reply.ReplyToMsgID
				}
				topic, err = getTopic(id)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if topic == nil {
		return nil, fmt.Errorf("找不到该话题，或链接指向不属于话题的普通消息")
	}
	if topic.Closed && !raw.Creator && !admin {
		return nil, fmt.Errorf("该话题已关闭")
	}
	if target.Explicit && target.Message != topic.ID {
		msg, err := b.message(ctx, target.Message)
		if err != nil {
			return nil, err
		}
		if msg == nil || messageTopic(msg) != topic.ID {
			return nil, fmt.Errorf("链接中的消息不属于指定话题")
		}
	}
	b.Destination = Destination{PeerID: peer.ID(), Topic: topic.ID, Name: peer.VisibleName() + " / " + topic.Title, Link: fmt.Sprintf("https://t.me/c/%d/%d", peer.ID(), topic.ID), FloorID: topic.TopMessage}
	return b, nil
}

func (b *Telegram) Upload(ctx context.Context, entry Entry, hooks *uploader.Hooks, progress func(uploader.ProgressState)) error {
	kind := entry.File.Kind
	if kind == kindPhoto && b.OriginalImages {
		kind = kindDocument
	}
	if b.Ban != nil && ((kind == kindVideo && b.Ban.SendVideos) || (kind == kindPhoto && b.Ban.SendPhotos) || (kind == kindDocument && b.Ban.SendDocs)) {
		return fmt.Errorf("当前账号没有发送该类媒体的权限")
	}
	mode := "stream"
	encoder := b.Encoder
	if entry.File.Kind == kindVideo {
		info, err := mediautil.ProbeVideo(ctx, entry.File.Path)
		if err != nil {
			return err
		}
		compatible := (info.Codec == "h264" || info.Codec == "hevc" || info.Codec == "av1") && (!info.HasAudio || (info.AudioCodec == "aac" && info.AudioChannels <= 2))
		if !compatible {
			if !b.AllowTranscode {
				return fmt.Errorf("%w（视频 %s，音频 %s）", ErrNeedsTranscode, info.Codec, info.AudioCodec)
			}
			mode = "compatible"
			hooks.Transcoded = true
			if encoder == "auto" {
				encoder = chooseEncoder(ctx)
			}
		}
	}
	if encoder == "auto" {
		encoder = "h264_nvenc"
	}
	return up.Single(ctx, b.Client, b.Manager, entry.File.Path, strconv.FormatInt(b.Destination.PeerID, 10), b.Destination.Topic, mode, encoder, b.Threads, entry.File.Kind == kindPhoto && !b.OriginalImages, hooks, progress)
}

func chooseEncoder(ctx context.Context) string {
	if data, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output(); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return "h264_nvenc"
	}
	return "libx264"
}

func (b *Telegram) message(ctx context.Context, id int) (*tg.Message, error) {
	if msg, ok := b.checked[id]; ok {
		return msg, nil
	}
	items, err := b.messages(ctx, []int{id})
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if msg, ok := item.(*tg.Message); ok && msg.ID == id {
			return msg, nil
		}
	}
	return nil, nil
}

func messageList(result tg.MessagesMessagesClass) ([]tg.MessageClass, error) {
	if list, ok := result.(interface{ GetMessages() []tg.MessageClass }); ok {
		return list.GetMessages(), nil
	}
	return nil, fmt.Errorf("服务器未返回可核对的消息列表：%T", result)
}

func (b *Telegram) messages(ctx context.Context, messageIDs []int) ([]tg.MessageClass, error) {
	ids := make([]tg.InputMessageClass, 0, len(messageIDs))
	for _, id := range messageIDs {
		ids = append(ids, &tg.InputMessageID{ID: id})
	}
	var result tg.MessagesMessagesClass
	var err error
	if b.Channel != nil {
		result, err = b.Client.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{Channel: b.Channel, ID: ids})
	} else {
		result, err = b.Client.MessagesGetMessages(ctx, ids)
	}
	if err != nil {
		return nil, err
	}
	return messageList(result)
}

// CheckExisting reads receipts in batches so resuming a large folder does not
// require a separate network request for every previously sent file.
func (b *Telegram) CheckExisting(ctx context.Context, task Task, scan Scan) error {
	b.checked = make(map[int]*tg.Message)
	var ids []int
	for _, file := range scan.Files {
		if entry := task.Entries[file.Hash]; entry != nil && entry.MessageID > 0 {
			ids = append(ids, entry.MessageID)
			b.checked[entry.MessageID] = nil
		}
	}
	for start := 0; start < len(ids); start += 100 {
		items, err := b.messages(ctx, ids[start:min(start+100, len(ids))])
		if err != nil {
			return fmt.Errorf("核对既有消息失败：%w", err)
		}
		for _, item := range items {
			if msg, ok := item.(*tg.Message); ok {
				b.checked[msg.ID] = msg
			}
		}
	}
	return nil
}

func messageTopic(msg *tg.Message) int {
	if reply, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && reply.ForumTopic {
		if reply.ReplyToTopID > 0 {
			return reply.ReplyToTopID
		}
		return reply.ReplyToMsgID
	}
	return 0
}

func ValidateMessage(msg *tg.Message, entry Entry, dest Destination) error {
	if msg == nil {
		return ErrRemoteMissing
	}
	if tutil.GetPeerID(msg.PeerID) != dest.PeerID || (dest.Topic != 0 && messageTopic(msg) != dest.Topic) {
		return fmt.Errorf("消息归属的群组或话题不一致")
	}
	ref, err := Ref(msg.Media)
	if err != nil {
		return err
	}
	if ref.Kind != entry.Media.Kind || ref.ID != entry.Media.ID {
		return fmt.Errorf("对应消息的媒体已改变")
	}
	if ref.Kind == kindPhoto {
		return nil
	}
	if ref.Size != entry.Media.Size {
		return fmt.Errorf("服务器文件大小不一致")
	}
	if ref.Kind == kindVideo {
		media := msg.Media.(*tg.MessageMediaDocument)
		doc := media.Document.(*tg.Document)
		if !media.Video || len(doc.Thumbs) == 0 {
			return fmt.Errorf("视频类型或封面缺失")
		}
		for _, item := range doc.Attributes {
			if attr, ok := item.(*tg.DocumentAttributeVideo); ok {
				if !attr.SupportsStreaming || attr.W != entry.Media.Width || attr.H != entry.Media.Height || math.Abs(attr.Duration-entry.Media.Duration) > 0.5 {
					return fmt.Errorf("视频流式属性、尺寸或时长不一致")
				}
			}
		}
	}
	return nil
}

func (b *Telegram) Verify(ctx context.Context, entry Entry) error {
	msg, err := b.message(ctx, entry.MessageID)
	if err != nil {
		return err
	}
	return ValidateMessage(msg, entry, b.Destination)
}

func (b *Telegram) history(ctx context.Context, offset, minID int) ([]tg.MessageClass, error) {
	var result tg.MessagesMessagesClass
	var err error
	if b.Destination.Topic > 0 {
		result, err = b.Client.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{Peer: b.Peer, MsgID: b.Destination.Topic, OffsetID: offset, MinID: minID, Limit: 100})
	} else {
		result, err = b.Client.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: b.Peer, OffsetID: offset, MinID: minID, Limit: 100})
	}
	if err != nil {
		return nil, err
	}
	return messageList(result)
}

func (b *Telegram) Find(ctx context.Context, entry Entry, task Task) (int, error) {
	used := map[int]bool{}
	for _, other := range task.Entries {
		if other.File.Hash != entry.File.Hash && other.MessageID > 0 {
			used[other.MessageID] = true
		}
	}
	offset := 0
	for page := 0; page < 100; page++ {
		messages, err := b.history(ctx, offset, task.Destination.FloorID)
		if err != nil {
			return 0, err
		}
		if len(messages) == 0 {
			return 0, nil
		}
		next := offset
		for _, item := range messages {
			if next == 0 || item.GetID() < next {
				next = item.GetID()
			}
			msg, ok := item.(*tg.Message)
			if !ok || msg.ID <= task.Destination.FloorID || used[msg.ID] || !msg.Out || msg.Message != entry.Caption {
				continue
			}
			if ValidateMessage(msg, entry, b.Destination) == nil {
				return msg.ID, nil
			}
		}
		if next <= task.Destination.FloorID || next == offset {
			return 0, nil
		}
		offset = next
	}
	return 0, fmt.Errorf("待核对消息超过检索范围，暂不自动重发")
}
