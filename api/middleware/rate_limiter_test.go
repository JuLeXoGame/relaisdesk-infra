package middleware

import (
	dbpkg "database"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStrictAuthLimiterPersistenceAcrossRestarts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_limiter.db")
	db, err := dbpkg.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase() error: %v", err)
	}
	defer db.Close()

	// Dummy handler that fails by default
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth") == "valid" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})

	limiter1 := StrictAuthLimiter(3, time.Minute, db)(handler)

	reqIP := "203.0.113.50:12345"

	// 1. Attempt 1: 401
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = reqIP
	limiter1.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("attempt 1: expected 401, got %d", rec.Code)
	}

	// 2. Attempt 2: 401
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = reqIP
	limiter1.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("attempt 2: expected 401, got %d", rec.Code)
	}

	// 3. Attempt 3: 401 -> threshold reached, banned!
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = reqIP
	limiter1.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("attempt 3: expected 401, got %d", rec.Code)
	}

	// 4. Attempt 4: Should be 429 Too Many Requests
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = reqIP
	limiter1.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 4: expected 429, got %d", rec.Code)
	}

	// 5. SIMULATE API RESTART: Create a brand new limiter instance with empty memory
	limiter2 := StrictAuthLimiter(3, time.Minute, db)(handler)

	// Attempt on new instance: Should STILL be 429 because ban was restored from SQLite!
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/admin/login", nil)
	req.RemoteAddr = reqIP
	limiter2.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after restart: expected 429 from persistent ban, got %d", rec.Code)
	}
}

func TestRateLimiterEvictionDoesNotDoS(t *testing.T) {
	limiter := newIPRateLimiter(10, time.Minute)
	limiter.maxIPs = 3 // Small threshold for testing

	now := time.Now()

	// Fill table
	limiter.getOrCreate("1.1.1.1", now)
	limiter.getOrCreate("2.2.2.2", now)
	limiter.getOrCreate("3.3.3.3", now)

	if len(limiter.limiters) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(limiter.limiters))
	}

	// Add 4th IP: should evict oldest instead of crashing or refusing
	entry := limiter.getOrCreate("4.4.4.4", now.Add(time.Second))
	if entry == nil {
		t.Fatalf("expected valid entry for 4.4.4.4")
	}
	if len(limiter.limiters) > 3 {
		t.Fatalf("expected limiters to remain capped at maxIPs (3), got %d", len(limiter.limiters))
	}
	if _, exists := limiter.limiters["4.4.4.4"]; !exists {
		t.Fatalf("expected 4.4.4.4 to exist in limiter")
	}
}

func TestDeviceRateLimitKeysByDevice(t *testing.T) {
	var seenBodies []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenBodies = append(seenBodies, string(body))
		w.WriteHeader(http.StatusOK)
	})
	limited := DeviceRateLimit(100, 2, time.Minute)(handler)

	// Same device 3 times from the same IP: third is rejected…
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", strings.NewReader(`{"device_id":"DEV-AAA"}`))
		req.RemoteAddr = "198.51.100.9:1111"
		limited.ServeHTTP(rec, req)
		if i < 2 && rec.Code != http.StatusOK {
			t.Fatalf("requête %d = %d, want 200", i+1, rec.Code)
		}
		if i == 2 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("requête 3 = %d, want 429", rec.Code)
		}
	}
	// …while another device behind the same NAT IP still passes.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", strings.NewReader(`{"device_id":"DEV-BBB"}`))
	req.RemoteAddr = "198.51.100.9:2222"
	limited.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("autre device = %d, want 200", rec.Code)
	}
	// The body must reach the handler intact (middleware peeks, then restores).
	if len(seenBodies) != 3 || seenBodies[0] != `{"device_id":"DEV-AAA"}` {
		t.Fatalf("corps restaurés = %q", seenBodies)
	}
}
