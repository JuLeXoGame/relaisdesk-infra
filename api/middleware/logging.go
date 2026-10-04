package middleware

import (
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// customResponseWriter wraps http.ResponseWriter to capture the status code.
type customResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (rw *customResponseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// isPrivateOrLoopbackIP checks if an IP is local/private (e.g. from reverse proxy).
func isPrivateOrLoopbackIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// GetClientIP extracts the real client IP, safely honoring headers only when behind a trusted proxy.
func GetClientIP(r *http.Request) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	// Nginx on loopback overwrites X-Real-IP. Cloudflare/X-Forwarded-For
	// headers may originate with the caller and must never override it.
	if isPrivateOrLoopbackIP(remoteHost) {
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			if net.ParseIP(realIP) != nil {
				return realIP
			}
		}
	}

	return remoteHost
}

// Logging returns a middleware that logs each request with method, path, IP, duration, and status.
func Logging(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			crw := &customResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(crw, r)

			duration := time.Since(start)
			ip := GetClientIP(r)

			logger.Printf("%s %s - %d - %v - IP: %s\n", r.Method, r.URL.Path, crw.statusCode, duration, ip)
		})
	}
}

// rateLimitEntry tracks the number of requests from a single IP within a window.
type rateLimitEntry struct {
	count       int
	windowStart time.Time
	failed      int
	bannedUntil time.Time
}

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimitEntry
	limit    int
	window   time.Duration
	maxIPs   int
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	rl := &ipRateLimiter{
		limiters: make(map[string]*rateLimitEntry),
		limit:    limit,
		window:   window,
		maxIPs:   10000,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, entry := range rl.limiters {
			if !entry.bannedUntil.IsZero() && now.Before(entry.bannedUntil) {
				continue
			}
			if now.Sub(entry.windowStart) > rl.window {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *ipRateLimiter) getOrCreate(ip string, now time.Time) *rateLimitEntry {
	entry, exists := rl.limiters[ip]
	if exists {
		return entry
	}

	// Prevent memory exhaustion via LRU/expired eviction instead of general DoS
	if len(rl.limiters) >= rl.maxIPs {
		var oldestIP string
		var oldestTime time.Time
		foundExpired := false

		for k, v := range rl.limiters {
			if (v.bannedUntil.IsZero() || now.After(v.bannedUntil)) && now.Sub(v.windowStart) > rl.window {
				delete(rl.limiters, k)
				foundExpired = true
				break
			}
			if oldestTime.IsZero() || v.windowStart.Before(oldestTime) {
				oldestIP = k
				oldestTime = v.windowStart
			}
		}
		if !foundExpired && oldestIP != "" {
			delete(rl.limiters, oldestIP)
		}
	}

	entry = &rateLimitEntry{count: 0, windowStart: now}
	rl.limiters[ip] = entry
	return entry
}

// RateLimit returns a middleware that limits requests per IP to `limit` within `window`.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(limit, window)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := GetClientIP(r)

			limiter.mu.Lock()
			now := time.Now()

			entry := limiter.getOrCreate(ip, now)

			// Reset window if expired
			if now.Sub(entry.windowStart) > limiter.window {
				entry.count = 0
				entry.windowStart = now
			}

			entry.count++
			if entry.count > limiter.limit {
				limiter.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error": "Too many requests. Try again later."}`)
				return
			}
			limiter.mu.Unlock()

			next.ServeHTTP(w, r)
		})
	}
}

// DeviceRateLimit limits fleet traffic on two keys: a generous per-IP flood
// guard plus a strict per-device bucket. Corporate parks sit behind one NAT
// egress IP, so IP-only limits would cap a whole park at a few hundred
// devices; keying on the authenticated device_id instead lets parks scale
// while keeping per-device abuse (runaway loops, replay floods) contained.
// The device_id is only read here for budgeting — authentication still
// happens in the handler via the signed proof.
func DeviceRateLimit(ipLimit, deviceLimit int, window time.Duration) func(http.Handler) http.Handler {
	ipLimiter := newIPRateLimiter(ipLimit, window)
	deviceLimiter := newIPRateLimiter(deviceLimit, window)

	allow := func(limiter *ipRateLimiter, key string) bool {
		limiter.mu.Lock()
		defer limiter.mu.Unlock()
		now := time.Now()
		entry := limiter.getOrCreate(key, now)
		if now.Sub(entry.windowStart) > limiter.window {
			entry.count = 0
			entry.windowStart = now
		}
		entry.count++
		return entry.count <= limiter.limit
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deviceID := ""
			if r.Body != nil {
				body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				if err == nil {
					r.Body = io.NopCloser(bytes.NewReader(body))
					var peek struct {
						DeviceID string `json:"device_id"`
					}
					if json.Unmarshal(body, &peek) == nil {
						deviceID = strings.TrimSpace(peek.DeviceID)
					}
				}
			}
			if deviceID != "" && !allow(deviceLimiter, "device:"+deviceID) {
				writeRateLimitExceeded(w)
				return
			}
			if !allow(ipLimiter, GetClientIP(r)) {
				writeRateLimitExceeded(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeRateLimitExceeded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"error": "Too many requests. Try again later."}`)
}

// LimitRequestBody returns a middleware that limits the maximum size of the request body.
func LimitRequestBody(limit int64) func(http.Handler) http.Handler {
	return LimitRequestBodyWithOverrides(limit, nil)
}

// LimitRequestBodyWithOverrides applies a default limit and larger explicit
// limits only to endpoints that genuinely need them, such as invoice uploads.
func LimitRequestBodyWithOverrides(limit int64, overrides map[string]int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				effectiveLimit := limit
				if override, ok := overrides[r.URL.Path]; ok && override > 0 {
					effectiveLimit = override
				}
				r.Body = http.MaxBytesReader(w, r.Body, effectiveLimit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

var defaultAllowedOrigins = map[string]bool{
	"https://relaisdesk.fr":                      true,
	"https://www.relaisdesk.fr":                  true,
	"https://informatiqueadomicile03.fr":         true,
	"https://api.relaisdesk.fr":                  true,
	"https://relaist.cluster129.hosting.ovh.net": true,
}

func isOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	normOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}

	// 1. Origines autorisées par défaut
	if defaultAllowedOrigins[normOrigin] {
		return true
	}

	// 2. Variable d'environnement CORS_ORIGINS
	if corsEnv := os.Getenv("CORS_ORIGINS"); corsEnv != "" {
		for _, configured := range strings.Split(corsEnv, ",") {
			o, valid := normalizeOrigin(configured)
			if valid && o == normOrigin {
				return true
			}
		}
	}

	// 3. Local development must be explicitly enabled.
	parsed, _ := url.Parse(normOrigin)
	if strings.EqualFold(os.Getenv("ALLOW_LOCALHOST_CORS"), "true") &&
		(parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		return true
	}

	return false
}

func normalizeOrigin(origin string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), true
}

// CORS adds Cross-Origin Resource Sharing headers to the response with strict validation.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if isOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else if strings.HasPrefix(r.URL.Path, "/api/v1/public/") || strings.HasPrefix(r.URL.Path, "/api/v1/downloads/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Stripe-Signature, Accept, X-Requested-With, Origin")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recovery catches panics and returns a 500 error without crashing the server.
func Recovery(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Printf("PANIC RECOVERED: %v\n", err)
					http.Error(w, `{"error": "Internal Server Error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// StrictAuthLimiter limits sensitive auth routes and bans an IP for 15 minutes
// StrictAuthLimiter limits sensitive auth routes and bans an IP for 15 minutes
// after repeated authentication failures, persisting bans to SQLite when a database is provided.
func StrictAuthLimiter(limit int, window time.Duration, dbs ...*sql.DB) func(http.Handler) http.Handler {
	var db *sql.DB
	if len(dbs) > 0 {
		db = dbs[0]
	}
	limiter := newIPRateLimiter(limit, window)
	const banDuration = 15 * time.Minute

	// Restore active bans from database on startup
	if db != nil {
		if activeBans, err := dbpkg.LoadActiveBans(db, time.Now().UTC()); err == nil && len(activeBans) > 0 {
			limiter.mu.Lock()
			now := time.Now()
			for ip, bannedUntil := range activeBans {
				limiter.limiters[ip] = &rateLimitEntry{
					bannedUntil: bannedUntil,
					windowStart: now,
				}
			}
			limiter.mu.Unlock()
			log.Printf("[RateLimiter] %d bans d'authentification actifs restaurés depuis la base de données", len(activeBans))
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := GetClientIP(r)

			limiter.mu.Lock()
			now := time.Now()

			entry := limiter.getOrCreate(ip, now)

			if !entry.bannedUntil.IsZero() && now.Before(entry.bannedUntil) {
				limiter.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error": "Trop de tentatives. Veuillez réessayer plus tard."}`)
				return
			}

			if now.Sub(entry.windowStart) > limiter.window {
				entry.count = 0
				entry.windowStart = now
			}

			entry.count++
			if entry.count > limiter.limit {
				limiter.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error": "Trop de tentatives. Veuillez réessayer plus tard."}`)
				return
			}
			limiter.mu.Unlock()

			crw := &customResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(crw, r)

			limiter.mu.Lock()
			defer limiter.mu.Unlock()
			entry = limiter.limiters[ip]
			if entry == nil {
				return
			}

			if crw.statusCode == http.StatusUnauthorized || crw.statusCode == http.StatusForbidden {
				entry.failed++
				if db != nil {
					isBanned, bannedUntil, err := dbpkg.RecordAuthFailure(db, ip, limit, banDuration, r.URL.Path)
					if err == nil && isBanned {
						entry.failed = 0
						entry.bannedUntil = bannedUntil
					}
				} else if entry.failed >= limit {
					entry.failed = 0
					entry.bannedUntil = time.Now().Add(banDuration)
				}
				return
			}

			if crw.statusCode < http.StatusBadRequest {
				entry.failed = 0
				if db != nil {
					_ = dbpkg.RecordAuthSuccess(db, ip)
				}
			}
		})
	}
}

// SecurityHeaders adds strict security headers to the response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.Header.Get("Authorization") != "" || strings.HasPrefix(r.URL.Path, "/api/v1/customer/") ||
			strings.HasPrefix(r.URL.Path, "/api/v1/admin/") || strings.HasPrefix(r.URL.Path, "/api/v1/technician/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
		}

		next.ServeHTTP(w, r)
	})
}
