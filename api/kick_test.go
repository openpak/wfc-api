package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKickDoesNotReportSuccessBeforeRemoteDelivery(t *testing.T) {
	before := apiSecret
	apiSecret = "test-secret"
	defer func() { apiSecret = before }()
	key := strings.Repeat("k", 32)
	t.Setenv("WFC_INTERNAL_KEY", key)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-WFC-Internal-Key") != key {
			t.Error("missing transport auth")
		}
		http.Error(w, "unavailable", 503)
	}))
	defer upstream.Close()
	t.Setenv("WFC_GPCM_URL", upstream.URL)
	w := httptest.NewRecorder()
	HandleKick(w, httptest.NewRequest("POST", "/api/kick", strings.NewReader(`{"secret":"test-secret","pid":7,"reason":"test"}`)))
	if w.Code != 503 {
		t.Fatalf("remote failure reported as %d", w.Code)
	}
}
