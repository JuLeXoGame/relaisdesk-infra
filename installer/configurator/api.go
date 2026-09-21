package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ActivationResponse struct {
	Status              string `json:"status"`
	LicenseID           string `json:"license_id"`
	Email               string `json:"email"`
	ExpiresAt           string `json:"expires_at"`
	ServerIP            string `json:"server_ip"`
	RendezvousPort      int    `json:"rendezvous_port"`
	RelayPort           int    `json:"relay_port"`
	PublicKey           string `json:"public_key"`
	UDPEnabled          bool   `json:"udp_enabled"`
	Error               string `json:"error"`
	NetworkTokenFile    string `json:"-"`
	NetworkProofKeyFile string `json:"-"`
}

func validateLicense(licenseKey string) (*ActivationResponse, error) {
	payload := map[string]string{"license_key": licenseKey}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erreur de serialisation JSON: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, APIURL+"/api/v1/activate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("erreur de creation de la requete: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"-Configurator/1.0")

	resp, err := client.Do(req)
	if err != nil {
		if isTLSError(err) {
			return nil, fmt.Errorf("erreur de sécurité lors de la connexion au serveur (certificat invalide ou expiré). Réessayez plus tard ou contactez le support")
		}
		return nil, fmt.Errorf("impossible de contacter le serveur. Verifiez votre connexion internet: %w", err)
	}
	defer resp.Body.Close()

	var activationResp ActivationResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&activationResp); err != nil {
		return nil, fmt.Errorf("erreur de lecture de la reponse du serveur: %w", err)
	}

	if resp.StatusCode != http.StatusOK || activationResp.Status != "valid" {
		if activationResp.Error != "" {
			return nil, fmt.Errorf("activation refusee: %s", activationResp.Error)
		}
		return nil, fmt.Errorf("activation refusee par le serveur (HTTP %d)", resp.StatusCode)
	}
	if err := validateActivationResponse(&activationResp); err != nil {
		return nil, err
	}

	return &activationResp, nil
}

func validateActivationResponse(result *ActivationResponse) error {
	if result == nil {
		return errors.New("configuration serveur incomplète")
	}
	if strings.TrimSpace(result.ServerIP) == "" {
		return errors.New("configuration serveur incomplète: serveur manquant")
	}
	if err := validateRendezvousPort(result.RendezvousPort); err != nil {
		return fmt.Errorf("configuration serveur incomplète: port rendez-vous invalide: %w", err)
	}
	if err := validateRendezvousPort(result.RelayPort); err != nil {
		return fmt.Errorf("configuration serveur incomplète: port relais invalide: %w", err)
	}
	if err := validateRustDeskPublicKey(result.PublicKey); err != nil {
		return fmt.Errorf("configuration serveur incomplète: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		return errors.New("configuration serveur incomplète: expiration invalide")
	}
	if !expiresAt.After(time.Now()) {
		return errors.New("configuration serveur incomplète: licence expirée")
	}
	return nil
}

// Technician APIs

type TechnicianLoginResponse struct {
	RestrictedToFolders bool   `json:"restricted_to_folders"`
	Valid               bool   `json:"valid"`
	Requires2FA         bool   `json:"requires_2fa"`
	ChallengeToken      string `json:"challenge_token,omitempty"`
	LicenseID           string `json:"license_id,omitempty"`
	Token               string `json:"token"`
	Email               string `json:"email"`
	DeviceToken         string `json:"device_token,omitempty"`
	ExpiresAt           string `json:"expires_at"`
	ServerIP            string `json:"server_ip"`
	RendezvousPort      int    `json:"rendezvous_port"`
	RelayPort           int    `json:"relay_port"`
	PublicKey           string `json:"public_key"`
	Error               string `json:"error"`
}

type NetworkTokenResponse struct {
	Valid        bool   `json:"valid"`
	NetworkToken string `json:"network_token"`
	ExpiresAt    string `json:"network_token_expires_at"`
	KeyID        string `json:"key_id"`
	Error        string `json:"error"`
}

type CodeStat struct {
	Code             string `json:"code"`
	ClientEmail      string `json:"client_email"`
	ClientRustDeskID string `json:"client_rustdesk_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	ExpiresAt        string `json:"expires_at"`
	IsActive         bool   `json:"is_active"`
	Status           string `json:"status"`
	RevokedReason    string `json:"revoked_reason"`
}

type TechnicianDashboardResponse struct {
	RestrictedToFolders bool       `json:"restricted_to_folders"`
	TotalCodes          int        `json:"total_codes"`
	ActiveCodes         int        `json:"active_codes"`
	ExpiredCodes        int        `json:"expired_codes"`
	Codes               []CodeStat `json:"codes"`
	Error               string     `json:"error"`
}

type TechnicianGenerateResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
	Error     string `json:"error"`
}

func doTechnicianReq(method, path, token string, reqBody interface{}, respObj interface{}) error {
	var bodyBuffer *bytes.Buffer
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyBuffer = bytes.NewBuffer(jsonData)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(method, APIURL+path, bodyBuffer)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		if isTLSError(err) {
			return fmt.Errorf("erreur de sécurité lors de la connexion au serveur (certificat invalide ou expiré). Réessayez plus tard ou contactez le support")
		}
		return fmt.Errorf("impossible de contacter le serveur")
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("reponse invalide")
	}

	if resp.StatusCode >= 400 {
		var errObj struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errObj) == nil && errObj.Error != "" {
			return errors.New(errObj.Error)
		}
		return fmt.Errorf("erreur HTTP %d", resp.StatusCode)
	}

	if err := json.Unmarshal(bodyBytes, respObj); err != nil {
		return fmt.Errorf("reponse invalide")
	}

	return nil
}

func loginTechnician(identifier, secret string, extra ...string) (*TechnicianLoginResponse, error) {
	req := make(map[string]string)
	var teamLicense, deviceToken string
	if len(extra) > 0 {
		teamLicense = extra[0]
	}
	if len(extra) > 1 {
		deviceToken = extra[1]
	}
	if strings.Contains(identifier, "@") {
		req["email"] = identifier
		req["password"] = secret
		if strings.TrimSpace(teamLicense) != "" {
			req["license_id"] = strings.ToUpper(strings.TrimSpace(teamLicense))
		}
	} else {
		req["license_id"] = identifier
		req["license_key"] = secret
	}
	if strings.TrimSpace(deviceToken) != "" {
		req["device_token"] = strings.TrimSpace(deviceToken)
	}
	var res TechnicianLoginResponse
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/login", "", req, &res)
	if err != nil {
		return nil, err
	}
	if !res.Valid {
		if res.Error != "" {
			return nil, errors.New(res.Error)
		}
		return nil, fmt.Errorf("identifiants invalides ou licence expirée")
	}
	return &res, nil
}

func loginTechnicianGoogle(credential string, extra ...string) (*TechnicianLoginResponse, error) {
	req := map[string]string{
		"credential": credential,
	}
	if len(extra) > 0 && strings.TrimSpace(extra[0]) != "" {
		req["license_id"] = strings.ToUpper(strings.TrimSpace(extra[0]))
	}
	if len(extra) > 1 && strings.TrimSpace(extra[1]) != "" {
		req["device_token"] = strings.TrimSpace(extra[1])
	}
	var res TechnicianLoginResponse
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/login/google", "", req, &res)
	if err != nil {
		return nil, err
	}
	if !res.Valid {
		if res.Error != "" {
			return nil, errors.New(res.Error)
		}
		return nil, fmt.Errorf("échec de l'authentification Google")
	}
	return &res, nil
}

func requestTechnician2FAEmailCode(challengeToken string) (string, error) {
	req := map[string]string{
		"challenge_token": challengeToken,
	}
	var res struct {
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		EmailMasked string `json:"email_masked"`
		Error       string `json:"error"`
	}
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/login/email-code", "", req, &res)
	if err != nil {
		return "", err
	}
	if !res.Success {
		if res.Error != "" {
			return "", errors.New(res.Error)
		}
		return "", fmt.Errorf("échec de l'envoi du code par e-mail")
	}
	return res.EmailMasked, nil
}

func loginTechnician2FA(challengeToken, code string, rememberDevice ...bool) (*TechnicianLoginResponse, error) {
	rem := false
	if len(rememberDevice) > 0 {
		rem = rememberDevice[0]
	}
	req := map[string]any{
		"challenge_token": challengeToken,
		"code":            code,
		"remember_device": rem,
		"device_name":     PRODUCT_NAME + " Client",
	}
	var res TechnicianLoginResponse
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/login/2fa", "", req, &res)
	if err != nil {
		return nil, err
	}
	if !res.Valid || res.Token == "" {
		if res.Error != "" {
			return nil, errors.New(res.Error)
		}
		return nil, fmt.Errorf("code d'authentification incorrect ou expiré")
	}
	return &res, nil
}

func getDashboard(token string) (*TechnicianDashboardResponse, error) {
	var res TechnicianDashboardResponse
	err := doTechnicianReq(http.MethodGet, "/api/v1/technician/dashboard", token, nil, &res)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	return &res, nil
}

func getTechnicianNetworkToken(token, devicePublicKey string) (*NetworkTokenResponse, error) {
	req := map[string]string{"device_public_key": devicePublicKey}
	var res NetworkTokenResponse
	if err := doTechnicianReq(http.MethodPost, "/api/v1/technician/network-token", token, req, &res); err != nil {
		return nil, err
	}
	if !res.Valid || res.Error != "" {
		if res.Error == "" {
			res.Error = "autorisation réseau refusée"
		}
		return nil, errors.New(res.Error)
	}
	return &res, nil
}

func generateViewerCode(token, email string) (*TechnicianGenerateResponse, error) {
	req := map[string]string{
		"client_email": email,
	}
	var res TechnicianGenerateResponse
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/viewer-codes/generate", token, req, &res)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	return &res, nil
}

func revokeViewerCode(token, code string) error {
	var res map[string]string
	err := doTechnicianReq(http.MethodPut, "/api/v1/technician/viewer-codes/"+code+"/revoke", token, nil, &res)
	if err != nil {
		return err
	}
	if res["error"] != "" {
		return errors.New(res["error"])
	}
	return nil
}

func deleteViewerCode(token, code string) error {
	var res map[string]string
	err := doTechnicianReq(http.MethodDelete, "/api/v1/technician/viewer-codes/"+code, token, nil, &res)
	if err != nil {
		return err
	}
	if res["error"] != "" {
		return errors.New(res["error"])
	}
	return nil
}

func logoutTechnician(token string) error {
	var res map[string]string
	return doTechnicianReq(http.MethodPost, "/api/v1/technician/logout", token, nil, &res)
}

func technicianConnectDevice(token, deviceID string) (string, error) {
	var res struct {
		Success      bool `json:"success"`
		Intervention struct {
			InterventionID string `json:"intervention_id"`
		} `json:"intervention"`
		Error string `json:"error"`
	}
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/devices/"+deviceID+"/connect", token, nil, &res)
	if err != nil {
		return "", err
	}
	if res.Error != "" {
		return "", errors.New(res.Error)
	}
	return res.Intervention.InterventionID, nil
}

func technicianConnectViewerCode(token, code string) (string, error) {
	var res struct {
		Success      bool `json:"success"`
		Intervention struct {
			InterventionID string `json:"intervention_id"`
		} `json:"intervention"`
		Error string `json:"error"`
	}
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/viewer-codes/"+code+"/connect", token, nil, &res)
	if err != nil {
		return "", err
	}
	if res.Error != "" {
		return "", errors.New(res.Error)
	}
	return res.Intervention.InterventionID, nil
}

func technicianCompleteIntervention(token, interventionID, summary string) error {
	if interventionID == "" {
		return nil
	}
	req := map[string]string{
		"summary": summary,
	}
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/technician/interventions/"+interventionID+"/complete", token, req, &res)
}

func technicianCancelIntervention(token, interventionID string) error {
	if interventionID == "" {
		return nil
	}
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/technician/interventions/"+interventionID+"/cancel", token, map[string]string{}, &res)
}

type DeviceItem struct {
	EnrollmentState string     `json:"enrollment_state"`
	ID              int64      `json:"id"`
	DeviceID        string     `json:"device_id"`
	CustomerID      *int64     `json:"customer_id,omitempty"`
	LicenseID       string     `json:"license_id"`
	FolderID        string     `json:"folder_id"`
	PermanentCode   string     `json:"permanent_code"`
	RustDeskID      string     `json:"rustdesk_id"`
	Alias           string     `json:"alias"`
	Hostname        string     `json:"hostname"`
	OS              string     `json:"os"`
	Status          string     `json:"status"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	LastIP          string     `json:"last_ip"`
	MACAddress      string     `json:"mac_address,omitempty"`
	SubnetBroadcast string     `json:"subnet_broadcast,omitempty"`
	AgentVersion    string     `json:"agent_version,omitempty"`
	Notes           string     `json:"notes"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type WakeDeviceResponse struct {
	Success          bool   `json:"success"`
	DeviceID         string `json:"device_id"`
	Alias            string `json:"alias"`
	MACAddress       string `json:"mac_address"`
	OnlineRelayPeers int    `json:"online_relay_peers"`
	Message          string `json:"message"`
}

func wakeTechnicianDevice(token, deviceID string) (*WakeDeviceResponse, error) {
	var resp WakeDeviceResponse
	err := doTechnicianReq(http.MethodPost, fmt.Sprintf("/api/v1/technician/devices/%s/wake", deviceID), token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type UpdateDeviceResponse struct {
	Success  bool   `json:"success"`
	DeviceID string `json:"device_id"`
	Message  string `json:"message"`
}

func triggerTechnicianDeviceUpdate(token, deviceID string, targetVer ...string) (*UpdateDeviceResponse, error) {
	var resp UpdateDeviceResponse
	req := map[string]string{}
	if len(targetVer) > 0 && targetVer[0] != "" {
		req["target_version"] = targetVer[0]
	}
	err := doTechnicianReq(http.MethodPost, fmt.Sprintf("/api/v1/technician/devices/%s/update", deviceID), token, req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type TechnicianEnrollCodeResponse struct {
	Valid         bool   `json:"valid"`
	PermanentCode string `json:"permanent_code"`
	DeviceID      string `json:"device_id"`
	Alias         string `json:"alias"`
	Notes         string `json:"notes"`
	Error         string `json:"error,omitempty"`
}

func listTechnicianDevices(token string) ([]DeviceItem, error) {
	devices := []DeviceItem{}
	after := int64(0)
	for {
		var page struct {
			Devices []DeviceItem `json:"devices"`
			Next    int64        `json:"next_cursor"`
		}
		if err := doTechnicianReq(http.MethodGet, fmt.Sprintf("/api/v1/technician/devices?after=%d", after), token, nil, &page); err != nil {
			return nil, err
		}
		devices = append(devices, page.Devices...)
		if page.Next == 0 {
			return devices, nil
		}
		if page.Next <= after {
			return nil, errors.New("pagination de parc invalide")
		}
		after = page.Next
	}
}

func generateTechnicianPermanentCode(token, alias, notes string) (*TechnicianEnrollCodeResponse, error) {
	req := map[string]string{
		"alias": strings.TrimSpace(alias),
		"notes": strings.TrimSpace(notes),
	}
	var envelope struct {
		Device DeviceItem `json:"device"`
	}
	err := doTechnicianReq(http.MethodPost, "/api/v1/technician/devices/enrollment-code", token, req, &envelope)
	if err != nil {
		return nil, err
	}
	if envelope.Device.PermanentCode == "" || envelope.Device.DeviceID == "" {
		return nil, errors.New("réponse d'enrôlement invalide")
	}
	return &TechnicianEnrollCodeResponse{Valid: true, PermanentCode: envelope.Device.PermanentCode, DeviceID: envelope.Device.DeviceID, Alias: envelope.Device.Alias, Notes: envelope.Device.Notes}, nil
}

func deleteTechnicianDevice(token, deviceID string) error {
	var res map[string]interface{}
	return doTechnicianReq(http.MethodDelete, "/api/v1/technician/devices/"+deviceID, token, nil, &res)
}

func updateTechnicianDeviceAlias(token, deviceID, alias, notes string) error {
	req := map[string]string{
		"alias": strings.TrimSpace(alias),
		"notes": strings.TrimSpace(notes),
	}
	var res map[string]interface{}
	return doTechnicianReq(http.MethodPut, "/api/v1/technician/devices/"+deviceID, token, req, &res)
}

type DeviceFolderItem struct {
	ID             int    `json:"id,omitempty"`
	FolderID       string `json:"folder_id"`
	ParentFolderID string `json:"parent_folder_id"`
	Name           string `json:"name"`
	CustomerID     any    `json:"customer_id,omitempty"`
	LicenseID      string `json:"license_id,omitempty"`
	CreatedAt      any    `json:"created_at,omitempty"`
	UpdatedAt      any    `json:"updated_at,omitempty"`
}

func listTechnicianFolders(token string) ([]DeviceFolderItem, error) {
	var envelope struct {
		Folders       []DeviceFolderItem `json:"folders"`
		DeviceFolders []DeviceFolderItem `json:"device_folders"`
	}
	if err := doTechnicianReq(http.MethodGet, "/api/v1/technician/device-folders", token, nil, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Folders) > 0 {
		return envelope.Folders, nil
	}
	return envelope.DeviceFolders, nil
}

func createTechnicianFolder(token, name, parentFolderID string) (*DeviceFolderItem, error) {
	req := map[string]string{
		"name":             strings.TrimSpace(name),
		"parent_folder_id": strings.TrimSpace(parentFolderID),
	}
	var envelope struct {
		Folder DeviceFolderItem `json:"folder"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/technician/device-folders", token, req, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Folder, nil
}

func updateTechnicianFolder(token, folderID, name string) error {
	req := map[string]string{
		"name": strings.TrimSpace(name),
	}
	var envelope struct {
		Folder DeviceFolderItem `json:"folder"`
	}
	return doTechnicianReq(http.MethodPut, "/api/v1/technician/device-folders/"+folderID, token, req, &envelope)
}

func deleteTechnicianFolder(token, folderID string) error {
	var res map[string]interface{}
	return doTechnicianReq(http.MethodDelete, "/api/v1/technician/device-folders/"+folderID, token, nil, &res)
}

func updateTechnicianDevice(token, deviceID, alias, notes, folderID string, macAddress ...string) error {
	req := map[string]string{
		"alias":     strings.TrimSpace(alias),
		"notes":     strings.TrimSpace(notes),
		"folder_id": strings.TrimSpace(folderID),
	}
	if len(macAddress) > 0 && strings.TrimSpace(macAddress[0]) != "" {
		req["mac_address"] = strings.TrimSpace(macAddress[0])
	}
	var res map[string]interface{}
	return doTechnicianReq(http.MethodPut, "/api/v1/technician/devices/"+deviceID, token, req, &res)
}

func updateTechnicianDeviceFolder(token, deviceID, folderID string) error {
	req := map[string]string{
		"folder_id": strings.TrimSpace(folderID),
	}
	var res map[string]interface{}
	return doTechnicianReq(http.MethodPut, "/api/v1/technician/devices/"+deviceID, token, req, &res)
}

// isTLSError checks if an error is a TLS-related error (certificate expired, untrusted, etc.)
func isTLSError(err error) bool {
	if err == nil {
		return false
	}
	var tlsRecordErr *tls.RecordHeaderError
	if errors.As(err, &tlsRecordErr) {
		return true
	}
	// Check for common TLS error strings
	errStr := err.Error()
	return strings.Contains(errStr, "certificate") ||
		strings.Contains(errStr, "tls:") ||
		strings.Contains(errStr, "x509:")
}
