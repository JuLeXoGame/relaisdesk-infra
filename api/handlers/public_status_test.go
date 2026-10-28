package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"api/mailer"
)

func TestPublicStatusCoalescesParallelProbesAndSurvivesCallerCancellation(t *testing.T) {
	oldProbe, oldPing := publicTCPProbe, publicDBPing
	defer func() { publicTCPProbe, publicDBPing = oldProbe, oldPing; resetPublicStatusCache() }()
	resetPublicStatusCache()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	wait := func(ctx context.Context) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	publicTCPProbe = func(ctx context.Context, _ string, _ int) (bool, int64) { wait(ctx); return true, 1 }
	publicDBPing = func(ctx context.Context, _ *sql.DB) bool { wait(ctx); return true }
	h := PublicStatusHandler(nil, ServerSettings{}, mailer.NewMailer("smtp.example.test", 587, "", "", "test@example.test"))
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan struct{})
	go func() {
		h(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(ctx))
		close(firstDone)
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Error("probes did not start independently")
		}
	}
	cancel()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Error("cancelled caller remained blocked")
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			h(w, httptest.NewRequest(http.MethodGet, "/status", nil))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"operational"`) {
				t.Errorf("unexpected status: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 4 {
		t.Fatalf("performed %d probes for 31 callers, want 4", calls.Load())
	}
}

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

	var calls atomic.Int32
	publicTCPProbe = func(ctx context.Context, host string, port int) (bool, int64) {
		calls.Add(1)
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
	callsBefore := calls.Load()
	if _, err := getSecond(t); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != callsBefore {
		t.Errorf("probes ran %d times, want cached (%d)", calls.Load(), callsBefore)
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
