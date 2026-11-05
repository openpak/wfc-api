package api

import (
	"net/http"
	"openpak/wfc-common/common"
	"openpak/wfc-common/database"
)

var (
	db database.Connection

	apiSecret string
)

func StartServer(reload bool) {
	// Get config
	config := common.GetConfig()

	apiSecret = config.APISecret

	// Start SQL
	db = database.Start(config)

	_ = db.EnsurePairing()

	db.RegisterEvents(config, []string{
		"profile_kicked",
		"profile_banned",
		"profile_unbanned",
	})
}

func Shutdown() {
	db.Close()
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/groups", HandleGroups)
	mux.HandleFunc("/api/stats", HandleStats)
	mux.HandleFunc("/api/ban", HandleBan)
	mux.HandleFunc("/api/unban", HandleUnban)
	mux.HandleFunc("/api/kick", HandleKick)
	mux.HandleFunc("/api/baninfo", HandleBanInfo)
	mux.HandleFunc("/api/profile", HandleProfile)
	mux.HandleFunc("/api/pair/lookup", HandlePairPreview) // the pre-link-first name
	mux.HandleFunc("/api/pair/preview", HandlePairPreview)
	mux.HandleFunc("/api/pair/approve", HandlePairApprove)
	mux.HandleFunc("/api/pair/consoles", HandlePairConsoles)
	mux.HandleFunc("/api/pair/unlink", HandlePairUnlink)
	mux.HandleFunc("/api/pair/stats", HandlePairStats)
	mux.HandleFunc("/api/pair/redeem", HandlePairRedeem)
	mux.HandleFunc("/api/pair/forget", HandlePairForget)
}
