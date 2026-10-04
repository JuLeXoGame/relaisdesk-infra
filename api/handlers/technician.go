package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"api/mailer"
	"api/middleware"
)

type technicianLoginRequest struct {
	LicenseID   string `json:"license_id,omitempty"`
	LicenseKey  string `json:"license_key,omitempty"`
	Email       string `json:"email,omitempty"`
	Password    string `json:"password,omitempty"`
	TotpCode    string `json:"totp_code,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
}

type technicianLogin2FARequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
	RememberDevice bool   `json:"remember_device,omitempty"`
	DeviceName     string `json:"device_name,omitempty"`
}

type technician2FASendEmailCodeRequest struct {
	ChallengeToken string `json:"challenge_token"`
}

type generateCodeRequest struct {
	ClientEmail string `json:"client_email"`
}

// TechnicianLoginHandler validates credentials (email/password or license) and returns session credentials or 2FA challenge.
func TechnicianLoginHandler(db *sql.DB, settings ServerSettings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req technicianLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("[TechnicianLogin] Erreur décodage JSON: %v", err)
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.LicenseID = strings.ToUpper(strings.TrimSpace(req.LicenseID))
		req.LicenseKey = strings.TrimSpace(req.LicenseKey)
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		req.Password = strings.TrimSpace(req.Password)
		req.TotpCode = strings.TrimSpace(req.TotpCode)
		req.DeviceToken = strings.TrimSpace(req.DeviceToken)

		var authRes *dbpkg.TechnicianAuthResult
		var lic *dbpkg.License
		var err error

		if req.Email != "" && req.Password != "" {
			authRes, lic, err = dbpkg.ValidateTechnicianEmailPassword(db, req.Email, req.Password, req.TotpCode, req.LicenseID, req.DeviceToken)
		} else if req.LicenseID != "" && req.LicenseKey != "" {
			authRes, lic, err = dbpkg.ValidateTechnicianCredentialsWith2FA(db, req.LicenseID, req.LicenseKey, req.TotpCode, req.DeviceToken)
		} else {
			writeJSONError(w, "Identifiants requis (e-mail/mot de passe ou licence)", http.StatusBadRequest)
			return
		}

		if err != nil {
			log.Printf("[TechnicianLogin] Échec d'authentification: %v", err)
			time.Sleep(300 * time.Millisecond)
			// Generic message: backend errors distinguish unknown account,
			// unknown email and wrong password (enumeration oracle).
			writeJSONError(w, "Identifiants incorrects", http.StatusUnauthorized)
			return
		}

		if authRes.Requires2FA {
			log.Printf("[TechnicianLogin] 2FA requis pour %s", MaskLicenseID(authRes.LicenseID))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":           true,
				"requires_2fa":    true,
				"challenge_token": authRes.ChallengeToken,
				"license_id":      authRes.LicenseID,
			})
			return
		}

		publicKey, err := dbpkg.GetActiveServerPublicKey(db)
		if err != nil {
			log.Printf("[TechnicianLogin] Erreur clé publique: %v", err)
			writeJSONError(w, "Configuration serveur incomplète", http.StatusInternalServerError)
			return
		}

		log.Printf("[TechnicianLogin] Connexion réussie pour %s", MaskLicenseID(lic.LicenseID))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":                 true,
			"requires_2fa":          false,
			"token":                 authRes.SessionToken,
			"license_id":            lic.LicenseID,
			"email":                 authRes.Email,
			"restricted_to_folders": technicianIsTeamMember(db, authRes.SessionToken),
			"expires_at":            lic.ExpiresAt.Format(time.RFC3339),
			"server_ip":             settings.ServerIP,
			"rendezvous_port":       settings.RendezvousPort,
			"relay_port":            settings.RelayPort,
			"public_key":            publicKey,
		})
	}
}

type technicianGoogleLoginRequest struct {
	Credential  string `json:"credential"`
	LicenseID   string `json:"license_id,omitempty"`
	TotpCode    string `json:"totp_code,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
}

// TechnicianGoogleLoginHandler authenticates an eligible technician using their verified Google account credential.
func TechnicianGoogleLoginHandler(db *sql.DB, settings ServerSettings, expectedClientID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(expectedClientID) == "" {
			writeJSONError(w, "Connexion Google non configurée", http.StatusServiceUnavailable)
			return
		}
		var req technicianGoogleLoginRequest
		if err := decodeSingleJSON(r, &req); err != nil || strings.TrimSpace(req.Credential) == "" {
			writeJSONError(w, "Jeton Google manquant ou invalide", http.StatusBadRequest)
			return
		}

		info, err := verifyGoogleToken(req.Credential, expectedClientID)
		if err != nil {
			log.Printf("[TechnicianGoogleLogin] échec validation Google: %v", err)
			writeJSONError(w, "Échec de l'authentification Google", http.StatusUnauthorized)
			return
		}

		req.LicenseID = strings.ToUpper(strings.TrimSpace(req.LicenseID))
		req.TotpCode = strings.TrimSpace(req.TotpCode)
		req.DeviceToken = strings.TrimSpace(req.DeviceToken)

		authRes, lic, err := dbpkg.ValidateTechnicianGoogle(db, info.Email, req.TotpCode, req.LicenseID, req.DeviceToken)
		if err != nil {
			log.Printf("[TechnicianGoogleLogin] échec pour %s: %v", info.Email, err)
			time.Sleep(300 * time.Millisecond)
			writeJSONError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if authRes.Requires2FA {
			log.Printf("[TechnicianGoogleLogin] 2FA requis pour %s", MaskLicenseID(authRes.LicenseID))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":           true,
				"requires_2fa":    true,
				"challenge_token": authRes.ChallengeToken,
				"license_id":      authRes.LicenseID,
			})
			return
		}

		publicKey, err := dbpkg.GetActiveServerPublicKey(db)
		if err != nil {
			log.Printf("[TechnicianGoogleLogin] Erreur clé publique: %v", err)
			writeJSONError(w, "Configuration serveur incomplète", http.StatusInternalServerError)
			return
		}

		log.Printf("[TechnicianGoogleLogin] Connexion réussie pour %s (%s)", MaskLicenseID(lic.LicenseID), info.Email)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":                 true,
			"requires_2fa":          false,
			"token":                 authRes.SessionToken,
			"license_id":            lic.LicenseID,
			"email":                 authRes.Email,
			"restricted_to_folders": technicianIsTeamMember(db, authRes.SessionToken),
			"expires_at":            lic.ExpiresAt.Format(time.RFC3339),
			"server_ip":             settings.ServerIP,
			"rendezvous_port":       settings.RendezvousPort,
			"relay_port":            settings.RelayPort,
			"public_key":            publicKey,
		})
	}
}

// TechnicianLogin2FAHandler completes 2FA challenge for a technician and issues credentials.
func TechnicianLogin2FAHandler(db *sql.DB, settings ServerSettings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req technicianLogin2FARequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.ChallengeToken = strings.TrimSpace(req.ChallengeToken)
		req.Code = strings.TrimSpace(req.Code)
		if req.ChallengeToken == "" || req.Code == "" {
			writeJSONError(w, "Jeton de challenge et code requis", http.StatusBadRequest)
			return
		}

		devName := strings.TrimSpace(req.DeviceName)
		if devName == "" {
			devName = r.UserAgent()
			if len(devName) > 80 {
				devName = devName[:80]
			}
		}

		token, lic, devToken, err := dbpkg.VerifyTechnician2FAChallengeWithDevice(db, req.ChallengeToken, req.Code, req.RememberDevice, devName)
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			writeJSONError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		publicKey, err := dbpkg.GetActiveServerPublicKey(db)
		if err != nil {
			log.Printf("[TechnicianLogin2FA] Erreur clé publique: %v", err)
			writeJSONError(w, "Configuration serveur incomplète", http.StatusInternalServerError)
			return
		}

		log.Printf("[TechnicianLogin2FA] Connexion réussie pour %s", MaskLicenseID(lic.LicenseID))
		email := lic.Email
		if m, e := dbpkg.TeamMemberForSession(db, token); e == nil && m != nil {
			email = m.Email
		}

		respData := map[string]interface{}{
			"valid":                 true,
			"requires_2fa":          false,
			"token":                 token,
			"email":                 email,
			"license_id":            lic.LicenseID,
			"restricted_to_folders": technicianIsTeamMember(db, token),
			"expires_at":            lic.ExpiresAt.Format(time.RFC3339),
			"server_ip":             settings.ServerIP,
			"rendezvous_port":       settings.RendezvousPort,
			"relay_port":            settings.RelayPort,
			"public_key":            publicKey,
		}
		if devToken != "" {
			respData["device_token"] = devToken
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respData)
	}
}

// Technician2FASendEmailCodeHandler generates and emails a 6-digit 2FA login code to the technician.
func Technician2FASendEmailCodeHandler(db *sql.DB, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req technician2FASendEmailCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.ChallengeToken = strings.TrimSpace(req.ChallengeToken)
		if req.ChallengeToken == "" {
			writeJSONError(w, "Jeton de challenge requis", http.StatusBadRequest)
			return
		}

		code, email, err := dbpkg.SendTechnician2FAEmailCode(db, req.ChallengeToken)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if mail != nil {
			go func() {
				if err := mail.SendCustomer2FACode(email, code, 15*time.Minute); err != nil {
					log.Printf("[Technician 2FA] Échec envoi code email à %s: %v", email, err)
				}
			}()
		}

		masked := maskEmail(email)
		writeJSON(w, http.StatusOK, map[string]any{
			"success":      true,
			"message":      "Un code de sécurité vient d'être envoyé par e-mail.",
			"email_masked": masked,
		})
	}
}

type dashboardCodeItem struct {
	ID                  int    `json:"id"`
	Code                string `json:"code"`
	TechnicianLicenseID string `json:"technician_license_id"`
	ClientEmail         string `json:"client_email"`
	ClientRustDeskID    string `json:"client_rustdesk_id,omitempty"`
	CreatedAt           string `json:"created_at"`
	ExpiresAt           string `json:"expires_at"`
	UsedAt              string `json:"used_at,omitempty"`
	IsActive            bool   `json:"is_active"`
	Status              string `json:"status"` // "active", "expired", "revoked"
	MaxConnections      int    `json:"max_connections"`
	CurrentConnections  int    `json:"current_connections"`
	RevokedReason       string `json:"revoked_reason,omitempty"`
}

// TechnicianDashboardHandler returns statistics and viewer codes list.
func TechnicianDashboardHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
			writeJSON(w, http.StatusOK, map[string]any{"total_codes": 0, "active_codes": 0, "expired_codes": 0, "codes": []dashboardCodeItem{}, "restricted_to_folders": true})
			return
		}
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		codes, err := dbpkg.ListViewerCodes(db, licenseID)
		if err != nil {
			writeJSONError(w, "Erreur lors de la récupération des codes", http.StatusInternalServerError)
			return
		}

		total := len(codes)
		active := 0
		expired := 0

		now := time.Now().UTC()
		formattedCodes := make([]dashboardCodeItem, 0, len(codes))

		for _, c := range codes {
			var status string
			if !c.IsActive {
				status = "revoked"
			} else if now.After(c.ExpiresAt) {
				status = "expired"
				expired++
			} else {
				status = "active"
				active++
			}

			item := dashboardCodeItem{
				ID:                  c.ID,
				Code:                c.Code,
				TechnicianLicenseID: c.TechnicianLicenseID,
				ClientEmail:         c.ClientEmail,
				ClientRustDeskID:    c.ClientRustDeskID,
				CreatedAt:           c.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
				ExpiresAt:           c.ExpiresAt.UTC().Format("2006-01-02 15:04:05"),
				IsActive:            c.IsActive && c.ExpiresAt.After(now),
				Status:              status,
				MaxConnections:      c.MaxConnections,
				CurrentConnections:  c.CurrentConnections,
				RevokedReason:       c.RevokedReason,
			}
			if c.UsedAt != nil {
				item.UsedAt = c.UsedAt.UTC().Format("2006-01-02 15:04:05")
			}
			formattedCodes = append(formattedCodes, item)
		}

		responseJSON := map[string]interface{}{
			"total_codes":   total,
			"active_codes":  active,
			"expired_codes": expired,
			"codes":         formattedCodes,
		}

		respBytes, err := json.Marshal(responseJSON)
		if err != nil {
			writeJSONError(w, "Erreur d'encodage JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBytes)
	}
}

// TechnicianGenerateCodeHandler generates a viewer code for the authenticated technician.
func TechnicianGenerateCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		var req generateCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		code, err := dbpkg.CreateViewerCode(db, licenseID, req.ClientEmail)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":       code.Code,
			"expires_at": code.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// TechnicianRevokeCodeHandler revokes a viewer code belonging to the authenticated technician.
func TechnicianRevokeCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/technician/viewer-codes/")
		trimmed = strings.TrimSuffix(trimmed, "/revoke")
		codeStr := strings.Trim(trimmed, "/")

		if codeStr == "" {
			writeJSONError(w, "Code requis", http.StatusBadRequest)
			return
		}

		// Ensure the code actually belongs to this technician!
		var ownerID string
		err := db.QueryRow("SELECT technician_license_id FROM viewer_codes WHERE code = ?", codeStr).Scan(&ownerID)
		if err != nil {
			writeJSONError(w, "Code introuvable", http.StatusNotFound)
			return
		}

		if ownerID != licenseID {
			writeJSONError(w, "Non autorisé", http.StatusForbidden)
			return
		}

		if err := dbpkg.RevokeViewerCode(db, codeStr); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "revoked",
			"code":   codeStr,
		})
	}
}

// TechnicianDeleteCodeHandler permanently removes a viewer code for this technician.
func TechnicianDeleteCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Non autorisé", http.StatusUnauthorized)
			return
		}

		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/technician/viewer-codes/")
		trimmed = strings.TrimSuffix(trimmed, "/delete")
		codeStr := strings.Trim(trimmed, "/")

		if codeStr == "" {
			writeJSONError(w, "Code requis", http.StatusBadRequest)
			return
		}

		// Ensure the code actually belongs to this technician!
		var ownerID string
		err := db.QueryRow("SELECT technician_license_id FROM viewer_codes WHERE code = ?", codeStr).Scan(&ownerID)
		if err != nil {
			writeJSONError(w, "Code introuvable", http.StatusNotFound)
			return
		}

		if ownerID != licenseID {
			writeJSONError(w, "Non autorisé", http.StatusForbidden)
			return
		}

		if err := dbpkg.DeleteViewerCode(db, codeStr, licenseID); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "deleted",
			"code":   codeStr,
		})
	}
}

// TechnicianLogoutHandler deletes the technician's session.
func TechnicianLogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			dbpkg.DeleteTechnicianSession(db, tokenStr)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
	}
}

// TechnicianConnectCodeHandler marks an intervention for a viewer code as in_progress when the technician connects.
func TechnicianConnectCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/technician/viewer-codes/")
		trimmed = strings.TrimSuffix(trimmed, "/connect")
		codeStr := strings.Trim(trimmed, "/")

		if codeStr == "" {
			writeJSONError(w, "Code requis", http.StatusBadRequest)
			return
		}

		item, err := dbpkg.StartInterventionByViewerCode(db, licenseID, codeStr)
		if err != nil {
			log.Printf("[TechnicianConnectCode] Erreur: %v", err)
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":      true,
			"code":         codeStr,
			"intervention": item,
		})
	}
}
