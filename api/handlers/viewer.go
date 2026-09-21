package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type generateViewerCodeRequest struct {
	TechnicianLicenseID string `json:"technician_license_id"`
	ClientEmail         string `json:"client_email"`
}

type viewerActivateRequest struct {
	Code string `json:"code"`
}

type viewerActivateResponse struct {
	Valid          bool   `json:"valid"`
	Error          string `json:"error,omitempty"`
	ServerIP       string `json:"server_ip,omitempty"`
	RendezvousPort int    `json:"rendezvous_port,omitempty"`
	RelayPort      int    `json:"relay_port,omitempty"`
	PublicKey      string `json:"public_key,omitempty"`
	ExpiresAt      string `json:"expires_at,omitempty"`
}

func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// ViewerActivateHandler is a public endpoint to activate a viewer code and retrieve connection details.
func ViewerActivateHandler(db *sql.DB, settings ServerSettings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req viewerActivateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		config, err := dbpkg.ValidateViewerCode(db, req.Code)
		if err != nil {
			// Do not leak specific errors (like "code inexistant"), just generic ones for security
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(viewerActivateResponse{
				Valid: false,
				Error: "Code expiré ou invalide", // Always generic!
			})
			return
		}

		// Inject server specific settings
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(viewerActivateResponse{
			Valid:          true,
			ServerIP:       settings.ServerIP,
			RendezvousPort: settings.RendezvousPort,
			RelayPort:      settings.RelayPort,
			PublicKey:      config.PublicKey,
			ExpiresAt:      config.ExpiresAt.Format(time.RFC3339),
		})
	}
}

type viewerAnnounceRequest struct {
	Code            string `json:"code"`
	RustDeskID      string `json:"rustdesk_id"`
	DevicePublicKey string `json:"device_public_key"`
}

// ViewerAnnounceHandler registers the RustDesk ID announced by the viewer after launching.
func ViewerAnnounceHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req viewerAnnounceRequest
		if err := decodeSingleJSON(r, &req); err != nil || req.Code == "" || req.RustDeskID == "" || req.DevicePublicKey == "" {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		if err := dbpkg.SetViewerRustDeskID(db, req.Code, req.RustDeskID, req.DevicePublicKey); err != nil {
			writeJSONError(w, "Code expiré, invalide ou déjà révoqué", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "ID RustDesk enregistré",
		})
	}
}

// AdminGenerateViewerCodeHandler generates a viewer code for a given technician.
func AdminGenerateViewerCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req generateViewerCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		code, err := dbpkg.CreateViewerCode(db, req.TechnicianLicenseID, req.ClientEmail)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":                  code.Code,
			"technician_license_id": code.TechnicianLicenseID,
			"client_email":          code.ClientEmail,
			"created_at":            code.CreatedAt.Format(time.RFC3339),
			"expires_at":            code.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// AdminListViewerCodesHandler lists viewer codes for a technician.
func AdminListViewerCodesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		techID := r.URL.Query().Get("technician_license_id")
		if techID == "" {
			writeJSONError(w, "technician_license_id est requis", http.StatusBadRequest)
			return
		}

		codes, err := dbpkg.ListViewerCodes(db, techID)
		if err != nil {
			writeJSONError(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		if codes == nil {
			codes = []dbpkg.ViewerCode{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"codes": codes,
		})
	}
}

// AdminRevokeViewerCodeHandler revokes a viewer code.
func AdminRevokeViewerCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 6 { // /api/v1/admin/viewer-codes/{code}/revoke
			writeJSONError(w, "Paramètres invalides", http.StatusBadRequest)
			return
		}

		code := pathParts[5] // The {code} segment

		if err := dbpkg.RevokeViewerCode(db, code); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "success",
			"message": "Code revoqué avec succès",
		})
	}
}
