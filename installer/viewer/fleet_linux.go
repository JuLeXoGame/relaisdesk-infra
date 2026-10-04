//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

const linuxRustDeskConfig = "/root/.config/rustdesk"

func fleetPeerAuthVersion() int {
	target, err := validateLinuxFleetBinary()
	if err != nil {
		return 0
	}
	return fleetPeerVersionInFile(target)
}

func linuxFleetCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "XDG_CONFIG_HOME=/root/.config", "LANG=C", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("commande %s échouée : %w", filepath.Base(name), err)
	}
	return out, nil
}
func linuxFleetSystemctl(args ...string) ([]byte, error) {
	return linuxFleetCommand("/usr/bin/systemctl", args...)
}

// Never adopt an unprivileged directory, or follow a symlink during root writes.
func checkLinuxFleetPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("chemin système invalide")
	}
	if path != "/" {
		if err := checkLinuxFleetPath(filepath.Dir(path)); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("chemin non protégé ou lien symbolique : %s", path)
	}
	if !info.IsDir() && (!info.Mode().IsRegular() || st.Nlink != 1) {
		return fmt.Errorf("fichier système invalide : %s", path)
	}
	return nil
}
func linuxFleetMkdir(path string, mode os.FileMode) error {
	if err := checkLinuxFleetPath(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
		return err
	}
	if err := checkLinuxFleetPath(path); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("dossier de service attendu")
	}
	return os.Chmod(path, mode)
}
func linuxFleetRead(path string, max int64, private bool) ([]byte, error) {
	if err := checkLinuxFleetPath(path); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > max || (private && info.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("fichier invalide ou permissions trop larges : %s", path)
	}
	return os.ReadFile(path)
}
func linuxFleetWriteNew(path string, data []byte, mode os.FileMode) error {
	if err := checkLinuxFleetPath(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	return errors.Join(writeErr, syncErr, f.Close())
}
func linuxFleetLock(name string) (*os.File, error) {
	path := filepath.Join(linuxFleetDir, name)
	if err := checkLinuxFleetPath(linuxFleetDir); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err = checkLinuxFleetPath(path); err == nil {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
	}
	if err != nil {
		f.Close()
		return nil, errors.New("opération de parc déjà en cours ou verrou non protégé")
	}
	return f, nil
}

func fleetServiceExists() bool {
	_, err := os.Lstat(linuxFleetUnitPath)
	return err == nil || !os.IsNotExist(err)
}
func fleetRustDeskRunning() bool {
	_, err := linuxFleetSystemctl("is-active", "--quiet", "rustdesk.service")
	return err == nil
}
func startFleetRustDesk() error {
	_, err := linuxFleetSystemctl("start", "rustdesk.service")
	if err == nil && !fleetRustDeskRunning() {
		return errors.New("le service RustDesk n'est pas actif")
	}
	return err
}
func stopFleetRustDesk() error {
	_, err := linuxFleetSystemctl("stop", "rustdesk.service")
	return err
}

func validateLinuxFleetBinary() (string, error) {
	binary, err := filepath.EvalSymlinks("/usr/bin/rustdesk")
	if err != nil {
		return "", err
	}
	if err = checkLinuxFleetPath(binary); err != nil {
		return "", err
	}
	if validPinnedSHA256(RUSTDESK_EXPECTED_SHA256) && !fileMatchesSHA256(binary, RUSTDESK_EXPECTED_SHA256) {
		return "", errors.New("le service RustDesk ne correspond pas au fork épinglé dans ce Viewer")
	}
	const library = "/usr/share/rustdesk/lib/librustdesk.so"
	if validPinnedSHA256(RUSTDESK_SO_EXPECTED_SHA256) {
		if err = checkLinuxFleetPath(library); err != nil {
			return "", err
		}
		if !fileMatchesSHA256(library, RUSTDESK_SO_EXPECTED_SHA256) {
			return "", errors.New("empreinte de librustdesk.so incompatible ; recompilez le Viewer avec la bibliothèque du fork")
		}
	} else if _, err = os.Lstat(library); err == nil {
		if err = checkLinuxFleetPath(library); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	for _, libName := range []string{"libvpx.so.7", "libaom.so.3", "libyuv.so.0", "libjpeg.so.8"} {
		libPath := "/usr/share/rustdesk/lib/" + libName
		if _, err := os.Lstat(libPath); err != nil {
			return "", fmt.Errorf("bibliothèque multimédia requise absente (%s) : %w", libName, err)
		}
	}
	return binary, nil
}
func linuxFleetContains(r io.Reader, needle []byte) bool {
	buf := make([]byte, 32768)
	tail := []byte{}
	for {
		n, err := r.Read(buf)
		part := append(tail, buf[:n]...)
		if bytes.Contains(part, needle) {
			return true
		}
		if err != nil {
			return false
		}
		keep := len(needle) - 1
		if len(part) < keep {
			keep = len(part)
		}
		tail = append([]byte(nil), part[len(part)-keep:]...)
	}
}
func checkRustDeskServiceProperties(props string) error {
	if !strings.Contains(props, "LoadState=loaded") {
		return errors.New("le service rustdesk.service n'est pas chargé dans systemd")
	}
	if !strings.Contains(props, "User=root") && !strings.Contains(props, "User=\n") && strings.Contains(props, "User=") {
		return errors.New("le service rustdesk.service doit être exécuté par root")
	}
	if !strings.Contains(props, "/usr/bin/rustdesk") || !strings.Contains(props, "--service") {
		return errors.New("le service rustdesk.service doit exécuter /usr/bin/rustdesk --service")
	}
	for _, line := range strings.Split(props, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "EnvironmentFiles=") && line != "EnvironmentFiles=" {
			return errors.New("service rustdesk.service avec EnvironmentFile personnalisé non supporté")
		}
		if strings.HasPrefix(line, "RootDirectory=") && line != "RootDirectory=" {
			return errors.New("service rustdesk.service avec RootDirectory personnalisé non supporté")
		}
	}
	return nil
}

func validateLinuxFleetPrerequisites() (string, error) {
	if os.Geteuid() != 0 {
		return "", errors.New("droits administrateur requis : utilisez sudo relaisdesk-viewer --enroll (le code sera demandé)")
	}
	root, err := user.LookupId("0")
	if err != nil || root.HomeDir != "/root" {
		return "", errors.New("cette intégration attend le compte système root avec /root comme dossier personnel")
	}
	if _, err = os.Stat("/run/systemd/system"); err != nil {
		return "", errors.New("un système Linux démarré avec systemd est requis")
	}
	if err = checkLinuxFleetPath("/usr/bin/systemctl"); err != nil {
		return "", err
	}
	binary, err := validateLinuxFleetBinary()
	if err != nil {
		return "", err
	}
	out, err := linuxFleetSystemctl("show", "--all", "rustdesk.service", "-p", "LoadState", "-p", "User", "-p", "ExecStart", "-p", "EnvironmentFiles", "-p", "RootDirectory")
	if err != nil {
		return "", err
	}
	if err = checkRustDeskServiceProperties(string(out)); err != nil {
		return "", err
	}
	b, err := linuxFleetRead(linuxRustDeskConfig+"/RustDesk.toml", 1<<20, false)
	if err != nil {
		return "", fmt.Errorf("initialisez d'abord l'identité et le mot de passe permanent du service RustDesk : %w", err)
	}
	var cfg struct {
		Password string `toml:"password"`
	}
	if _, err = toml.Decode(string(b), &cfg); err != nil || strings.TrimSpace(cfg.Password) == "" {
		return "", errors.New("configurez d'abord un mot de passe permanent fort dans le service RustDesk ; il ne sera jamais envoyé à l'API")
	}
	return binary, nil
}

func writeFleetConfiguration(state *fleetState) error {
	_ = os.MkdirAll(linuxRustDeskConfig, 0700)
	path := linuxRustDeskConfig + "/RustDesk2.toml"
	previous, err := linuxFleetRead(path, 1<<20, false)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := linuxFleetConfig(previous, state.Activation)
	if err != nil {
		return err
	}
	if err = checkLinuxFleetPath(linuxRustDeskConfig); err != nil {
		return err
	}
	f, err := os.CreateTemp(linuxRustDeskConfig, ".relaisdesk-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_ = f.Chmod(0644)
	_, writeErr := f.Write(data)
	err = errors.Join(writeErr, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	_ = os.Chmod(path, 0644)
	resetRustDeskKeyConfirmed(linuxRustDeskConfig)

	// Export only generated connection settings, not the root account's other
	// options (which can contain credentials).
	desktopData, err := linuxFleetConfig(nil, state.Activation)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir("/home"); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				if err := writeDesktopFleetConfig(filepath.Join("/home", entry.Name()), desktopData); err != nil && !os.IsNotExist(err) {
					log.Printf("Parc : configuration du bureau ignorée (dossier absent ou non sûr) : %v", err)
				}
			}
		}
	}
	return nil
}

func installFleet(code string) (_ *DeviceEnrollResponse, resultErr error) {
	binary, err := validateLinuxFleetPrerequisites()
	if err != nil {
		return nil, err
	}
	if err = linuxFleetMkdir(linuxFleetDir, 0755); err != nil {
		return nil, err
	}
	lock, err := linuxFleetLock("install.lock")
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	for _, path := range []string{linuxFleetUnitPath, linuxFleetDropinPath, linuxFleetDir + "/state.json", linuxFleetDir + "/viewer-agent"} {
		if _, err = os.Lstat(path); err == nil {
			return nil, errors.New("installation de parc déjà présente ou interrompue ; utilisez --unenroll avant de recommencer")
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if err = startFleetRustDesk(); err != nil {
		return nil, err
	}
	var id string
	var lastErr error
	var lastOut []byte
	for attempt := 0; attempt < 20; attempt++ {
		out, err := linuxFleetCommand(binary, "--get-id")
		lastOut = out
		lastErr = err
		if err == nil {
			cand := parseNumericRustDeskID(string(out))
			if isNumericRustDeskID(cand) {
				id = cand
				break
			}
		}
		if content, err := os.ReadFile(linuxRustDeskConfig + "/RustDesk.toml"); err == nil {
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "id =") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						cand := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
						if isNumericRustDeskID(cand) {
							id = cand
							break
						}
					}
				}
			}
			if id != "" {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !isNumericRustDeskID(id) {
		if lastErr != nil || len(lastOut) > 0 {
			return nil, fmt.Errorf("le service RustDesk n'a pas encore d'identifiant réel (sortie: %q, err: %v)", strings.TrimSpace(string(lastOut)), lastErr)
		}
		return nil, errors.New("le service RustDesk n'a pas encore d'identifiant réel")
	}
	if _, err = os.Lstat(linuxFleetDir + "/proof-key"); err == nil {
		if _, err = linuxFleetRead(linuxFleetDir+"/proof-key", 256, true); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	key, err := loadOrCreateViewerProofKey(linuxFleetDir + "/proof-key")
	if err != nil {
		return nil, err
	}
	response, err := enrollFleet(context.Background(), code, id, key)
	if err != nil {
		return nil, err
	}
	state := &fleetState{DeviceID: response.DeviceID, RustDeskID: id, Activation: ActivationResponse{Valid: true, ServerIP: response.ServerIP, RendezvousPort: response.RendezvousPort, RelayPort: response.RelayPort, PublicKey: response.PublicKey}}
	if err = writeFleetConfiguration(state); err != nil {
		return nil, err
	}
	if err = saveFleetState(linuxFleetDir, state); err != nil {
		return nil, err
	}
	if err = stopFleetRustDesk(); err != nil {
		return nil, err
	}
	// Copy the running image, not a path that a download-directory owner can replace.
	src, err := os.Open("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dst, err := os.OpenFile(linuxFleetDir+"/viewer-agent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(dst, src)
	if err = errors.Join(copyErr, dst.Sync(), dst.Close()); err != nil {
		return nil, err
	}
	if err = linuxFleetMkdir(filepath.Dir(linuxFleetDropinPath), 0755); err != nil {
		return nil, err
	}
	if err = linuxFleetWriteNew(linuxFleetDropinPath, []byte(linuxFleetDropin), 0644); err != nil {
		return nil, err
	}
	if err = linuxFleetWriteNew(linuxFleetUnitPath, []byte(linuxFleetUnit), 0644); err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			_, stopErr := linuxFleetSystemctl("stop", "rustdesk.service", "relaisdesk-fleet.service")
			resultErr = errors.Join(resultErr, stopErr)
		}
	}()
	if _, err = linuxFleetSystemctl("daemon-reload"); err != nil {
		return nil, err
	}
	if _, err = linuxFleetSystemctl("enable", "--now", "relaisdesk-fleet.service"); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(90 * time.Second)
	confirmDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err = linuxFleetRead(linuxFleetDir+"/ready", 64, true); err == nil && fleetRustDeskRunning() {
			if isRustDeskKeyConfirmed(linuxRustDeskConfig) || time.Now().After(confirmDeadline) {
				return response, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, errors.New("installation enregistrée mais autorisation non confirmée ; consultez sudo journalctl -u relaisdesk-fleet puis retirez l'installation incomplète avec --unenroll si nécessaire")
}

func fleetCheckAuthorization() error {
	if os.Geteuid() != 0 {
		return errors.New("commande réservée au service système")
	}
	b, err := linuxFleetRead(linuxFleetDir+"/network-token", 8192, true)
	if err != nil {
		return err
	}
	if !fleetTokenLive(strings.TrimSpace(string(b)), time.Now()) {
		return errors.New("autorisation réseau absente ou expirée")
	}
	return nil
}

func runFleetService() (resultErr error) {
	if os.Geteuid() != 0 {
		return errors.New("service système root requis")
	}
	// Repair permissions from earlier versions, only on validated root-owned
	// files. Desktop processes obtain proofs through the broker, never the key.
	for _, name := range []string{"state.json", "proof-key", "network-token"} {
		path := filepath.Join(linuxFleetDir, name)
		if err := checkLinuxFleetPath(path); err != nil {
			if name == "network-token" && os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
	}
	if _, err := linuxFleetRead(linuxFleetDir+"/state.json", 8192, true); err != nil {
		return err
	}
	if _, err := linuxFleetRead(linuxFleetDir+"/proof-key", 256, true); err != nil {
		return err
	}
	if _, err := validateLinuxFleetBinary(); err != nil {
		return err
	}
	lock, err := linuxFleetLock("service.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err = stopFleetRustDesk(); err != nil {
		return err
	}
	if err = os.Remove(linuxFleetDir + "/broker-ready"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = os.Remove(linuxFleetDir + "/network-token"); err != nil && !os.IsNotExist(err) {
		return err
	}
	closeBroker, err := startLinuxFleetBroker(ctx)
	if err != nil {
		return err
	}
	defer closeBroker()
	return runFleetAgent(ctx, linuxFleetDir)
}

func uninstallFleet() error {
	if os.Geteuid() != 0 {
		return errors.New("droits administrateur requis : sudo relaisdesk-viewer --unenroll")
	}
	if _, err := os.Lstat(linuxFleetDir); os.IsNotExist(err) {
		if fleetServiceExists() {
			return errors.New("unité de parc présente sans dossier d'état ; vérification manuelle requise")
		}
		return nil
	}
	lock, err := linuxFleetLock("install.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	managed := false
	for path, expected := range map[string]string{linuxFleetUnitPath: linuxFleetUnit, linuxFleetDropinPath: linuxFleetDropin} {
		b, err := linuxFleetRead(path, 8192, false)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if string(b) != expected {
			return fmt.Errorf("fichier système modifié, retrait automatique refusé : %s", path)
		}
		managed = true
	}
	if _, stateErr := os.Lstat(linuxFleetDir + "/state.json"); stateErr == nil {
		managed = true
	} else if !os.IsNotExist(stateErr) {
		return stateErr
	}
	if fleetServiceExists() {
		if _, err = linuxFleetSystemctl("disable", "--now", "relaisdesk-fleet.service"); err != nil {
			return err
		}
	}
	if managed {
		if err = stopFleetRustDesk(); err != nil {
			return err
		}
	}
	for _, path := range []string{linuxFleetUnitPath, linuxFleetDropinPath, linuxFleetDir + "/network-token", linuxFleetDir + "/proof-key", linuxFleetDir + "/state.json", linuxFleetDir + "/viewer-agent", linuxFleetDir + "/ready", linuxFleetDir + "/broker-ready"} {
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if managed {
		_, err = linuxFleetSystemctl("daemon-reload")
	}
	return err
}

func linuxFleetPeerAllowed(pid int, binaryInfo os.FileInfo) bool {
	info, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || !os.SameFile(info, binaryInfo) {
		return false
	}
	// sudo can move the desktop child to a session scope: check ancestry instead
	// of requiring a particular cgroup path. A separate user-launched client is refused.
	for i := 0; i < 32 && pid > 1; i++ {
		cmd, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			return false
		}
		args := bytes.Split(bytes.TrimRight(cmd, "\x00"), []byte{0})
		if len(args) == 2 && string(args[1]) == "--service" {
			info, e := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
			status, e2 := os.Stat(fmt.Sprintf("/proc/%d", pid))
			if e == nil && e2 == nil && os.SameFile(info, binaryInfo) {
				st, ok := status.Sys().(*syscall.Stat_t)
				if ok && st.Uid == 0 {
					return true
				}
			}
		}
		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return false
		}
		parent := 0
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "PPid:") {
				parent, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PPid:")))
			}
		}
		if parent <= 0 || parent == pid {
			return false
		}
		pid = parent
	}
	return false
}

func startLinuxFleetBroker(ctx context.Context) (func(), error) {
	if err := linuxFleetMkdir(linuxFleetRunDir, 0755); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(linuxFleetSocket); err == nil {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("socket de parc préexistant non sûr")
		}
		if err = os.Remove(linuxFleetSocket); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: linuxFleetSocket, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(true)
	if err = os.Chmod(linuxFleetSocket, 0666); err != nil {
		listener.Close()
		return nil, err
	}
	key, err := loadOrCreateViewerProofKey(linuxFleetDir + "/proof-key")
	if err != nil {
		listener.Close()
		return nil, err
	}
	binaryInfo, err := os.Stat("/usr/bin/rustdesk")
	if err != nil {
		listener.Close()
		return nil, err
	}
	var workers sync.WaitGroup
	var firstProof sync.Once
	slots := make(chan struct{}, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			select {
			case slots <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-slots }()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				raw, err := conn.SyscallConn()
				if err != nil {
					return
				}
				var creds *unix.Ucred
				var credentialErr error
				if raw.Control(func(fd uintptr) {
					creds, credentialErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
				}) != nil || credentialErr != nil || creds == nil {
					return
				}
				if !linuxFleetPeerAllowed(int(creds.Pid), binaryInfo) {
					return
				}
				line, err := bufio.NewReader(io.LimitReader(conn, 2048)).ReadBytes('\n')
				if err != nil || len(line) >= 2048 {
					return
				}
				var req fleetSocketRequest
				if json.Unmarshal(line, &req) != nil || ctx.Err() != nil {
					return
				}
				b, err := linuxFleetRead(linuxFleetDir+"/network-token", 8192, true)
				if err != nil {
					return
				}
				proof, err := makeFleetSocketProof(req, strings.TrimSpace(string(b)), key, time.Now())
				if err != nil {
					return
				}
				if json.NewEncoder(conn).Encode(proof) == nil {
					firstProof.Do(func() { _ = os.WriteFile(linuxFleetDir+"/broker-ready", []byte("proof-served\n"), 0600) })
				}
			}()
		}
	}()
	return func() { listener.Close(); <-done; workers.Wait() }, nil
}

func setFleetPermanentPassword(password string) error {
	password = strings.TrimSpace(password)
	if len(password) < 6 {
		return errors.New("le mot de passe permanent doit comporter au moins 6 caractères")
	}
	_ = os.MkdirAll(linuxRustDeskConfig, 0700)
	// Never on argv (world-readable via ps): hand the secret to the core
	// through a one-shot 0600 file and --password-file.
	pwdFile, err := writeEnrollPasswordFile(password)
	if err != nil {
		return fmt.Errorf("fichier de mot de passe temporaire impossible: %w", err)
	}
	defer os.Remove(pwdFile)
	binary := "/usr/bin/rustdesk"
	cmd := exec.Command(binary, "--password-file", pwdFile)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/root", "XDG_CONFIG_HOME=/root/.config", "LANG=C", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("échec de la définition du mot de passe permanent : %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func isElevated() bool {
	return os.Geteuid() == 0
}

func relaunchElevated(args []string) error {
	exe := "/usr/bin/relaisdesk-viewer"
	if _, err := os.Stat(exe); err != nil {
		if curExe, err := os.Executable(); err == nil {
			exe = curExe
		}
	}
	cmdArgs := append([]string{exe}, args...)

	// 1. Si un écran graphique est disponible (X11 / Wayland), tenter pkexec en priorité
	// pour afficher la boîte de dialogue d'authentification native du bureau Linux.
	if isDisplayAvailable() {
		if pkexec, err := exec.LookPath("pkexec"); err == nil {
			cmd := exec.Command(pkexec, cmdArgs...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return nil
			}
			outStr := strings.TrimSpace(string(out))
			if strings.Contains(outStr, "Échec") || strings.Contains(outStr, "requis") || strings.Contains(outStr, "Erreur") {
				return fmt.Errorf("%s", outStr)
			}
		}

		// Repli graphique avec sudo si disponible
		if sudo, err := exec.LookPath("sudo"); err == nil {
			sudoPwd, ok := promptPasswordDialog("RelaisDesk - Privilèges Administrateur", "Authentification requise pour installer l'accès permanent.\nEntrez votre mot de passe administrateur système :")
			if ok && strings.TrimSpace(sudoPwd) != "" {
				cmd := exec.Command(sudo, append([]string{"-S", "-p", ""}, cmdArgs...)...)
				cmd.Stdin = strings.NewReader(strings.TrimSpace(sudoPwd) + "\n")
				out, err := cmd.CombinedOutput()
				if err == nil {
					return nil
				}
				outStr := strings.TrimSpace(string(out))
				if outStr != "" {
					return fmt.Errorf("%s", outStr)
				}
				return fmt.Errorf("authentification administrateur refusée")
			}
		}
	}

	// 2. En console (ou si pkexec n'est pas présent), tenter sudo
	if sudo, err := exec.LookPath("sudo"); err == nil {
		cmd := exec.Command(sudo, cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return errors.New("sudo ou pkexec requis pour l'élévation des privilèges")
}

func ensureRustDeskServiceInstalled() (string, error) {
	if os.Geteuid() != 0 {
		return "", errors.New("droits administrateur requis pour installer le service RustDesk")
	}
	root, err := user.LookupId("0")
	if err != nil || root.HomeDir != "/root" {
		return "", errors.New("cette intégration attend le compte système root avec /root comme dossier personnel")
	}
	if _, err = os.Stat("/run/systemd/system"); err != nil {
		return "", errors.New("un système Linux démarré avec systemd est requis")
	}
	if err = checkLinuxFleetPath("/usr/bin/systemctl"); err != nil {
		return "", err
	}
	if _, err := validateLinuxFleetBinary(); err != nil {
		if installErr := installRustDesk(); installErr != nil {
			return "", fmt.Errorf("installation automatique de RustDesk impossible : %w", installErr)
		}
	}
	binary, err := validateLinuxFleetBinary()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat("/etc/systemd/system/rustdesk.service"); os.IsNotExist(err) {
		if _, err := os.Stat("/usr/lib/systemd/system/rustdesk.service"); os.IsNotExist(err) {
			if data, err := os.ReadFile("/usr/share/rustdesk/files/systemd/rustdesk.service"); err == nil {
				content := strings.ReplaceAll(string(data), "pkill", "/usr/bin/pkill")
				_ = os.WriteFile("/etc/systemd/system/rustdesk.service", []byte(content), 0644)
			}
		}
	}
	_, _ = linuxFleetSystemctl("daemon-reload")
	_, _ = linuxFleetSystemctl("enable", "--now", "rustdesk.service")
	out, err := linuxFleetSystemctl("show", "--all", "rustdesk.service", "-p", "LoadState", "-p", "User", "-p", "ExecStart", "-p", "EnvironmentFiles", "-p", "RootDirectory")
	if err != nil {
		return "", err
	}
	if err = checkRustDeskServiceProperties(string(out)); err != nil {
		return "", err
	}
	return binary, nil
}

func hasFleetPermanentPassword() (bool, error) {
	b, err := linuxFleetRead(linuxRustDeskConfig+"/RustDesk.toml", 1<<20, false)
	if err != nil {
		return false, err
	}
	var cfg struct {
		Password string `toml:"password"`
	}
	if _, err = toml.Decode(string(b), &cfg); err != nil {
		return false, err
	}
	return strings.TrimSpace(cfg.Password) != "", nil
}

func openRustDeskSettings() error {
	if p, err := exec.LookPath("rustdesk"); err == nil {
		return exec.Command(p).Start()
	}
	return nil
}

func applyServiceUpdate(dir, newBinaryPath string) error {
	targetExe := filepath.Join(dir, "viewer-agent")
	if err := os.Chmod(newBinaryPath, 0700); err != nil {
		return err
	}
	if err := os.Rename(newBinaryPath, targetExe); err != nil {
		return err
	}
	cmd := exec.Command("/usr/bin/systemctl", "restart", "relaisdesk-fleet.service")
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
