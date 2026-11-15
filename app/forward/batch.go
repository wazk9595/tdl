package forward

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"

	"github.com/iyear/tdl/core/util/tutil"
	"github.com/iyear/tdl/pkg/tmessage"
)

type historyAPI interface {
	MessagesGetHistory(context.Context, *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error)
}

type batchPlan struct {
	Source                          peers.Peer
	Messages                        map[int]*tg.Message
	Groups                          map[int][]*tg.Message
	IDs                             []int
	Matched, Skipped, SkippedGroups int
	Total, Expanded                 int
}

func sourcePeer(ctx context.Context, manager *peers.Manager, name string) (peers.Peer, error) {
	name = strings.TrimSpace(name)
	if isSaved(name) {
		return manager.Self(ctx)
	}
	return tutil.GetInputPeer(ctx, manager, strings.TrimPrefix(name, "@"))
}

func isSaved(name string) bool { return name == "me" || name == "self" || name == "saved" }

func samePeer(actual tg.PeerClass, expected tg.InputPeerClass, selfID int64) bool {
	switch p := expected.(type) {
	case *tg.InputPeerSelf:
		v, ok := actual.(*tg.PeerUser)
		return ok && v.UserID == selfID
	case *tg.InputPeerUser:
		v, ok := actual.(*tg.PeerUser)
		return ok && v.UserID == p.UserID
	case *tg.InputPeerChat:
		v, ok := actual.(*tg.PeerChat)
		return ok && v.ChatID == p.ChatID
	case *tg.InputPeerChannel:
		v, ok := actual.(*tg.PeerChannel)
		return ok && v.ChannelID == p.ChannelID
	}
	return false
}

func historyMessages(result tg.MessagesMessagesClass) ([]tg.MessageClass, error) {
	if list, ok := result.(interface{ GetMessages() []tg.MessageClass }); ok {
		return list.GetMessages(), nil
	}
	return nil, fmt.Errorf("服务器没有返回可读取的消息列表：%T", result)
}

func fetchSelection(ctx context.Context, api historyAPI, peer tg.InputPeerClass, selfID int64, selection *IDSelection) ([]*tg.Message, error) {
	byID := map[int]*tg.Message{}
	for _, span := range selection.Include {
		upper := 0
		if span.Last < maxMessageID {
			upper = span.Last + 1
		}
		offset := upper
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, OffsetID: offset, MinID: span.First - 1, MaxID: upper, Limit: 100})
			if err != nil {
				return nil, err
			}
			items, err := historyMessages(result)
			if err != nil {
				return nil, err
			}
			if len(items) == 0 {
				break
			}
			next := 0
			for _, item := range items {
				id := item.GetID()
				if id > 0 && (next == 0 || id < next) {
					next = id
				}
				msg, ok := item.(*tg.Message)
				if ok && id >= span.First && id <= span.Last && samePeer(msg.PeerID, peer, selfID) {
					byID[id] = msg
				}
			}
			if next == 0 || (offset != 0 && next >= offset) {
				return nil, fmt.Errorf("读取消息范围时分页没有前进，已停止")
			}
			if next <= span.First {
				break
			}
			offset = next
		}
	}
	messages := make([]*tg.Message, 0, len(byID))
	for _, msg := range byID {
		messages = append(messages, msg)
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	return messages, nil
}

type groupLookup func(context.Context, *tg.Message) ([]*tg.Message, error)

// Explicit exclusions win over album expansion: an affected album is skipped.
func planBatch(ctx context.Context, messages []*tg.Message, selection *IDSelection, single bool, lookup groupLookup) (*batchPlan, error) {
	plan := &batchPlan{Messages: map[int]*tg.Message{}, Groups: map[int][]*tg.Message{}, Matched: len(messages)}
	seen := map[int]bool{}
	groups := map[int64]bool{}
	for _, msg := range messages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[msg.ID] || !selection.Includes(msg.ID) {
			continue
		}
		items := []*tg.Message{msg}
		if msg.GroupedID != 0 && !single {
			if groups[msg.GroupedID] {
				continue
			}
			groups[msg.GroupedID] = true
			var err error
			items, err = lookup(ctx, msg)
			if err != nil {
				return nil, fmt.Errorf("获取消息 %d 的完整媒体组失败：%w", msg.ID, err)
			}
			if len(items) == 0 {
				return nil, fmt.Errorf("消息 %d 的媒体组为空，已停止", msg.ID)
			}
			blocked := false
			foundSeed := false
			for _, item := range items {
				if item.GroupedID != msg.GroupedID || !sameMessagePeer(item.PeerID, msg.PeerID) {
					return nil, fmt.Errorf("媒体组返回了不匹配的消息，已停止")
				}
				foundSeed = foundSeed || item.ID == msg.ID
				blocked = blocked || selection.Excluded(item.ID)
			}
			if !foundSeed {
				return nil, fmt.Errorf("媒体组未包含原消息 %d，已停止", msg.ID)
			}
			if blocked {
				plan.SkippedGroups++
				continue
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		for _, item := range items {
			seen[item.ID] = true
			plan.Messages[item.ID] = item
			plan.Total++
			if !selection.Includes(item.ID) {
				plan.Expanded++
			}
		}
		seed := items[0].ID
		plan.IDs = append(plan.IDs, seed)
		if msg.GroupedID != 0 && !single {
			plan.Groups[seed] = items
		}
	}
	for _, msg := range messages {
		if !seen[msg.ID] {
			plan.Skipped++
		}
	}
	sort.Ints(plan.IDs)
	return plan, nil
}

func sameMessagePeer(a, b tg.PeerClass) bool {
	switch p := a.(type) {
	case *tg.PeerUser:
		v, ok := b.(*tg.PeerUser)
		return ok && p.UserID == v.UserID
	case *tg.PeerChat:
		v, ok := b.(*tg.PeerChat)
		return ok && p.ChatID == v.ChatID
	case *tg.PeerChannel:
		v, ok := b.(*tg.PeerChannel)
		return ok && p.ChannelID == v.ChannelID
	}
	return false
}

func collectSelected(ctx context.Context, client *tg.Client, manager *peers.Manager, opts Options, selection *IDSelection) (*batchPlan, []*tmessage.Dialog, error) {
	from, err := sourcePeer(ctx, manager, opts.From[0])
	if err != nil {
		return nil, nil, err
	}
	messages, err := fetchSelection(ctx, client, from.InputPeer(), from.ID(), selection)
	if err != nil {
		return nil, nil, err
	}
	plan, err := planBatch(ctx, messages, selection, opts.Single, func(ctx context.Context, msg *tg.Message) ([]*tg.Message, error) {
		return tutil.GetGroupedMessages(ctx, client, from.InputPeer(), msg)
	})
	if err != nil {
		return nil, nil, err
	}
	plan.Source = from
	if opts.Desc {
		for i, j := 0, len(plan.IDs)-1; i < j; i, j = i+1, j-1 {
			plan.IDs[i], plan.IDs[j] = plan.IDs[j], plan.IDs[i]
		}
	}
	return plan, []*tmessage.Dialog{{Peer: from.InputPeer(), Messages: plan.IDs}}, nil
}
