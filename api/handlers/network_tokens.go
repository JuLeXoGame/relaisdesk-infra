package handlers

import (
	"api/middleware"
	"api/networkauth"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type networkTokenRequest struct {
	DevicePublicKey string `json:"device_public_key"`
}

type viewerNetworkTokenRequest struct {
	Code            string `json:"code"`
	DevicePublicKey string `json:"device_public_key"`
}

type networkTokenResponse struct {
	Valid     bool   `json:"valid"`
	Token     string `json:"network_token,omitempty"`
	ExpiresAt string `json:"network_token_expires_at,omitempty"`
	KeyID     string `json:"key_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

func TechnicianNetworkTokenHandler(db *sql.DB, signer *networkauth.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSON(w, http.StatusUnauthorized, networkTokenResponse{Error: "Authentification requise"})
			return
		}
		var req networkTokenRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, networkTokenResponse{Error: "Requête invalide"})
			return
		}
		license, err := dbpkg.GetLicenseByID(db, licenseID)
		if err != nil || license.Status != "active" || !license.ExpiresAt.After(time.Now().UTC()) {
			writeJSON(w, http.StatusUnauthorized, networkTokenResponse{Error: "Licence invalide"})
			return
		}
		request := networkauth.IssueRequest{
			Subject:         license.LicenseID,
			Tenant:          license.LicenseID,
			Role:            "technician",
			DevicePublicKey: req.DevicePublicKey,
			MaxSessions:     license.MaxConnections,
			EntitlementEnds: license.ExpiresAt,
		}
		if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
			request.Subject = "team:" + member.MemberID
			request.Role = "folder_technician"
			request.FolderAccess = member.Folders
			if end := time.Now().Add(time.Minute); end.Before(request.EntitlementEnds) {
				request.EntitlementEnds = end
			}
		}
		issued, err := signer.Issue(request)
		if err != nil {
			log.Printf("[NetworkToken] Émission technicien refusée pour %s: %v", MaskLicenseID(licenseID), err)
			writeJSON(w, http.StatusBadRequest, networkTokenResponse{Error: "Appareil ou licence invalide"})
			return
		}
		writeIssuedNetworkToken(w, issued)
	}
}

func ViewerNetworkTokenHandler(db *sql.DB, signer *networkauth.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req viewerNetworkTokenRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, networkTokenResponse{Error: "Requête invalide"})
			return
		}
		devicePublicKey, err := networkauth.NormalizeDevicePublicKey(req.DevicePublicKey)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, networkTokenResponse{Error: "Appareil ou code invalide"})
			return
		}
		viewer, err := dbpkg.ValidateViewerCode(db, req.Code)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, networkTokenResponse{Error: "Code expiré ou invalide"})
			return
		}
		license, err := dbpkg.GetLicenseByID(db, viewer.TechnicianLicenseID)
		if err != nil || license.Status != "active" || !license.ExpiresAt.After(time.Now().UTC()) {
			writeJSON(w, http.StatusUnauthorized, networkTokenResponse{Error: "Code expiré ou invalide"})
			return
		}
		entitlementEnds := viewer.ExpiresAt
		if license.ExpiresAt.Before(entitlementEnds) {
			entitlementEnds = license.ExpiresAt
		}
		maxSessions := viewer.MaxConnections
		if maxSessions < 1 {
			maxSessions = 1
		}
		issued, err := signer.Issue(networkauth.IssueRequest{
			Subject:         fmt.Sprintf("viewer:%d", viewer.ViewerID),
			Tenant:          viewer.TechnicianLicenseID,
			Role:            "viewer",
			DevicePublicKey: devicePublicKey,
			MaxSessions:     maxSessions,
			EntitlementEnds: entitlementEnds,
		})
		if err != nil {
			log.Printf("[NetworkToken] Émission viewer refusée: %v", err)
			writeJSON(w, http.StatusBadRequest, networkTokenResponse{Error: "Appareil ou code invalide"})
			return
		}
		if err := dbpkg.BindViewerNetworkDeviceKey(db, viewer.ViewerID, devicePublicKey); err != nil {
			log.Printf("[NetworkToken] Renouvellement viewer refusé: %v", err)
			writeJSON(w, http.StatusUnauthorized, networkTokenResponse{Error: "Code déjà associé à un autre appareil"})
			return
		}
		writeIssuedNetworkToken(w, issued)
	}
}

func writeIssuedNetworkToken(w http.ResponseWriter, issued *networkauth.IssuedToken) {
	writeJSON(w, http.StatusOK, networkTokenResponse{
		Valid:     true,
		Token:     issued.Token,
		ExpiresAt: issued.ExpiresAt.UTC().Format(time.RFC3339),
		KeyID:     issued.KeyID,
	})
}

func decodeSingleJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("plusieurs valeurs JSON")
		}
		return err
	}
	return nil
}
