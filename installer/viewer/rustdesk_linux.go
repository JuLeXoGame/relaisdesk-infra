//go:build linux

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxRustDeskDownloadSize = 250 << 20

//go:embed embedded/rustdesk.deb
var embeddedRustDeskPackage []byte

func ensureRustDesk() (string, error) {
	p, err := findRustDesk()
	if err == nil {
		return p, nil
	}
	if installErr := installRustDesk(); installErr != nil {
		return "", installErr
	}
	return findRustDesk()
}

func findRustDesk() (string, error) {
	if !validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) {
		return "", fmt.Errorf("empreinte du client RustDesk RelaisDesk non configurée")
	}
	path, err := exec.LookPath("rustdesk")
	if err != nil || !fileMatchesSHA256(path, RUSTDESK_EXPECTED_SHA256) {
		return "", fmt.Errorf("client RustDesk RelaisDesk introuvable")
	}
	if validPinnedSHA256(RUSTDESK_SO_EXPECTED_SHA256) {
		soPath := "/usr/share/rustdesk/lib/librustdesk.so"
		if !fileMatchesSHA256(soPath, RUSTDESK_SO_EXPECTED_SHA256) {
			return "", fmt.Errorf("bibliothèque RustDesk RelaisDesk obsolète")
		}
	}
	for _, libName := range []string{"libvpx.so.7", "libaom.so.3", "libyuv.so.0", "libjpeg.so.8"} {
		libPath := "/usr/share/rustdesk/lib/" + libName
		if _, err := os.Lstat(libPath); err != nil {
			return "", fmt.Errorf("bibliothèque multimédia requise absente (%s)", libName)
		}
	}
	return path, nil
}

func installRustDesk() error {
	if !validPinnedSHA256(RUSTDESK_PACKAGE_EXPECTED_SHA256) ||
		!validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) {
		return fmt.Errorf("empreintes du client RustDesk RelaisDesk non configurées")
	}
	if len(embeddedRustDeskPackage) == 0 || len(embeddedRustDeskPackage) > maxRustDeskDownloadSize {
		return fmt.Errorf("paquet RustDesk RelaisDesk intégré invalide")
	}
	if !bytesMatchSHA256(embeddedRustDeskPackage, RUSTDESK_PACKAGE_EXPECTED_SHA256) {
		return fmt.Errorf("empreinte SHA-256 invalide pour le paquet RustDesk RelaisDesk")
	}
	out, err := os.CreateTemp("", "relaisdesk-viewer-rustdesk-*.deb")
	if err != nil {
		return err
	}
	tempPath := out.Name()
	defer os.Remove(tempPath)

	if _, err := out.Write(embeddedRustDeskPackage); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tempPath, 0644)

	var cmd *exec.Cmd
	if isElevated() {
		cmd = exec.Command("dpkg", "-i", tempPath)
	} else {
		cmd = exec.Command("pkexec", "dpkg", "-i", tempPath)
	}
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback to apt-get install --reinstall in case of missing dependencies
		if isElevated() {
			cmd = exec.Command("apt-get", "install", "--reinstall", "-y", tempPath)
		} else {
			cmd = exec.Command("pkexec", "apt-get", "install", "--reinstall", "-y", tempPath)
		}
		outBytes2, err2 := cmd.CombinedOutput()
		if err2 != nil {
			return fmt.Errorf("échec de l'installation de RustDesk: %s / %s (%v)", strings.TrimSpace(string(outBytes)), strings.TrimSpace(string(outBytes2)), err2)
		}
	}
	return nil
}

func validPinnedSHA256(expected string) bool {
	if len(expected) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(expected)
	return err == nil
}

func bytesMatchSHA256(data []byte, expected string) bool {
	if !validPinnedSHA256(expected) {
		return false
	}
	sum := sha256.Sum256(data)
	return strings.EqualFold(hex.EncodeToString(sum[:]), expected)
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

	_ = exec.Command("pkill", "-9", "-f", "rustdesk").Run()

	if err := os.MkdirAll(RUSTDESK_CONFIG_DIR, 0700); err != nil {
		return err
	}

	cleanupCorruptedConfig(RUSTDESK_CONFIG_DIR)

	rendezvousWithPort := net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RendezvousPort))
	relayServer := rendezvousHost
	if activation.RelayPort != RUSTDESK_RELAY_PORT {
		relayServer = net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RelayPort))
	}
	configData := fmt.Sprintf("rendezvous_server = %s\n"+
		"nat_type = 1\n"+
		"serial = 0\n\n"+
		"[options]\n"+
		"custom-rendezvous-server = %s\n"+
		"relay-server = %s\n"+
		"api-server = %s\n"+
		"key = %s\n"+
		"allow-websocket = 'N'\n"+
		"stop-service = 'N'\n"+
		"relaisdesk-token-file = %s\n"+
		"relaisdesk-proof-key-file = %s\n",
		tomlString(rendezvousWithPort),
		tomlString(rendezvousHost),
		tomlString(relayServer),
		tomlString(APIURL),
		tomlString(activation.PublicKey),
		tomlString(activation.NetworkTokenFile),
		tomlString(activation.NetworkProofKeyFile)) + videoCodecTomlLines(false)

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
	// Not creating desktop shortcut on Linux since standard install adds it to applications menu.
	return nil
}

func isRustDeskRunning() bool {
	if exec.Command("pgrep", "-x", "rustdesk").Run() == nil {
		return true
	}
	return exec.Command("pgrep", "-f", "/usr/bin/rustdesk").Run() == nil
}

func launchRustDesk(path string) error {
	return launchRustDeskBackground(path)
}

func launchRustDeskBackground(path string) error {
	if isRustDeskRunning() {
		return nil
	}
	cmd := exec.Command(path, "--tray")
	return cmd.Start()
}
