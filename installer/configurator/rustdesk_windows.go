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
	"time"
)

//go:embed embedded/rustdesk.exe
var embeddedRustDesk []byte

func ensureRustDesk() (string, error) {
	if !validPinnedRustDeskSHA256() {
		return "", fmt.Errorf("empreinte du client RustDesk RelaisDesk non configurée")
	}
	// N'exécuter un binaire existant que s'il correspond exactement à la
	// version officielle épinglée. Le configurateur peut être lancé avec des
	// droits élevés par NSIS : un exécutable trouvé dans le PATH ou dans un
	// dossier utilisateur ne doit donc jamais être accepté sans contrôle.
	searchPaths := []string{
		os.ExpandEnv(RUSTDESK_DEFAULT_PATH_X64),
		os.ExpandEnv(RUSTDESK_DEFAULT_PATH_X86),
		os.ExpandEnv(RUSTDESK_USER_PATH),
	}

	for _, p := range searchPaths {
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
	// 2. Si %APPDATA%\RustDesk\config\RustDesk.toml existe avec key_pair vide ([[], []]) ou key_confirmed = false → le supprimer
	rustdeskTomlPath := filepath.Join(dir, RUSTDESK_CONFIG_FILE)
	if c, err := os.ReadFile(rustdeskTomlPath); err == nil {
		if isCorruptedRustDeskConfig(c) {
			_ = os.Remove(rustdeskTomlPath)
		}
	}
}

func generateRustDesk2Toml(rendezvousWithPort, rendezvousHost, relayServer, publicKey, tokenFile, proofKeyFile string) string {
	var sb strings.Builder
	sb.WriteString("rendezvous_server = " + tomlString(rendezvousWithPort) + "\r\n")
	sb.WriteString("nat_type = 1\r\n")
	sb.WriteString("serial = 0\r\n\r\n")

	sb.WriteString("[options]\r\n")
	sb.WriteString("custom-rendezvous-server = " + tomlString(rendezvousHost) + "\r\n")
	sb.WriteString("relay-server = " + tomlString(relayServer) + "\r\n")
	sb.WriteString("api-server = " + tomlString(APIURL) + "\r\n")
	sb.WriteString("key = " + tomlString(publicKey) + "\r\n")
	sb.WriteString("relaisdesk-token-file = " + tomlString(tokenFile) + "\r\n")
	sb.WriteString("relaisdesk-proof-key-file = " + tomlString(proofKeyFile) + "\r\n")
	sb.WriteString(videoCodecTomlLines(true))

	return sb.String()
}

func configureRustDesk(response *ActivationResponse, _ string) error {
	if response == nil {
		return fmt.Errorf("configuration serveur incomplète")
	}
	rendezvousHost := strings.TrimSpace(response.ServerIP)
	if err := validateRendezvousHost(rendezvousHost); err != nil {
		return err
	}
	if err := validateRustDeskPublicKey(response.PublicKey); err != nil {
		return err
	}
	if !filepath.IsAbs(response.NetworkTokenFile) || !filepath.IsAbs(response.NetworkProofKeyFile) {
		return fmt.Errorf("fichiers d'autorisation RelaisDesk invalides")
	}

	if err := validateRendezvousPort(response.RendezvousPort); err != nil {
		return err
	}
	if err := validateRendezvousPort(response.RelayPort); err != nil {
		return err
	}

	// Arrêter RustDesk seulement après avoir validé toute la configuration reçue.
	killCmd := exec.Command("taskkill", "/F", "/IM", "rustdesk.exe")
	killCmd.SysProcAttr = hideWindowSysProcAttr()
	_ = killCmd.Run()

	rendezvousWithPort := net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", response.RendezvousPort))
	relayServer := rendezvousHost
	if response.RelayPort != 21117 {
		relayServer = net.JoinHostPort(rendezvousHost, fmt.Sprintf("%d", response.RelayPort))
	}

	tomlContent := generateRustDesk2Toml(
		rendezvousWithPort,
		rendezvousHost,
		relayServer,
		response.PublicKey,
		response.NetworkTokenFile,
		response.NetworkProofKeyFile,
	)

	var lastErr error
	written := false
	for _, dir := range rustDeskConfigDirs() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			lastErr = err
			continue
		}
		// 2. Si RustDesk.toml existe avec key_pair vide → le supprimer
		cleanupCorruptedConfig(dir)

		// 3. ÉCRASER à chaque lancement RustDesk2.toml avec exactement la configuration attendue
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

func upsertToml(path string, root map[string]string, sections map[string]map[string]string, removeKeys map[string][]string, removeSections []string) error {
	content, _ := os.ReadFile(path)
	lines := []string{}
	if len(content) > 0 {
		lines = strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	}

	for _, sec := range removeSections {
		lines = removeSection(lines, sec)
	}

	lines = upsertSection(lines, "", root, removeKeys[""])
	for section, values := range sections {
		lines = upsertSection(lines, section, values, removeKeys[section])
	}

	output := strings.Join(lines, "\r\n")
	if !strings.HasSuffix(output, "\r\n") {
		output += "\r\n"
	}
	if err := os.WriteFile(path, []byte(output), 0600); err != nil {
		return fmt.Errorf("erreur d'ecriture de la configuration %s: %w", path, err)
	}
	return nil
}

func removeSection(lines []string, section string) []string {
	start, end := sectionBounds(lines, section)
	if start == -1 {
		return lines
	}
	headerIdx := start - 1
	var newLines []string
	newLines = append(newLines, lines[:headerIdx]...)
	newLines = append(newLines, lines[end:]...)
	return newLines
}

func upsertSection(lines []string, section string, values map[string]string, remove []string) []string {
	// Créer un set des clés à supprimer
	toRemove := make(map[string]bool)
	for _, k := range remove {
		toRemove[k] = true
	}

	start, end := sectionBounds(lines, section)
	if start == -1 {
		if len(values) == 0 {
			return lines
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		if section != "" {
			lines = append(lines, "["+section+"]")
		}
		for key, value := range values {
			lines = append(lines, tomlLine(key, value))
		}
		return lines
	}

	seen := map[string]bool{}
	newLines := make([]string, 0, len(lines)+len(values))
	newLines = append(newLines, lines[:start]...)

	for i := start; i < end; i++ {
		key := lineKey(lines[i])
		if toRemove[key] {
			continue // Supprimer la clé obsolète
		}
		if value, ok := values[key]; ok {
			newLines = append(newLines, tomlLine(key, value))
			seen[key] = true
		} else {
			newLines = append(newLines, lines[i])
		}
	}

	for key, value := range values {
		if !seen[key] {
			newLines = append(newLines, tomlLine(key, value))
		}
	}

	newLines = append(newLines, lines[end:]...)
	return newLines
}

func sectionBounds(lines []string, section string) (int, int) {
	if section == "" {
		start := 0
		end := len(lines)
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "[") {
				end = i
				break
			}
		}
		return start, end
	}

	header := "[" + section + "]"
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			start = i + 1
			continue
		}
		if start != -1 && strings.HasPrefix(trimmed, "[") {
			return start, i
		}
	}
	if start == -1 {
		return -1, -1
	}
	return start, len(lines)
}

func lineKey(line string) string {
	before, _, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(before)
}

func tomlLine(key, value string) string {
	if key == "nat_type" || key == "serial" {
		return fmt.Sprintf("%s = %s", key, value)
	}
	return fmt.Sprintf("%s = %s", key, tomlString(value))
}

func createDesktopShortcut(_ string) error {
	desktopDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("impossible de trouver le repertoire utilisateur: %w", err)
	}
	relaisDeskPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("impossible de trouver l'exécutable RelaisDesk: %w", err)
	}

	shortcutPath := filepath.Join(desktopDir, "Desktop", DESKTOP_SHORTCUT_NAME)
	psScript := fmt.Sprintf(`
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut('%s')
$Shortcut.TargetPath = '%s'
$Shortcut.WorkingDirectory = '%s'
$Shortcut.Description = '%s - Acces distant securise'
$Shortcut.WindowStyle = 1
$Shortcut.Save()
`,
		strings.ReplaceAll(shortcutPath, `'`, `''`),
		strings.ReplaceAll(relaisDeskPath, `'`, `''`),
		strings.ReplaceAll(filepath.Dir(relaisDeskPath), `'`, `''`),
		PRODUCT_NAME,
	)

	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	cmd.SysProcAttr = hideWindowSysProcAttr()
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("erreur de creation du raccourci: %w\nSortie: %s", err, string(output))
	}
	return nil
}

func hideWindowSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW: empêche la création d'une console noire CMD
	}
}

func launchRustDesk(path string, targetID ...string) error {
	args := []string{}
	for _, id := range targetID {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			args = append(args, trimmed)
		}
	}
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("erreur de lancement de RustDesk: %w", err)
	}
	return nil
}

func terminateRustDesk() {
	killCmd := exec.Command("taskkill", "/F", "/IM", "rustdesk.exe")
	killCmd.SysProcAttr = hideWindowSysProcAttr()
	_ = killCmd.Run()
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

// restartRustDeskForConfig restarts a running engine so it reloads the freshly
// written options (the engine reads its config once at startup), or starts it
// when absent.
func restartRustDeskForConfig(path string) error {
	if isRustDeskRunning() {
		terminateRustDesk()
		if !waitForConditionFalse(isRustDeskRunning, 10*time.Second, 250*time.Millisecond) {
			return fmt.Errorf("le moteur RustDesk ne s'est pas arrêté")
		}
	}
	return launchRustDeskBackground(path)
}

func launchRustDeskSessionCmd(path string, targetID string, password ...string) (*exec.Cmd, error) {
	cleanID := strings.ReplaceAll(strings.TrimSpace(targetID), " ", "")
	if cleanID == "" {
		return nil, fmt.Errorf("identifiant distant manquant")
	}
	args := []string{"--connect", cleanID}
	if len(password) > 0 && strings.TrimSpace(password[0]) != "" {
		args = append(args, strings.TrimSpace(password[0]))
	}
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    false,
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func launchRustDeskSession(path string, targetID string, password ...string) error {
	_, err := launchRustDeskSessionCmd(path, targetID, password...)
	return err
}

// rustDesk2TomlPaths lists known RustDesk2.toml locations for --set-hwcodec.
func rustDesk2TomlPaths() []string {
	var paths []string
	for _, dir := range rustDeskConfigDirs() {
		paths = append(paths, filepath.Join(dir, RUSTDESK_CONFIG2_FILE))
	}
	return paths
}
