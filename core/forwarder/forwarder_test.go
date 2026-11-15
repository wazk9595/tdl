package forwarder

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

type testPeer struct {
	peers.Peer
	id int64
}

func (p testPeer) ID() int64                    { return p.id }
func (p testPeer) InputPeer() tg.InputPeerClass { return &tg.InputPeerUser{UserID: p.id} }

type testElem struct {
	msg   *tg.Message
	group []*tg.Message
	mode  Mode
}

func (e testElem) From() peers.Peer                     { return testPeer{id: 7} }
func (e testElem) To() peers.Peer                       { return testPeer{id: 8} }
func (e testElem) Msg() *tg.Message                     { return e.msg }
func (e testElem) Mode() Mode                           { return e.mode }
func (testElem) Thread() int                            { return 42 }
func (testElem) AsSilent() bool                         { return true }
func (testElem) AsDryRun() bool                         { return false }
func (testElem) AsGrouped() bool                        { return true }
func (e testElem) PreparedGroup() ([]*tg.Message, bool) { return e.group, true }

type testIter struct {
	elems   []Elem
	current Elem
}

func (i *testIter) Next(context.Context) bool {
	if len(i.elems) == 0 {
		return false
	}
	i.current = i.elems[0]
	i.elems = i.elems[1:]
	return true
}
func (i *testIter) Value() Elem { return i.current }
func (*testIter) Err() error    { return nil }

type testProgress struct {
	done     int
	failures int
}

func (*testProgress) OnAdd(Elem)                  {}
func (*testProgress) OnClone(Elem, ProgressState) {}
func (p *testProgress) OnDone(_ Elem, err error) {
	p.done++
	if err != nil {
		p.failures++
	}
}

type testRPC struct {
	err          error
	direct, copy int
	ids          []int
}

func (r *testRPC) Invoke(_ context.Context, request bin.Encoder, _ bin.Decoder) error {
	switch req := request.(type) {
	case *tg.MessagesForwardMessagesRequest:
		r.direct++
		r.ids = append([]int(nil), req.ID...)
	case *tg.MessagesSendMultiMediaRequest, *tg.MessagesSendMediaRequest, *tg.MessagesSendMessageRequest:
		r.copy++
	}
	return r.err
}

type testPool struct{ client *tg.Client }

func (p testPool) Client(context.Context, int) *tg.Client  { return p.client }
func (p testPool) Takeout(context.Context, int) *tg.Client { return p.client }
func (p testPool) Default(context.Context) *tg.Client      { return p.client }
func (testPool) Close() error                              { return nil }

func testMessage(id int, group int64) *tg.Message {
	m := &tg.Message{ID: id, Message: "test", PeerID: &tg.PeerUser{UserID: 7}}
	if group != 0 {
		m.SetGroupedID(group)
		m.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: int64(id), Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 2, H: 2, Size: 64}}}})
	}
	return m
}

func TestFailureIsReturnedAndNotMarkedSent(t *testing.T) {
	rpc := &testRPC{err: errors.New("response lost")}
	p := &testProgress{}
	e := testElem{msg: testMessage(100, 0)}
	f := New(Options{Pool: testPool{tg.NewClient(rpc)}, Iter: &testIter{elems: []Elem{e}}, Progress: p})
	if err := f.Forward(context.Background()); err == nil {
		t.Fatal("failed request reported success")
	}
	if len(f.sent) != 0 || p.failures != 1 {
		t.Fatal("failed message marked sent")
	}
	if rpc.direct != 1 || rpc.copy != 0 {
		t.Fatal("uncertain request was retried as clone")
	}
}

func TestEmptyGroupAndCancellationReportFailure(t *testing.T) {
	p := &testProgress{}
	e := testElem{msg: testMessage(100, 8)}
	f := New(Options{Iter: &testIter{elems: []Elem{e}}, Progress: p})
	if err := f.Forward(context.Background()); err == nil || p.failures != 1 {
		t.Fatal("empty album ignored")
	}
	rpc := &testRPC{err: context.Canceled}
	p = &testProgress{}
	e = testElem{msg: testMessage(100, 0)}
	f = New(Options{Pool: testPool{tg.NewClient(rpc)}, Iter: &testIter{elems: []Elem{e, e}}, Progress: p})
	if err := f.Forward(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if rpc.direct != 1 {
		t.Fatal("continued sending after cancellation")
	}
}

func TestPreparedAlbumIsSentOnce(t *testing.T) {
	rpc := &testRPC{}
	p := &testProgress{}
	first, second := testMessage(100, 8), testMessage(160, 8)
	e := testElem{msg: first, group: []*tg.Message{first, second}}
	f := New(Options{Pool: testPool{tg.NewClient(rpc)}, Iter: &testIter{elems: []Elem{e, e}}, Progress: p})
	if err := f.Forward(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rpc.direct != 1 || len(rpc.ids) != 2 || rpc.ids[0] != 100 || rpc.ids[1] != 160 || len(f.sent) != 2 {
		t.Fatal("album split or repeated")
	}
}
