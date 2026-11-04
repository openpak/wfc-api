package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairEndpointsNeedTheSecret(t *testing.T) {
	before := apiSecret
	defer func() { apiSecret = before }()
	for _, secret := range []string{"", "right"} {
		apiSecret = secret
		for name, h := range map[string]func(w *httptest.ResponseRecorder, body string){
			"lookup": func(w *httptest.ResponseRecorder, b string) {
				HandlePairLookup(w, httptest.NewRequest("POST", "/api/pair/lookup", strings.NewReader(b)))
			},
			"redeem": func(w *httptest.ResponseRecorder, b string) {
				HandlePairRedeem(w, httptest.NewRequest("POST", "/api/pair/redeem", strings.NewReader(b)))
			},
			"forget": func(w *httptest.ResponseRecorder, b string) {
				HandlePairForget(w, httptest.NewRequest("POST", "/api/pair/forget", strings.NewReader(b)))
			},
		} {
			w := httptest.NewRecorder()
			h(w, `{"secret":"wrong","code":"ABCD2345","user_id":1}`)
			if w.Code != 401 {
				t.Errorf("%s with secret %q and a wrong guess: %d", name, secret, w.Code)
			}
			w = httptest.NewRecorder()
			h(w, `{"code":"ABCD2345","user_id":1}`)
			if w.Code != 401 {
				t.Errorf("%s without a secret (server %q): %d", name, secret, w.Code)
			}
		}
	}
}
