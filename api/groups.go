package api

import (
	"net/http"
	qr2 "openpak/wfc-common/qr2client"
)

func HandleGroups(w http.ResponseWriter, r *http.Request) {
	query, err := parseGet(r, w, RoleNone)
	if err != nil {
		return
	}

	groups, upstreamErr := qr2.GetGroups(query["game"], query["id"], true)
	if upstreamErr != nil {
		http.Error(w, "QR2 unavailable", 503)
		return
	}

	if len(groups) == 0 {
		// I would return No Content, but here is compatibility
		replyOK(w, "[]")
		return
	}

	replyOK(w, groups)
}
