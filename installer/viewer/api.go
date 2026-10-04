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

type DeviceUpdateTarget struct {
	Version        string `json:"version"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256,omitempty"`
	StaggerSeconds int    `json:"stagger_seconds,omitempty"`
}

type DeviceEnrollResponse struct {
	NetworkToken        string              `json:"network_token"`
	ExpiresAt           string              `json:"expires_at"`
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


type ActivationResponse struct {
	Valid               bool   `json:"valid"`
	Error               string `json:"error,omitempty"`
	ServerIP            string `json:"server_ip,omitempty"`
	RendezvousPort      int    `json:"rendezvous_port,omitempty"`
	RelayPort           int    `json:"relay_port,omitempty"`
	PublicKey           string `json:"public_key,omitempty"`
	ExpiresAt           string `json:"expires_at,omitempty"`
	NetworkTokenFile    string `json:"-"`
	NetworkProofKeyFile string `json:"-"`
}

type NetworkTokenResponse struct {
	Valid        bool   `json:"valid"`
	NetworkToken string `json:"network_token"`
	ExpiresAt    string `json:"network_token_expires_at"`
	KeyID        string `json:"key_id"`
	Error        string `json:"error"`
}

func activateViewerCode(code string) (*ActivationResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	payload := map[string]string{"code": strings.ToUpper(strings.TrimSpace(code))}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erreur de préparation de la requête: %w", err)
	}

	endpoint := strings.TrimRight(APIURL, "/") + "/api/v1/viewer/activate"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("erreur de création de la requête: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"/1.0")

	resp, err := client.Do(req)
	if err != nil {
		if isTLSError(err) {
			return nil, fmt.Errorf("erreur de sécurité lors de la connexion au serveur (certificat invalide ou expiré). Réessayez plus tard ou contactez le support")
		}
		return nil, fmt.Errorf("impossible de contacter le serveur. Vérifiez votre connexion")
	}
	defer resp.Body.Close()

	// Status first: a 429/401 with an empty or non-JSON body must still
	// surface its dedicated error instead of "réponse serveur invalide".
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errors.New("réponse serveur invalide")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("trop de tentatives. Réessayez dans quelques minutes")
	}
	var result ActivationResponse
	// Best-effort off-200: a 401 with an empty body must report the auth
	// error below, not a decode failure.
	if err := json.Unmarshal(respBody, &result); err != nil && resp.StatusCode == http.StatusOK {
		return nil, errors.New("réponse serveur invalide")
	}
	if resp.StatusCode == http.StatusUnauthorized || !result.Valid {
		if result.Error != "" {
			return nil, errors.New(result.Error)
		}
		return nil, errors.New("code expiré ou invalide")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur serveur temporaire (HTTP %d)", resp.StatusCode)
	}

	if err := validateActivationResponse(&result); err != nil {
		return nil, err
	}
	return &result, nil
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
		return errors.New("configuration serveur incomplète: code expiré")
	}
	return nil
}

func announceViewerID(code, rustdeskID, devicePublicKey string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	payload := map[string]string{
		"code":              strings.ToUpper(strings.TrimSpace(code)),
		"rustdesk_id":       strings.TrimSpace(rustdeskID),
		"device_public_key": devicePublicKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(APIURL, "/") + "/api/v1/viewer/announce"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("statut HTTP %d", resp.StatusCode)
	}
	return nil
}

func getViewerNetworkToken(code, devicePublicKey string) (*NetworkTokenResponse, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	payload := map[string]string{
		"code":              strings.ToUpper(strings.TrimSpace(code)),
		"device_public_key": devicePublicKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(APIURL, "/") + "/api/v1/viewer/network-token"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("impossible de renouveler l’autorisation réseau: %w", err)
	}
	defer resp.Body.Close()
	var result NetworkTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, errors.New("réponse d'autorisation invalide")
	}
	if resp.StatusCode != http.StatusOK || !result.Valid {
		if result.Error == "" {
			result.Error = "autorisation réseau refusée"
		}
		return nil, errors.New(result.Error)
	}
	return &result, nil
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
	errStr := err.Error()
	return strings.Contains(errStr, "certificate") ||
		strings.Contains(errStr, "tls:") ||
		strings.Contains(errStr, "x509:")
}
