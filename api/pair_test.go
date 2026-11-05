package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openpak/wfc-common/database"
	"openpak/wfc-common/mkwboard"
)

func TestPairEndpointsNeedTheSecret(t *testing.T) {
	before := apiSecret
	defer func() { apiSecret = before }()
	handlers := map[string]func(w *httptest.ResponseRecorder, body string){}
	for name, h := range map[string]func(w2 *httptest.ResponseRecorder, r string){
		"lookup":   func(w *httptest.ResponseRecorder, b string) { HandlePairPreview(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
		"approve":  func(w *httptest.ResponseRecorder, b string) { HandlePairApprove(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
		"consoles": func(w *httptest.ResponseRecorder, b string) { HandlePairConsoles(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
		"unlink":   func(w *httptest.ResponseRecorder, b string) { HandlePairUnlink(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
		"redeem":   func(w *httptest.ResponseRecorder, b string) { HandlePairRedeem(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
		"forget":   func(w *httptest.ResponseRecorder, b string) { HandlePairForget(w, httptest.NewRequest("POST", "/", strings.NewReader(b))) },
	} {
		handlers[name] = h
	}
	for _, secret := range []string{"", "right"} {
		apiSecret = secret
		for name, h := range handlers {
			w := httptest.NewRecorder()
			h(w, `{"secret":"wrong","code":"ABCD2345","user_id":1,"core_account_id":"a","link_id":"l"}`)
			if w.Code != 401 {
				t.Errorf("%s with secret %q and a wrong guess: %d", name, secret, w.Code)
			}
			w = httptest.NewRecorder()
			h(w, `{"code":"ABCD2345","user_id":1,"core_account_id":"a","link_id":"l"}`)
			if w.Code != 401 {
				t.Errorf("%s without a secret (server %q): %d", name, secret, w.Code)
			}
		}
	}
}

// fakeStore is an in-memory pairStore with one console and one code.
type fakeStore struct {
	code      database.PairingTarget
	approving string
	linked    map[uint64]database.LinkedConsole
	erased    []int64
	aborted   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{code: database.PairingTarget{Code: "ABCD2345", UserID: 77, Unit: database.UnitDS, GameCode: "AMCE", Name: "TOBAGIN",
		ExpiresAt: time.Now().Add(time.Minute)}, linked: map[uint64]database.LinkedConsole{}}
}

func (f *fakeStore) LookupPairingCode(ctx context.Context, raw string, proof database.NumberProof) (database.PairingTarget, error) {
	if raw != f.code.Code || f.approving != "" {
		return database.PairingTarget{}, database.ErrPairingCode
	}
	return f.code, nil
}
func (f *fakeStore) BeginApproval(ctx context.Context, raw, acct string, proof database.NumberProof) (database.PairingTarget, bool, error) {
	if raw != f.code.Code || (f.approving != "" && f.approving != acct) {
		return database.PairingTarget{}, false, database.ErrPairingCode
	}
	if l, ok := f.linked[f.code.UserID]; ok {
		if l.AccountID == acct {
			return f.code, true, nil
		}
		return database.PairingTarget{}, false, database.ErrConsoleLinked
	}
	f.approving = acct
	if f.code.Subject == "" {
		f.code.Subject = "77.1"
	}
	return f.code, false, nil
}
func (f *fakeStore) FinishApproval(ctx context.Context, t database.PairingTarget, acct, link string) error {
	f.linked[t.UserID] = database.LinkedConsole{UserID: t.UserID, Subject: t.Subject, LinkID: link, AccountID: acct, Unit: t.Unit}
	f.code.Code = "used"
	return nil
}
func (f *fakeStore) AbortApproval(ctx context.Context, code, acct string) error {
	f.approving, f.code.Subject = "", ""
	f.aborted++
	return nil
}
func (f *fakeStore) StuckApprovals(ctx context.Context) ([]database.StuckApproval, error) {
	if f.approving == "" {
		return nil, nil
	}
	return []database.StuckApproval{{PairingTarget: f.code, AccountID: f.approving}}, nil
}
func (f *fakeStore) ConsolesOf(ctx context.Context, acct string) ([]database.LinkedConsole, error) {
	var out []database.LinkedConsole
	for _, l := range f.linked {
		if l.AccountID == acct {
			out = append(out, l)
		}
	}
	return out, nil
}
func (f *fakeStore) ForgetSubject(ctx context.Context, subject string) (uint64, error) {
	for uid, l := range f.linked {
		if l.Subject == subject {
			delete(f.linked, uid)
			return uid, nil
		}
	}
	return 0, nil
}
func (f *fakeStore) ClearConsoleData(ctx context.Context, ids []int64) (int, error) {
	f.erased = append(f.erased, ids...)
	return len(ids), nil
}
func (f *fakeStore) PairingCounters(ctx context.Context) (database.PairingStats, error) {
	return database.PairingStats{}, nil
}

type fakeCore struct {
	reserveErr, activateErr error
	links                   map[string]string // subject -> account (active)
	unlinked                []string
	keys                    []string
}

func (c *fakeCore) Reserve(ctx context.Context, acct, subject, key string) (string, error) {
	c.keys = append(c.keys, key)
	if c.reserveErr != nil {
		return "", c.reserveErr
	}
	return "link-" + subject, nil
}
func (c *fakeCore) Activate(ctx context.Context, link, key string) error {
	c.keys = append(c.keys, key)
	return c.activateErr
}
func (c *fakeCore) Unlink(ctx context.Context, link, key string) error {
	c.unlinked = append(c.unlinked, link)
	return nil
}
func (c *fakeCore) LinkActive(ctx context.Context, subject string) (string, string, bool, error) {
	if a, ok := c.links[subject]; ok {
		return "link-" + subject, a, true, nil
	}
	return "", "", false, nil
}

func TestApproveLinksAndIsIdempotent(t *testing.T) {
	s, c := newFakeStore(), &fakeCore{}
	spec, err := approve(context.Background(), s, c, "ABCD2345", "acct-1", database.NumberProof{})
	if err != nil || spec.Status != "linked" || spec.Unit != "ds" || spec.LinkID != "link-77.1" {
		t.Fatalf("approve: %+v %v", spec, err)
	}
	if strings.Join(c.keys, ",") != "wfcpair:77.1:reserve,wfcpair:77.1:activate" {
		t.Fatalf("keys %v", c.keys)
	}
	if l := s.linked[77]; l.AccountID != "acct-1" || l.Subject != "77.1" {
		t.Fatalf("cache %+v", l)
	}
	if _, err := approve(context.Background(), s, c, "ABCD2345", "", database.NumberProof{}); !errors.Is(err, database.ErrPairingCode) {
		t.Fatalf("no account: %v", err)
	}
}

func TestApproveRollsBackOnCoreConflict(t *testing.T) {
	s, c := newFakeStore(), &fakeCore{activateErr: mkwboard.ErrLinkConflict}
	if _, err := approve(context.Background(), s, c, "ABCD2345", "acct-1", database.NumberProof{}); !errors.Is(err, mkwboard.ErrLinkConflict) {
		t.Fatalf("err %v", err)
	}
	if len(c.unlinked) != 1 || s.aborted != 1 || len(s.linked) != 0 {
		t.Fatalf("rollback: unlinked %v aborted %d linked %v", c.unlinked, s.aborted, s.linked)
	}
	// A transient core error leaves the approval to resume.
	s2, c2 := newFakeStore(), &fakeCore{activateErr: errors.New("timeout")}
	if _, err := approve(context.Background(), s2, c2, "ABCD2345", "acct-1", database.NumberProof{}); err == nil || s2.aborted != 0 || s2.approving != "acct-1" {
		t.Fatalf("transient: %v aborted %d", err, s2.aborted)
	}
	c2.activateErr = nil
	if spec, err := approve(context.Background(), s2, c2, "ABCD2345", "acct-1", database.NumberProof{}); err != nil || spec.Status != "linked" {
		t.Fatalf("resume: %+v %v", spec, err)
	}
}

func TestApproveConsoleCap(t *testing.T) {
	s, c := newFakeStore(), &fakeCore{}
	for i := 0; i < MaxConsolesPerAccount; i++ {
		s.linked[uint64(1000+i)] = database.LinkedConsole{UserID: uint64(1000 + i), AccountID: "acct-1"}
	}
	if _, err := approve(context.Background(), s, c, "ABCD2345", "acct-1", database.NumberProof{}); !errors.Is(err, errConsoleCap) || s.aborted != 1 {
		t.Fatalf("cap: %v aborted %d", err, s.aborted)
	}
}

func TestUnlinkKicksAndErases(t *testing.T) {
	s, c := newFakeStore(), &fakeCore{}
	if _, err := approve(context.Background(), s, c, "ABCD2345", "acct-1", database.NumberProof{}); err != nil {
		t.Fatal(err)
	}
	var kicked []uint64
	before := kickUser
	kickUser = func(ctx context.Context, uid uint64, reason string) (int, error) {
		kicked = append(kicked, uid)
		return 1, nil
	}
	defer func() { kickUser = before }()
	if found, err := unlink(context.Background(), s, c, "acct-2", "link-77.1", false); found || err != nil {
		t.Fatalf("other account: %v %v", found, err)
	}
	if found, err := unlink(context.Background(), s, c, "acct-1", "link-77.1", true); !found || err != nil {
		t.Fatalf("unlink: %v %v", found, err)
	}
	if len(c.unlinked) != 1 || len(s.linked) != 0 || len(kicked) != 1 || kicked[0] != 77 || len(s.erased) != 1 {
		t.Fatalf("unlinked %v linked %v kicked %v erased %v", c.unlinked, s.linked, kicked, s.erased)
	}
}

func TestReaperFinishesOrRollsBack(t *testing.T) {
	s, c := newFakeStore(), &fakeCore{links: map[string]string{"77.1": "acct-1"}}
	if _, _, err := s.BeginApproval(context.Background(), "ABCD2345", "acct-1", database.NumberProof{}); err != nil {
		t.Fatal(err)
	}
	reapApprovals(context.Background(), s, c)
	if s.linked[77].LinkID != "link-77.1" {
		t.Fatalf("finish: %+v", s.linked)
	}
	s2, c2 := newFakeStore(), &fakeCore{}
	_, _, _ = s2.BeginApproval(context.Background(), "ABCD2345", "acct-1", database.NumberProof{})
	reapApprovals(context.Background(), s2, c2)
	if s2.aborted != 1 || len(s2.linked) != 0 {
		t.Fatalf("rollback: %d %v", s2.aborted, s2.linked)
	}
}
