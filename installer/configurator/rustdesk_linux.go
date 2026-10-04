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
	"time"
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

// findRustDesk checks if RustDesk is installed on the system.
func findRustDesk() (string, error) {
	if !validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) {
		return "", fmt.Errorf("empreinte du client RustDesk RelaisDesk non configurée")
	}
	if !matchesRustDeskLibrary("/usr/share/rustdesk/lib/librustdesk.so") {
		return "", fmt.Errorf("bibliothèque RustDesk RelaisDesk absente, obsolète ou empreinte non configurée")
	}
	paths := []string{
		RUSTDESK_DEFAULT_PATH_X64,
	}

	for _, p := range paths {
		if fileMatchesSHA256(p, RUSTDESK_EXPECTED_SHA256) {
			return p, nil
		}
	}

	// Fallback to checking PATH
	path, err := exec.LookPath("rustdesk")
	if err == nil && fileMatchesSHA256(path, RUSTDESK_EXPECTED_SHA256) {
		return path, nil
	}

	return "", fmt.Errorf("rustdesk introuvable")
}

func matchesRustDeskLibrary(path string) bool {
	return validPinnedSHA256(RUSTDESK_SO_EXPECTED_SHA256) && fileMatchesSHA256(path, RUSTDESK_SO_EXPECTED_SHA256)
}

// installRustDesk installs the pinned RelaisDesk client fork via pkexec.
func installRustDesk() error {
	if !validPinnedSHA256(RUSTDESK_PACKAGE_EXPECTED_SHA256) ||
		!validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) ||
		!validPinnedSHA256(RUSTDESK_SO_EXPECTED_SHA256) {
		return fmt.Errorf("empreintes du client RustDesk RelaisDesk non configurées")
	}
	if len(embeddedRustDeskPackage) == 0 || len(embeddedRustDeskPackage) > maxRustDeskDownloadSize {
		return fmt.Errorf("paquet RustDesk RelaisDesk intégré invalide")
	}
	if !bytesMatchSHA256(embeddedRustDeskPackage, RUSTDESK_PACKAGE_EXPECTED_SHA256) {
		return fmt.Errorf("empreinte SHA-256 invalide pour le paquet RustDesk RelaisDesk")
	}
	out, err := os.CreateTemp("", "relaisdesk-rustdesk-*.deb")
	if err != nil {
		return fmt.Errorf("erreur de creation du fichier temporaire: %v", err)
	}
	tempPath := out.Name()
	defer os.Remove(tempPath)

	if _, err := out.Write(embeddedRustDeskPackage); err != nil {
		return fmt.Errorf("erreur lors de l'écriture du paquet: %v", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("erreur de synchronisation du paquet: %v", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("erreur de finalisation du paquet: %v", err)
	}

	// 3. Install using pkexec (Polkit handles the GUI password prompt)
	cmd := exec.Command("pkexec", "apt", "install", "-y", tempPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("echec de l'installation (requiert les droits administrateur): %v", err)
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

// configureRustDesk writes the configuration so RustDesk uses our direct server.
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

	// Arrêter RustDesk seulement après avoir validé toute la configuration reçue.
	_ = exec.Command("pkill", "-9", "-f", "rustdesk").Run()

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
	configData := fmt.Sprintf("rendezvous_server = %s\n"+
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
		tomlString(activation.PublicKey),
		tomlString(activation.NetworkTokenFile),
		tomlString(activation.NetworkProofKeyFile)) + videoCodecTomlLines(false)

	rustdesk2Path := filepath.Join(configDir, RUSTDESK_CONFIG2_FILE)
	return os.WriteFile(rustdesk2Path, []byte(configData), 0600)
}

func cleanupRustDesk2Toml() {
	if homeDir, err := os.UserHomeDir(); err == nil {
		configDir := filepath.Join(homeDir, RUSTDESK_CONFIG_DIR_SUFFIX)
		_ = os.Remove(filepath.Join(configDir, RUSTDESK_CONFIG2_FILE))
	}
}

// createDesktopShortcut does nothing on Linux as the .deb handles the menu shortcut.
func createDesktopShortcut(rustdeskPath string) error {
	return nil
}

// launchRustDesk runs RustDesk.
func launchRustDesk(rustdeskPath string, targetID ...string) error {
	args := []string{}
	for _, id := range targetID {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			args = append(args, trimmed)
		}
	}
	cmd := exec.Command(rustdeskPath, args...)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("erreur de lancement: %v", err)
	}
	return nil
}

func terminateRustDesk() {
	killCmd := exec.Command("pkill", "-9", "-f", "rustdesk")
	_ = killCmd.Run()
}

func isRustDeskRunning() bool {
	cmd := exec.Command("pgrep", "-x", "rustdesk")
	return cmd.Run() == nil
}

func launchRustDeskBackground(rustdeskPath string) error {
	if isRustDeskRunning() {
		return nil
	}
	cmd := exec.Command(rustdeskPath, "--tray")
	return cmd.Start()
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

func launchRustDeskSession(rustdeskPath string, targetID string, password ...string) error {
	_, err := launchRustDeskSessionCmd(rustdeskPath, targetID, password...)
	return err
}

// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(homeDir, RUSTDESK_CONFIG_DIR_SUFFIX, RUSTDESK_CONFIG2_FILE)}
}
