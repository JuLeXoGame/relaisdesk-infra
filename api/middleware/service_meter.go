package middleware

import (
	dbpkg "database"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// Only authenticated metering pulses use licence capacity instead of the shared-IP budget.
func IsServiceMeterPulse(r *http.Request) bool {
	const prefix = "/api/v1/technician/service-billing/SVC-"
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/pulse") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/pulse")
	_, err := hex.DecodeString(id)
	return len(id) == 32 && err == nil
}

func ServiceOperationRateLimit(next http.Handler) http.Handler {
	limited := RateLimit(180, time.Minute)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsServiceMeterPulse(r) {
			next.ServeHTTP(w, r)
			return
		}
		limited.ServeHTTP(w, r)
	})
}

func ServiceAwareRateLimit(db *sql.DB, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		ordinary := RateLimit(limit, window)(next)
		meter := newIPRateLimiter(1, time.Minute)
		failures := newIPRateLimiter(300, time.Minute)
		authenticated := TechnicianAuth(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := r.Context().Value(TechnicianLicenseContextKey).(string)
			lic, err := dbpkg.GetLicense(db, id)
			if err != nil {
				http.Error(w, `{"error":"Licence indisponible"}`, http.StatusForbidden)
				return
			}
			capacity := lic.MaxConnections
			if capacity < 1 {
				capacity = 1
			}
			if capacity > 500 {
				capacity = 500
			}
			meter.mu.Lock()
			now := time.Now()
			entry := meter.getOrCreate(id, now)
			if now.Sub(entry.windowStart) > time.Minute {
				entry.count = 0
				entry.windowStart = now
			}
			entry.count++
			allowed := entry.count <= capacity*60+60
			meter.mu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"Trop de mesures pour cette licence"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		}))
		// A hard outer ceiling also bounds failed authentication attempts.
		pulses := RateLimit(31000, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := GetClientIP(r)
			failures.mu.Lock()
			now := time.Now()
			entry := failures.getOrCreate(ip, now)
			if now.Sub(entry.windowStart) > time.Minute {
				entry.count = 0
				entry.windowStart = now
			}
			blocked := entry.count >= failures.limit
			failures.mu.Unlock()
			if blocked {
				http.Error(w, `{"error":"Trop de tentatives refusées"}`, http.StatusTooManyRequests)
				return
			}
			rw := &customResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			authenticated.ServeHTTP(rw, r)
			if rw.statusCode == http.StatusUnauthorized || rw.statusCode == http.StatusForbidden {
				failures.mu.Lock()
				failures.getOrCreate(ip, time.Now()).count++
				failures.mu.Unlock()
			}
		}))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsServiceMeterPulse(r) {
				pulses.ServeHTTP(w, r)
				return
			}
			ordinary.ServeHTTP(w, r)
		})
	}
}
