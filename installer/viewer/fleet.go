package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type fleetProof struct {
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
	Ready     bool   `json:"ready"`
}
type fleetState struct {
	DeviceID   string             `json:"device_id"`
	RustDeskID string             `json:"rustdesk_id"`
	Activation ActivationResponse `json:"activation"`
}

var errFleetRevoked = errors.New("autorisation du poste refusée")

func fleetProofMessage(action, id, rustdeskID, hostname, osName, key string, p fleetProof) []byte {
	b, _ := json.Marshal([]any{"relaisdesk-fleet-v1", action, id, rustdeskID, hostname, osName, key, p.Ready, p.Timestamp, p.Nonce})
	return b
}
func signFleetRequest(key ed25519.PrivateKey, action, id, rustdeskID, host, osName string, ready bool) (fleetProof, error) {
	if len(key) != ed25519.PrivateKeySize {
		return fleetProof{}, errors.New("identité de poste invalide")
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return fleetProof{}, err
	}
	p := fleetProof{Timestamp: time.Now().Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ready: ready}
	public := base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	p.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, fleetProofMessage(action, id, rustdeskID, host, osName, public, p)))
	return p, nil
}
func fleetRequest(ctx context.Context, path string, payload any) (*DeviceEnrollResponse, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(APIURL, "/")+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("serveur d'autorisation indisponible")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		return nil, errFleetRevoked
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("autorisation réseau indisponible (HTTP %d)", resp.StatusCode)
	}
	var result DeviceEnrollResponse
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || !result.Valid {
		return nil, errors.New("réponse d'autorisation invalide")
	}
	if _, err = validateViewerNetworkTokenResponse(&NetworkTokenResponse{Valid: result.Valid, NetworkToken: result.NetworkToken, ExpiresAt: result.ExpiresAt}); err != nil {
		return nil, err
	}
	return &result, nil
}
func enrollFleet(ctx context.Context, code, id string, key ed25519.PrivateKey) (*DeviceEnrollResponse, error) {
	if !isNumericRustDeskID(id) {
		return nil, errors.New("l'ID RustDesk réel doit être disponible avant l'enrôlement")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	proof, err := signFleetRequest(key, "enroll", code, id, hostname, runtime.GOOS, false)
	if err != nil {
		return nil, err
	}
	netInfo := getPrimaryNetworkInfo()
	payload := struct {
		fleetProof
		Code            string `json:"permanent_code"`
		RustDeskID      string `json:"rustdesk_id"`
		Hostname        string `json:"hostname"`
		OS              string `json:"os"`
		PublicKey       string `json:"device_public_key"`
		PeerAuthVersion int    `json:"peer_auth_version"`
		MACAddress      string `json:"mac_address,omitempty"`
		SubnetBroadcast string `json:"subnet_broadcast,omitempty"`
		AgentVersion    string `json:"agent_version,omitempty"`
	}{proof, code, id, hostname, runtime.GOOS, base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), fleetPeerAuthVersion(), netInfo.MACAddress, netInfo.SubnetBroadcast, APP_VERSION}
	return fleetRequest(ctx, "/api/v1/devices/enroll", payload)
}
func refreshFleet(ctx context.Context, state *fleetState, key ed25519.PrivateKey, ready bool) (*DeviceEnrollResponse, error) {
	proof, err := signFleetRequest(key, "heartbeat", state.DeviceID, "", "", "", ready)
	if err != nil {
		return nil, err
	}
	netInfo := getPrimaryNetworkInfo()
	return fleetRequest(ctx, "/api/v1/devices/heartbeat", struct {
		fleetProof
		DeviceID        string `json:"device_id"`
		PeerAuthVersion int    `json:"peer_auth_version"`
		MACAddress      string `json:"mac_address,omitempty"`
		SubnetBroadcast string `json:"subnet_broadcast,omitempty"`
		AgentVersion    string `json:"agent_version,omitempty"`
	}{proof, state.DeviceID, fleetPeerAuthVersion(), netInfo.MACAddress, netInfo.SubnetBroadcast, APP_VERSION})
}
func saveFleetState(dir string, state *fleetState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".fleet-state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(dir, "state.json"))
}
func readFleetState(dir string) (*fleetState, error) {
	path := filepath.Join(dir, "state.json")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return nil, errors.New("état de parc invalide")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state fleetState
	if err = json.Unmarshal(b, &state); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(state.DeviceID, "DEV-") || !isNumericRustDeskID(state.RustDeskID) {
		return nil, errors.New("identité de parc invalide")
	}
	// Secret paths are derived locally, never trusted from JSON or the API.
	state.Activation.NetworkTokenFile = filepath.Join(dir, "network-token")
	state.Activation.NetworkProofKeyFile = filepath.Join(dir, "proof-key")
	return &state, nil
}

func cryptoRandInt64(max int64) int64 {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return n.Int64()
}

// fleetCalculateNextWait calculates the next wait duration using:
// 1. Server-directed interval (default 45s if not specified).
// 2. Uniform ±15% random jitter to desynchronize fleet agents and prevent thundering herd.
// 3. Full-jitter exponential backoff on consecutive connection errors (starts at 5s, doubles up to 120s max).
func fleetCalculateNextWait(serverIntervalSec int, consecutiveErrors int) time.Duration {
	if consecutiveErrors > 0 {
		multiplier := 1 << (consecutiveErrors - 1)
		if multiplier > 24 {
			multiplier = 24
		}
		maxBackoff := time.Duration(5*multiplier) * time.Second
		if maxBackoff > 120*time.Second {
			maxBackoff = 120 * time.Second
		}
		minBackoff := 2 * time.Second
		if maxBackoff <= minBackoff {
			return minBackoff
		}
		jitterRange := maxBackoff - minBackoff
		jitter := time.Duration(cryptoRandInt64(int64(jitterRange)))
		return minBackoff + jitter
	}

	baseSec := serverIntervalSec
	if baseSec <= 0 {
		baseSec = 45
	}
	baseDuration := time.Duration(baseSec) * time.Second

	// ±15% uniform jitter: 0.85 to 1.15
	baseMs := baseDuration.Milliseconds()
	minMs := int64(float64(baseMs) * 0.85)
	maxMs := int64(float64(baseMs) * 1.15)
	if maxMs <= minMs {
		return baseDuration
	}
	jitterMs := minMs + cryptoRandInt64(maxMs-minMs+1)
	return time.Duration(jitterMs) * time.Millisecond
}

// fleetCalculateUpdateStagger determines the delay before starting an OTA download.
// If staggerSeconds is provided, applies ±10% jitter.
// Otherwise picks a random delay between 5s and 45s.
func fleetCalculateUpdateStagger(staggerSeconds int) time.Duration {
	if staggerSeconds > 0 {
		base := time.Duration(staggerSeconds) * time.Second
		baseMs := base.Milliseconds()
		minMs := int64(float64(baseMs) * 0.90)
		maxMs := int64(float64(baseMs) * 1.10)
		if maxMs <= minMs {
			return base
		}
		return time.Duration(minMs+cryptoRandInt64(maxMs-minMs+1)) * time.Millisecond
	}
	minMs := int64(5000)
	maxMs := int64(45000)
	return time.Duration(minMs+cryptoRandInt64(maxMs-minMs+1)) * time.Millisecond
}

type fleetAgentHooks struct {
	configure func(*fleetState) error
	running   func() bool
	start     func() error
	stop      func() error
	wait      func(context.Context, time.Duration) bool
}

func runFleetAgent(ctx context.Context, dir string) error {
	return runFleetAgentWithHooks(ctx, dir, fleetAgentHooks{
		configure: writeFleetConfiguration, running: fleetRustDeskRunning,
		start: startFleetRustDesk, stop: stopFleetRustDesk,
		wait: func(ctx context.Context, d time.Duration) bool {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return false
			case <-timer.C:
				return true
			}
		},
	})
}

func runFleetAgentWithHooks(ctx context.Context, dir string, hooks fleetAgentHooks) (resultErr error) {
	defer func() {
		removeErr := os.Remove(filepath.Join(dir, "network-token"))
		if os.IsNotExist(removeErr) {
			removeErr = nil
		}
		stopErr := hooks.stop()
		resultErr = errors.Join(resultErr, stopErr, removeErr)
	}()
	readyPath := filepath.Join(dir, "ready")
	if err := os.Remove(readyPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	defer os.Remove(readyPath)
	state, err := readFleetState(dir)
	if err != nil {
		return err
	}
	// An enrolled service must never silently replace a missing device identity.
	if _, err = os.Lstat(filepath.Join(dir, "proof-key")); err != nil {
		return err
	}
	key, err := loadOrCreateViewerProofKey(filepath.Join(dir, "proof-key"))
	if err != nil {
		return err
	}
	if err = hooks.configure(state); err != nil {
		return err
	}
	serverInterval := 45
	consecutiveErrors := 0
	expires := time.Time{}
	lastRefreshError := ""
	for {
		if ctx.Err() != nil {
			return nil
		}
		response, refreshErr := refreshFleet(ctx, state, key, hooks.running())
		if refreshErr != nil {
			consecutiveErrors++
			if refreshErr.Error() != lastRefreshError {
				log.Printf("Parc : %v (échec %d, attente adaptative)", refreshErr, consecutiveErrors)
				lastRefreshError = refreshErr.Error()
			}
			if errors.Is(refreshErr, errFleetRevoked) || !time.Now().Before(expires) {
				_ = os.Remove(readyPath)
				if err = os.Remove(state.Activation.NetworkTokenFile); err != nil && !os.IsNotExist(err) {
					return err
				}
				if err = hooks.stop(); err != nil {
					return err
				}
			}
		} else {
			consecutiveErrors = 0
			if lastRefreshError != "" {
				log.Print("Parc : autorisation réseau rétablie")
				lastRefreshError = ""
			}
			if response.NextIntervalSeconds > 0 {
				serverInterval = response.NextIntervalSeconds
			}
			if len(response.WakeTargets) > 0 {
				for _, targetMAC := range response.WakeTargets {
					if err := sendWakeOnLAN(targetMAC); err != nil {
						log.Printf("Parc : échec de transmission Wake-on-LAN pour %s: %v", targetMAC, err)
					}
				}
			}
			if response.UpdateTarget != nil {
				target := response.UpdateTarget
				stagger := fleetCalculateUpdateStagger(target.StaggerSeconds)
				go func(t *DeviceUpdateTarget, delay time.Duration) {
					log.Printf("Parc : mise à jour vers v%s programmée dans %v pour étalement de charge", t.Version, delay.Round(time.Second))
					select {
					case <-ctx.Done():
						return
					case <-time.After(delay):
						checkAndApplyFleetUpdate(dir, t)
					}
				}(target, stagger)
			}
			expiration, _ := time.Parse(time.RFC3339, response.ExpiresAt)
			if err = writeViewerSecretAtomically(state.Activation.NetworkTokenFile, response.NetworkToken); err != nil {
				return err
			}
			expires = expiration
			if !hooks.running() {
				if err = hooks.start(); err != nil {
					return err
				}
			}
			if err = os.WriteFile(readyPath, []byte("authorized\n"), 0600); err != nil {
				return err
			}
		}

		nextWait := fleetCalculateNextWait(serverInterval, consecutiveErrors)
		if !hooks.wait(ctx, nextWait) {
			return nil
		}
	}
}

func isRustDeskKeyConfirmed(dir string) bool {
	content, err := os.ReadFile(filepath.Join(dir, "RustDesk.toml"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "key_confirmed") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) == "true" {
				return true
			}
		}
	}
	return false
}

func resetRustDeskKeyConfirmed(dir string) {
	tomlPath := filepath.Join(dir, "RustDesk.toml")
	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(content), "\n")
	var newLines []string
	found := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "key_confirmed") {
			newLines = append(newLines, "key_confirmed = false")
			found = true
		} else {
			newLines = append(newLines, line)
		}
	}
	if !found {
		newLines = append(newLines, "key_confirmed = false")
	}
	_ = os.WriteFile(tomlPath, []byte(strings.Join(newLines, "\n")), 0600)
}
