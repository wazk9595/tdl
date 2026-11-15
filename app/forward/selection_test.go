package forward

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gotd/td/tg"
)

func TestIDSelection(t *testing.T) {
	for _, test := range []struct {
		ids, exclude string
		yes, no      []int
	}{
		{"100-160", "", []int{100, 150, 160}, []int{99, 161}},
		{"100,160", "", []int{100, 160}, []int{99, 101, 159, 161}},
		{"100-160", "150", []int{100, 149, 151, 160}, []int{150, 161}},
		{" 160-180,100-120,110-130,100,150 ", "110-115,150,175-180", []int{100, 109, 116, 130, 160, 174}, []int{110, 115, 131, 150, 175, 180}},
		{"1-2147483647", "2-2147483646", []int{1, maxMessageID}, []int{2, maxMessageID - 1}},
	} {
		s, err := ParseSelection(test.ids, test.exclude)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range test.yes {
			if !s.Includes(id) {
				t.Fatalf("%s excludes wanted %d", test.ids, id)
			}
		}
		for _, id := range test.no {
			if s.Includes(id) {
				t.Fatalf("%s includes unwanted %d", test.ids, id)
			}
		}
	}
	for _, invalid := range []string{" ", "0", "-1", "1-", "-2", "2-1", "1--2", "1,", "1,,2", "1:2", "+1", "2147483648", "1.2"} {
		if _, err := ParseSelection(invalid, ""); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	if _, err := ParseSelection("", "150"); err == nil {
		t.Fatal("exclude without include accepted")
	}
	if _, err := ValidateSelection(Options{From: []string{"me"}}); err == nil {
		t.Fatal("unbounded saved source accepted")
	}
	if _, err := ValidateSelection(Options{From: []string{"me", "123"}, IDs: "100", To: "me"}); err == nil {
		t.Fatal("ambiguous source accepted")
	}
	if _, err := ValidateSelection(Options{From: []string{"me"}, IDs: "100-160", To: "me"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSelection(Options{From: []string{"me"}, IDs: "100-160"}); err == nil {
		t.Fatal("missing target accepted")
	}
}

func batchMessage(id int, group int64) *tg.Message {
	return &tg.Message{ID: id, GroupedID: group, PeerID: &tg.PeerUser{UserID: 7}, Message: "test"}
}

func TestBatchAlbumExpansionAndExclusion(t *testing.T) {
	album := []*tg.Message{batchMessage(99, 8), batchMessage(100, 8), batchMessage(101, 8)}
	lookup := func(context.Context, *tg.Message) ([]*tg.Message, error) {
		return append([]*tg.Message(nil), album...), nil
	}
	selection, _ := ParseSelection("100-102,100", "")
	plan, err := planBatch(context.Background(), []*tg.Message{album[1], album[2], batchMessage(102, 0)}, selection, false, lookup)
	if err != nil || plan.Total != 4 || plan.Expanded != 1 || !reflect.DeepEqual(plan.IDs, []int{99, 102}) {
		t.Fatal(plan, err)
	}
	// Exclusions outside the requested bounds also block album expansion.
	selection, _ = ParseSelection("100-102", "99")
	plan, err = planBatch(context.Background(), []*tg.Message{album[1], album[2], batchMessage(102, 0)}, selection, false, lookup)
	if err != nil || plan.Total != 1 || plan.SkippedGroups != 1 || plan.Skipped != 2 || !reflect.DeepEqual(plan.IDs, []int{102}) {
		t.Fatal(plan, err)
	}
	selection, _ = ParseSelection("99-102", "100")
	plan, err = planBatch(context.Background(), append(append([]*tg.Message(nil), album...), batchMessage(102, 0)), selection, true, lookup)
	if err != nil || plan.Total != 3 || len(plan.Groups) != 0 || plan.Messages[100] != nil {
		t.Fatal(plan, err)
	}
}

func TestAlbumLookupFailureStopsBeforeSending(t *testing.T) {
	s, _ := ParseSelection("100", "")
	lookup := func(context.Context, *tg.Message) ([]*tg.Message, error) { return nil, errors.New("network lost") }
	if _, err := planBatch(context.Background(), []*tg.Message{batchMessage(100, 8)}, s, false, lookup); err == nil {
		t.Fatal("album lookup failure ignored")
	}
	lookup = func(context.Context, *tg.Message) ([]*tg.Message, error) {
		return []*tg.Message{batchMessage(100, 9)}, nil
	}
	if _, err := planBatch(context.Background(), []*tg.Message{batchMessage(100, 8)}, s, false, lookup); err == nil {
		t.Fatal("wrong album accepted")
	}
}

type fakeHistory struct {
	t        *testing.T
	pages    [][]tg.MessageClass
	requests []*tg.MessagesGetHistoryRequest
}

func (h *fakeHistory) MessagesGetHistory(_ context.Context, req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	h.requests = append(h.requests, req)
	if len(h.pages) == 0 {
		h.t.Fatal("unexpected extra history request")
	}
	page := h.pages[0]
	h.pages = h.pages[1:]
	return &tg.MessagesMessages{Messages: page}, nil
}

func TestFetchSelectionUsesPeerBoundsAndPaginates(t *testing.T) {
	s, _ := ParseSelection("100-160", "150")
	wrongPeer := batchMessage(140, 0)
	wrongPeer.PeerID = &tg.PeerUser{UserID: 999}
	h := &fakeHistory{t: t, pages: [][]tg.MessageClass{
		{batchMessage(160, 0), batchMessage(150, 0), wrongPeer},
		{batchMessage(120, 0), batchMessage(100, 0)},
	}}
	items, err := fetchSelection(context.Background(), h, &tg.InputPeerSelf{}, 7, s)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	// Keep excluded entries until album planning, so group exclusions remain visible.
	if !reflect.DeepEqual(ids, []int{100, 120, 150, 160}) {
		t.Fatal(ids)
	}
	if len(h.requests) != 2 || h.requests[0].MinID != 99 || h.requests[0].MaxID != 161 || h.requests[1].OffsetID != 140 {
		t.Fatal(h.requests)
	}
}

func TestMaximumIDAndCancelledSelection(t *testing.T) {
	s, _ := ParseSelection("2147483647", "")
	h := &fakeHistory{t: t, pages: [][]tg.MessageClass{{batchMessage(maxMessageID, 0)}}}
	if _, err := fetchSelection(context.Background(), h, &tg.InputPeerSelf{}, 7, s); err != nil {
		t.Fatal(err)
	}
	if h.requests[0].MaxID != 0 || h.requests[0].OffsetID != 0 {
		t.Fatal("max ID overflow")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchSelection(ctx, h, &tg.InputPeerSelf{}, 7, s); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
