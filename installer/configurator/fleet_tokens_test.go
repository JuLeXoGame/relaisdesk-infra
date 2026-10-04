package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParkTokenClientRoundTrip(t *testing.T) {
	previousURL := APIURL
	t.Cleanup(func() { APIURL = previousURL })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sess-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/technician/device-park-tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "PARK-SECRET",
				"park_token": map[string]any{
					"id": 7, "prefix": "PARK-SECRET", "label": "Vague 1",
					"max_uses": 50, "use_count": 0, "is_active": true,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/technician/device-park-tokens":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"park_tokens": []any{map[string]any{
					"id": 7, "prefix": "PARK-SECRET", "label": "Vague 1",
					"max_uses": 50, "use_count": 3, "is_active": true,
				}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/technician/device-park-tokens/7/revoke":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	APIURL = srv.URL

	created, err := createTechnicianParkToken("sess-test", "Vague 1", "", 50, 7)
	if err != nil {
		t.Fatal(err)
	}
	if created.Token != "PARK-SECRET" || created.Park.ID != 7 {
		t.Fatalf("created = %+v", created)
	}
	toks, err := listTechnicianParkTokens("sess-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 1 || toks[0].UseCount != 3 {
		t.Fatalf("list = %+v", toks)
	}
	if err := revokeTechnicianParkToken("sess-test", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := createTechnicianParkToken("bad", "x", "", 1, 1); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad session = %v, want 401", err)
	}
}
