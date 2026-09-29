package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api/mailer"
)

func resetPublicStatusCache() {
	publicStatusCache.Lock()
	publicStatusCache.at = time.Time{}
	publicStatusCache.body = nil
	publicStatusCache.Unlock()
}

func TestPublicStatus(t *testing.T) {
	oldProbe, oldPing, oldTTL := publicTCPProbe, publicDBPing, publicStatusCacheTTL
	t.Cleanup(func() {
		publicTCPProbe, publicDBPing, publicStatusCacheTTL = oldProbe, oldPing, oldTTL
		resetPublicStatusCache()
	})
	publicStatusCacheTTL = time.Hour
	resetPublicStatusCache()

	calls := 0
	publicTCPProbe = func(ctx context.Context, host string, port int) (bool, int64) {
		calls++
		if port == 21117 { // relay port in fixture: simulate outage
			return false, 12
		}
		return true, 3
	}
	publicDBPing = func(ctx context.Context, db *sql.DB) bool { return true }

	settings := ServerSettings{ServerIP: "203.0.113.7", RendezvousPort: 21116, RelayPort: 21117}
	mail := mailer.NewMailer("smtp.example.test", 587, "", "", "RelaisDesk <test@example.com>")

	get := func() (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/status", nil)
		rec := httptest.NewRecorder()
		PublicStatusHandler(nil, settings, mail)(rec, req)
		var payload map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return rec.Code, payload
	}

	code, p := get()
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if p["status"] != "degraded" {
		t.Errorf("overall = %v, want degraded (relay down)", p["status"])
	}
	services, _ := p["services"].([]any)
	if len(services) != 3 {
		t.Fatalf("services = %d, want 3", len(services))
	}
	byName := map[string]map[string]any{}
	for _, s := range services {
		m := s.(map[string]any)
		byName[m["name"].(string)] = m
	}
	if byName["api"]["status"] != "operational" || byName["remote"]["status"] != "outage" || byName["notifications"]["status"] != "operational" {
		t.Errorf("services = %v", p["services"])
	}

	// Sanitization: no internal host, port or version leaks.
	raw, _ := json.Marshal(p)
	for _, secret := range []string{"203.0.113.7", "21116", "21117", "smtp.example.test", "587"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("response leaks %q", secret)
		}
	}

	// Second call served from cache: no new probe runs.
	callsBefore := calls
	if _, err := getSecond(t); err != nil {
		t.Fatal(err)
	}
	if calls != callsBefore {
		t.Errorf("probes ran %d times, want cached (%d)", calls, callsBefore)
	}
}

func getSecond(t *testing.T) (map[string]any, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/status", nil)
	rec := httptest.NewRecorder()
	PublicStatusHandler(nil, ServerSettings{}, nil)(rec, req)
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func TestPublicStatusOutage(t *testing.T) {
	oldProbe, oldPing, oldTTL := publicTCPProbe, publicDBPing, publicStatusCacheTTL
	t.Cleanup(func() {
		publicTCPProbe, publicDBPing, publicStatusCacheTTL = oldProbe, oldPing, oldTTL
		resetPublicStatusCache()
	})
	publicStatusCacheTTL = -1 // disable cache
	resetPublicStatusCache()

	publicTCPProbe = func(ctx context.Context, host string, port int) (bool, int64) { return true, 1 }
	publicDBPing = func(ctx context.Context, db *sql.DB) bool { return false }

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/status", nil)
	rec := httptest.NewRecorder()
	PublicStatusHandler(nil, ServerSettings{}, &mailer.Mailer{})(rec, req)
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "outage" {
		t.Errorf("overall = %v, want outage (db down)", payload["status"])
	}
}
