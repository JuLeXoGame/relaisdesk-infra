//go:build windows

package main

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed embedded/rustdesk.exe
var embeddedRustDesk []byte

func ensureRustDesk() (string, error) {
	if !validPinnedRustDeskSHA256() {
		return "", fmt.Errorf("empreinte du client RustDesk RelaisDesk non configurée")
	}
	// Le viewer peut être démarré depuis un installateur élevé. Ne jamais
	// exécuter un RustDesk trouvé dans un dossier utilisateur sans vérifier
	// qu'il s'agit exactement du binaire officiel épinglé.
	paths := []string{
		`C:\Program Files\RustDesk\rustdesk.exe`,
		`C:\Program Files (x86)\RustDesk\rustdesk.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `RustDesk\rustdesk.exe`),
	}
	for _, p := range paths {
		if fileMatchesRustDeskSHA256(p) {
			return p, nil
		}
	}

	if len(embeddedRustDesk) == 0 {
		return "", fmt.Errorf("binaire RustDesk intégré introuvable")
	}
	if !bytesMatchRustDeskSHA256(embeddedRustDesk) {
		return "", fmt.Errorf("empreinte SHA-256 invalide pour le binaire RustDesk intégré")
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", fmt.Errorf("variable LOCALAPPDATA introuvable")
	}
	targetDir := filepath.Join(localAppData, "RelaisDesk", "bin")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return "", fmt.Errorf("impossible de préparer le dossier RustDesk: %w", err)
	}

	targetPath := filepath.Join(targetDir, "rustdesk.exe")
	if fileMatchesRustDeskSHA256(targetPath) {
		return targetPath, nil
	}

	terminateRustDesk()
	_ = os.Remove(filepath.Join(localAppData, "rustdesk", "meta.toml"))
	if err := writeRustDeskAtomically(targetDir, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

func bytesMatchRustDeskSHA256(data []byte) bool {
	if !validPinnedRustDeskSHA256() {
		return false
	}
	sum := sha256.Sum256(data)
	return strings.EqualFold(fmt.Sprintf("%x", sum), RUSTDESK_EXPECTED_SHA256)
}

func fileMatchesRustDeskSHA256(path string) bool {
	if !validPinnedRustDeskSHA256() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return strings.EqualFold(fmt.Sprintf("%x", hash.Sum(nil)), RUSTDESK_EXPECTED_SHA256)
}

func validPinnedRustDeskSHA256() bool {
	if len(RUSTDESK_EXPECTED_SHA256) != sha256.Size*2 {
		return false
	}
	for _, char := range RUSTDESK_EXPECTED_SHA256 {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func writeRustDeskAtomically(targetDir, targetPath string) error {
	tmp, err := os.CreateTemp(targetDir, "rustdesk-*.exe")
	if err != nil {
		return fmt.Errorf("impossible de créer le fichier RustDesk temporaire: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(embeddedRustDesk); err != nil {
		tmp.Close()
		return fmt.Errorf("impossible d'extraire RustDesk: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("impossible de finaliser RustDesk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("impossible de fermer le fichier RustDesk: %w", err)
	}
	if !fileMatchesRustDeskSHA256(tmpPath) {
		return fmt.Errorf("empreinte SHA-256 invalide après extraction de RustDesk")
	}

	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("impossible de remplacer l'ancien RustDesk: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("impossible d'installer RustDesk localement: %w", err)
	}
	return nil
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
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("rendezvous_server = '%s'\r\n", rendezvousWithPort))
	sb.WriteString("nat_type = 1\r\n")
	sb.WriteString("serial = 0\r\n\r\n")

	sb.WriteString("[options]\r\n")
	sb.WriteString(fmt.Sprintf("custom-rendezvous-server = '%s'\r\n", rendezvousHost))
	sb.WriteString(fmt.Sprintf("relay-server = '%s'\r\n", relayServer))
	sb.WriteString(fmt.Sprintf("api-server = '%s'\r\n", tomlEscape(APIURL)))
	sb.WriteString(fmt.Sprintf("key = '%s'\r\n", publicKey))
	sb.WriteString(fmt.Sprintf("relaisdesk-token-file = '%s'\r\n", tomlEscape(tokenFile)))
	sb.WriteString(fmt.Sprintf("relaisdesk-proof-key-file = '%s'\r\n", tomlEscape(proofKeyFile)))
	sb.WriteString(videoCodecTomlLines(true))

	return sb.String()
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

	killCmd := exec.Command("taskkill", "/F", "/IM", "rustdesk.exe")
	killCmd.SysProcAttr = hideWindowSysProcAttr()
	_ = killCmd.Run()

	rendezvousWithPort := net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RendezvousPort))
	relayServer := rendezvousHost
	if activation.RelayPort != RUSTDESK_RELAY_PORT {
		relayServer = net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", activation.RelayPort))
	}

	tomlContent := generateViewerRustDesk2Toml(
		rendezvousWithPort,
		rendezvousHost,
		relayServer,
		activation.PublicKey,
		activation.NetworkTokenFile,
		activation.NetworkProofKeyFile,
	)

	var lastErr error
	written := false
	for _, dir := range rustDeskConfigDirs() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			lastErr = err
			continue
		}
		cleanupCorruptedConfig(dir)

		r2Path := filepath.Join(dir, RUSTDESK_CONFIG2_FILE)
		if err := os.WriteFile(r2Path, []byte(tomlContent), 0600); err != nil {
			lastErr = fmt.Errorf("erreur d'ecriture de la configuration %s: %w", r2Path, err)
			continue
		}
		written = true
	}
	if written {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("aucun dossier de configuration RustDesk disponible")
}

func cleanupRustDesk2Toml() {
	for _, dir := range rustDeskConfigDirs() {
		_ = os.Remove(filepath.Join(dir, RUSTDESK_CONFIG2_FILE))
	}
}

func terminateRustDesk() {
	killCmd := exec.Command("taskkill", "/F", "/IM", "rustdesk.exe")
	killCmd.SysProcAttr = hideWindowSysProcAttr()
	_ = killCmd.Run()
}

// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	var paths []string
	for _, dir := range rustDeskConfigDirs() {
		paths = append(paths, filepath.Join(dir, RUSTDESK_CONFIG2_FILE))
	}
	return paths
}

func rustDeskConfigDirs() []string {
	dirs := make([]string, 0, 2)
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		dirs = append(dirs, filepath.Join(appData, "RustDesk", "config"))
	}
	if systemDrive := strings.TrimSpace(os.Getenv("SystemDrive")); systemDrive != "" {
		if !strings.HasSuffix(systemDrive, `\`) {
			systemDrive += `\`
		}
		dirs = append(dirs, filepath.Join(systemDrive, "Windows", "ServiceProfiles", "LocalService", "AppData", "Roaming", "RustDesk", "config"))
	}
	return dirs
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
	// 1. Essayer le binaire RustDesk extrait dans AppData\Local s'il existe
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		innerPath := filepath.Join(localAppData, "rustdesk", "rustdesk.exe")
		if _, err := os.Stat(innerPath); err == nil {
			cmd := exec.Command(innerPath, "--get-id")
			cmd.SysProcAttr = hideWindowSysProcAttr()
			if out, err := cmd.Output(); err == nil {
				if id := parseNumericRustDeskID(string(out)); id != "" {
					return id
				}
			}
		}
	}

	// 2. Essayer via le chemin fourni (binaire portable packé)
	if rustdeskPath != "" {
		cmd := exec.Command(rustdeskPath, "--get-id")
		cmd.SysProcAttr = hideWindowSysProcAttr()
		if out, err := cmd.Output(); err == nil {
			if id := parseNumericRustDeskID(string(out)); id != "" {
				return id
			}
		}
	}

	// 3. Fallback en lisant RustDesk.toml
	for _, dir := range rustDeskConfigDirs() {
		tomlPath := filepath.Join(dir, RUSTDESK_CONFIG_FILE)
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
	}
	return ""
}

func createDesktopShortcut(_ string) error {
	profilePath, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("impossible de trouver le repertoire utilisateur: %w", err)
	}
	relaisDeskPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("impossible de trouver l'exécutable RelaisDesk: %w", err)
	}
	desktopPath := filepath.Join(profilePath, "Desktop", DESKTOP_SHORTCUT_NAME)
	psScript := fmt.Sprintf(`
$WshShell = New-Object -comObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut('%s')
$Shortcut.TargetPath = '%s'
$Shortcut.Save()
`,
		strings.ReplaceAll(desktopPath, `'`, `''`),
		strings.ReplaceAll(relaisDeskPath, `'`, `''`),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	cmd.SysProcAttr = hideWindowSysProcAttr()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("erreur de creation du raccourci: %w (sortie: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func hideWindowSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW: empêche la création d'une console noire CMD
	}
}

func isRustDeskRunning() bool {
	cmd := exec.Command("tasklist", "/NH", "/FI", "IMAGENAME eq rustdesk.exe")
	cmd.SysProcAttr = hideWindowSysProcAttr()
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "rustdesk.exe") {
			return true
		}
	}
	return false
}

func launchRustDesk(path string) error {
	return launchRustDeskBackground(path)
}

func launchRustDeskBackground(path string) error {
	if isRustDeskRunning() {
		return nil
	}
	cmd := exec.Command(path, "--tray")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
	return cmd.Start()
}
