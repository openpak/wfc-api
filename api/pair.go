package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"openpak/wfc-common/common"
	"openpak/wfc-common/database"
	"openpak/wfc-common/gpcmclient"
	"openpak/wfc-common/logging"
	"openpak/wfc-common/mkwboard"
)

// The pairing endpoints back openpak.org/link for a Wii or DS (prds repos/wfc-link-first-design.md).
// The website calls them with the API secret, after its own sign-in and two-factor, and passes
// the signed-in account; players never call them. This service owns the approval: it makes the
// account-core link (Reserve, Activate, idempotent keys, rollback on failure), the way xb-account
// does for the Xbox. Codes are never logged.

const (
	APIErrorInvalidPairingCode APIErrorString = "invalid_pairing_code"
	APIErrorConflict           APIErrorString = "conflict"
	APIErrorNotEligible        APIErrorString = "account_not_eligible"
	APIErrorApprovalFailed     APIErrorString = "approval_failed"
	APIErrorUnavailable        APIErrorString = "unavailable"
	APIErrorGone               APIErrorString = "gone"
)

// MaxConsolesPerAccount caps the Wii and DS consoles one account may link (decision D7).
const MaxConsolesPerAccount = 8

// pairCore is the slice of the account core approval needs; tests swap it.
type pairCore interface {
	Reserve(ctx context.Context, accountID, subject, key string) (string, error)
	Activate(ctx context.Context, linkID, key string) error
	Unlink(ctx context.Context, linkID, key string) error
	LinkActive(ctx context.Context, subject string) (linkID, accountID string, active bool, err error)
}

// pairStore is the slice of the database approval needs; tests swap it.
type pairStore interface {
	LookupPairingCode(ctx context.Context, raw string, proof database.NumberProof) (database.PairingTarget, error)
	BeginApproval(ctx context.Context, raw, accountID string, proof database.NumberProof) (database.PairingTarget, bool, error)
	FinishApproval(ctx context.Context, t database.PairingTarget, accountID, linkID string) error
	AbortApproval(ctx context.Context, code, accountID string) error
	StuckApprovals(ctx context.Context) ([]database.StuckApproval, error)
	ConsolesOf(ctx context.Context, accountID string) ([]database.LinkedConsole, error)
	ForgetSubject(ctx context.Context, subject string) (uint64, error)
	EraseUsers(ctx context.Context, userIDs []int64) (int, error)
	PairingCounters(ctx context.Context) (database.PairingStats, error)
}

var (
	pairCoreOnce sync.Once
	pairCoreImpl pairCore
	pairCoreErr  error
	// kickUser closes a console's GPCM sessions; swapped in tests.
	kickUser = gpcmclient.KickUser
)

func corePair() (pairCore, error) {
	pairCoreOnce.Do(func() {
		cfg := common.GetConfig()
		if cfg.CoreAddress == "" {
			pairCoreErr = errors.New("WFC_CORE_ADDRESS not set")
			return
		}
		pairCoreImpl, _, pairCoreErr = mkwboard.DialCore(cfg.CoreAddress, cfg.CoreKey)
	})
	return pairCoreImpl, pairCoreErr
}

type pairRequest struct {
	AuthInfo
	Code      string `json:"code"`
	AccountID string `json:"core_account_id"`
	LinkID    string `json:"link_id"`
	Erase     bool   `json:"erase"`
	UserID    uint64 `json:"user_id"`
	// A claim by the 5-digit number needs one of these (a code needs neither): the browser's
	// address, compared as a network tag with the console's, or the console's in-game name.
	ClientIP string `json:"client_ip"`
	Name     string `json:"name"`
}

func (r pairRequest) proof() database.NumberProof {
	return database.NumberProof{NetTags: database.NetTags(database.NetTagKey(), r.ClientIP, time.Now()), Name: r.Name}
}

// PairResponseSpec is the console behind a pairing code, or the result of an approval.
type PairResponseSpec struct {
	Status        string `json:"status,omitempty"`
	Unit          string `json:"unit,omitempty"` // "wii" or "ds"
	GameCode      string `json:"gsbrcd,omitempty"`
	Name          string `json:"name,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	AlreadyLinked bool   `json:"already_linked,omitempty"`
	LinkID        string `json:"link_id,omitempty"`
}

// ConsoleSpec is one linked console as the account page lists it. The console's user id is a
// bearer value on the DS and is never returned.
type ConsoleSpec struct {
	LinkID   string   `json:"link_id"`
	Unit     string   `json:"unit"`
	Games    []string `json:"games"`
	LinkedAt string   `json:"linked_at"`
}

func unitString(u byte) string {
	if u == database.UnitDS {
		return "ds"
	}
	return "wii"
}

// pairParse is parsePost that also refuses to serve at all when no API secret is configured,
// so an unset secret cannot turn the pairing endpoints into an open door.
func pairParse(w http.ResponseWriter, r *http.Request, req *pairRequest) bool {
	if apiSecret == "" {
		replyError(w, http.StatusUnauthorized, APIErrorFailedAuthentication)
		return false
	}
	return parsePost(r, w, req, RoleAdmin) == nil
}

func pairFail(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, database.ErrPairingCode):
		replyError(w, http.StatusNotFound, APIErrorInvalidPairingCode)
	case errors.Is(err, database.ErrConsoleLinked), errors.Is(err, errConsoleCap):
		replyError(w, http.StatusConflict, APIErrorConflict)
	case errors.Is(err, mkwboard.ErrLinkConflict):
		replyError(w, http.StatusUnprocessableEntity, APIErrorApprovalFailed)
	default:
		logging.Error("API", "pairing", what+":", err)
		replyError(w, http.StatusServiceUnavailable, APIErrorUnavailable)
	}
}

func targetSpec(t database.PairingTarget) PairResponseSpec {
	return PairResponseSpec{Unit: unitString(t.Unit), GameCode: t.GameCode, Name: t.Name,
		ExpiresAt: t.ExpiresAt.UTC().Format(time.RFC3339)}
}

// HandlePairPreview: POST {secret, code} -> the console a code or link number belongs to, no
// side effect. /api/pair/lookup is the same (the name before link-first).
func HandlePairPreview(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	t, err := db.LookupPairingCode(r.Context(), req.Code, req.proof())
	if err != nil {
		pairFail(w, "preview", err)
		return
	}
	replyOK(w, targetSpec(t))
}

var errConsoleCap = errors.New("account has the maximum number of consoles")

// approve links the console behind a code to an account. Resumable and idempotent: the
// subject is fixed at BeginApproval and every core step has a deterministic key.
func approve(ctx context.Context, store pairStore, core pairCore, raw, accountID string, proof database.NumberProof) (PairResponseSpec, error) {
	if accountID == "" {
		return PairResponseSpec{}, database.ErrPairingCode
	}
	t, already, err := store.BeginApproval(ctx, raw, accountID, proof)
	if err != nil {
		return PairResponseSpec{}, err
	}
	spec := targetSpec(t)
	if already {
		spec.Status, spec.AlreadyLinked = "linked", true
		return spec, nil
	}
	if list, err := store.ConsolesOf(ctx, accountID); err != nil {
		return PairResponseSpec{}, err
	} else if len(list) >= MaxConsolesPerAccount {
		_ = store.AbortApproval(ctx, t.Code, accountID)
		return PairResponseSpec{}, errConsoleCap
	}

	key := "wfcpair:" + t.Subject
	linkID, err := core.Reserve(ctx, accountID, t.Subject, key+":reserve")
	if err != nil {
		if errors.Is(err, mkwboard.ErrLinkConflict) {
			_ = store.AbortApproval(ctx, t.Code, accountID)
		}
		return PairResponseSpec{}, err
	}
	if err := core.Activate(ctx, linkID, key+":activate"); err != nil {
		if errors.Is(err, mkwboard.ErrLinkConflict) {
			_ = core.Unlink(ctx, linkID, key+":rollback")
			_ = store.AbortApproval(ctx, t.Code, accountID)
		}
		return PairResponseSpec{}, err
	}
	if err := store.FinishApproval(ctx, t, accountID, linkID); err != nil {
		// The core link exists; the gate heals the cache on the console's next login and the
		// reaper finishes it otherwise.
		return PairResponseSpec{}, err
	}
	logging.Notice("API", "pair approve: linked user", t.UserID, "account", accountID, "link", linkID, "unit", unitString(t.Unit))
	spec.Status, spec.LinkID = "linked", linkID
	return spec, nil
}

// HandlePairApprove: POST {secret, code, core_account_id} -> links the console.
func HandlePairApprove(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	core, err := corePair()
	if err != nil {
		pairFail(w, "approve", err)
		return
	}
	spec, err := approve(r.Context(), &db, core, req.Code, req.AccountID, req.proof())
	if err != nil {
		pairFail(w, "approve", err)
		return
	}
	replyOK(w, spec)
}

// HandlePairConsoles: POST {secret, core_account_id} -> the account's linked consoles.
func HandlePairConsoles(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	if req.AccountID == "" {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return
	}
	list, err := db.ConsolesOf(r.Context(), req.AccountID)
	if err != nil {
		pairFail(w, "consoles", err)
		return
	}
	out := make([]ConsoleSpec, 0, len(list))
	for _, c := range list {
		out = append(out, ConsoleSpec{LinkID: c.LinkID, Unit: unitString(c.Unit), Games: c.Games, LinkedAt: c.LinkedAt.UTC().Format(time.RFC3339)})
	}
	replyOK(w, map[string]any{"consoles": out})
}

// unlink revokes one console link of an account and optionally erases the console's online
// game data. The local cache, the kick and the website all follow from here at once; the
// core's link_unlinked event repeats the same steps idempotently.
func unlink(ctx context.Context, store pairStore, core pairCore, accountID, linkID string, erase bool) (bool, error) {
	list, err := store.ConsolesOf(ctx, accountID)
	if err != nil {
		return false, err
	}
	var target *database.LinkedConsole
	for i := range list {
		if list[i].LinkID == linkID {
			target = &list[i]
		}
	}
	if target == nil {
		return false, nil
	}
	if err := core.Unlink(ctx, linkID, "wfcunlink:"+linkID); err != nil {
		return false, err
	}
	if _, err := store.ForgetSubject(ctx, target.Subject); err != nil {
		return false, err
	}
	if n, err := kickUser(ctx, target.UserID, "unlinked"); err != nil {
		logging.Warn("API", "unlink: kick:", err)
	} else if n > 0 {
		logging.Notice("API", "unlink: kicked", n, "sessions of user", target.UserID)
	}
	if erase {
		n, err := store.EraseUsers(ctx, []int64{int64(target.UserID)})
		if err != nil {
			return true, err
		}
		logging.Notice("API", "unlink: erased console data user", target.UserID, "rows", n)
	}
	logging.Notice("API", "pair unlink: user", target.UserID, "account", accountID, "link", linkID, "erase", erase)
	return true, nil
}

// HandlePairUnlink: POST {secret, core_account_id, link_id, erase} -> 204, or 404 when the
// account holds no such console.
func HandlePairUnlink(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	if req.AccountID == "" || req.LinkID == "" {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return
	}
	core, err := corePair()
	if err != nil {
		pairFail(w, "unlink", err)
		return
	}
	found, err := unlink(r.Context(), &db, core, req.AccountID, req.LinkID, req.Erase)
	if err != nil {
		pairFail(w, "unlink", err)
		return
	}
	if !found {
		replyError(w, http.StatusNotFound, APIErrorInvalidQuery)
		return
	}
	replyOK(w, nil)
}

// HandlePairRedeem answered the website's pre-link-first flow (website made the core link,
// then spent the code here). Approval is this service's now: /api/pair/approve.
func HandlePairRedeem(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	replyError(w, http.StatusGone, APIErrorGone)
}

// HandlePairForget: POST {secret, user_id} -> 204; the console is offered a code again.
// Kept for the website's pre-link-first unlink; unlinking now runs through /api/pair/unlink
// and the core event.
func HandlePairForget(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	if req.UserID == 0 {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return
	}
	if err := db.ForgetPairing(r.Context(), req.UserID); err != nil {
		pairFail(w, "forget", err)
		return
	}
	replyOK(w, nil)
}

// HandlePairStats: GET -> pairing counters (no secret: counts only).
func HandlePairStats(w http.ResponseWriter, r *http.Request) {
	s, err := db.PairingCounters(r.Context())
	if err != nil {
		pairFail(w, "stats", err)
		return
	}
	replyOK(w, s)
}

// PairingHealth is the status-page check: the pairing tables answer and the core answers a
// link lookup within a second.
func PairingHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if _, err := db.PairingCounters(ctx); err != nil {
		http.Error(w, "pairing tables: unavailable\n", http.StatusServiceUnavailable)
		return
	}
	core, err := corePair()
	if err != nil {
		http.Error(w, "core: not configured\n", http.StatusServiceUnavailable)
		return
	}
	cctx, ccancel := context.WithTimeout(ctx, time.Second)
	defer ccancel()
	if _, _, _, err := core.LinkActive(cctx, "0.healthcheck"); err != nil {
		http.Error(w, "core: unavailable\n", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok\n"))
}

// reapApprovals finishes or rolls back approvals whose code expired mid-way: a live core link
// for the same account is finished (the cache written), anything else is unlinked and the code
// released.
func reapApprovals(ctx context.Context, store pairStore, core pairCore) {
	stuck, err := store.StuckApprovals(ctx)
	if err != nil {
		logging.Warn("API", "pair reaper:", err)
		return
	}
	for _, s := range stuck {
		linkID, account, active, err := core.LinkActive(ctx, s.Subject)
		if err != nil {
			logging.Warn("API", "pair reaper: core:", err)
			return
		}
		if active && account == s.AccountID {
			if err := store.FinishApproval(ctx, s.PairingTarget, s.AccountID, linkID); err == nil {
				logging.Notice("API", "pair reaper: finished user", s.UserID, "link", linkID)
			}
			continue
		}
		if linkID != "" {
			_ = core.Unlink(ctx, linkID, "wfcpair:"+s.Subject+":rollback")
		}
		if err := store.AbortApproval(ctx, s.Code, s.AccountID); err == nil {
			logging.Notice("API", "pair reaper: rolled back user", s.UserID)
		}
	}
}

// StartPairReaper runs reapApprovals every minute until ctx ends.
func StartPairReaper(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			core, err := corePair()
			if err != nil {
				continue
			}
			reapApprovals(ctx, &db, core)
		}
	}()
}

// LinkEvents applies the account core's link events to this family (corebridge calls it).
type LinkEvents struct{}

// Unlinked: the core revoked a "wfc" link. The console is forgotten and kicked.
func (LinkEvents) Unlinked(ctx context.Context, subject string) {
	uid, err := db.ForgetSubject(ctx, subject)
	if err != nil {
		logging.Warn("API", "link_unlinked:", err)
		return
	}
	if uid == 0 {
		if id, ok := database.SubjectUserID(subject); ok {
			uid = id // a pre-link-first link: subject is the bare user id
			_ = db.ForgetPairing(ctx, uid)
		}
	}
	n := 0
	if uid != 0 {
		n, _ = kickUser(ctx, uid, "unlinked")
	}
	logging.Notice("COREBRIDGE", "link_unlinked wfc subject", subject, "-> forgot user", uid, "kicked", n)
}

// AccountGone: the account was deleted (or banned, reason "banned"). Its consoles are
// forgotten (deleted) or kicked (banned).
func (LinkEvents) AccountGone(ctx context.Context, accountID, reason string) {
	if reason == "banned" {
		list, err := db.ConsolesOf(ctx, accountID)
		if err != nil {
			logging.Warn("API", "account_banned:", err)
			return
		}
		for _, c := range list {
			n, _ := kickUser(ctx, c.UserID, "banned")
			logging.Notice("COREBRIDGE", "account_banned -> kicked user", c.UserID, "sessions", n)
		}
		return
	}
	ids, err := db.ForgetAccount(ctx, accountID)
	if err != nil {
		logging.Warn("API", "account_deleted:", err)
		return
	}
	for _, uid := range ids {
		n, _ := kickUser(ctx, uid, "account_deleted")
		logging.Notice("COREBRIDGE", "account_deleted -> forgot user", uid, "kicked", n)
	}
}
