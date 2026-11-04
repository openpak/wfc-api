package api

import (
	"errors"
	"net/http"
	"time"

	"openpak/wfc-common/database"
	"openpak/wfc-common/logging"
)

// The pairing endpoints back openpak.org/link for a Wii or DS. The website calls them with the
// API secret; players never do. A code is looked up (no side effect), then, once the website
// has made the core link, redeemed (spent for good). Unlinking on the website forgets the
// console so its next login is offered a code again. Codes are never logged.

const APIErrorInvalidPairingCode APIErrorString = "invalid_pairing_code"

type pairRequest struct {
	AuthInfo
	Code   string `json:"code"`
	UserID uint64 `json:"user_id"`
}

// PairResponseSpec is the console behind a pairing code.
type PairResponseSpec struct {
	UserID    uint64 `json:"user_id"`
	GameCode  string `json:"gsbrcd,omitempty"`
	Name      string `json:"name,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
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
	if errors.Is(err, database.ErrPairingCode) {
		replyError(w, http.StatusNotFound, APIErrorInvalidPairingCode)
		return
	}
	logging.Error("API", "pairing", what+":", err)
	replyError(w, http.StatusInternalServerError, APIErrorInvalidQuery)
}

// HandlePairLookup: POST {secret, code} -> the console the code belongs to.
func HandlePairLookup(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	t, err := db.LookupPairingCode(r.Context(), req.Code)
	if err != nil {
		pairFail(w, "lookup", err)
		return
	}
	replyOK(w, PairResponseSpec{UserID: t.UserID, GameCode: t.GameCode, Name: t.Name,
		ExpiresAt: t.ExpiresAt.UTC().Format(time.RFC3339)})
}

// HandlePairRedeem: POST {secret, code} -> spends the code, marks the console linked.
func HandlePairRedeem(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if !pairParse(w, r, &req) {
		return
	}
	uid, err := db.RedeemPairingCode(r.Context(), req.Code)
	if err != nil {
		pairFail(w, "redeem", err)
		return
	}
	logging.Info("API", "pairing code redeemed for user", uid)
	replyOK(w, PairResponseSpec{UserID: uid})
}

// HandlePairForget: POST {secret, user_id} -> the console is unlinked again.
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
