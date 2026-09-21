//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const fleetServiceName = "RelaisDeskFleet"

func fleetPeerAuthVersion() int {
	target, err := fleetEngineExecutable()
	if err != nil || !fileMatchesFleetService(target) {
		return 0
	}
	return fleetPeerVersionInFile(target)
}

func fleetCheckAuthorization() error { return errors.New("commande réservée au service Linux") }

func fleetDirectory() (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	return filepath.Join(dir, "RelaisDeskFleet"), err
}

func protectFleetDirectory(dir string) error {
	created := true
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	} else if os.IsExist(err) {
		created = false
	}
	if !created {
		previous, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		owner, _, err := previous.Owner()
		if err != nil {
			return err
		}
		if owner.String() != "S-1-5-18" && owner.String() != "S-1-5-32-544" {
			return errors.New("le dossier du service préexiste avec un propriétaire non autorisé")
		}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("dossier du service invalide")
	}
	sd, err := windows.SecurityDescriptorFromString("O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;LS)")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil)
}

func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func relaunchElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	argStr := strings.Join(args, " ")
	argPtr, _ := windows.UTF16PtrFromString(argStr)
	cwdPtr, _ := windows.UTF16PtrFromString(filepath.Dir(exe))

	var showCmd int32 = windows.SW_NORMAL
	err = windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		return fmt.Errorf("élévation administrateur annulée ou refusée: %w", err)
	}
	return nil
}

func fleetServiceExists() bool {
	manager, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(fleetServiceName)
	if err != nil {
		return false
	}
	service.Close()
	return true
}

func rustDeskServiceExists() bool {
	manager, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("RustDesk")
	if err != nil {
		return false
	}
	service.Close()
	return true
}

func extractNativeRustDeskPayload() (exePath string, sciterPath string, err error) {
	// Validate embedded data before touching any filesystem path or executing code.
	payload, err := verifiedNativePayload()
	if err != nil {
		return "", "", err
	}
	if !isElevated() {
		return "", "", errors.New("installation du service réservée à l'administrateur")
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(root, "RelaisDeskEngine")
	if err = protectFleetDirectory(dir); err != nil {
		return "", "", err
	}
	if err = installNativePayload(dir, payload); err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "rustdesk.exe"), filepath.Join(dir, "sciter.dll"), nil
}

func ensureRustDeskServiceInstalled() (string, error) {
	if !isElevated() {
		return "", errors.New("relancez le Viewer en administrateur pour installer l'accès permanent")
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", fmt.Errorf("impossible de localiser Program Files: %w", err)
	}
	targetDir := filepath.Join(root, "RelaisDeskEngine")
	if _, err := verifiedNativePayload(); err != nil {
		return "", err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return "", fmt.Errorf("accès au gestionnaire de services Windows refusé: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("RustDesk")
	if err == nil {
		defer service.Close()
		cfg, e := service.Config()
		if e != nil {
			return "", e
		}
		if e = checkFleetServiceCommand(cfg.BinaryPathName); e != nil {
			return "", e
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return "", err
	}
	if err := protectFleetDirectory(targetDir); err != nil {
		return "", fmt.Errorf("impossible de préparer le dossier RustDesk: %w", err)
	}
	targetExe := filepath.Join(targetDir, "rustdesk.exe")
	targetSciter := filepath.Join(targetDir, "sciter.dll")

	needDeploy := !fileMatchesFleetService(targetExe)
	if !needDeploy {
		if _, err := os.Stat(targetSciter); err != nil {
			needDeploy = true
		}
	}

	if needDeploy {
		if service != nil {
			if err := stopFleetRustDesk(); err != nil {
				return "", err
			}
		}
		if err := terminateFleetEngineProcesses(); err != nil {
			return "", err
		}

		if _, _, err := extractNativeRustDeskPayload(); err != nil {
			return "", err
		}
		if !fileMatchesFleetService(targetExe) {
			return "", errors.New("composants du service non vérifiés")
		}
	}

	if service == nil {
		service, err = manager.CreateService("RustDesk", targetExe, mgr.Config{
			DisplayName: "RustDesk Service",
			StartType:   mgr.StartAutomatic,
			Description: "RustDesk Remote Desktop Service",
		}, "--service")
		if err != nil {
			return "", fmt.Errorf("impossible de créer le service Windows RustDesk: %w", err)
		}
		defer service.Close()
		_ = service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}}, 86400)
		_ = service.SetRecoveryActionsOnNonCrashFailures(true)
	} else {
		cfg, err := service.Config()
		if err != nil {
			return "", err
		}
		{
			cfg.BinaryPathName = `"` + targetExe + `" --service`
			cfg.StartType = mgr.StartAutomatic
			cfg.DisplayName = "RustDesk Service"
			if err = service.UpdateConfig(cfg); err != nil {
				return "", err
			}
		}
	}
	status, err := service.Query()
	if err == nil && status.State != svc.Running {
		if status.State == svc.Stopped {
			if err = service.Start(); err != nil {
				return "", fmt.Errorf("impossible de démarrer le service Windows RustDesk: %w", err)
			}
			if err = waitFleetService(service, svc.Running); err != nil {
				return "", fmt.Errorf("le service Windows RustDesk n'a pas pu démarrer dans les temps: %w", err)
			}
		}
	}

	return targetExe, nil
}

func hasFleetPermanentPassword() (bool, error) {
	dir, err := fleetServiceConfigDir()
	if err != nil {
		return false, err
	}
	tomlPath := filepath.Join(dir, "RustDesk.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		return false, nil
	}
	var cfg struct {
		Password string `toml:"password"`
	}
	if _, err = toml.DecodeFile(tomlPath, &cfg); err != nil || strings.TrimSpace(cfg.Password) == "" {
		return false, nil
	}
	return true, nil
}

func setFleetPermanentPassword(password string) error {
	password = strings.TrimSpace(password)
	if len(password) < 6 {
		return errors.New("le mot de passe permanent doit comporter au moins 6 caractères")
	}
	binary, err := ensureRustDeskServiceInstalled()
	if err != nil {
		return err
	}
	if err = startFleetRustDesk(); err != nil {
		return fmt.Errorf("impossible de démarrer le service RustDesk pour appliquer le mot de passe : %w", err)
	}
	cmd := exec.Command(binary, "--password", password)
	cmd.SysProcAttr = hideWindowSysProcAttr()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("échec de la définition du mot de passe permanent : %s (%w)", strings.TrimSpace(string(out)), err)
	}
	time.Sleep(300 * time.Millisecond)
	hasPwd, _ := hasFleetPermanentPassword()
	if !hasPwd {
		return errors.New("le mot de passe permanent n'a pas pu être validé dans la configuration du service")
	}

	// S'assurer que RustDesk2.toml contient les options d'accès non surveillé (pour les postes déjà enrôlés)
	if dir, err := fleetServiceConfigDir(); err == nil {
		r2Path := filepath.Join(dir, "RustDesk2.toml")
		if content, err := os.ReadFile(r2Path); err == nil {
			s := string(content)
			opts := []string{
				"verification-method = 'use-permanent-password'",
				"approve-mode = 'password'",
				"allow-hide-cm = 'Y'",
				"enable-keyboard = 'Y'",
				"enable-clipboard = 'Y'",
				"enable-file-transfer = 'Y'",
				"enable-audio = 'Y'",
				"enable-remote-restart = 'Y'",
			}
			modified := false
			for _, opt := range opts {
				k := strings.Split(opt, "=")[0]
				if !strings.Contains(s, k) {
					s = strings.TrimRight(s, "\r\n") + "\r\n" + opt + "\r\n"
					modified = true
				}
			}
			if modified {
				_ = os.WriteFile(r2Path, []byte(s), 0600)
				_ = stopFleetRustDesk()
				_ = startFleetRustDesk()
			}
		}
	}

	// Mettre à jour l'agent de service s'il existe dans ProgramData
	if dir, err := fleetDirectory(); err == nil {
		agentPath := filepath.Join(dir, "viewer-agent.exe")
		if _, err := os.Stat(agentPath); err == nil {
			if self, err := os.Executable(); err == nil {
				_ = copyFileAtomically(self, agentPath)
			}
		}
	}

	return nil
}

func openRustDeskSettings() error {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	targetExe := filepath.Join(root, "RelaisDeskEngine", "rustdesk.exe")
	if !fileMatchesFleetService(targetExe) {
		return errors.New("composants natifs non vérifiés")
	}
	cmd := exec.Command(targetExe)
	return cmd.Start()
}

func withRustDeskService(action func(*mgr.Service) error) error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("RustDesk")
	if err != nil {
		return errors.New("le fork RustDesk doit être installé en service Windows avant l'enrôlement")
	}
	defer service.Close()
	cfg, err := service.Config()
	if err != nil {
		return err
	}
	if err = checkFleetServiceCommand(cfg.BinaryPathName); err != nil {
		return err
	}
	return action(service)
}
func fleetRustDeskRunning() bool {
	running := false
	_ = withRustDeskService(func(s *mgr.Service) error {
		status, err := s.Query()
		running = err == nil && status.State == svc.Running
		return err
	})
	return running
}
func waitFleetService(s *mgr.Service, target svc.State) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == target {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("délai de changement d'état du service dépassé")
}
func startFleetRustDesk() error {
	target, err := fleetEngineExecutable()
	if err != nil {
		return err
	}
	if !fileMatchesFleetService(target) {
		return errors.New("composants du service non vérifiés ; démarrage refusé")
	}
	return withRustDeskService(func(s *mgr.Service) error {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State == svc.StopPending {
			if err = waitFleetService(s, svc.Stopped); err != nil {
				return err
			}
		}
		if err = s.Start(); err != nil {
			return err
		}
		return waitFleetService(s, svc.Running)
	})
}
func stopFleetRustDesk() error {
	return withRustDeskService(func(s *mgr.Service) error {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			return nil
		}
		if status.State != svc.StopPending {
			if _, err = s.Control(svc.Stop); err != nil {
				return err
			}
		}
		return waitFleetService(s, svc.Stopped)
	})
}

func fleetServiceConfigDir() (string, error) {
	dir, err := windows.GetWindowsDirectory()
	return filepath.Join(dir, "ServiceProfiles", "LocalService", "AppData", "Roaming", "RustDesk", "config"), err
}
func validateFleetPrerequisites() (string, error) {
	if !isElevated() {
		return "", errors.New("relancez le Viewer en administrateur pour installer l'accès permanent")
	}
	_, err := ensureRustDeskServiceInstalled()
	if err != nil {
		return "", err
	}
	var binary string
	err = withRustDeskService(func(s *mgr.Service) error {
		cfg, err := s.Config()
		if err != nil {
			return err
		}
		args, err := windows.DecomposeCommandLine(cfg.BinaryPathName)
		if err != nil || len(args) != 2 || args[1] != "--service" || !filepath.IsAbs(args[0]) || !fileMatchesFleetService(args[0]) {
			return errors.New("le service RustDesk installé ne correspond pas au binaire du fork vérifié par ce Viewer")
		}
		binary = args[0]
		return nil
	})
	if err != nil {
		return "", err
	}
	hasPwd, err := hasFleetPermanentPassword()
	if err != nil {
		return "", err
	}
	if !hasPwd {
		return binary, errors.New("configurez d'abord un mot de passe permanent fort dans les paramètres de sécurité du service RustDesk ; il n'est jamais envoyé à l'API")
	}
	return binary, nil
}

func fileMatchesFleetService(path string) bool {
	payload, err := verifiedNativePayload()
	if err != nil {
		return false
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return false
	}
	expected := filepath.Join(root, "RelaisDeskEngine", "rustdesk.exe")
	real, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(real, expected) {
		return false
	}
	return nativeDirectoryMatches(filepath.Dir(real), payload)
}

func writeFleetConfiguration(state *fleetState) error {
	a := state.Activation
	if err := validateRendezvousHost(a.ServerIP); err != nil {
		return err
	}
	if err := validateRendezvousPort(a.RendezvousPort); err != nil {
		return err
	}
	if err := validateRendezvousPort(a.RelayPort); err != nil {
		return err
	}
	if err := validateRustDeskPublicKey(a.PublicKey); err != nil {
		return err
	}
	dir, err := fleetServiceConfigDir()
	if err != nil {
		return err
	}
	relay := a.ServerIP
	if a.RelayPort != 21117 {
		relay = fmt.Sprintf("%s:%d", a.ServerIP, a.RelayPort)
	}
	config := generateViewerRustDesk2Toml(fmt.Sprintf("%s:%d", a.ServerIP, a.RendezvousPort), a.ServerIP, relay, a.PublicKey, a.NetworkTokenFile, a.NetworkProofKeyFile)
	config += "verification-method = 'use-permanent-password'\r\n"
	config += "approve-mode = 'password'\r\n"
	config += "allow-hide-cm = 'Y'\r\n"
	config += "enable-keyboard = 'Y'\r\n"
	config += "enable-clipboard = 'Y'\r\n"
	config += "enable-file-transfer = 'Y'\r\n"
	config += "enable-audio = 'Y'\r\n"
	config += "enable-remote-restart = 'Y'\r\n"
	// Keep RustDesk's own encrypted password and identity in RustDesk.toml.
	resetRustDeskKeyConfirmed(dir)
	return os.WriteFile(filepath.Join(dir, "RustDesk2.toml"), []byte(config), 0600)
}

func installFleet(code string) (*DeviceEnrollResponse, error) {
	if fleetServiceExists() {
		return nil, errors.New("un accès permanent existe déjà ; retirez-le explicitement avec --unenroll avant de le remplacer")
	}
	binary, err := validateFleetPrerequisites()
	if err != nil {
		return nil, err
	}
	if err = startFleetRustDesk(); err != nil {
		return nil, err
	}
	var id string
	for attempt := 0; attempt < 20; attempt++ {
		cmd := exec.Command(binary, "--get-id")
		cmd.SysProcAttr = hideWindowSysProcAttr()
		out, err := cmd.Output()
		if err == nil {
			cand := parseNumericRustDeskID(string(out))
			if isNumericRustDeskID(cand) {
				id = cand
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !isNumericRustDeskID(id) {
		return nil, errors.New("le service RustDesk n'a pas encore d'identifiant valide")
	}
	dir, err := fleetDirectory()
	if err != nil {
		return nil, err
	}
	if err = protectFleetDirectory(dir); err != nil {
		return nil, err
	}
	if _, err = os.Lstat(filepath.Join(dir, "state.json")); err == nil {
		return nil, errors.New("un enrôlement est déjà enregistré ; utilisez --unenroll avant de recommencer")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	key, err := loadOrCreateViewerProofKey(filepath.Join(dir, "proof-key"))
	if err != nil {
		return nil, err
	}
	response, err := enrollFleet(context.Background(), code, id, key)
	if err != nil {
		return nil, err
	}
	state := &fleetState{DeviceID: response.DeviceID, RustDeskID: id, Activation: ActivationResponse{Valid: true, ServerIP: response.ServerIP, RendezvousPort: response.RendezvousPort, RelayPort: response.RelayPort, PublicKey: response.PublicKey}}
	if err = saveFleetState(dir, state); err != nil {
		return nil, err
	}
	if err = os.Remove(filepath.Join(dir, "ready")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	source, err := os.Executable()
	if err != nil {
		return nil, err
	}
	src, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	agent := filepath.Join(dir, "viewer-agent.exe")
	dst, err := os.OpenFile(agent, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(dst, src)
	closeErr := dst.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	manager, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(fleetServiceName, agent, mgr.Config{DisplayName: "RelaisDesk — autorisation du parc", StartType: mgr.StartAutomatic, DelayedAutoStart: true}, "--fleet-service")
	if err != nil {
		return nil, err
	}
	defer service.Close()
	if err = service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 30 * time.Second}}, 86400); err != nil {
		return nil, err
	}
	if err = service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return nil, err
	}
	if err = service.Start(); err != nil {
		return nil, err
	}
	if err = waitFleetService(service, svc.Running); err != nil {
		return nil, err
	}
	rustDeskCfgDir, _ := fleetServiceConfigDir()
	deadline := time.Now().Add(30 * time.Second)
	confirmDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			if (rustDeskCfgDir != "" && isRustDeskKeyConfirmed(rustDeskCfgDir)) || time.Now().After(confirmDeadline) {
				return response, nil
			}
		}
		status, err := service.Query()
		if err != nil {
			return nil, err
		}
		if status.State == svc.Stopped {
			return nil, errors.New("le service d'autorisation s'est arrêté ; installation à vérifier")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, errors.New("service enregistré mais connexion non confirmée ; ne pas considérer l'accès permanent comme prêt")
}

type fleetWindowsService struct{}

func (fleetWindowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	dir, err := fleetDirectory()
	if err != nil {
		return false, 1
	}
	if err = stopFleetRustDesk(); err != nil {
		return false, 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runFleetAgent(ctx, dir) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if err != nil {
				return false, 3
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				return false, 0
			}
		}
	}
}
func runFleetService() error {
	yes, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !yes {
		return errors.New("cette commande est réservée au gestionnaire de services Windows")
	}
	return svc.Run(fleetServiceName, fleetWindowsService{})
}
func uninstallFleet() error {
	if !isElevated() {
		return errors.New("droits administrateur requis")
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(fleetServiceName)
	if err == nil {
		defer service.Close()
		status, e := service.Query()
		if e != nil {
			return e
		}
		if status.State != svc.Stopped {
			if _, e = service.Control(svc.Stop); e != nil {
				return e
			}
			if e = waitFleetService(service, svc.Stopped); e != nil {
				return e
			}
		}
		if e = service.Delete(); e != nil {
			return e
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}

	// Only delete the engine service owned by this installation.
	rdService, err := manager.OpenService("RustDesk")
	if err == nil {
		defer rdService.Close()
		cfg, e := rdService.Config()
		if e != nil {
			return e
		}
		if checkFleetServiceCommand(cfg.BinaryPathName) == nil {
			if e = stopFleetRustDesk(); e != nil {
				return e
			}
			if e = rdService.Delete(); e != nil {
				return e
			}
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	if err = terminateFleetEngineProcesses(); err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)

	dir, err := fleetDirectory()
	if err != nil {
		return err
	}
	for _, name := range []string{"network-token", "proof-key", "state.json", "viewer-agent.exe", "ready"} {
		if err = os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_ = os.Remove(dir)

	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	// Only our dedicated engine files; never recursively remove an unrelated
	// RustDesk installation, or follow a substituted directory.
	rdDir := filepath.Join(root, "RelaisDeskEngine")
	if info, e := os.Lstat(rdDir); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("dossier du moteur non sûr")
		}
		for _, name := range []string{"rustdesk.exe", "sciter.dll", "dylib_virtual_display.dll"} {
			if e = os.Remove(filepath.Join(rdDir, name)); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		_ = os.Remove(rdDir) // Preserve any other contents.
	} else if !os.IsNotExist(e) {
		return e
	}

	return nil
}

func applyServiceUpdate(dir, newBinaryPath string) error {
	batchFile := filepath.Join(dir, "apply-update.cmd")
	targetExe := filepath.Join(dir, "viewer-agent.exe")
	batchContent := fmt.Sprintf(`@echo off
ping 127.0.0.1 -n 3 >nul
net stop %s >nul 2>&1
move /y "%s" "%s" >nul
net start %s >nul
del "%%~f0" >nul 2>&1
`, fleetServiceName, newBinaryPath, targetExe, fleetServiceName)

	if err := os.WriteFile(batchFile, []byte(batchContent), 0700); err != nil {
		return err
	}

	cmd := exec.Command("cmd.exe", "/c", batchFile)
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}

