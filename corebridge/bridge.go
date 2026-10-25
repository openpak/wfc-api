// Package corebridge is the Wii/DS family's translator of the universal social
// model (prds/universal-social-prd.md §4): it consumes the account core's
// social events the same way every other adapter does, and translates each
// kind into this family's transport — of which WFC has none. Nothing can
// reach a console that is not already in a game, and a running game listens
// for nothing social, so every event is delivered nowhere and the drop is
// recorded rather than pretended (§4b.3).
//
// This adapter is the model's control group (prds/platform-wii-ds-prd.md §2):
// a family with no accounts, no push and no identity must be expressible as
// one boring file. If a new event kind ever needs its own branch here, the
// "universal" contract has grown a console-shaped assumption and the contract
// — not this file — is wrong.
package corebridge

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	accountv1 "openpak/wfc-api/corebridge/accountpb"
	"openpak/wfc-common/logging"
)

// kinds are the events this family has no transport for. The list documents
// the contract; the switch in handle does not read it — every kind takes the
// same branch, which is the point.
var kinds = []string{
	"friend_requested",
	"friend_accepted",
	"friend_removed",
	"presence_changed",
	"invitation",
	"message",
}

const (
	pollEvery = 30 * time.Second // nothing here is latency-bound: there is nothing to deliver
	apiKey    = "X-API-Key"
)

// Bridge consumes the core's event stream for the Wii/DS family.
type Bridge struct {
	core    accountv1.EventsClient
	coreKey string
	pool    *pgxpool.Pool
	period  time.Duration // overridable in tests
	events  chan<- string // optional sink for tests; non-transactional, diagnostics only
	dropped uint64
}

// Start returns a running Bridge, or nil when no core is configured — a
// deployment without the core keeps serving WFC exactly as before. The DSN is
// the server's own Postgres string; the bridge adds one small pool and one
// cursor table, nothing else.
func Start(coreAddress, coreKey, dsn string) *Bridge {
	if coreAddress == "" || dsn == "" {
		return nil
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err == nil {
		err = pool.Ping(context.Background()) // v5 connects lazily; v4 failed here on a bad DSN
	}
	if err != nil {
		logging.Error("COREBRIDGE", "database:", err)
		if pool != nil {
			pool.Close()
		}
		return nil
	}
	conn, err := grpc.NewClient(coreAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logging.Error("COREBRIDGE", "dial core:", err)
		pool.Close()
		return nil
	}
	b := &Bridge{
		core:    accountv1.NewEventsClient(conn),
		coreKey: coreKey,
		pool:    pool,
		period:  pollEvery,
	}
	if _, err := pool.Exec(context.Background(),
		`CREATE TABLE IF NOT EXISTS core_events_cursor (id integer PRIMARY KEY, version bigint NOT NULL)`); err != nil {
		logging.Error("COREBRIDGE", "cursor table:", err)
		pool.Close()
		return nil
	}
	go b.loop()
	logging.Notice("COREBRIDGE", "watching the core's social events; every kind delivers nowhere on this family — each drop is logged")
	return b
}

// Dropped reports how many events this family could not deliver. True to the
// name, nothing else in the family reads it; it exists so the drop is a
// number somebody can query and not just silence.
func (b *Bridge) Dropped() uint64 { return b.dropped }

func (b *Bridge) loop() {
	version := b.loadCursor()
	for {
		page, err := b.poll(version)
		if err != nil {
			logging.Warn("COREBRIDGE", "poll:", err)
			time.Sleep(5 * b.period)
			continue
		}
		for _, e := range page.GetEvents() {
			b.handle(e)
			version = e.GetVersion()
		}
		if len(page.GetEvents()) > 0 {
			b.saveCursor(version)
			continue // drain the backlog before sleeping
		}
		time.Sleep(b.period)
	}
}

func (b *Bridge) poll(version uint64) (*accountv1.PollEventsResponse, error) {
	ctx, cancel := context.WithTimeout(b.ctxWithKey(), 10*time.Second)
	defer cancel()
	return b.core.PollEvents(ctx, &accountv1.PollEventsRequest{SinceVersion: version, Limit: 500})
}

func (b *Bridge) ctxWithKey() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), apiKey, b.coreKey)
}

// handle translates one event into this family's transport. It is deliberately
// kind-blind: WFC has no transport for anything, so every kind lands here and
// nowhere else. No branch, no special case, nothing to get wrong per kind.
func (b *Bridge) handle(e *accountv1.InvalidationEvent) {
	b.dropped++
	logging.Info("COREBRIDGE", e.GetType(), e.GetEventId(), "account", e.GetAccountId(),
		"version", e.GetVersion(), "-> no delivery: WFC has no push of any kind")
	if b.events != nil {
		select {
		case b.events <- e.GetType():
		default:
		}
	}
}

func (b *Bridge) loadCursor() uint64 {
	var v int64
	if row := b.pool.QueryRow(context.Background(),
		`SELECT version FROM core_events_cursor WHERE id = 1`); row != nil {
		if err := row.Scan(&v); err == nil {
			return uint64(v)
		}
	}
	// A fresh cursor starts at the head. History holds nothing this family
	// could replay even if it wanted to: there is nowhere to deliver it.
	if head, ok := b.head(); ok {
		b.saveCursor(head)
		return head
	}
	return 0 // core unreachable; the first real poll reports and retries
}

func (b *Bridge) head() (uint64, bool) {
	page, err := b.poll(1<<63 - 1)
	if err != nil {
		return 0, false
	}
	return page.GetMaxVersion(), true
}

func (b *Bridge) saveCursor(v uint64) {
	_, err := b.pool.Exec(context.Background(),
		`INSERT INTO core_events_cursor (id, version) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version`, int64(v))
	if err != nil {
		logging.Warn("COREBRIDGE", "cursor:", err)
	}
}
