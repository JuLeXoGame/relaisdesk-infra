package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	dbpkg "database"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

func VerifyAdminToken(providedToken, expectedToken string) bool {
	if providedToken == "" || expectedToken == "" {
		return false
	}
	providedHash := sha256.Sum256([]byte(providedToken))
	expectedHash := sha256.Sum256([]byte(expectedToken))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}

// IsAdminLicense centralizes the only database-backed admin role marker.
// License identifiers are public-facing references and must never grant a role.
func IsAdminLicense(lic *dbpkg.License) bool {
	return lic != nil && strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN")
}

func AdminAuth(adminToken string, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.Fields(authHeader)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			token := parts[1]

			// The master secret is accepted only by the login endpoint and is
			// exchanged there for a short-lived opaque session.
			if db != nil && dbpkg.ValidateAdminSession(db, token) == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Admin licenses also use an opaque, database-backed session.
			if db != nil {
				licenseID, err := dbpkg.ValidateTechnicianSession(db, token)
				if err == nil {
					lic, err := dbpkg.GetLicense(db, licenseID)
					if err == nil && lic.Status == "active" && lic.ExpiresAt.After(time.Now().UTC()) &&
						IsAdminLicense(lic) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		})
	}
}
