//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

func findRustDesk() (string, error) {
	candidates := []string{
		RUSTDESK_DEFAULT_APP_PATH,
		RUSTDESK_LEGACY_APP_PATH,
	}

	// Si l'exécutable courant est dans un bundle .app macOS, vérifier s'il contient le binaire RustDesk
	if self, err := os.Executable(); err == nil {
		bundleDir := filepath.Dir(self)
		candidates = append(candidates, filepath.Join(bundleDir, "rustdesk"), filepath.Join(bundleDir, "RelaisDesk"))
	}

	for _, c := range candidates {
		if fileMatchesSHA256(c, RUSTDESK_EXPECTED_SHA256) {
			return c, nil
		}
		// Si pas de hash épinglé en mode dev, vérifier l'existence
		if RUSTDESK_EXPECTED_SHA256 == "" {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c, nil
			}
		}
	}

	if path, err := exec.LookPath("rustdesk"); err == nil {
		if fileMatchesSHA256(path, RUSTDESK_EXPECTED_SHA256) || RUSTDESK_EXPECTED_SHA256 == "" {
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
	return fmt.Sprintf("rendezvous_server = '%s'\n"+
		"nat_type = 1\n"+
		"serial = 0\n\n"+
		"[options]\n"+
		"custom-rendezvous-server = '%s'\n"+
		"relay-server = '%s'\n"+
		"api-server = '%s'\n"+
		"key = '%s'\n"+
		"relaisdesk-token-file = '%s'\n"+
		"relaisdesk-proof-key-file = '%s'\n",
		rendezvousWithPort,
		rendezvousHost,
		relayServer,
		tomlEscapeDarwin(APIURL),
		tomlEscapeDarwin(publicKey),
		tomlEscapeDarwin(tokenFile),
		tomlEscapeDarwin(proofKeyFile))
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

func tomlEscapeDarwin(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
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
