package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	dbpkg "database"
)

type contextKey string

const TechnicianLicenseContextKey contextKey = "technician_license_id"
const TechnicianTeamMemberContextKey contextKey = "technician_team_member"

// TechnicianAuth validates a database-backed Bearer session token and ensures
// the related technician license still exists and is active.
func TechnicianAuth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			tokenStr := parts[1]

			licenseID, err := dbpkg.ValidateTechnicianSession(db, tokenStr)
			if err != nil {
				if db != nil && dbpkg.ValidateAdminSession(db, tokenStr) == nil {
					var adminLic string
					errAdmin := db.QueryRow("SELECT license_id FROM licences WHERE UPPER(TRIM(notes)) = 'ADMIN' AND status = 'active' AND julianday(expires_at) > julianday('now') ORDER BY id ASC LIMIT 1").Scan(&adminLic)
					if errAdmin == nil && adminLic != "" {
						ctx := context.WithValue(r.Context(), TechnicianLicenseContextKey, adminLic)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
				http.Error(w, `{"error": "Session invalide ou expirée"}`, http.StatusUnauthorized)
				return
			}

			// Le middleware doit vérifier que la licence est encore active (pas juste le token)
			lic, err := dbpkg.GetLicense(db, licenseID)
			if err != nil || lic.Status != "active" || time.Now().After(lic.ExpiresAt) {
				http.Error(w, `{"error": "Licence expirée ou révoquée"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), TechnicianLicenseContextKey, licenseID)
			member, err := dbpkg.TeamMemberForSession(db, tokenStr)
			if err != nil {
				http.Error(w, `{"error":"Accès d'équipe refusé"}`, http.StatusForbidden)
				return
			}
			if member != nil {
				allowed := r.Method == http.MethodGet && (r.URL.Path == "/api/v1/technician/dashboard" || r.URL.Path == "/api/v1/technician/devices" ||
					r.URL.Path == "/api/v1/technician/device-folders" || strings.HasPrefix(r.URL.Path, "/api/v1/technician/devices/"))
				allowed = allowed || r.Method == http.MethodPost && r.URL.Path == "/api/v1/technician/network-token"
				allowed = allowed || (r.Method == http.MethodGet || r.Method == http.MethodPost) && (r.URL.Path == "/api/v1/technician/service-billing" || strings.HasPrefix(r.URL.Path, "/api/v1/technician/service-billing/"))
				if !allowed {
					http.Error(w, `{"error":"Cette action est réservée au propriétaire de la licence"}`, http.StatusForbidden)
					return
				}
				ctx = context.WithValue(ctx, TechnicianTeamMemberContextKey, member)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
