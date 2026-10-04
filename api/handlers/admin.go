package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	netmail "net/mail"
	"strconv"
	"strings"
	"time"

	"api/middleware"
)

type createLicenseRequest struct {
	Email          string `json:"email"`
	Days           int    `json:"days"`
	MaxConnections int    `json:"max_connections"`
	Plan           string `json:"plan"`
	Technicians    int    `json:"technicians"`
	Notes          string `json:"notes"`
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

type extendRequest struct {
	Days int `json:"days"`
}

type MonthlyRevenueItem struct {
	Month       string  `json:"month"`
	Label       string  `json:"label"`
	Revenue     float64 `json:"revenue"`
	OrdersCount int     `json:"orders_count"`
}

type AdminFinancialsResponse struct {
	TotalRevenue            float64              `json:"total_revenue"`
	PaidOrdersCount         int                  `json:"paid_orders_count"`
	MonthlyRecurringRevenue float64              `json:"monthly_recurring_revenue"`
	ActiveSubscribersCount  int                  `json:"active_subscribers_count"`
	MonthlyHistory          []MonthlyRevenueItem `json:"monthly_history"`
}

type adminLoginRequest struct {
	AdminToken     string `json:"admin_token"`
	LicenseID      string `json:"license_id"`
	LicenseKey     string `json:"license_key"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	TOTPCode       string `json:"totp_code"`
	Code           string `json:"code"`
	ChallengeToken string `json:"challenge_token"`
}

// AdminLoginHandler allows logging in with:
// 1. Email and account password associated with an active ADMIN license
// 2. 2FA challenge verification for an admin account
// 3. The master admin token
// 4. An active license explicitly marked with the ADMIN role
func AdminLoginHandler(db *sql.DB, adminToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Requête invalide"})
			return
		}

		// Mode 2FA: Challenge token verification
		code := strings.TrimSpace(req.TOTPCode)
		if code == "" {
			code = strings.TrimSpace(req.Code)
		}
		if req.ChallengeToken != "" && code != "" {
			authRes, lic, err := dbpkg.VerifyAdmin2FAChallenge(db, req.ChallengeToken, code)
			if err != nil {
				time.Sleep(350 * time.Millisecond)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}
			auditLog(r, "ADMIN LOGIN", "2FA challenge success, licence "+MaskLicenseID(lic.LicenseID))
			writeJSON(w, http.StatusOK, map[string]any{
				"valid":      true,
				"token":      authRes.SessionToken,
				"role":       "admin",
				"email":      lic.Email,
				"license_id": lic.LicenseID,
			})
			return
		}

		// Mode 1: Secret Admin Token
		if req.AdminToken != "" && middleware.VerifyAdminToken(req.AdminToken, adminToken) {
			sessionToken, err := dbpkg.CreateAdminSession(db)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Impossible de créer la session admin"})
				return
			}
			auditLog(r, "ADMIN LOGIN", "master token session created")
			writeJSON(w, http.StatusOK, map[string]any{
				"valid": true,
				"token": sessionToken,
				"role":  "master_admin",
			})
			return
		}

		// Mode 2: Email and Account Password with an active ADMIN license
		if req.Email != "" && req.Password != "" {
			authRes, lic, err := dbpkg.ValidateAdminEmailPassword(db, req.Email, req.Password)
			if err != nil {
				time.Sleep(350 * time.Millisecond)
				// Uniform 401 with a generic message: distinguishing valid
				// credentials without admin rights (403) from invalid ones
				// would let an attacker oracle credential validity, and the
				// raw error leaks internal account states.
				log.Printf("[Admin Login] échec mode email: %v", err)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants incorrects"})
				return
			}

			if authRes.Requires2FA {
				writeJSON(w, http.StatusOK, map[string]any{
					"valid":           false,
					"requires_2fa":    true,
					"challenge_token": authRes.ChallengeToken,
					"email":           lic.Email,
				})
				return
			}

			auditLog(r, "ADMIN LOGIN", "email login success, licence "+MaskLicenseID(lic.LicenseID))
			writeJSON(w, http.StatusOK, map[string]any{
				"valid":      true,
				"token":      authRes.SessionToken,
				"role":       "admin",
				"email":      lic.Email,
				"license_id": lic.LicenseID,
			})
			return
		}

		// Mode 3: license explicitly assigned the ADMIN role in the database.
		if req.LicenseID != "" && req.LicenseKey != "" {
			lic, err := dbpkg.ValidateLicenseCredentials(db, strings.ToUpper(strings.TrimSpace(req.LicenseID)), strings.TrimSpace(req.LicenseKey))
			if err != nil {
				time.Sleep(350 * time.Millisecond)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants de licence invalides"})
				return
			}

			if middleware.IsAdminLicense(lic) {
				auth, _, err := dbpkg.ValidateTechnicianCredentialsWith2FA(db, lic.LicenseID, req.LicenseKey, code)
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentification refusée"})
					return
				}
				if auth.Requires2FA {
					writeJSON(w, http.StatusOK, map[string]any{"valid": false, "requires_2fa": true, "challenge_token": auth.ChallengeToken})
					return
				}
				auditLog(r, "ADMIN LOGIN", "licence login success, licence "+MaskLicenseID(lic.LicenseID))
				writeJSON(w, http.StatusOK, map[string]any{"valid": true, "token": auth.SessionToken, "role": "admin", "email": lic.Email, "license_id": lic.LicenseID})
				return
			}

			time.Sleep(350 * time.Millisecond)
			// Uniform 401 (see Mode 2): a distinct 403 would oracle license
			// credential validity.
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants incorrects"})
			return
		}

		time.Sleep(350 * time.Millisecond)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants invalides ou manquants"})
	}
}

type adminPasswordSetupRequest struct {
	LicenseID   string `json:"license_id"`
	LicenseKey  string `json:"license_key"`
	NewPassword string `json:"new_password"`
}

// AdminPasswordSetupHandler configures an account password using active admin license credentials.
func AdminPasswordSetupHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminPasswordSetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Requête invalide"})
			return
		}
		// Malformed requests stay 400; credential failures below are 401 so
		// the StrictAuthLimiter counts them toward its ban (a 400 here would
		// let guessing run forever without ever triggering it).
		if strings.TrimSpace(req.LicenseID) == "" || strings.TrimSpace(req.LicenseKey) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Identifiant et clé de licence requis"})
			return
		}
		if len(req.NewPassword) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Le mot de passe doit comporter au moins 8 caractères"})
			return
		}

		email, err := dbpkg.SetAdminPasswordWithLicense(db, req.LicenseID, req.LicenseKey, req.NewPassword)
		if err != nil {
			time.Sleep(350 * time.Millisecond)
			// Generic 401: the raw errors distinguish "unknown license" from
			// "valid license without admin rights", an oracle for attackers.
			log.Printf("[Admin PasswordSetup] échec: %v", err)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants incorrects"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Mot de passe administrateur configuré avec succès.",
			"email":   email,
		})
	}
}

func AdminLogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.Fields(r.Header.Get("Authorization"))
		if len(authHeader) == 2 && authHeader[0] == "Bearer" {
			_ = dbpkg.DeleteAdminSession(db, authHeader[1])
			_ = dbpkg.DeleteTechnicianSession(db, authHeader[1])
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}

func AdminFinancialsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Calculate Real Total Revenue from paid orders
		rows, err := db.Query(`
			SELECT strftime('%Y-%m', COALESCE(paid_at, created_at)) as m,
			       SUM(price) as total_price,
			       COUNT(id) as count_orders
			FROM orders
			WHERE status = 'paid'
			GROUP BY m
			ORDER BY m ASC
		`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()

		var monthlyHistory []MonthlyRevenueItem
		var totalRevenue float64
		var totalPaidOrders int

		for rows.Next() {
			var m string
			var rev float64
			var cnt int
			if err := rows.Scan(&m, &rev, &cnt); err == nil {
				totalRevenue += rev
				totalPaidOrders += cnt
				monthlyHistory = append(monthlyHistory, MonthlyRevenueItem{
					Month:       m,
					Label:       m,
					Revenue:     rev,
					OrdersCount: cnt,
				})
			}
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// 2. Calculate Monthly Recurring Revenue (MRR) from active licenses
		licRows, err := db.Query(`
			SELECT max_connections, notes
			FROM licences
			WHERE status = 'active'
			  AND expires_at > CURRENT_TIMESTAMP
			  AND UPPER(TRIM(COALESCE(notes, ''))) != 'ADMIN'
		`)

		var mrr float64
		var activeSubs int

		if err == nil {
			defer licRows.Close()
			for licRows.Next() {
				var maxConn int
				var notes sql.NullString
				if err := licRows.Scan(&maxConn, &notes); err == nil {
					activeSubs++
					if maxConn >= 10 {
						price, _, _, calcErr := dbpkg.CalculateServerPrice("ultra", maxConn)
						if calcErr == nil {
							mrr += price
						} else {
							mrr += dbpkg.UltraBaseMonthlyPrice
						}
					} else if maxConn > 1 {
						mrr += dbpkg.ProMonthlyPrice
					} else {
						mrr += 24.90
					}
				}
			}
			if err := licRows.Err(); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}

		if len(monthlyHistory) == 0 {
			nowMonth := time.Now().Format("2006-01")
			monthlyHistory = append(monthlyHistory, MonthlyRevenueItem{
				Month:       nowMonth,
				Label:       nowMonth,
				Revenue:     totalRevenue,
				OrdersCount: totalPaidOrders,
			})
		}

		writeJSON(w, http.StatusOK, AdminFinancialsResponse{
			TotalRevenue:            totalRevenue,
			PaidOrdersCount:         totalPaidOrders,
			MonthlyRecurringRevenue: mrr,
			ActiveSubscribersCount:  activeSubs,
			MonthlyHistory:          monthlyHistory,
		})
	}
}

func AdminStatsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := dbpkg.GetStats(db)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total_licences":      stats.Total,
			"active_licences":     stats.Active,
			"expired_licences":    stats.Expired,
			"revoked_licences":    stats.Revoked,
			"current_connections": stats.CurrentConnections,
			// hbbs enforces the live quota in memory. The SQLite field is kept
			// for compatibility but must not be presented as live telemetry.
			"connection_count_available": false,
		})
	}
}

func AdminAlertsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := intQuery(r, "limit", 100)
		if limit <= 0 || limit > 500 {
			limit = 100
		}

		alerts, err := dbpkg.ListSecurityAlerts(db, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		items := make([]map[string]any, 0, len(alerts))
		for _, alert := range alerts {
			code := alert.Code
			if code != "" {
				code = dbpkg.MaskViewerCode(code)
			}
			items = append(items, map[string]any{
				"id":          alert.ID,
				"type":        alert.Type,
				"code":        code,
				"first_ip":    alert.FirstIP,
				"second_ip":   alert.SecondIP,
				"message":     alert.Message,
				"created_at":  alert.CreatedAt,
				"resolved_at": alert.ResolvedAt,
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"alerts": items})
	}
}

func AdminRevokeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := licenseIDFromRevokePath(r.URL.Path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		var req revokeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		req.Reason = strings.TrimSpace(req.Reason)
		if req.Reason == "" {
			req.Reason = "Révocation administrative"
		}
		if len(req.Reason) > 500 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason too long"})
			return
		}
		if err := dbpkg.RevokeLicense(db, licenseID, req.Reason); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}

		auditLog(r, "LICENCE REVOKE", "licence "+MaskLicenseID(licenseID)+" révoquée")
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "license_id": licenseID})
	}
}

func AdminExtendHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := licenseIDFromExtendPath(r.URL.Path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}

		var req extendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.Days <= 0 || req.Days > 3650 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 3650"})
			return
		}

		if err := dbpkg.ExtendLicense(db, licenseID, req.Days); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		auditLog(r, "LICENCE EXTEND", fmt.Sprintf("licence %s prolongée de %d jours", MaskLicenseID(licenseID), req.Days))

		lic, err := dbpkg.GetLicense(db, licenseID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"status": "extended", "license_id": licenseID})
			return
		}

		writeJSON(w, http.StatusOK, licensePayload(*lic, false))
	}
}

func AdminCreateLicenseHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createLicenseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.Days == 0 {
			req.Days = 30
		}
		if req.Days < 1 || req.Days > 3650 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 3650"})
			return
		}
		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		parsedEmail, err := netmail.ParseAddress(req.Email)
		if err != nil || parsedEmail.Address != req.Email || strings.ContainsAny(req.Email, "\r\n\t") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid email required"})
			return
		}
		if len(req.Notes) > 1000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "notes too long"})
			return
		}

		if req.Plan != "" {
			switch strings.ToLower(strings.TrimSpace(req.Plan)) {
			case "starter":
				req.MaxConnections = 1
				if req.Notes == "" {
					req.Notes = "Plan Starter (24,90€/mois)"
				}
			case "pro":
				req.MaxConnections = 5
				if req.Notes == "" {
					req.Notes = "Plan Pro (110€/mois)"
				}
			case "ultra":
				techs := req.Technicians
				if techs < 10 {
					techs = 10
				}
				req.MaxConnections = techs
				if req.Notes == "" {
					req.Notes = "Plan Ultra (" + strconv.Itoa(techs) + " techniciens)"
				}
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown plan"})
				return
			}
		}

		if req.MaxConnections == 0 {
			req.MaxConnections = 1
		}
		if req.MaxConnections < 1 || req.MaxConnections > 10000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "max_connections must be between 1 and 10000"})
			return
		}

		lic, err := dbpkg.CreateLicense(db, req.Email, req.Days, req.MaxConnections, req.Notes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		auditLog(r, "LICENCE CREATE", "licence "+MaskLicenseID(lic.LicenseID)+" créée pour "+lic.Email)
		writeJSON(w, http.StatusCreated, licensePayload(*lic, false))
	}
}

func AdminListLicensesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		email := r.URL.Query().Get("email")
		unmask := r.URL.Query().Get("unmask") == "true"
		search := r.URL.Query().Get("q")
		page, limit := paginationParams(r, 100)

		licences, err := dbpkg.ListLicenses(db, status, email, search, limit, (page-1)*limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		total, err := dbpkg.CountLicenses(db, status, email, search)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		items := make([]map[string]any, 0, len(licences))
		for _, lic := range licences {
			items = append(items, licensePayload(lic, !unmask))
		}
		if unmask {
			auditLog(r, "LICENCE EXPORT", fmt.Sprintf("export de %d clés de licence en clair", len(items)))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"licences": items,
			"total":    total,
			"page":     page,
			"limit":    limit,
		})
	}
}

func licenseIDFromRevokePath(path string) (string, bool) {
	const prefix = "/api/v1/admin/licences/"
	const suffix = "/revoke"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, id != ""
}

func licenseIDFromExtendPath(path string) (string, bool) {
	const prefix = "/api/v1/admin/licences/"
	const suffix = "/extend"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return id, id != ""
}

func intQuery(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// paginationParams reads page/limit query params shared by admin list
// endpoints. limit <= 0 (or absent with a zero default) means "no pagination".
func paginationParams(r *http.Request, defaultLimit int) (page, limit int) {
	page = intQuery(r, "page", 1)
	if page <= 0 {
		page = 1
	}
	limit = defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit = intQuery(r, "limit", defaultLimit)
	}
	if limit < 0 {
		limit = 0
	}
	if limit > 1000 {
		limit = 1000
	}
	return page, limit
}

// licensePayload renders a license for admin APIs. Plaintext keys exist only
// on structs returned at creation time (recognized by the "mpsk_" prefix);
// every database read carries the hash, which must never be displayed, so
// listings always show the support-safe hint instead.
func licensePayload(lic dbpkg.License, maskKey bool) map[string]any {
	key := lic.KeyHint
	recoverable := false
	if !maskKey && strings.HasPrefix(lic.LicenseKey, "mpsk_") {
		key = lic.LicenseKey // one-time creation display
		recoverable = true
	}
	if key == "" {
		key = MaskLicenseKey(lic.LicenseKey)
	}
	return map[string]any{
		"license_id":                 lic.LicenseID,
		"email":                      lic.Email,
		"license_key":                key,
		"key_hint":                   lic.KeyHint,
		"key_recoverable":            recoverable,
		"status":                     lic.Status,
		"created_at":                 lic.CreatedAt.Format("2006-01-02 15:04:05"),
		"expires_at":                 lic.ExpiresAt.Format("2006-01-02 15:04:05"),
		"max_connections":            lic.MaxConnections,
		"current_connections":        lic.CurrentConnections,
		"connection_count_available": false,
		"notes":                      lic.Notes,
		"revoked_reason":             lic.RevokeReason,
	}
}
