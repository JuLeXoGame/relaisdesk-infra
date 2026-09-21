//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func RunGUI() error {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "daemon":
			return runViewerDaemon(os.Args[2:])
		case "--status", "-s":
			printViewerStatus()
			return nil
		case "--stop":
			stopViewerSession()
			return nil
		}
	}

	code, err := promptViewerCode()
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return nil
	}

	return handleActivationAndLaunch(strings.TrimSpace(code))
}

func RunGUIWithPreset(preset string, isPermanent bool) error {
	preset = strings.TrimSpace(preset)
	if preset == "" {
		return RunGUI()
	}
	return handleActivationAndLaunch(preset)
}

func promptViewerCode() (string, error) {
	if isDisplayAvailable() {
		if code, ok := promptCodeZenity(); ok {
			return code, nil
		}
		if code, ok := promptCodeKdialog(); ok {
			return code, nil
		}
	}

	// Console fallback
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s\n", PRODUCT_NAME)
	fmt.Print("Entrez le code temporaire ou permanent (ex: PERM-XXXX-XXXX) : ")
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func isDisplayAvailable() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func promptCodeZenity() (string, bool) {
	zenityPath, err := exec.LookPath("zenity")
	if err != nil {
		return "", false
	}
	cmd := exec.Command(zenityPath, "--entry",
		"--title="+PRODUCT_NAME,
		"--text=Entrez le code temporaire ou permanent (ex: PERM-XXXX-XXXX) :",
		"--width=420")
	out, err := cmd.Output()
	if err != nil {
		// User canceled dialog
		return "", true
	}
	return strings.TrimSpace(string(out)), true
}

func promptCodeKdialog() (string, bool) {
	kdialogPath, err := exec.LookPath("kdialog")
	if err != nil {
		return "", false
	}
	cmd := exec.Command(kdialogPath, "--inputbox",
		"Entrez le code temporaire ou permanent (ex: PERM-XXXX-XXXX) :",
		"--title", PRODUCT_NAME)
	out, err := cmd.Output()
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(string(out)), true
}

func showLinuxDialog(title, message string, isError bool) {
	if !isDisplayAvailable() {
		if isError {
			fmt.Fprintf(os.Stderr, "Erreur: %s\n", message)
		} else {
			fmt.Println(message)
		}
		return
	}
	if zenityPath, err := exec.LookPath("zenity"); err == nil {
		msgFlag := "--info"
		if isError {
			msgFlag = "--error"
		}
		_ = exec.Command(zenityPath, msgFlag, "--title="+title, "--text="+message, "--width=350").Run()
		return
	}
	if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
		msgFlag := "--msgbox"
		if isError {
			msgFlag = "--error"
		}
		_ = exec.Command(kdialogPath, msgFlag, message, "--title", title).Run()
		return
	}
	if isError {
		fmt.Fprintf(os.Stderr, "Erreur: %s\n", message)
	} else {
		fmt.Println(message)
	}
}

func notifyDesktop(title, message string) {
	if !isDisplayAvailable() {
		return
	}
	if notifyPath, err := exec.LookPath("notify-send"); err == nil {
		_ = exec.Command(notifyPath, "-a", "RelaisDesk", title, message).Run()
	}
}

func promptPasswordDialog(title, prompt string) (string, bool) {
	if isDisplayAvailable() {
		if pwd, ok := promptPasswordZenity(title, prompt); ok {
			return pwd, true
		}
		if pwd, ok := promptPasswordKdialog(title, prompt); ok {
			return pwd, true
		}
	}
	return promptPasswordConsole(prompt)
}

func promptPasswordZenity(title, prompt string) (string, bool) {
	zenityPath, err := exec.LookPath("zenity")
	if err != nil {
		return "", false
	}
	cmd := exec.Command(zenityPath, "--entry", "--hide-text",
		"--title="+title,
		"--text="+prompt,
		"--width=380")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func promptPasswordKdialog(title, prompt string) (string, bool) {
	kdialogPath, err := exec.LookPath("kdialog")
	if err != nil {
		return "", false
	}
	cmd := exec.Command(kdialogPath, "--password", prompt, "--title", title)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func promptPasswordConsole(prompt string) (string, bool) {
	fmt.Print(prompt + " ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(line), true
}

func promptPermanentPasswordWithConfirmation() (string, bool) {
	for {
		pwd, confirm, ok := promptPasswordAndConfirmDialog()
		if !ok {
			return "", false
		}
		pwd = strings.TrimSpace(pwd)
		confirm = strings.TrimSpace(confirm)
		if len(pwd) < 6 {
			showLinuxDialog("RelaisDesk", "Le mot de passe permanent doit comporter au moins 6 caractères.", true)
			continue
		}
		if pwd != confirm {
			showLinuxDialog("RelaisDesk", "Les deux mots de passe ne correspondent pas. Veuillez réessayer.", true)
			continue
		}
		return pwd, true
	}
}

func promptPasswordAndConfirmDialog() (string, string, bool) {
	if isDisplayAvailable() {
		if pwd, confirm, ok := promptPasswordFormsZenity(); ok {
			return pwd, confirm, true
		}
		p1, ok1 := promptPasswordZenity("RelaisDesk - Accès Permanent", "Définissez un mot de passe permanent (min. 6 caractères) :")
		if !ok1 {
			p1, ok1 = promptPasswordKdialog("RelaisDesk - Accès Permanent", "Définissez un mot de passe permanent (min. 6 caractères) :")
		}
		if !ok1 || p1 == "" {
			return "", "", false
		}
		p2, ok2 := promptPasswordZenity("RelaisDesk - Confirmation", "Confirmez le mot de passe permanent :")
		if !ok2 {
			p2, ok2 = promptPasswordKdialog("RelaisDesk - Confirmation", "Confirmez le mot de passe permanent :")
		}
		if !ok2 {
			return "", "", false
		}
		return p1, p2, true
	}

	p1, ok1 := promptPasswordConsole("Définissez un mot de passe permanent (min. 6 caractères) :")
	if !ok1 || p1 == "" {
		return "", "", false
	}
	p2, ok2 := promptPasswordConsole("Confirmez le mot de passe permanent :")
	if !ok2 {
		return "", "", false
	}
	return p1, p2, true
}

func promptPasswordFormsZenity() (string, string, bool) {
	zenityPath, err := exec.LookPath("zenity")
	if err != nil {
		return "", "", false
	}
	cmd := exec.Command(zenityPath, "--forms",
		"--title="+PRODUCT_NAME+" - Accès Permanent",
		"--text=Définissez un mot de passe permanent pour ce poste :",
		"--add-password=Mot de passe permanent :",
		"--add-password=Confirmer le mot de passe :",
		"--separator=|")
	out, err := cmd.Output()
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.TrimRight(string(out), "\r\n"), "|")
	if len(parts) >= 2 {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func handleActivationAndLaunch(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if strings.HasPrefix(code, "PERM-") {
		if !isElevated() {
			pwd, ok := promptPermanentPasswordWithConfirmation()
			if !ok {
				return nil
			}

			args := []string{"--enroll", code, pwd}
			notifyDesktop("RelaisDesk", "Authentification requise pour installer l'accès permanent...")
			if err := relaunchElevated(args); err != nil {
				showLinuxDialog("RelaisDesk", fmt.Sprintf("Échec de l'installation ou élévation refusée : %v", err), true)
				return err
			}
			showLinuxDialog("RelaisDesk", "Accès permanent configuré avec succès !\nCe poste est maintenant accessible depuis l'Espace Technicien.", false)
			return nil
		}

		if _, err := ensureRustDeskServiceInstalled(); err != nil {
			showLinuxDialog("RelaisDesk", fmt.Sprintf("Échec de l'installation du service : %v", err), true)
			return err
		}
		hasPwd, _ := hasFleetPermanentPassword()
		if !hasPwd {
			pwd, ok := promptPermanentPasswordWithConfirmation()
			if !ok {
				showLinuxDialog("RelaisDesk", "Un mot de passe permanent d'au moins 6 caractères est requis.", true)
				return errors.New("mot de passe requis")
			}
			if err := setFleetPermanentPassword(pwd); err != nil {
				showLinuxDialog("RelaisDesk", fmt.Sprintf("Échec de la configuration du mot de passe : %v", err), true)
				return err
			}
		}

		response, err := installFleet(code)
		if err != nil {
			if isFleetReplaceableError(err) {
				_ = uninstallFleet()
				response, err = installFleet(code)
			}
			if err != nil {
				showLinuxDialog("RelaisDesk", err.Error(), true)
				return err
			}
		}
		showLinuxDialog("RelaisDesk", "Service permanent installé pour "+response.DeviceID+". Vous pouvez fermer cette fenêtre. Vérifiez une connexion réelle depuis le poste technicien.", false)
		return nil
	}
	if fleetServiceExists() {
		return fmt.Errorf("ce poste utilise l'accès permanent ; retirez-le avec sudo relaisdesk-viewer --unenroll avant une assistance temporaire")
	}
	if code == "" {
		return nil
	}

	fmt.Println("Vérification du code...")
	activation, err := activateViewerCode(code)
	if err != nil {
		showLinuxDialog("RelaisDesk", err.Error(), true)
		return err
	}

	fmt.Println("Recherche de RustDesk...")
	rustdeskPath, err := findRustDesk()
	if err != nil {
		fmt.Println("Installation de RustDesk en cours...")
		if installErr := installRustDesk(); installErr != nil {
			msg := fmt.Sprintf("Échec de l'installation de RustDesk: %v", installErr)
			showLinuxDialog("RelaisDesk", msg, true)
			return installErr
		}
		rustdeskPath, err = findRustDesk()
		if err != nil {
			err = fmt.Errorf("rustdesk installé mais introuvable: %v", err)
			showLinuxDialog("RelaisDesk", err.Error(), true)
			return err
		}
	}

	fmt.Println("Autorisation sécurisée de l’appareil...")
	networkAuthorization, err := prepareViewerNetworkAuthorization(code)
	if err != nil {
		msg := fmt.Sprintf("Autorisation réseau: %v", err)
		showLinuxDialog("RelaisDesk", msg, true)
		return fmt.Errorf("autorisation réseau: %w", err)
	}
	activation.NetworkTokenFile = networkAuthorization.TokenFile
	activation.NetworkProofKeyFile = networkAuthorization.ProofKeyFile

	fmt.Println("Configuration de RustDesk...")
	if err := configureRustDesk(activation, code); err != nil {
		showLinuxDialog("RelaisDesk", err.Error(), true)
		return err
	}

	if err := createDesktopShortcut(rustdeskPath); err != nil {
		fmt.Printf("Raccourci non créé: %s\n", err.Error())
	}

	fmt.Println("Lancement de RustDesk...")
	if err := launchRustDesk(rustdeskPath); err != nil {
		showLinuxDialog("RelaisDesk", err.Error(), true)
		return err
	}

	for i := 0; i < 30; i++ {
		time.Sleep(1 * time.Second)
		id := getRustDeskID(rustdeskPath)
		if id != "" {
			if isRustDeskKeyConfirmed(RUSTDESK_CONFIG_DIR) || i >= 3 {
				_ = announceViewerID(code, id, networkAuthorization.DevicePublicKey)
				break
			}
		}
	}

	expDate, _ := time.Parse(time.RFC3339, activation.ExpiresAt)
	expStr := expDate.Format("02/01/2006 à 15:04")
	fmt.Printf("Configuration réussie ! Votre code expire le %s.\n", expStr)
	fmt.Println("Lancement du démon de maintien de session en tâche de fond...")

	// Démarrer le démon détaché
	expiresAtRFC := networkAuthorization.ExpiresAt.Format(time.RFC3339)
	if err := spawnViewerDaemon(code, expiresAtRFC); err != nil {
		fmt.Printf("Avertissement: détachement du démon: %v (maintien au premier plan)\n", err)
		return runViewerDaemon([]string{"--code", code, "--expires", expiresAtRFC})
	}

	notifyDesktop("RelaisDesk", fmt.Sprintf("Assistance prête ! Votre technicien peut se connecter.\nCode valide jusqu'au %s.", expStr))
	fmt.Println("RelaisDesk est actif en arrière-plan. Vous pouvez fermer cette fenêtre en toute sécurité.")
	return nil
}

func spawnViewerDaemon(code, expiresAt string) error {
	binPath, err := os.Executable()
	if err != nil {
		binPath = "/usr/bin/relaisdesk-viewer"
	}
	cmd := exec.Command(binPath, "daemon", "--code", code, "--expires", expiresAt)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func runViewerDaemon(args []string) error {
	if fleetServiceExists() {
		return fmt.Errorf("accès permanent installé : démon temporaire refusé")
	}
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	codeFlag := fs.String("code", "", "Code viewer")
	expiresFlag := fs.String("expires", "", "Date expiration du jeton actuel")
	if err := fs.Parse(args); err != nil || *codeFlag == "" {
		return fmt.Errorf("paramètres invalides pour le démon viewer")
	}
	code := strings.ToUpper(strings.TrimSpace(*codeFlag))

	logger, logCloser := initViewerDaemonLogger()
	if logCloser != nil {
		defer logCloser()
	}

	logger.Printf("=== Démarrage du démon RelaisDesk Viewer (code: %s, PID: %d) ===", code, os.Getpid())

	// Gestion du fichier PID
	pidPath := getViewerPIDPath()
	_ = os.MkdirAll(filepath.Dir(pidPath), 0o700)
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600)
	defer os.Remove(pidPath)

	publicKey, proofKeyFile, tokenFile, err := ensureViewerNetworkIdentity()
	if err != nil {
		logger.Printf("Erreur identité réseau: %v", err)
		return err
	}

	expiresAt := time.Now().Add(5 * time.Minute)
	if *expiresFlag != "" {
		if t, err := time.Parse(time.RFC3339, *expiresFlag); err == nil {
			expiresAt = t
		}
	}

	authorization := &NetworkAuthorization{
		DevicePublicKey: publicKey,
		ProofKeyFile:    proofKeyFile,
		TokenFile:       tokenFile,
		ExpiresAt:       expiresAt,
	}

	// Capture des signaux d'arrêt, IGNORER SIGHUP
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sighupChan := make(chan os.Signal, 1)
	signal.Notify(sighupChan, syscall.SIGHUP)
	go func() {
		for range sighupChan {
			logger.Printf("Signal SIGHUP reçu et ignoré (fermeture de terminal détectée, maintien de l'assistance)")
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	consecutiveNotRunning := 0
	logger.Printf("Surveillance de RustDesk active. Expiration du jeton actuel: %s", authorization.ExpiresAt.Format("15:04:05"))

	for {
		select {
		case <-ctx.Done():
			logger.Printf("Arrêt du démon demandé par signal. Nettoyage de la session.")
			cleanupViewerSession(authorization)
			return nil

		case <-ticker.C:
			// 1. Vérifier si RustDesk est toujours en cours d'exécution
			if !isRustDeskRunning() {
				consecutiveNotRunning++
				logger.Printf("RustDesk non détecté (%d/3)...", consecutiveNotRunning)
				if consecutiveNotRunning >= 3 {
					logger.Printf("RustDesk n'est plus actif. Fin de session d'assistance et nettoyage automatique.")
					cleanupViewerSession(authorization)
					return nil
				}
				continue
			}
			consecutiveNotRunning = 0

			// 2. Renouvellement du jeton s'il reste 2 minutes ou moins
			remaining := time.Until(authorization.ExpiresAt)
			if remaining <= 2*time.Minute {
				logger.Printf("Renouvellement du jeton réseau (reste %v)...", remaining.Round(time.Second))
				issued, err := getViewerNetworkToken(code, authorization.DevicePublicKey)
				if err != nil {
					logger.Printf("Avertissement: échec renouvellement jeton: %v", err)
				} else {
					newExp, valErr := validateViewerNetworkTokenResponse(issued)
					if valErr != nil {
						logger.Printf("Avertissement: réponse jeton invalide: %v", valErr)
					} else {
						if writeErr := writeViewerSecretAtomically(authorization.TokenFile, issued.NetworkToken); writeErr != nil {
							logger.Printf("Erreur: écriture jeton atomique: %v", writeErr)
						} else {
							authorization.ExpiresAt = newExp
							logger.Printf("Jeton réseau renouvelé avec succès ! Prochaine expiration: %s", newExp.Format("15:04:05"))
						}
					}
				}
			}
		}
	}
}

func getViewerPIDPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "RelaisDesk", "authorization", "viewer-daemon.pid")
}

func printViewerStatus() {
	if fleetServiceExists() {
		fmt.Println("Accès permanent installé (systemd). Journaux : sudo journalctl -u relaisdesk-fleet")
		if fleetRustDeskRunning() {
			fmt.Println("Service RustDesk actif ; la joignabilité du bureau reste à tester.")
		}
		return
	}
	pidPath := getViewerPIDPath()
	pidBytes, err := os.ReadFile(pidPath)
	if err == nil {
		pidStr := strings.TrimSpace(string(pidBytes))
		fmt.Printf("Démon RelaisDesk Viewer actif (PID: %s)\n", pidStr)
	} else {
		fmt.Println("Aucun démon RelaisDesk Viewer actif.")
	}

	if isRustDeskRunning() {
		fmt.Println("RustDesk est en cours d'exécution.")
	} else {
		fmt.Println("RustDesk n'est pas lancé.")
	}

	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".local", "share", "relaisdesk", "viewer.log")
	if content, err := os.ReadFile(logPath); err == nil {
		fmt.Println("\n--- Dernières lignes du journal (viewer.log) ---")
		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		start := 0
		if len(lines) > 15 {
			start = len(lines) - 15
		}
		for _, l := range lines[start:] {
			fmt.Println(l)
		}
	}
}

func stopViewerSession() {
	if fleetServiceExists() {
		fmt.Println("Le mode permanent se retire avec sudo relaisdesk-viewer --unenroll ; --stop concerne les sessions temporaires.")
		return
	}
	pidPath := getViewerPIDPath()
	if pidBytes, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes))); err == nil && pid > 0 {
			proc, err := os.FindProcess(pid)
			if err == nil {
				_ = proc.Signal(syscall.SIGTERM)
				fmt.Printf("Signal d'arrêt envoyé au démon (PID %d)\n", pid)
			}
		}
		_ = os.Remove(pidPath)
	}
	terminateRustDesk()
	cleanupRustDesk2Toml()
	home, _ := os.UserHomeDir()
	tokenPath := filepath.Join(home, ".config", "RelaisDesk", "authorization", "viewer-network-token")
	_ = os.Remove(tokenPath)
	fmt.Println("Session RelaisDesk arrêtée et nettoyée.")
}

func cleanupViewerSession(auth *NetworkAuthorization) {
	if auth != nil && auth.TokenFile != "" {
		_ = os.Remove(auth.TokenFile)
	}
	cleanupRustDesk2Toml()
}

func initViewerDaemonLogger() (*log.Logger, func()) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	logDir := filepath.Join(home, ".local", "share", "relaisdesk")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "viewer.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return log.Default(), nil
	}
	logger := log.New(f, "", log.LstdFlags)
	return logger, func() { _ = f.Close() }
}
