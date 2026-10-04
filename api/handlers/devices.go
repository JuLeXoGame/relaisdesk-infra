package handlers

import (
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"api/config"
	"api/middleware"
	"api/networkauth"
	"api/releasemanifest"
)

type DeviceUpdateTarget struct {
	Version        string `json:"version"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	StaggerSeconds int    `json:"stagger_seconds,omitempty"`
}

type deviceEnrollRequest struct {
	dbpkg.DeviceProof
	PeerAuthVersion int    `json:"peer_auth_version,omitempty"`
	PermanentCode   string `json:"permanent_code"`
	RustDeskID      string `json:"rustdesk_id"`
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	DevicePublicKey string `json:"device_public_key"`
	MACAddress      string `json:"mac_address,omitempty"`
	SubnetBroadcast string `json:"subnet_broadcast,omitempty"`
	AgentVersion    string `json:"agent_version,omitempty"`
}

type deviceEnrollResponse struct {
	NetworkToken        string              `json:"network_token,omitempty"`
	ExpiresAt           string              `json:"expires_at,omitempty"`
	Valid               bool                `json:"valid"`
	Error               string              `json:"error,omitempty"`
	DeviceID            string              `json:"device_id,omitempty"`
	Alias               string              `json:"alias,omitempty"`
	ServerIP            string              `json:"server_ip,omitempty"`
	RendezvousPort      int                 `json:"rendezvous_port,omitempty"`
	RelayPort           int                 `json:"relay_port,omitempty"`
	PublicKey           string              `json:"public_key,omitempty"`
	WakeTargets         []string            `json:"wake_targets,omitempty"`
	UpdateTarget        *DeviceUpdateTarget `json:"update_target,omitempty"`
	NextIntervalSeconds int                 `json:"next_interval_seconds,omitempty"`
}

type deviceHeartbeatRequest struct {
	dbpkg.DeviceProof
	PeerAuthVersion int    `json:"peer_auth_version,omitempty"`
	DeviceID        string `json:"device_id"`
	PermanentCode   string `json:"permanent_code"`
	MACAddress      string `json:"mac_address,omitempty"`
	SubnetBroadcast string `json:"subnet_broadcast,omitempty"`
	AgentVersion    string `json:"agent_version,omitempty"`
}

type customerCreateEnrollmentRequest struct {
	LicenseID string `json:"license_id"`
	Alias     string `json:"alias"`
	Notes     string `json:"notes"`
}

type updateDeviceRequest struct {
	Alias      string `json:"alias"`
	Notes      string `json:"notes"`
	FolderID   string `json:"folder_id"`
	MACAddress string `json:"mac_address,omitempty"`
}

type createFolderRequest struct {
	Name           string `json:"name"`
	ParentFolderID string `json:"parent_folder_id"`
	LicenseID      string `json:"license_id,omitempty"`
}

type updateFolderRequest struct {
	Name           string  `json:"name"`
	ParentFolderID *string `json:"parent_folder_id"`
}

// applyFolderUpdate executes a folder rename and/or move. Either field may be
// provided alone; at least one action is required.
func applyFolderUpdate(db *sql.DB, customerID int64, licenseID, folderID string, req updateFolderRequest) (renamed, moved bool, err error) {
	if req.ParentFolderID != nil {
		if err := dbpkg.MoveDeviceFolder(db, customerID, licenseID, folderID, *req.ParentFolderID); err != nil {
			return false, false, err
		}
		moved = true
	}
	if strings.TrimSpace(req.Name) != "" {
		if err := dbpkg.UpdateDeviceFolder(db, customerID, licenseID, folderID, req.Name); err != nil {
			return renamed, moved, err
		}
		renamed = true
	}
	return renamed, moved, nil
}

func getDeviceClientIP(r *http.Request) string {
	// Only the local Nginx connection may supply the originating address.
	if strings.HasPrefix(r.RemoteAddr, "127.0.0.1:") || strings.HasPrefix(r.RemoteAddr, "[::1]:") {
		r = r.Clone(r.Context())
		r.Header.Del("CF-Connecting-IP")
		return middleware.GetClientIP(r)
	}
	clone := r.Clone(r.Context())
	clone.Header.Del("CF-Connecting-IP")
	clone.Header.Del("X-Real-IP")
	clone.Header.Del("X-Forwarded-For")
	return middleware.GetClientIP(clone)
}

// DeviceEnrollHandler registers an unattended remote machine with its permanent code.
func DeviceEnrollHandler(db *sql.DB, settings ServerSettings, signers ...*networkauth.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(signers) != 1 || signers[0] == nil {
			writeJSONError(w, "Autorisation réseau indisponible", http.StatusServiceUnavailable)
			return
		}
		publicKey, err := dbpkg.GetActiveServerPublicKey(db)
		if err != nil {
			writeJSONError(w, "Configuration serveur incomplète", http.StatusServiceUnavailable)
			return
		}
		var req deviceEnrollRequest
		if err := decodeSingleJSON(r, &req); err != nil || req.PermanentCode == "" || req.RustDeskID == "" {
			writeJSONError(w, "Requête d'enrôlement invalide", http.StatusBadRequest)
			return
		}

		ip := getDeviceClientIP(r)
		dev, err := dbpkg.EnrollDeviceWithFullInfo(db, req.PermanentCode, req.RustDeskID, req.Hostname, req.OS, req.DevicePublicKey, ip, req.MACAddress, req.SubnetBroadcast, req.AgentVersion, req.DeviceProof)
		if err != nil {
			if errors.Is(err, dbpkg.ErrDeviceLimit) {
				writeJSONError(w, err.Error(), http.StatusConflict)
				return
			}
			log.Printf("[DeviceEnroll] Échec d'enrôlement: %v", err)
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, http.StatusUnauthorized, deviceEnrollResponse{
				Valid: false,
				Error: "Code permanent expiré, révoqué ou licence inactive",
			})
			return
		}

		issued, err := issueDeviceNetworkToken(db, signers[0], dev, req.PeerAuthVersion)
		if err != nil {
			log.Printf("[DeviceEnroll] Erreur clé publique: %v", err)
			writeJSONError(w, "Configuration serveur incomplète", http.StatusInternalServerError)
			return
		}

		log.Printf("[DeviceEnroll] Poste enrôlé avec succès: %s (%s, %s, MAC: %s, Ver: %s)", dev.DeviceID, dev.Hostname, dev.OS, dev.MACAddress, dev.AgentVersion)

		writeJSON(w, http.StatusOK, deviceEnrollResponse{
			NetworkToken:        issued.Token,
			ExpiresAt:           issued.ExpiresAt.UTC().Format(time.RFC3339),
			Valid:               true,
			DeviceID:            dev.DeviceID,
			Alias:               dev.Alias,
			ServerIP:            settings.ServerIP,
			RendezvousPort:      settings.RendezvousPort,
			RelayPort:           settings.RelayPort,
			PublicKey:           publicKey,
			NextIntervalSeconds: 45,
		})
	}
}

// DeviceHeartbeatHandler keeps device online status active.
func DeviceHeartbeatHandler(db *sql.DB, cfg *config.Config, signers ...*networkauth.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req deviceHeartbeatRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		identifier := strings.TrimSpace(req.DeviceID)
		if identifier == "" {
			writeJSONError(w, "Identifiant manquant", http.StatusBadRequest)
			return
		}

		ip := getDeviceClientIP(r)
		if err := dbpkg.DeviceHeartbeatWithFullInfo(db, identifier, ip, req.MACAddress, req.SubnetBroadcast, req.AgentVersion, req.DeviceProof); err != nil {
			writeJSONError(w, "Autorisation de poste refusée", http.StatusUnauthorized)
			return
		}
		dev, err := dbpkg.GetDeviceByID(db, identifier)
		if err != nil {
			writeJSONError(w, "Poste révoqué", http.StatusUnauthorized)
			return
		}
		if len(signers) != 1 {
			writeJSONError(w, "Autorisation réseau indisponible", http.StatusServiceUnavailable)
			return
		}
		issued, err := issueDeviceNetworkToken(db, signers[0], dev, req.PeerAuthVersion)
		if err != nil {
			writeJSONError(w, "Autorisation de poste refusée", http.StatusUnauthorized)
			return
		}

		// Retrieve any pending Wake-on-LAN requests or OTA update targets
		var wakeTargets []string
		var updateTarget *DeviceUpdateTarget
		if tx, err := db.Begin(); err == nil {
			wakeTargets, _ = dbpkg.PopPendingDeviceWakes(tx, dev.LicenseID)
			targetVer, _ := dbpkg.PopPendingDeviceUpdate(tx, identifier)
			_ = tx.Commit()
			if targetVer != "" {
				updateTarget = resolveDeviceUpdateTarget(cfg, dev.OS, targetVer, identifier)
			}
		}

		writeJSON(w, http.StatusOK, deviceEnrollResponse{
			Valid:               true,
			DeviceID:            identifier,
			NetworkToken:        issued.Token,
			ExpiresAt:           issued.ExpiresAt.UTC().Format(time.RFC3339),
			WakeTargets:         wakeTargets,
			UpdateTarget:        updateTarget,
			NextIntervalSeconds: 45,
		})
	}
}

func computeDeviceStagger(deviceID string) int {
	if deviceID == "" {
		return 15
	}
	var h uint32
	for i := 0; i < len(deviceID); i++ {
		h = h*31 + uint32(deviceID[i])
	}
	return int(5 + (h % 55)) // between 5 and 59 seconds
}

func resolveDeviceUpdateTarget(cfg *config.Config, osName, targetVer string, deviceID ...string) *DeviceUpdateTarget {
	stagger := 15
	if len(deviceID) > 0 && deviceID[0] != "" {
		stagger = computeDeviceStagger(deviceID[0])
	}
	// OTA trust comes from configuration (RELEASE_PUBLIC_KEY/RELEASE_KEY_ID),
	// never from a hardcoded key: rotation must not require a rebuild.
	publicKey := ""
	keyID := "release-1"
	manifestPaths := []string{}
	if cfg != nil {
		publicKey = strings.TrimSpace(cfg.ReleasePublicKey)
		keyID = releaseSigningKeyID(cfg)
		if p := strings.TrimSpace(cfg.ReleaseManifestPath); p != "" {
			manifestPaths = append(manifestPaths, p)
		}
	}
	if publicKey == "" {
		log.Printf("[Devices] mise à jour OTA indisponible sans clé publique configurée (os=%s)", osName)
		return nil
	}
	manifestPaths = append(manifestPaths,
		"/opt/relaisdesk/downloads/release-manifest.json",
		"relaisdesk/downloads/release-manifest.json",
		"../relaisdesk/downloads/release-manifest.json",
		"../../relaisdesk/downloads/release-manifest.json",
	)
	for _, p := range manifestPaths {
		if m, err := releasemanifest.LoadVerified(p, publicKey, keyID); err == nil {
			if target := updateTargetFromManifest(m, osName, targetVer, stagger); target != nil {
				return target
			}
		}
	}
	// Fail closed: without a verified manifest artifact there is no
	// operator-signed SHA256, so no update is offered. Devices keep
	// polling and update once the manifest is back.
	log.Printf("[Devices] mise à jour OTA indisponible sans manifeste vérifié (os=%s)", osName)
	return nil
}

// updateTargetFromManifest builds the OTA target from a verified manifest,
// or nil when the artifact is absent. There is deliberately no unverified
// fallback: devices must never install an update the operator did not sign.
func updateTargetFromManifest(manifest *releasemanifest.Manifest, osName, targetVer string, stagger int) *DeviceUpdateTarget {
	if manifest == nil {
		return nil
	}
	artifactName := "RelaisDesk_Portable.exe"
	if strings.EqualFold(osName, "linux") {
		artifactName = "RelaisDesk_viewer.deb"
	}
	for _, a := range manifest.Artifacts {
		if a.Name != artifactName {
			continue
		}
		ver := manifest.Version
		if targetVer != "" {
			ver = targetVer
		}
		return &DeviceUpdateTarget{
			Version:        ver,
			URL:            a.URL,
			SHA256:         a.SHA256,
			StaggerSeconds: stagger,
		}
	}
	return nil
}

func issueDeviceNetworkToken(db *sql.DB, signer *networkauth.Signer, dev *dbpkg.Device, peerVersion ...int) (*networkauth.IssuedToken, error) {
	version := 0
	if len(peerVersion) > 0 && peerVersion[0] == 1 {
		version = 1
	}
	if _, err := db.Exec(`UPDATE devices SET peer_auth_version=? WHERE device_id=?`, version, dev.DeviceID); err != nil {
		return nil, err
	}
	lic, err := dbpkg.GetLicense(db, dev.LicenseID)
	if err != nil || lic.Status != "active" || !lic.ExpiresAt.After(time.Now()) {
		return nil, dbpkg.ErrDeviceAuthorization
	}
	end := time.Now().Add(5 * time.Minute)
	if lic.ExpiresAt.Before(end) {
		end = lic.ExpiresAt
	}
	folders, err := dbpkg.TeamFolderPath(db, dev.LicenseID, dev.FolderID)
	if err != nil {
		return nil, err
	}
	return signer.Issue(networkauth.IssueRequest{Subject: "device:" + dev.DeviceID, Tenant: dev.LicenseID, Role: "viewer", DevicePublicKey: dev.DevicePublicKey, MaxSessions: lic.MaxConnections, EntitlementEnds: end, FolderPath: folders, PeerAuthVersion: version})
}

func devicePage(w http.ResponseWriter, r *http.Request, db *sql.DB, customer int64, license string) {
	after := 0
	var err error
	if r.URL.Query().Get("after") != "" {
		after, err = strconv.Atoi(r.URL.Query().Get("after"))
	}
	if err != nil || after < 0 {
		writeJSONError(w, "Pagination invalide", http.StatusBadRequest)
		return
	}
	var items []dbpkg.Device
	member, _ := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember)
	if member != nil {
		items, err = dbpkg.ListTeamDevicePage(db, member.MemberID, after, 100)
	} else {
		items, err = dbpkg.ListDevicePage(db, customer, license, after, 100)
	}
	if err != nil {
		writeJSONError(w, "Impossible de charger les postes", http.StatusInternalServerError)
		return
	}
	next := 0
	if len(items) == 100 {
		next = items[len(items)-1].ID
	}
	quotas, err := dbpkg.ListFleetQuotas(db, customer, license)
	if err != nil {
		writeJSONError(w, "Impossible de charger les quotas du parc", http.StatusInternalServerError)
		return
	}
	var folders []dbpkg.DeviceFolder
	if member != nil {
		folders, err = dbpkg.ListTeamFolders(db, member.MemberID)
	} else {
		folders, err = dbpkg.ListDeviceFolders(db, customer, license)
	}
	if err != nil {
		folders = []dbpkg.DeviceFolder{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": items, "next_cursor": next, "quotas": quotas, "folders": folders, "device_folders": folders})
}

// CustomerListDevicesHandler returns all permanent devices belonging to the authenticated customer.
func CustomerListDevicesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		devicePage(w, r, db, identity.ID, "")
	}
}

// CustomerCreateDeviceEnrollmentHandler generates a new permanent onboarding code.
func CustomerCreateDeviceEnrollmentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		var req customerCreateEnrollmentRequest
		if err := decodeSingleJSON(r, &req); err != nil || req.LicenseID == "" {
			writeJSONError(w, "Requête invalide: licence requise", http.StatusBadRequest)
			return
		}

		if !dbpkg.CustomerOwnsLicenseID(db, identity.ID, req.LicenseID) {
			writeJSONError(w, "Licence introuvable ou non autorisée", http.StatusForbidden)
			return
		}

		dev, err := dbpkg.CreatePermanentEnrollment(db, identity.ID, req.LicenseID, req.Alias, req.Notes)
		if err != nil {
			if errors.Is(err, dbpkg.ErrDeviceLimit) {
				writeJSONError(w, err.Error(), http.StatusConflict)
				return
			}
			log.Printf("[CustomerCreateEnrollment] Erreur: %v", err)
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"device": dev,
		})
	}
}

// CustomerDeviceActionHandler updates alias or deletes a device.
func CustomerDeviceActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		path := r.URL.Path
		isDeleteAction := strings.HasSuffix(path, "/delete")
		isWakeAction := strings.HasSuffix(path, "/wake")
		isUpdateAction := strings.HasSuffix(path, "/update")
		isConnectAction := strings.HasSuffix(path, "/connect")
		var deviceID string
		if isConnectAction {
			deviceID = customerPathID(path, "/api/v1/customer/devices", "/connect")
		} else if isWakeAction {
			deviceID = customerPathID(path, "/api/v1/customer/devices", "/wake")
		} else if isUpdateAction {
			deviceID = customerPathID(path, "/api/v1/customer/devices", "/update")
		} else if isDeleteAction {
			deviceID = customerPathID(path, "/api/v1/customer/devices", "/delete")
		} else {
			deviceID = customerPathID(path, "/api/v1/customer/devices", "")
		}

		if deviceID == "" {
			writeJSONError(w, "Identifiant de poste invalide", http.StatusBadRequest)
			return
		}

		if isConnectAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			dev, err := dbpkg.GetDeviceByID(db, deviceID)
			if err != nil || dev.CustomerID != identity.ID {
				writeJSONError(w, "Poste introuvable", http.StatusNotFound)
				return
			}
			intervention, err := dbpkg.CreateDeviceIntervention(db, dev.LicenseID, dev.DeviceID, dev.Alias, dev.Hostname)
			if err != nil {
				log.Printf("[CustomerConnectDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible d'enregistrer l'intervention", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":      true,
				"device_id":    dev.DeviceID,
				"alias":        dev.Alias,
				"rustdesk_id":  dev.RustDeskID,
				"intervention": intervention,
			})
			return
		}

		if isUpdateAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			var updateReq struct {
				TargetVersion string `json:"target_version,omitempty"`
			}
			_ = decodeSingleJSON(r, &updateReq)
			if updateReq.TargetVersion == "" {
				updateReq.TargetVersion = "1.0.0"
			}
			if err := dbpkg.QueueDeviceUpdate(db, deviceID, identity.ID, "", updateReq.TargetVersion); err != nil {
				if errors.Is(err, dbpkg.ErrUpdateAlreadyPending) {
					writeJSONError(w, "Une mise à jour est déjà programmée pour ce poste.", http.StatusConflict)
					return
				}
				if errors.Is(err, dbpkg.ErrDeviceNotFound) {
					writeJSONError(w, "Poste introuvable", http.StatusNotFound)
					return
				}
				log.Printf("[CustomerUpdateDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de programmer la mise à jour du poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":   true,
				"device_id": deviceID,
				"message":   "Mise à jour programmée. Le poste se mettra à jour lors de son prochain battement de cœur.",
			})
			return
		}

		if isWakeAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			target, onlinePeers, err := dbpkg.QueueDeviceWake(db, deviceID, identity.ID, "")
			if err != nil {
				if errors.Is(err, dbpkg.ErrDeviceNoMAC) {
					writeJSONError(w, "Adresse MAC non renseignée pour ce poste. Impossible d'émettre le paquet Wake-on-LAN.", http.StatusBadRequest)
					return
				}
				if errors.Is(err, dbpkg.ErrDeviceWakePending) {
					writeJSONError(w, "Un signal de réveil a déjà été transmis récemment pour ce poste.", http.StatusTooManyRequests)
					return
				}
				log.Printf("[CustomerWakeDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de programmer le réveil du poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":            true,
				"device_id":          target.DeviceID,
				"alias":              target.Alias,
				"mac_address":        target.MACAddress,
				"online_relay_peers": onlinePeers,
				"message":            "Signal Wake-on-LAN programmé auprès des postes actifs du réseau",
			})
			return
		}

		if r.Method == http.MethodDelete || (r.Method == http.MethodPost && isDeleteAction) {
			if err := dbpkg.DeleteDevice(db, identity.ID, deviceID); err != nil {
				log.Printf("[CustomerDeleteDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de supprimer le poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			var req updateDeviceRequest
			if err := decodeSingleJSON(r, &req); err != nil {
				writeJSONError(w, "Requête invalide", http.StatusBadRequest)
				return
			}
			if err := dbpkg.UpdateDeviceAlias(db, identity.ID, deviceID, req.Alias, req.Notes, req.FolderID); err != nil {
				log.Printf("[CustomerUpdateDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de mettre à jour le poste", http.StatusBadRequest)
				return
			}
			if req.MACAddress != "" {
				_ = dbpkg.UpdateDeviceMAC(db, deviceID, req.MACAddress, identity.ID, "")
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		writeJSONError(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

// TechnicianListDevicesHandler returns all permanent devices assigned to the technician's license.
func TechnicianListDevicesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		devicePage(w, r, db, 0, licenseID)
	}
}

// TechnicianCreateDeviceEnrollmentHandler generates an enrollment code for the technician's license.
func TechnicianCreateDeviceEnrollmentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		var req updateDeviceRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		dev, err := dbpkg.CreatePermanentEnrollment(db, 0, licenseID, req.Alias, req.Notes)
		if err != nil {
			if errors.Is(err, dbpkg.ErrDeviceLimit) {
				writeJSONError(w, err.Error(), http.StatusConflict)
				return
			}
			log.Printf("[TechnicianCreateEnrollment] Erreur: %v", err)
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"device": dev,
		})
	}
}

// TechnicianDeviceActionHandler updates alias or deletes a device for a technician.
func TechnicianDeviceActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}

		path := r.URL.Path
		isDeleteAction := strings.HasSuffix(path, "/delete")
		isWakeAction := strings.HasSuffix(path, "/wake")
		isUpdateAction := strings.HasSuffix(path, "/update")
		isConnectAction := strings.HasSuffix(path, "/connect")
		var deviceID string
		if isConnectAction {
			deviceID = customerPathID(path, "/api/v1/technician/devices", "/connect")
		} else if isWakeAction {
			deviceID = customerPathID(path, "/api/v1/technician/devices", "/wake")
		} else if isUpdateAction {
			deviceID = customerPathID(path, "/api/v1/technician/devices", "/update")
		} else if isDeleteAction {
			deviceID = customerPathID(path, "/api/v1/technician/devices", "/delete")
		} else {
			deviceID = customerPathID(path, "/api/v1/technician/devices", "")
		}

		if deviceID == "" {
			writeJSONError(w, "Identifiant de poste invalide", http.StatusBadRequest)
			return
		}

		if isConnectAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			dev, err := dbpkg.GetDeviceByID(db, deviceID)
			if err != nil || dev.LicenseID != licenseID {
				writeJSONError(w, "Poste introuvable", http.StatusNotFound)
				return
			}
			if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
				allowed, e := dbpkg.TeamMemberCanAccessDevice(db, member, dev)
				if e != nil || !allowed {
					writeJSONError(w, "Poste non autorisé", http.StatusForbidden)
					return
				}
			}
			intervention, err := dbpkg.CreateDeviceIntervention(db, licenseID, dev.DeviceID, dev.Alias, dev.Hostname)
			if err != nil {
				log.Printf("[TechnicianConnectDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible d'enregistrer l'intervention", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":      true,
				"device_id":    dev.DeviceID,
				"alias":        dev.Alias,
				"rustdesk_id":  dev.RustDeskID,
				"intervention": intervention,
			})
			return
		}

		if isUpdateAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
				dev, err := dbpkg.GetDeviceByID(db, deviceID)
				if err != nil || dev.LicenseID != licenseID {
					writeJSONError(w, "Poste introuvable", http.StatusNotFound)
					return
				}
				allowed, e := dbpkg.TeamMemberCanAccessDevice(db, member, dev)
				if e != nil || !allowed {
					writeJSONError(w, "Poste non autorisé", http.StatusForbidden)
					return
				}
			}
			var updateReq struct {
				TargetVersion string `json:"target_version,omitempty"`
			}
			_ = decodeSingleJSON(r, &updateReq)
			if updateReq.TargetVersion == "" {
				updateReq.TargetVersion = "1.0.0"
			}
			if err := dbpkg.QueueDeviceUpdate(db, deviceID, 0, licenseID, updateReq.TargetVersion); err != nil {
				if errors.Is(err, dbpkg.ErrUpdateAlreadyPending) {
					writeJSONError(w, "Une mise à jour est déjà programmée pour ce poste.", http.StatusConflict)
					return
				}
				if errors.Is(err, dbpkg.ErrDeviceNotFound) {
					writeJSONError(w, "Poste introuvable", http.StatusNotFound)
					return
				}
				log.Printf("[TechnicianUpdateDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de programmer la mise à jour du poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":   true,
				"device_id": deviceID,
				"message":   "Mise à jour programmée. Le poste se mettra à jour lors de son prochain battement de cœur.",
			})
			return
		}

		if isWakeAction && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
				dev, err := dbpkg.GetDeviceByID(db, deviceID)
				if err != nil || dev.LicenseID != licenseID {
					writeJSONError(w, "Poste introuvable", http.StatusNotFound)
					return
				}
				allowed, e := dbpkg.TeamMemberCanAccessDevice(db, member, dev)
				if e != nil || !allowed {
					writeJSONError(w, "Poste non autorisé", http.StatusForbidden)
					return
				}
			}
			target, onlinePeers, err := dbpkg.QueueDeviceWake(db, deviceID, 0, licenseID)
			if err != nil {
				if errors.Is(err, dbpkg.ErrDeviceNoMAC) {
					writeJSONError(w, "Adresse MAC non renseignée pour ce poste. Impossible d'émettre le paquet Wake-on-LAN.", http.StatusBadRequest)
					return
				}
				if errors.Is(err, dbpkg.ErrDeviceWakePending) {
					writeJSONError(w, "Un signal de réveil a déjà été transmis récemment pour ce poste.", http.StatusTooManyRequests)
					return
				}
				log.Printf("[TechnicianWakeDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de programmer le réveil du poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success":            true,
				"device_id":          target.DeviceID,
				"alias":              target.Alias,
				"mac_address":        target.MACAddress,
				"online_relay_peers": onlinePeers,
				"message":            "Signal Wake-on-LAN programmé auprès des postes actifs du réseau",
			})
			return
		}

		if r.Method == http.MethodGet {
			dev, err := dbpkg.GetDeviceByID(db, deviceID)
			if err != nil || dev.LicenseID != licenseID {
				writeJSONError(w, "Poste introuvable", http.StatusNotFound)
				return
			}
			if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
				allowed, e := dbpkg.TeamMemberCanAccessDevice(db, member, dev)
				if e != nil || !allowed {
					writeJSONError(w, "Poste introuvable", http.StatusNotFound)
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"device": dev})
			return
		}

		if r.Method == http.MethodDelete || (r.Method == http.MethodPost && isDeleteAction) {
			if err := dbpkg.TechnicianDeleteDevice(db, licenseID, deviceID); err != nil {
				log.Printf("[TechnicianDeleteDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de supprimer le poste", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			var req updateDeviceRequest
			if err := decodeSingleJSON(r, &req); err != nil {
				writeJSONError(w, "Requête invalide", http.StatusBadRequest)
				return
			}
			if err := dbpkg.TechnicianUpdateDevice(db, licenseID, deviceID, req.Alias, req.Notes, req.FolderID); err != nil {
				log.Printf("[TechnicianUpdateDevice] Erreur: %v", err)
				writeJSONError(w, "Impossible de mettre à jour le poste", http.StatusBadRequest)
				return
			}
			if req.MACAddress != "" {
				_ = dbpkg.UpdateDeviceMAC(db, deviceID, req.MACAddress, 0, licenseID)
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		writeJSONError(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

// CustomerListFoldersHandler returns all device folders belonging to the authenticated customer.
func CustomerListFoldersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		folders, err := dbpkg.ListDeviceFolders(db, identity.ID, "")
		if err != nil {
			writeJSONError(w, "Impossible de charger les dossiers", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"folders": folders, "device_folders": folders})
	}
}

// CustomerCreateFolderHandler creates a new folder for the customer.
func CustomerCreateFolderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		var req createFolderRequest
		if err := decodeSingleJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeJSONError(w, "Nom de dossier requis", http.StatusBadRequest)
			return
		}
		folder, err := dbpkg.CreateDeviceFolder(db, identity.ID, "", req.Name, req.ParentFolderID)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"folder": folder})
	}
}

// CustomerFolderActionHandler handles PUT and DELETE for customer folders.
func CustomerFolderActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		path := r.URL.Path
		folderID := customerPathID(path, "/api/v1/customer/device-folders", "")
		if folderID == "" {
			writeJSONError(w, "Identifiant de dossier manquant", http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodDelete {
			if err := dbpkg.DeleteDeviceFolder(db, identity.ID, "", folderID); err != nil {
				writeJSONError(w, "Impossible de supprimer le dossier", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			var req updateFolderRequest
			if err := decodeSingleJSON(r, &req); err != nil {
				writeJSONError(w, "Requête invalide", http.StatusBadRequest)
				return
			}
			renamed, moved, err := applyFolderUpdate(db, identity.ID, "", folderID, req)
			if err != nil {
				if !renamed && !moved {
					writeJSONError(w, "Impossible de renommer ou déplacer le dossier", http.StatusBadRequest)
				} else {
					writeJSONError(w, "Impossible de renommer le dossier", http.StatusBadRequest)
				}
				return
			}
			if !renamed && !moved {
				writeJSONError(w, "Nom de dossier invalide", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		writeJSONError(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

// TechnicianListFoldersHandler returns all device folders for the technician's license.
func TechnicianListFoldersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		folders, err := dbpkg.ListDeviceFolders(db, 0, licenseID)
		if member, ok := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember); ok {
			folders, err = dbpkg.ListTeamFolders(db, member.MemberID)
		}
		if err != nil {
			writeJSONError(w, "Impossible de charger les dossiers", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"folders": folders, "device_folders": folders})
	}
}

// TechnicianCreateFolderHandler creates a new folder for the technician's license.
func TechnicianCreateFolderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		var req createFolderRequest
		if err := decodeSingleJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeJSONError(w, "Nom de dossier requis", http.StatusBadRequest)
			return
		}
		folder, err := dbpkg.CreateDeviceFolder(db, 0, licenseID, req.Name, req.ParentFolderID)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"folder": folder})
	}
}

// TechnicianFolderActionHandler handles PUT and DELETE for technician folders.
func TechnicianFolderActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		path := r.URL.Path
		folderID := customerPathID(path, "/api/v1/technician/device-folders", "")
		if folderID == "" {
			writeJSONError(w, "Identifiant de dossier manquant", http.StatusBadRequest)
			return
		}

		if r.Method == http.MethodDelete {
			if err := dbpkg.DeleteDeviceFolder(db, 0, licenseID, folderID); err != nil {
				writeJSONError(w, "Impossible de supprimer le dossier", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			var req updateFolderRequest
			if err := decodeSingleJSON(r, &req); err != nil {
				writeJSONError(w, "Requête invalide", http.StatusBadRequest)
				return
			}
			renamed, moved, err := applyFolderUpdate(db, 0, licenseID, folderID, req)
			if err != nil {
				if !renamed && !moved {
					writeJSONError(w, "Impossible de renommer ou déplacer le dossier", http.StatusBadRequest)
				} else {
					writeJSONError(w, "Impossible de renommer le dossier", http.StatusBadRequest)
				}
				return
			}
			if !renamed && !moved {
				writeJSONError(w, "Nom de dossier invalide", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"success": true})
			return
		}

		writeJSONError(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

type createParkTokenRequest struct {
	LicenseID string `json:"license_id"`
	Label     string `json:"label"`
	FolderID  string `json:"folder_id"`
	MaxUses   int    `json:"max_uses"`
	TTLDays   int    `json:"ttl_days"`
}

// createParkTokenForOwner mints a park token after an ownership check done by
// the caller. The plaintext token is returned once, at creation.
func createParkTokenForOwner(w http.ResponseWriter, r *http.Request, db *sql.DB, customerID int64, licenseID string) {
	var req createParkTokenRequest
	if err := decodeSingleJSON(r, &req); err != nil {
		writeJSONError(w, "Requête invalide", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.LicenseID) != "" {
		licenseID = req.LicenseID
	}
	tok, plaintext, err := dbpkg.CreateParkEnrollmentToken(db, customerID, licenseID, req.Label, req.FolderID, req.MaxUses, req.TTLDays)
	if err != nil {
		log.Printf("[CreateParkToken] Erreur: %v", err)
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	auditLog(r, "PARK TOKEN CREATE", "token "+tok.Prefix+"… créé pour licence "+MaskLicenseID(licenseID))
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      plaintext,
		"park_token": tok,
		"warning":    "Copiez ce token maintenant : il ne sera plus jamais affiché.",
	})
}

func listParkTokensForOwner(w http.ResponseWriter, r *http.Request, db *sql.DB, customerID int64, licenseID string) {
	if id := strings.TrimSpace(r.URL.Query().Get("license_id")); id != "" {
		licenseID = id
	}
	toks, err := dbpkg.ListParkEnrollmentTokens(db, customerID, licenseID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if toks == nil {
		toks = []dbpkg.ParkEnrollmentToken{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"park_tokens": toks})
}

func revokeParkTokenFromPath(w http.ResponseWriter, r *http.Request, db *sql.DB, customerID int64, licenseID, prefix string) {
	trimmed := strings.TrimPrefix(r.URL.Path, prefix)
	trimmed = strings.TrimSuffix(trimmed, "/revoke")
	id, err := strconv.ParseInt(strings.Trim(trimmed, "/"), 10, 64)
	if err != nil || id <= 0 {
		writeJSONError(w, "Identifiant de token invalide", http.StatusBadRequest)
		return
	}
	if err := dbpkg.RevokeParkEnrollmentToken(db, customerID, licenseID, id); err != nil {
		writeJSONError(w, "Token introuvable ou non autorisé", http.StatusNotFound)
		return
	}
	auditLog(r, "PARK TOKEN REVOKE", "token de parc révoqué pour licence "+MaskLicenseID(licenseID))
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func TechnicianCreateParkTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		createParkTokenForOwner(w, r, db, 0, licenseID)
	}
}

func TechnicianListParkTokensHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		listParkTokensForOwner(w, r, db, 0, licenseID)
	}
}

func TechnicianRevokeParkTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Erreur d'authentification interne", http.StatusInternalServerError)
			return
		}
		revokeParkTokenFromPath(w, r, db, 0, licenseID, "/api/v1/technician/device-park-tokens/")
	}
}

func CustomerCreateParkTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		var peek struct {
			LicenseID string `json:"license_id"`
		}
		// Ownership is enforced on the requested licence before minting.
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = json.Unmarshal(body, &peek)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if peek.LicenseID == "" || !dbpkg.CustomerOwnsLicenseID(db, identity.ID, peek.LicenseID) {
			writeJSONError(w, "Licence introuvable ou non autorisée", http.StatusForbidden)
			return
		}
		createParkTokenForOwner(w, r, db, identity.ID, peek.LicenseID)
	}
}

func CustomerListParkTokensHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		licenseID := strings.TrimSpace(r.URL.Query().Get("license_id"))
		if licenseID == "" || !dbpkg.CustomerOwnsLicenseID(db, identity.ID, licenseID) {
			writeJSONError(w, "Licence introuvable ou non autorisée", http.StatusForbidden)
			return
		}
		listParkTokensForOwner(w, r, db, identity.ID, licenseID)
	}
}

func CustomerRevokeParkTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		// Resolve the licence from the token row itself, then verify ownership.
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/customer/device-park-tokens/")
		trimmed = strings.TrimSuffix(trimmed, "/revoke")
		id, err := strconv.ParseInt(strings.Trim(trimmed, "/"), 10, 64)
		if err != nil || id <= 0 {
			writeJSONError(w, "Identifiant de token invalide", http.StatusBadRequest)
			return
		}
		licenseID := dbpkg.ParkTokenLicense(db, id)
		if licenseID == "" || !dbpkg.CustomerOwnsLicenseID(db, identity.ID, licenseID) {
			writeJSONError(w, "Token introuvable ou non autorisé", http.StatusNotFound)
			return
		}
		revokeParkTokenFromPath(w, r, db, identity.ID, licenseID, "/api/v1/customer/device-park-tokens/")
	}
}
