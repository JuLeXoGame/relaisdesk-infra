//go:build darwin

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var darwinViewerEngine struct {
	sync.Mutex
	command *exec.Cmd
	done    chan struct{}
}

func ensureRustDesk() (string, error) {
	p, err := findRustDesk()
	if err == nil {
		return p, nil
	}
	return "", fmt.Errorf("client RustDesk RelaisDesk introuvable sur macOS dans /Applications ou PATH: %w", err)
}

func darwinUnpinnedAllowed() bool {
	// Development-only override: shipped builds never run unpinned code
	// from user-writable locations without this explicit flag.
	return os.Getenv("RELAISDESK_ALLOW_UNPINNED") == "1"
}

func darwinAdminInstalledPath(path string) bool {
	return path == "/Applications" || strings.HasPrefix(path, "/Applications/")
}

// darwinCandidateUsable decides whether a candidate binary may be executed:
// a matching pinned hash always wins; with no verifiable pin only
// admin-installed locations (/Applications, root-owned) are accepted, plus
// any location under the explicit development override.
func darwinCandidateUsable(path, expectedSHA string, exists, unpinnedOverride bool) bool {
	if !exists {
		return false
	}
	if fileMatchesSHA256(path, expectedSHA) {
		return true
	}
	if validPinnedSHA256(expectedSHA) {
		return false
	}
	if darwinAdminInstalledPath(path) {
		return true
	}
	return unpinnedOverride
}

func darwinBundledEnginePath(executable string) string {
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Helpers", "RelaisDeskEngine.app", "Contents", "MacOS", "RelaisDesk"))
}

func findRustDesk() (string, error) {
	candidates := []string{
		RUSTDESK_DEFAULT_APP_PATH,
		RUSTDESK_LEGACY_APP_PATH,
	}

	if self, err := os.Executable(); err == nil {
		bundleDir := filepath.Dir(self)
		candidates = append(candidates, darwinBundledEnginePath(self), filepath.Join(bundleDir, "rustdesk"), filepath.Join(bundleDir, "RelaisDesk"))
	}

	for _, c := range candidates {
		info, err := os.Stat(c)
		exists := err == nil && !info.IsDir()
		if darwinCandidateUsable(c, RUSTDESK_EXPECTED_SHA256, exists, darwinUnpinnedAllowed()) {
			if !validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) {
				log.Printf("[macOS] binaire RustDesk non epingle accepte : %s", c)
			}
			return c, nil
		}
	}

	if path, err := exec.LookPath("rustdesk"); err == nil {
		// PATH entries are user-writable: a matching hash is mandatory,
		// with no unpinned fallback.
		if fileMatchesSHA256(path, RUSTDESK_EXPECTED_SHA256) {
			return path, nil
		}
	}

	return "", fmt.Errorf("application RelaisDesk introuvable")
}

func validPinnedSHA256(expected string) bool {
	if len(expected) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(expected)
	return err == nil
}

func fileMatchesSHA256(path, expected string) bool {
	if !validPinnedSHA256(expected) {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expected)
}

func cleanupCorruptedConfig(dir string) {
	if dir == "" {
		return
	}
	rustdeskTomlPath := filepath.Join(dir, RUSTDESK_CONFIG_FILE)
	if c, err := os.ReadFile(rustdeskTomlPath); err == nil {
		if isCorruptedRustDeskConfig(c) {
			_ = os.Remove(rustdeskTomlPath)
		}
	}
}

func generateViewerRustDesk2Toml(rendezvousWithPort, rendezvousHost, relayServer, publicKey, tokenFile, proofKeyFile string) string {
	return fmt.Sprintf("rendezvous_server = %s\n"+
		"nat_type = 1\n"+
		"serial = 0\n\n"+
		"[options]\n"+
		"custom-rendezvous-server = %s\n"+
		"relay-server = %s\n"+
		"api-server = %s\n"+
		"key = %s\n"+
		"stop-service = 'N'\n"+
		"relaisdesk-token-file = %s\n"+
		"relaisdesk-proof-key-file = %s\n",
		tomlString(rendezvousWithPort),
		tomlString(rendezvousHost),
		tomlString(relayServer),
		tomlString(APIURL),
		tomlString(publicKey),
		tomlString(tokenFile),
		tomlString(proofKeyFile)) + videoCodecTomlLines(false)
}

func configureRustDesk(activation *ActivationResponse, _ string) error {
	if activation == nil {
		return fmt.Errorf("configuration serveur incomplète")
	}
	rendezvousHost := strings.TrimSpace(activation.ServerIP)
	if err := validateRendezvousHost(rendezvousHost); err != nil {
		return err
	}
	if err := validateRendezvousPort(activation.RendezvousPort); err != nil {
		return err
	}
	if err := validateRendezvousPort(activation.RelayPort); err != nil {
		return err
	}
	if err := validateRustDeskPublicKey(activation.PublicKey); err != nil {
		return err
	}
	if !filepath.IsAbs(activation.NetworkTokenFile) || !filepath.IsAbs(activation.NetworkProofKeyFile) {
		return fmt.Errorf("fichiers d'autorisation RelaisDesk invalides")
	}

	if err := stopDarwinViewerEngine(); err != nil {
		return err
	}

	if err := os.MkdirAll(RUSTDESK_CONFIG_DIR, 0700); err != nil {
		return err
	}

	cleanupCorruptedConfig(RUSTDESK_CONFIG_DIR)
	if err := resetRustDeskKeyConfirmed(RUSTDESK_CONFIG_DIR); err != nil {
		return err
	}

	rendezvousWithPort := net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RendezvousPort))
	relayServer := rendezvousHost
	if activation.RelayPort != RUSTDESK_RELAY_PORT {
		relayServer = net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RelayPort))
	}

	configData := generateViewerRustDesk2Toml(
		rendezvousWithPort,
		rendezvousHost,
		relayServer,
		activation.PublicKey,
		activation.NetworkTokenFile,
		activation.NetworkProofKeyFile,
	)

	_, err := writeFileIfChangedAtomically(filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE), []byte(configData), 0600)
	return err
}

// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	return []string{filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE)}
}

func cleanupRustDesk2Toml() {
	_ = os.Remove(filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE))
}

func terminateRustDesk() {
	if err := stopDarwinViewerEngine(); err != nil {
		log.Printf("[macOS] arrêt du moteur de téléassistance : %v", err)
	}
}

func stopDarwinViewerEngine() error {
	darwinViewerEngine.Lock()
	defer darwinViewerEngine.Unlock()
	if darwinViewerEngine.command == nil {
		return nil
	}
	if err := darwinViewerEngine.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	<-darwinViewerEngine.done
	darwinViewerEngine.command = nil
	darwinViewerEngine.done = nil
	return nil
}

func parseNumericRustDeskID(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(lines[i])
		if isNumericRustDeskID(candidate) {
			return candidate
		}
	}
	return ""
}

func isNumericRustDeskID(id string) bool {
	if len(id) < 6 || len(id) > 16 || id == "0" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func createDesktopShortcut(rustdeskPath string) error {
	// Sur macOS, l'application est placée dans /Applications ou lancée depuis le DMG.
	return nil
}

func launchRustDesk(path string) error {
	darwinViewerEngine.Lock()
	defer darwinViewerEngine.Unlock()
	if darwinViewerEngine.command != nil {
		return fmt.Errorf("le moteur de téléassistance est déjà démarré")
	}
	cmd := exec.Command(path, "--server")
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	darwinViewerEngine.command = cmd
	darwinViewerEngine.done = done
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return nil
}

func awaitViewerAnnouncement(rustdeskPath, code, devicePublicKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return awaitDarwinViewerAnnouncement(ctx, rustdeskPath, code, devicePublicKey, time.Second)
}

func awaitDarwinViewerAnnouncement(ctx context.Context, rustdeskPath, code, devicePublicKey string, pollInterval time.Duration) error {
	darwinViewerEngine.Lock()
	done := darwinViewerEngine.done
	darwinViewerEngine.Unlock()
	if done == nil {
		return errors.New(T("err_engine_stopped"))
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return errors.New(T("err_engine_stopped"))
		case <-ctx.Done():
			return errors.New(T("err_registration_timeout"))
		case <-ticker.C:
		}
		if !isRustDeskKeyConfirmed(RUSTDESK_CONFIG_DIR) {
			continue
		}
		out, err := exec.CommandContext(ctx, rustdeskPath, "--get-id").Output()
		id := parseNumericRustDeskID(string(out))
		if err != nil || id == "" || !isRustDeskKeyConfirmed(RUSTDESK_CONFIG_DIR) {
			continue
		}
		select {
		case <-done:
			return errors.New(T("err_engine_stopped"))
		default:
		}
		if err := announceViewerID(ctx, code, id, devicePublicKey); err != nil {
			return errors.New(TF("err_announce_id", err))
		}
		return nil
	}
}
