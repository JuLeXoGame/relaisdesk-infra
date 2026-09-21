package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestCORSMiddleware(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := CORS(testHandler)

	tests := []struct {
		name           string
		method         string
		origin         string
		envCORS        string
		expectedAllow  string
		expectedStatus int
	}{
		{
			name:           "Cluster OVH https",
			method:         "OPTIONS",
			origin:         "https://relaist.cluster129.hosting.ovh.net",
			expectedAllow:  "https://relaist.cluster129.hosting.ovh.net",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Relaisdesk domain GET",
			method:         "GET",
			origin:         "https://relaisdesk.fr",
			expectedAllow:  "https://relaisdesk.fr",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Untrusted shared OVH subdomain",
			method:         "POST",
			origin:         "https://sub.cluster129.hosting.ovh.net",
			expectedAllow:  "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Custom origin from CORS_ORIGINS env",
			method:         "OPTIONS",
			origin:         "https://custom-admin.example.com",
			envCORS:        "https://custom-admin.example.com,https://other.com",
			expectedAllow:  "https://custom-admin.example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Unauthorized origin",
			method:         "GET",
			origin:         "https://malicious-site.com",
			expectedAllow:  "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envCORS != "" {
				os.Setenv("CORS_ORIGINS", tc.envCORS)
				defer os.Unsetenv("CORS_ORIGINS")
			} else {
				os.Unsetenv("CORS_ORIGINS")
			}

			req := httptest.NewRequest(tc.method, "/api/v1/admin/stats", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
			if allowOrigin != tc.expectedAllow {
				t.Errorf("expected Access-Control-Allow-Origin %q, got %q", tc.expectedAllow, allowOrigin)
			}
		})
	}
}

func TestRateLimitersAreIndependent(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	strict := RateLimit(1, time.Minute)(next)
	_ = RateLimit(100, time.Minute)(next)

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		rec := httptest.NewRecorder()
		strict.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := request(); got != http.StatusOK {
		t.Fatalf("first request status = %d", got)
	}
	if got := request(); got != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, limiter was likely overwritten", got)
	}
}

func TestGetClientIPDoesNotTrustPrivatePeers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.5:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	if got := GetClientIP(req); got != "10.0.0.5" {
		t.Fatalf("GetClientIP trusted spoofable private peer header: %q", got)
	}
}
