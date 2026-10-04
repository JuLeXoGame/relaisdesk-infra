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
)

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

func generateViewerRustDesk2Toml(rendezvousWithPort, rendezvousHost, relayServer, publicKey, tokenFile, proofKeyFile string) string {
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

	terminateRustDesk()

	if err := os.MkdirAll(RUSTDESK_CONFIG_DIR, 0700); err != nil {
		return err
	}

	cleanupCorruptedConfig(RUSTDESK_CONFIG_DIR)

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

	return os.WriteFile(filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE), []byte(configData), 0600)
}

// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	return []string{filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE)}
}

func cleanupRustDesk2Toml() {
	_ = os.Remove(filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG2_FILE))
}

func terminateRustDesk() {
	_ = exec.Command("pkill", "-9", "-f", "rustdesk").Run()
	_ = exec.Command("pkill", "-9", "-f", "RelaisDesk").Run()
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

func getRustDeskID(rustdeskPath string) string {
	if rustdeskPath != "" {
		cmd := exec.Command(rustdeskPath, "--get-id")
		if out, err := cmd.Output(); err == nil {
			if id := parseNumericRustDeskID(string(out)); id != "" {
				return id
			}
		}
	}

	tomlPath := filepath.Join(RUSTDESK_CONFIG_DIR, RUSTDESK_CONFIG_FILE)
	if content, err := os.ReadFile(tomlPath); err == nil {
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "id =") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					id := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
					if isNumericRustDeskID(id) {
						return id
					}
				}
			}
		}
	}
	return ""
}

func createDesktopShortcut(rustdeskPath string) error {
	// Sur macOS, l'application est placée dans /Applications ou lancée depuis le DMG.
	return nil
}

func isRustDeskRunning() bool {
	cmd := exec.Command("pgrep", "-f", "RustDesk")
	return cmd.Run() == nil
}

func launchRustDesk(path string) error {
	return launchRustDeskBackground(path)
}

func launchRustDeskBackground(path string) error {
	if isRustDeskRunning() {
		return nil
	}
	if strings.HasSuffix(path, ".app") {
		cmd := exec.Command("open", "-a", path, "--args", "--tray")
		return cmd.Start()
	}
	cmd := exec.Command(path, "--tray")
	return cmd.Start()
}
