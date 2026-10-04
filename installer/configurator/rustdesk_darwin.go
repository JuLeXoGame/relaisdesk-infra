//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func ensureRustDesk() (string, error) {
	p, err := findRustDesk()
	if err == nil {
		return p, nil
	}
	return "", fmt.Errorf("client RustDesk RelaisDesk introuvable sur macOS: %w", err)
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

func findRustDesk() (string, error) {
	candidates := []string{
		RUSTDESK_DEFAULT_APP_PATH,
		RUSTDESK_LEGACY_APP_PATH,
	}

	if self, err := os.Executable(); err == nil {
		bundleDir := filepath.Dir(self)
		candidates = append(candidates, filepath.Join(bundleDir, "rustdesk"), filepath.Join(bundleDir, "RelaisDesk"))
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

func generateTechnicianRustDesk2Toml(rendezvousWithPort, rendezvousHost, relayServer, publicKey, tokenFile, proofKeyFile string) string {
	return fmt.Sprintf("rendezvous_server = %s\n"+
		"nat_type = 1\n"+
		"serial = 0\n\n"+
		"[options]\n"+
		"custom-rendezvous-server = %s\n"+
		"relay-server = %s\n"+
		"api-server = %s\n"+
		"key = %s\n"+
		"relaisdesk-token-file = %s\n"+
		"relaisdesk-proof-key-file = %s\n",
		tomlString(rendezvousWithPort),
		tomlString(rendezvousHost),
		tomlString(relayServer),
		tomlString(APIURL),
		tomlString(publicKey),
		tomlString(tokenFile),
		tomlString(proofKeyFile))
}

func configureRustDesk(activation *ActivationResponse, _ string) error {
	if activation == nil {
		return fmt.Errorf("configuration serveur incomplète")
	}
	rendezvousHost := strings.TrimSpace(activation.ServerIP)
	if err := validateRendezvousHost(rendezvousHost); err != nil {
		return err
	}
	if err := validateRustDeskPublicKey(activation.PublicKey); err != nil {
		return err
	}
	if !filepath.IsAbs(activation.NetworkTokenFile) || !filepath.IsAbs(activation.NetworkProofKeyFile) {
		return fmt.Errorf("fichiers d'autorisation RelaisDesk invalides")
	}

	if err := validateRendezvousPort(activation.RendezvousPort); err != nil {
		return err
	}
	if err := validateRendezvousPort(activation.RelayPort); err != nil {
		return err
	}

	terminateRustDesk()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("impossible de trouver le dossier utilisateur: %v", err)
	}

	configDir := filepath.Join(homeDir, RUSTDESK_CONFIG_DIR_SUFFIX)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("erreur creation dossier config: %v", err)
	}

	cleanupCorruptedConfig(configDir)

	rendezvousWithPort := net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RendezvousPort))
	relayServer := rendezvousHost
	if activation.RelayPort != 21117 {
		relayServer = net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RelayPort))
	}

	configData := generateTechnicianRustDesk2Toml(
		rendezvousWithPort,
		rendezvousHost,
		relayServer,
		activation.PublicKey,
		activation.NetworkTokenFile,
		activation.NetworkProofKeyFile,
	) + videoCodecTomlLines(false)

	rustdesk2Path := filepath.Join(configDir, RUSTDESK_CONFIG2_FILE)
	return os.WriteFile(rustdesk2Path, []byte(configData), 0600)
}

func cleanupRustDesk2Toml() {
	if homeDir, err := os.UserHomeDir(); err == nil {
		configDir := filepath.Join(homeDir, RUSTDESK_CONFIG_DIR_SUFFIX)
		_ = os.Remove(filepath.Join(configDir, RUSTDESK_CONFIG2_FILE))
	}
}

func createDesktopShortcut(rustdeskPath string) error {
	return nil
}

func launchRustDesk(rustdeskPath string, targetID ...string) error {
	args := []string{}
	for _, id := range targetID {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			args = append(args, trimmed)
		}
	}
	if strings.HasSuffix(rustdeskPath, ".app") {
		openArgs := []string{"-a", rustdeskPath}
		if len(args) > 0 {
			openArgs = append(openArgs, "--args")
			openArgs = append(openArgs, args...)
		}
		cmd := exec.Command("open", openArgs...)
		return cmd.Start()
	}
	cmd := exec.Command(rustdeskPath, args...)
	return cmd.Start()
}

func terminateRustDesk() {
	_ = exec.Command("pkill", "-9", "-f", "rustdesk").Run()
	_ = exec.Command("pkill", "-9", "-f", "RelaisDesk").Run()
}

func isRustDeskRunning() bool {
	cmd := exec.Command("pgrep", "-ix", "rustdesk")
	return cmd.Run() == nil
}

func launchRustDeskBackground(rustdeskPath string) error {
	if isRustDeskRunning() {
		return nil
	}
	return launchRustDesk(rustdeskPath, "--tray")
}

// restartRustDeskForConfig restarts a running engine so it reloads the freshly
// written options (the engine reads its config once at startup), or starts it
// when absent.
func restartRustDeskForConfig(rustdeskPath string) error {
	if isRustDeskRunning() {
		terminateRustDesk()
		if !waitForConditionFalse(isRustDeskRunning, 10*time.Second, 250*time.Millisecond) {
			return fmt.Errorf("le moteur RustDesk ne s'est pas arrêté")
		}
	}
	return launchRustDeskBackground(rustdeskPath)
}

func launchRustDeskSession(rustdeskPath string, targetID string, password ...string) error {
	_, err := launchRustDeskSessionCmd(rustdeskPath, targetID, password...)
	return err
}

func launchRustDeskSessionCmd(rustdeskPath string, targetID string, password ...string) (*exec.Cmd, error) {
	cleanID := strings.ReplaceAll(strings.TrimSpace(targetID), " ", "")
	if cleanID == "" {
		return nil, fmt.Errorf("identifiant distant manquant")
	}
	args := []string{"--connect", cleanID}
	if len(password) > 0 && strings.TrimSpace(password[0]) != "" {
		args = append(args, strings.TrimSpace(password[0]))
	}
	cmd := exec.Command(rustdeskPath, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(homeDir, RUSTDESK_CONFIG_DIR_SUFFIX, RUSTDESK_CONFIG2_FILE)}
}

