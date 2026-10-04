package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

const genericLicenseError = "Cle de licence invalide ou expiree"

type ServerSettings struct {
	ServerIP       string
	RendezvousPort int
	RelayPort      int
}

type ActivateRequest struct {
	LicenseKey string `json:"license_key"`
}

type ActivateResponse struct {
	Status         string `json:"status"`
	LicenseID      string `json:"license_id,omitempty"`
	Email          string `json:"email,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	ServerIP       string `json:"server_ip,omitempty"`
	RendezvousPort int    `json:"rendezvous_port,omitempty"`
	RelayPort      int    `json:"relay_port,omitempty"`
	PublicKey      string `json:"public_key,omitempty"`
	UDPEnabled     bool   `json:"udp_enabled"`
	Error          string `json:"error,omitempty"`
}

func ActivateHandler(db *sql.DB, settings ServerSettings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ActivateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LicenseKey == "" {
			writeJSON(w, http.StatusBadRequest, ActivateResponse{Status: "invalid", Error: "Invalid request body"})
			return
		}

		lic, err := dbpkg.GetLicenseByKey(db, req.LicenseKey)
		if err != nil || !isActiveUsable(lic) {
			writeJSON(w, http.StatusUnauthorized, ActivateResponse{Status: "invalid", Error: genericLicenseError})
			return
		}

		publicKey, err := dbpkg.GetActiveServerPublicKey(db)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ActivateResponse{Status: "invalid", Error: "Configuration serveur incomplète"})
			return
		}

		writeJSON(w, http.StatusOK, ActivateResponse{
			Status:         "valid",
			LicenseID:      lic.LicenseID,
			Email:          lic.Email,
			ExpiresAt:      lic.ExpiresAt.UTC().Format(time.RFC3339),
			ServerIP:       settings.ServerIP,
			RendezvousPort: settings.RendezvousPort,
			RelayPort:      settings.RelayPort,
			PublicKey:      publicKey,
			UDPEnabled:     true,
		})
	}
}

type ValidateRequest struct {
	LicenseID  string `json:"license_id"`
	LicenseKey string `json:"license_key"`
}

type ValidateResponse struct {
	Valid         bool   `json:"valid"`
	LicenseID     string `json:"license_id,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	RemainingDays int    `json:"remaining_days,omitempty"`
	Error         string `json:"error,omitempty"`
}

func ValidateHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LicenseID == "" || req.LicenseKey == "" {
			writeJSON(w, http.StatusBadRequest, ValidateResponse{Valid: false, Error: "Invalid request"})
			return
		}

		lic, err := dbpkg.ValidateLicense(db, req.LicenseID, req.LicenseKey)
		if err != nil {
			resp := ValidateResponse{Valid: false, Error: "Licence invalide"}
			writeJSON(w, http.StatusUnauthorized, resp)
			return
		}

		remainingDays := int(time.Until(lic.ExpiresAt).Hours() / 24)
		resp := ValidateResponse{
			Valid:         true,
			LicenseID:     lic.LicenseID,
			ExpiresAt:     lic.ExpiresAt.UTC().Format(time.RFC3339),
			RemainingDays: remainingDays,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func HeartbeatHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LicenseID == "" || req.LicenseKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"status": "error"})
			return
		}

		lic, err := dbpkg.GetLicenseByKey(db, req.LicenseKey)
		if err != nil || lic.LicenseID != req.LicenseID || lic.Status != "active" || !lic.ExpiresAt.After(time.Now().UTC()) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"status": "invalid"})
			return
		}

		if _, err := db.Exec("UPDATE licences SET last_connection_at = CURRENT_TIMESTAMP WHERE license_id = ? AND license_key = ?", req.LicenseID, dbpkg.HashLicenseKey(req.LicenseKey)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func isActiveUsable(lic *dbpkg.License) bool {
	return lic != nil &&
		lic.Status == "active" &&
		lic.ExpiresAt.After(time.Now().UTC()) &&
		lic.MaxConnections > 0
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
