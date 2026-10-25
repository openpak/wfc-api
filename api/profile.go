package api

import (
	"net/http"

	"openpak/wfc-common/common"
)

// ProfileResponseSpec is what /api/profile returns: the profile a friend code names and the
// console-level user id behind it, which is what the OpenPak core links an account to.
type ProfileResponseSpec struct {
	ProfileID  uint32 `json:"pid"`
	UserID     uint64 `json:"user_id"`
	GameCode   string `json:"gsbrcd"`
	FriendCode string `json:"fc"`
	InGameName string `json:"name,omitempty"`
}

// HandleProfile resolves a friend code (?fc=1234-5678-9012) to its profile. The code is
// verified against the profile's own game code, so a typo lands on nothing rather than on a
// stranger's profile. Secret-gated: this is for the OpenPak website, not for players.
func HandleProfile(w http.ResponseWriter, r *http.Request) {
	query, err := parseGet(r, w, RoleAdmin)
	if err != nil {
		return
	}
	typed := query.Get("fc")
	raw, reversed, ok := common.ParseFriendCode(typed)
	if !ok {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return
	}
	for _, candidate := range []uint32{uint32(raw), uint32(reversed)} {
		user, found := db.GetProfile(candidate)
		if !found || !common.FriendCodeMatches(typed, candidate, user.GsbrCode) {
			continue
		}
		replyOK(w, ProfileResponseSpec{
			ProfileID:  user.ProfileId,
			UserID:     user.UserId,
			GameCode:   user.GsbrCode,
			FriendCode: common.CalcFriendCodeString(user.ProfileId, user.GsbrCode),
			InGameName: user.LastInGameSn,
		})
		return
	}
	replyError(w, http.StatusNotFound, APIErrorInvalidProfileID)
}
