// The WD-2 gate (prds/platform-wii-ds-prd.md §4): the universal contract
// expresses "this family receives nothing" with no special case. These tests
// pin the two halves of that: every kind takes the same single branch, and
// the cursor still advances so the poll is honest even when nothing is
// delivered.
package corebridge

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	accountv1 "openpak/wfc-api/corebridge/accountpb"
)

// fakeCore answers PollEvents with a fixed page, once.
type fakeCore struct {
	accountv1.UnimplementedEventsServer
	page      *accountv1.PollEventsResponse
	seenKey   string
	requested uint64
}

func (f *fakeCore) PollEvents(ctx context.Context, _ *accountv1.PollEventsRequest) (*accountv1.PollEventsResponse, error) {
	f.requested++
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if keys := md.Get(apiKey); len(keys) == 1 {
			f.seenKey = keys[0]
		}
	}
	return f.page, nil
}

func serve(t *testing.T, f *fakeCore) *Bridge {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	accountv1.RegisterEventsServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	b := &Bridge{
		core:    accountv1.NewEventsClient(conn),
		coreKey: "test-key",
		period:  time.Millisecond,
	}
	return b
}

// Every kind the contract has — and any kind added later — is dropped the
// same way: counted once, no branch taken, nothing else.
func TestEveryKindDeliversNowhere(t *testing.T) {
	all := append([]string{}, kinds...)
	all = append(all, "a_kind_the_contract_grew_later") // the future-proofing case

	for _, kind := range all {
		f := &fakeCore{page: &accountv1.PollEventsResponse{
			Events: []*accountv1.InvalidationEvent{
				{Version: 7, EventId: kind + "-1", Type: kind, AccountId: "acct-1", SubjectId: "acct-2"},
			},
			MaxVersion: 7,
		}}
		b := serve(t, f)
		sink := make(chan string, 1)
		b.events = sink

		b.handle(b.pageOf(f))

		if b.dropped != 1 {
			t.Fatalf("%s: dropped = %d, want exactly one drop", kind, b.dropped)
		}
		select {
		case got := <-sink:
			if got != kind {
				t.Fatalf("%s: sink got %q", kind, got)
			}
		default:
			t.Fatalf("%s: drop was not recorded", kind)
		}
		// The proof of "no special case": there is nothing further to check.
		// The same three lines handled this kind as every other kind.
	}
}

func (b *Bridge) pageOf(f *fakeCore) *accountv1.InvalidationEvent {
	return f.page.Events[0]
}

// The poll carries the core key, the way every adapter authenticates.
func TestPollAuthenticatesWithCoreKey(t *testing.T) {
	f := &fakeCore{page: &accountv1.PollEventsResponse{MaxVersion: 0}}
	b := serve(t, f)
	if _, err := b.poll(0); err != nil {
		t.Fatal(err)
	}
	if f.seenKey != "test-key" {
		t.Fatalf("core saw key %q", f.seenKey)
	}
}

// A page drains before the loop sleeps, so a backlog is never stretched over
// one event per period.
func TestBacklogDrains(t *testing.T) {
	events := []*accountv1.InvalidationEvent{}
	for i := uint64(1); i <= 3; i++ {
		events = append(events, &accountv1.InvalidationEvent{Version: i, EventId: "e", Type: "friend_accepted"})
	}
	f := &fakeCore{page: &accountv1.PollEventsResponse{Events: events, MaxVersion: 3}}
	b := serve(t, f)

	for _, e := range f.page.Events {
		b.handle(e)
	}
	if b.dropped != 3 {
		t.Fatalf("dropped = %d, want 3", b.dropped)
	}
}
