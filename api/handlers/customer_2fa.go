package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"api/mailer"
	dbpkg "database"
)

type customerLogin2FARequest struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
	RememberDevice bool   `json:"remember_device,omitempty"`
	DeviceName     string `json:"device_name,omitempty"`
}

// CustomerLogin2FAHandler completes 2FA challenge and issues customer session token.
func CustomerLogin2FAHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req customerLogin2FARequest
		if err := decodeSingleJSON(r, &req); err != nil {
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

		sessionToken, customerID, email, deviceToken, err := dbpkg.VerifyCustomer2FAChallengeWithDevice(db, req.ChallengeToken, req.Code, req.RememberDevice, devName)
		if err != nil {
			time.Sleep(300 * time.Millisecond) // Brute-force throttling
			writeJSONError(w, err.Error(), http.StatusUnauthorized)
			return
		}

		resp := map[string]string{
			"token":       sessionToken,
			"customer_id": customerID,
			"email":       email,
		}
		if deviceToken != "" {
			resp["device_token"] = deviceToken
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

type customer2FASendEmailCodeRequest struct {
	ChallengeToken string `json:"challenge_token"`
}

// Customer2FASendEmailCodeHandler generates and emails a 6-digit 2FA login code.
func Customer2FASendEmailCodeHandler(db *sql.DB, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req customer2FASendEmailCodeRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.ChallengeToken = strings.TrimSpace(req.ChallengeToken)
		if req.ChallengeToken == "" {
			writeJSONError(w, "Jeton de challenge requis", http.StatusBadRequest)
			return
		}

		code, email, err := dbpkg.SendCustomer2FAEmailCode(db, req.ChallengeToken)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		if mail != nil {
			go func() {
				if err := mail.SendCustomer2FACode(email, code, 15*time.Minute); err != nil {
					log.Printf("[Customer 2FA] Échec envoi code email à %s: %v", email, err)
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

type customer2FASetupTokenRequest struct {
	Token string `json:"token"`
}

// Customer2FASetupWithTokenHandler provides TOTP secret/recovery codes during onboarding/password setup.
func Customer2FASetupWithTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req customer2FASetupTokenRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.Token = strings.TrimSpace(req.Token)
		if len(req.Token) < 32 || len(req.Token) > 256 {
			writeJSONError(w, "Lien invalide ou expiré", http.StatusBadRequest)
			return
		}

		// Peek token to find associated customer email without consuming it yet
		var email string
		var expiresRaw any
		tokenHash := dbpkg.HashSessionToken(req.Token)
		err := db.QueryRow(`
			SELECT email, expires_at FROM customer_login_tokens
			WHERE token_hash = ? AND consumed_at IS NULL
		`, tokenHash).Scan(&email, &expiresRaw)
		if err != nil {
			writeJSONError(w, "Lien invalide ou expiré", http.StatusBadRequest)
			return
		}
		expiresAt, parseErr := dbpkg.ParseSQLiteTime(expiresRaw)
		if parseErr != nil || !expiresAt.After(time.Now().UTC()) {
			writeJSONError(w, "Lien invalide ou expiré", http.StatusBadRequest)
			return
		}

		secret, otpauthURL, recoveryCodes, err := dbpkg.SetupCustomerTOTP(db, email)
		if err != nil {
			writeJSONError(w, "Erreur génération 2FA: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"secret":         secret,
			"otpauth_url":    otpauthURL,
			"recovery_codes": recoveryCodes,
			"email":          email,
		})
	}
}

// Customer2FASetupHandler initiates 2FA configuration for an authenticated customer.
func Customer2FASetupHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		secret, otpauthURL, recoveryCodes, err := dbpkg.SetupCustomerTOTP(db, identity.Email)
		if err != nil {
			writeJSONError(w, "Erreur génération 2FA: "+err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"secret":         secret,
			"otpauth_url":    otpauthURL,
			"recovery_codes": recoveryCodes,
		})
	}
}

type customer2FAEnableRequest struct {
	Password      string   `json:"password"`
	Secret        string   `json:"secret"`
	Code          string   `json:"code"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// Customer2FAEnableHandler verifies code and enables 2FA on account.
func Customer2FAEnableHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		var req customer2FAEnableRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.Secret = strings.TrimSpace(req.Secret)
		req.Code = strings.TrimSpace(req.Code)
		if req.Secret == "" || req.Code == "" || len(req.RecoveryCodes) == 0 {
			writeJSONError(w, "Paramètres d'activation incomplets", http.StatusBadRequest)
			return
		}

		if err := dbpkg.EnableCustomerTOTPWithPassword(db, identity.Email, req.Password, req.Secret, req.Code, req.RecoveryCodes); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Authentification à deux facteurs activée avec succès.",
		})
	}
}

type customer2FAPasswordRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// Customer2FADisableHandler disables 2FA after password confirmation.
func Customer2FADisableHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		var req customer2FAPasswordRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.Password = strings.TrimSpace(req.Password)
		if req.Password == "" {
			writeJSONError(w, "Mot de passe requis", http.StatusBadRequest)
			return
		}

		if err := dbpkg.DisableCustomerTOTP(db, identity.Email, req.Password, req.Code); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Authentification à deux facteurs désactivée.",
		})
	}
}

// Customer2FAStatusHandler returns whether 2FA is active and how many recovery codes remain.
func Customer2FAStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		enabled, confirmedAt, remaining, err := dbpkg.GetCustomerTOTPStatus(db, identity.Email)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":                  enabled,
			"confirmed_at":             confirmedAt,
			"remaining_recovery_codes": remaining,
		})
	}
}

// Customer2FARecoveryCodesHandler regenerates recovery codes after verifying password.
func Customer2FARecoveryCodesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		var req customer2FAPasswordRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		req.Password = strings.TrimSpace(req.Password)
		if req.Password == "" {
			writeJSONError(w, "Mot de passe requis", http.StatusBadRequest)
			return
		}

		codes, err := dbpkg.RegenerateCustomerRecoveryCodes(db, identity.Email, req.Password, req.Code)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"recovery_codes": codes,
		})
	}
}
