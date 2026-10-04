package main

// Self-update engine shared by both launchers.
//
// This file is byte-identical in installer/viewer and installer/configurator
// (same convention as codec_perf.go): the two Go modules cannot share a
// package because the Linux test builder stages each module alone. The only
// per-module difference lives in selfupdate_product.go.
//
// Flow: SelfUpdateCheck (startup, background) -> user consent in the GUI ->
// SelfUpdateDownload (SHA-256 + size verified) -> SelfUpdateApply (per install
// kind) -> restart. Failures always leave the running version intact.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	selfUpdateManifestCap  = 1 << 20
	selfUpdateDownloadCap  = 250 << 20 // 250 MB, same ceiling as other launchers downloads
	selfUpdateHTTPTimeout  = 15 * time.Second
	selfUpdateFailureQuiet = 6 * time.Hour
)

// selfUpdateKind selects how an update is applied on this machine.
type selfUpdateKind int

const (
	kindUnsupported selfUpdateKind = iota
	kindPortable
	kindWindowsSetup
	kindLinuxDeb
	kindMacApp
)

type selfUpdateArtifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type selfUpdateManifest struct {
	Version     string               `json:"version"`
	PublishedAt string               `json:"published_at"`
	KeyID       string               `json:"key_id"`
	Artifacts   []selfUpdateArtifact `json:"artifacts"`
	Signature   string               `json:"signature"`
}

// SelfUpdateInfo describes an available update and its verified artifact.
type SelfUpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	Available      bool
	Kind           selfUpdateKind
	ArtifactName   string
	ArtifactURL    string
	ArtifactSHA256 string
	ArtifactSize   int64
}

var (
	selfUpdateFailMu      sync.Mutex
	selfUpdateFailVersion string
	selfUpdateFailTime    time.Time
	selfUpdateLaterMu     sync.Mutex
	selfUpdateLater       bool
)

// ErrSelfUpdateAssisted signals that the update file is verified and kept
// but needs a manual final step (message carries the guidance).
var ErrSelfUpdateAssisted = errors.New("action manuelle requise")

// SelfUpdateDismissForSession records a "Later" answer: no new prompt until restart.
func SelfUpdateDismissForSession() {
	selfUpdateLaterMu.Lock()
	selfUpdateLater = true
	selfUpdateLaterMu.Unlock()
}

func selfUpdateDismissed() bool {
	selfUpdateLaterMu.Lock()
	defer selfUpdateLaterMu.Unlock()
	return selfUpdateLater
}

// selfUpdateNoteFailure records a failed version to avoid nagging loops.
func selfUpdateNoteFailure(version string) {
	selfUpdateFailMu.Lock()
	selfUpdateFailVersion = version
	selfUpdateFailTime = time.Now()
	selfUpdateFailMu.Unlock()
}

func selfUpdateRecentlyFailed(version string) bool {
	selfUpdateFailMu.Lock()
	defer selfUpdateFailMu.Unlock()
	return selfUpdateFailVersion == version && time.Since(selfUpdateFailTime) < selfUpdateFailureQuiet
}

// SelfUpdateShouldCheck reports whether a startup check makes sense right now.
func SelfUpdateShouldCheck() bool {
	return !selfUpdateDismissed()
}

// SelfUpdateCleanupPending removes a stale ".old" binary left by a previous
// portable self-replace. Safe to call at every startup.
func SelfUpdateCleanupPending(exePath string) {
	if strings.TrimSpace(exePath) == "" {
		return
	}
	_ = os.Remove(exePath + ".old")
}

// ClassifyInstallKind detects how this launcher is installed from its
// executable path. Pure function of (exePath, goos, progFiles) so it stays
// unit-testable on every OS.
func ClassifyInstallKind(exePath, goos string, progFiles []string) selfUpdateKind {
	// Normalize both separators explicitly: filepath.ToSlash is host-dependent
	// (a no-op for backslashes on Linux), while this function must classify
	// any OS path from any test host.
	slashed := strings.ReplaceAll(filepath.Clean(exePath), "\\", "/")
	switch goos {
	case "windows":
		lower := strings.ToLower(slashed)
		for _, dir := range progFiles {
			dir = strings.ToLower(strings.TrimRight(strings.ReplaceAll(filepath.Clean(dir), "\\", "/"), "/"))
			if dir == "" {
				continue
			}
			if lower == dir || strings.HasPrefix(lower, dir+"/") {
				return kindWindowsSetup
			}
		}
		return kindPortable
	case "linux":
		for _, prefix := range []string{"/usr/bin/", "/usr/local/bin/", "/opt/", "/snap/"} {
			if strings.HasPrefix(slashed, prefix) {
				return kindLinuxDeb
			}
		}
		return kindPortable
	case "darwin":
		if strings.Contains(slashed, ".app/Contents/") {
			if strings.HasPrefix(slashed, "/Volumes/") {
				return kindUnsupported // running straight from the DMG
			}
			return kindMacApp
		}
		return kindPortable
	default:
		return kindUnsupported
	}
}

// SelfUpdateDetectKind classifies the running executable.
func SelfUpdateDetectKind() selfUpdateKind {
	exe, err := os.Executable()
	if err != nil {
		return kindUnsupported
	}
	var progFiles []string
	if runtime.GOOS == "windows" {
		progFiles = []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")}
	}
	return ClassifyInstallKind(exe, runtime.GOOS, progFiles)
}

// selfUpdateArtifactName maps (product, OS, arch, install kind) to the
// manifest artifact name. Empty string means "no supported artifact".
func selfUpdateArtifactName(product, goos, goarch string, kind selfUpdateKind) string {
	if product != "technician" && product != "viewer" {
		return ""
	}
	tech := product == "technician"
	switch goos {
	case "windows":
		if kind == kindWindowsSetup {
			if tech {
				return "RelaisDesk_Technicien_Setup_1.0.0.exe"
			}
			return "RelaisDesk_Setup.exe"
		}
		if kind == kindPortable {
			if tech {
				return "RelaisDesk_Technicien_Portable.exe"
			}
			return "RelaisDesk_Portable.exe"
		}
	case "linux":
		if kind == kindLinuxDeb {
			if tech {
				return "RelaisDesk_Technicien.deb"
			}
			return "RelaisDesk_viewer.deb"
		}
		if kind == kindPortable {
			if tech {
				return "RelaisDesk_Technicien_Linux"
			}
			return "RelaisDesk_Viewer_Linux"
		}
	case "darwin":
		if kind != kindMacApp && kind != kindPortable {
			return ""
		}
		intel := goarch == "amd64"
		if tech {
			if intel {
				return "RelaisDesk_Technicien_Mac_Intel.dmg"
			}
			return "RelaisDesk_Technicien_Mac.dmg"
		}
		if intel {
			return "RelaisDesk_Mac_Intel.dmg"
		}
		return "RelaisDesk_Mac.dmg"
	}
	return ""
}

// selfUpdateCompareVersions compares strict X.Y.Z versions (optional "v"
// prefix). Returns 1, 0 or -1.
func selfUpdateCompareVersions(left, right string) (int, error) {
	parse := func(value string) ([3]uint64, error) {
		var parsed [3]uint64
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "v")), ".")
		if len(parts) != 3 {
			return parsed, errors.New("format de version invalide")
		}
		for i, part := range parts {
			if part == "" {
				return parsed, errors.New("format de version invalide")
			}
			for _, c := range part {
				if c < '0' || c > '9' {
					return parsed, errors.New("format de version invalide")
				}
			}
			n, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return parsed, errors.New("format de version invalide")
			}
			parsed[i] = n
		}
		return parsed, nil
	}
	lv, err := parse(left)
	if err != nil {
		return 0, err
	}
	rv, err := parse(right)
	if err != nil {
		return 0, err
	}
	for i := range lv {
		if lv[i] > rv[i] {
			return 1, nil
		}
		if lv[i] < rv[i] {
			return -1, nil
		}
	}
	return 0, nil
}

// selfUpdateValidateURL accepts only https URLs (or http on loopback for
// local development), without userinfo or fragment.
func selfUpdateValidateURL(rawurl string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawurl))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("URL invalide")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" {
		host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
	}
	return errors.New("schéma d'URL refusé (https requis)")
}

func selfUpdateVerifyManifest(manifest *selfUpdateManifest, encodedPublicKey string) error {
	if manifest == nil || len(manifest.Artifacts) == 0 {
		return errors.New("manifeste de mise à jour vide")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encodedPublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("clé publique de mise à jour invalide")
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(manifest.Signature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("signature de mise à jour invalide")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return errors.New("date de publication invalide")
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == "" || strings.ContainsAny(artifact.Name, `/\`) {
			return errors.New("nom d'artefact invalide")
		}
		if err := selfUpdateValidateURL(artifact.URL); err != nil {
			return fmt.Errorf("URL d'artefact refusée: %w", err)
		}
		hash, hashErr := hex.DecodeString(strings.TrimSpace(artifact.SHA256))
		if len(hash) != sha256.Size || hashErr != nil {
			return errors.New("empreinte d'artefact invalide")
		}
		if artifact.Size < 1 || artifact.Size > selfUpdateDownloadCap {
			return errors.New("taille d'artefact invalide")
		}
	}
	sortedArtifacts := make([]selfUpdateArtifact, len(manifest.Artifacts))
	copy(sortedArtifacts, manifest.Artifacts)
	sort.Slice(sortedArtifacts, func(i, j int) bool { return sortedArtifacts[i].Name < sortedArtifacts[j].Name })
	payload, err := json.Marshal(struct {
		Version     string               `json:"version"`
		PublishedAt string               `json:"published_at"`
		KeyID       string               `json:"key_id"`
		Artifacts   []selfUpdateArtifact `json:"artifacts"`
	}{
		Version:     manifest.Version,
		PublishedAt: manifest.PublishedAt,
		KeyID:       manifest.KeyID,
		Artifacts:   sortedArtifacts,
	})
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("signature de mise à jour refusée")
	}
	return nil
}

func selfUpdateFetchManifest(ctx context.Context, apiURL, publicKey string) (*selfUpdateManifest, error) {
	base, err := url.Parse(strings.TrimSpace(apiURL))
	if err != nil || base.Host == "" || base.User != nil {
		return nil, errors.New("adresse API invalide")
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return nil, errors.New("adresse API invalide")
	}
	if base.Scheme == "http" {
		if err := selfUpdateValidateURL(base.String()); err != nil {
			return nil, errors.New("API non chiffrée hors localhost refusée")
		}
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/public/releases/latest"
	base.RawQuery = ""
	base.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: selfUpdateHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canal de mise à jour inaccessible: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("canal de mise à jour indisponible (HTTP %d)", resp.StatusCode)
	}
	var manifest selfUpdateManifest
	decoder := json.NewDecoder(io.LimitReader(resp.Body, selfUpdateManifestCap+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errors.New("manifeste de mise à jour invalide")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("manifeste de mise à jour ambigu")
	}
	if err := selfUpdateVerifyManifest(&manifest, publicKey); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// SelfUpdateCheck fetches the signed release manifest and reports whether a
// strictly newer version with a usable artifact exists for this machine.
func SelfUpdateCheck(ctx context.Context, apiURL, publicKey, currentVersion string) (*SelfUpdateInfo, error) {
	kind := SelfUpdateDetectKind()
	if kind == kindUnsupported {
		return nil, errors.New("mise à jour automatique non prise en charge sur cette installation")
	}
	manifest, err := selfUpdateFetchManifest(ctx, apiURL, publicKey)
	if err != nil {
		return nil, err
	}
	comparison, err := selfUpdateCompareVersions(manifest.Version, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("version publiée invalide: %w", err)
	}
	info := &SelfUpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  manifest.Version,
		Kind:           kind,
	}
	if comparison <= 0 {
		return info, nil // up to date (or newer than published): nothing to do
	}
	want := selfUpdateArtifactName(selfUpdateProduct, runtime.GOOS, runtime.GOARCH, kind)
	if want == "" {
		return nil, errors.New("aucun artefact de mise à jour pour cette plateforme")
	}
	for _, artifact := range manifest.Artifacts {
		if strings.EqualFold(artifact.Name, want) {
			info.Available = true
			info.ArtifactName = artifact.Name
			info.ArtifactURL = artifact.URL
			info.ArtifactSHA256 = artifact.SHA256
			info.ArtifactSize = artifact.Size
			return info, nil
		}
	}
	return nil, fmt.Errorf("artefact %s absent du manifeste %s", want, manifest.Version)
}

// SelfUpdateDownload fetches the update artifact to a temporary file,
// enforcing the manifest's exact size and SHA-256. The caller removes the
// file when done.
func SelfUpdateDownload(ctx context.Context, info *SelfUpdateInfo) (string, error) {
	if info == nil || !info.Available {
		return "", errors.New("aucune mise à jour à télécharger")
	}
	if err := selfUpdateValidateURL(info.ArtifactURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.ArtifactURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("téléchargement échoué: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("téléchargement refusé (HTTP %d)", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "relaisdesk-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(resp.Body, selfUpdateDownloadCap+1))
	closeErr := tmp.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("erreur de lecture: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}
	if written != info.ArtifactSize {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("taille inattendue (%d octets, manifeste: %d)", written, info.ArtifactSize)
	}
	calculated := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(calculated, strings.TrimSpace(info.ArtifactSHA256)) {
		_ = os.Remove(tmpPath)
		return "", errors.New("somme de contrôle SHA-256 non conforme")
	}
	return tmpPath, nil
}

// selfUpdateCheckMagic validates the downloaded file header for its kind.
func selfUpdateCheckMagic(kind selfUpdateKind, goos string, filePath string) error {
	head := make([]byte, 512)
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	st, statErr := f.Stat()
	_ = f.Close()
	if statErr != nil {
		return statErr
	}
	switch kind {
	case kindLinuxDeb:
		if !bytes.HasPrefix(head, []byte("!<arch>\n")) {
			return errors.New("le fichier téléchargé n'est pas un paquet .deb valide")
		}
		return nil
	case kindMacApp:
		if st.Size() < 512 {
			return errors.New("fichier .dmg trop petit")
		}
		tail := make([]byte, 512)
		f2, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, err = f2.ReadAt(tail, st.Size()-512)
		_ = f2.Close()
		if err != nil {
			return err
		}
		if !bytes.Contains(tail, []byte("koly")) {
			return errors.New("le fichier téléchargé n'est pas un .dmg valide")
		}
		return nil
	default:
		switch goos {
		case "windows":
			if !bytes.HasPrefix(head, []byte("MZ")) {
				return errors.New("le fichier téléchargé n'est pas un exécutable Windows valide")
			}
		case "linux":
			if !bytes.HasPrefix(head, []byte("\x7fELF")) {
				return errors.New("le fichier téléchargé n'est pas un exécutable Linux valide")
			}
		case "darwin":
			if !isMachOMagic(head) {
				return errors.New("le fichier téléchargé n'est pas un exécutable macOS valide")
			}
		default:
			return errors.New("plateforme non prise en charge")
		}
		return nil
	}
}

func isMachOMagic(head []byte) bool {
	if len(head) < 4 {
		return false
	}
	// Mach-O/FAT magics as stored on disk (MH_MAGIC, MH_MAGIC_64,
	// FAT_MAGIC, FAT_MAGIC_64 and their byte-swapped CIGAM forms).
	magics := [][]byte{
		{0xFE, 0xED, 0xFA, 0xCE},
		{0xFE, 0xED, 0xFA, 0xCF},
		{0xCA, 0xFE, 0xBA, 0xBE},
		{0xCA, 0xFE, 0xBA, 0xBF},
		{0xCE, 0xFA, 0xED, 0xFE},
		{0xCF, 0xFA, 0xED, 0xFE},
	}
	for _, m := range magics {
		if bytes.Equal(head[:4], m) {
			return true
		}
	}
	return false
}

// SelfUpdateApply installs a verified download according to the install kind,
// then restarts the application. It returns only on failure.
func SelfUpdateApply(info *SelfUpdateInfo, filePath string) error {
	if info == nil || !info.Available {
		return errors.New("aucune mise à jour à appliquer")
	}
	if err := selfUpdateCheckMagic(info.Kind, runtime.GOOS, filePath); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("exécutable introuvable: %w", err)
	}
	switch info.Kind {
	case kindPortable:
		if err := selfUpdateApplyPortable(exe, filePath); err != nil {
			return err
		}
		SelfUpdateRelaunch(exe)
		return nil
	case kindWindowsSetup:
		return selfUpdateApplyWindowsSetup(exe, filePath)
	case kindLinuxDeb:
		return selfUpdateApplyLinuxDeb(exe, filePath)
	case kindMacApp:
		return selfUpdateApplyMacApp(exe, filePath)
	default:
		return errors.New("mise à jour automatique non prise en charge sur cette installation")
	}
}

// selfUpdateApplyPortable swaps the running binary with the verified
// download. Renaming (not overwriting) also works on a running Windows exe.
func selfUpdateApplyPortable(exePath, newFile string) error {
	data, err := os.ReadFile(newFile)
	if err != nil {
		return err
	}
	current, err := os.Stat(exePath)
	if err != nil {
		return err
	}
	tmpPath := exePath + ".new"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, current.Mode().Perm()); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(exePath + ".old")
	if err := os.Rename(exePath, exePath+".old"); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("remplacement impossible (lancez en administrateur ?): %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Rename(exePath+".old", exePath) // best-effort rollback
		return fmt.Errorf("activation de la mise à jour impossible: %w", err)
	}
	return nil
}

// selfUpdateWindowsSetupWaiter builds the helper script that waits for this
// process to exit, runs the new installer silently, then restarts the app.
func selfUpdateWindowsSetupWaiter(setupPath, exePath string, pid int) string {
	quoted := func(p string) string {
		return `"` + strings.ReplaceAll(p, `"`, `""`) + `"`
	}
	return "@echo off\r\n" +
		"powershell -NoProfile -Command \"Wait-Process -Id " + strconv.Itoa(pid) + " -ErrorAction SilentlyContinue\"\r\n" +
		quoted(setupPath) + " /S\r\n" +
		"start \"\" " + quoted(exePath) + "\r\n" +
		"del " + quoted(setupPath) + " >nul 2>&1\r\n" +
		"del \"%~f0\" >nul 2>&1\r\n"
}

func selfUpdateApplyWindowsSetup(exePath, setupFile string) error {
	dest := filepath.Join(os.TempDir(), "relaisdesk-setup-"+strconv.Itoa(os.Getpid())+".exe")
	if err := copyFile(dest, setupFile, 0600); err != nil {
		return err
	}
	waiter := selfUpdateWindowsSetupWaiter(dest, exePath, os.Getpid())
	batPath := filepath.Join(os.TempDir(), "relaisdesk-update-"+strconv.Itoa(os.Getpid())+".cmd")
	if err := os.WriteFile(batPath, []byte(waiter), 0600); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if err := selfUpdateSpawnDetached("cmd.exe", []string{"/c", batPath}); err != nil {
		_ = os.Remove(dest)
		_ = os.Remove(batPath)
		return err
	}
	os.Exit(0)
	return nil
}

// selfUpdateDebPlan returns the install command for a verified .deb, or the
// assisted fallback when privilege escalation is unavailable.
func selfUpdateDebPlan(debPath string, hasPkexec bool) (args []string, assisted bool) {
	if hasPkexec {
		return []string{"pkexec", "dpkg", "-i", debPath}, false
	}
	return nil, true
}

func selfUpdateApplyLinuxDeb(exePath, debPath string) error {
	hasPkexec := false
	if _, err := exec.LookPath("pkexec"); err == nil {
		hasPkexec = true
	}
	args, assisted := selfUpdateDebPlan(debPath, hasPkexec)
	if assisted {
		if opener, err := exec.LookPath("xdg-open"); err == nil {
			_ = exec.Command(opener, debPath).Start()
		}
		return fmt.Errorf("%w : paquet vérifié enregistré à %s (installez-le avec votre gestionnaire de paquets)", ErrSelfUpdateAssisted, debPath)
	}
	_ = exePath
	cmd := exec.Command(args[0], args[1:]...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installation du paquet refusée: %v %s", err, strings.TrimSpace(string(out)))
	}
	if exe, err := os.Executable(); err == nil {
		SelfUpdateRelaunch(exe)
	}
	return nil
}

// selfUpdateMacScript builds the helper script that mounts the verified .dmg,
// copies the application to /Applications and relaunches it.
func selfUpdateMacScript(dmgPath string) string {
	quoted := func(p string) string {
		return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
	}
	return "#!/bin/sh\nset -u\n" +
		"DMG=" + quoted(dmgPath) + "\n" +
		"MNT=$(hdiutil attach -nobrowse -mountrandom /tmp \"$DMG\" 2>/dev/null | awk '{print $NF}')\n" +
		"[ -z \"$MNT\" ] && exit 10\n" +
		"APP=$(ls -d \"$MNT\"/*.app 2>/dev/null | head -1)\n" +
		"[ -z \"$APP\" ] && { hdiutil detach \"$MNT\" >/dev/null 2>&1; exit 11; }\n" +
		"ditto \"$APP\" \"/Applications/$(basename \"$APP\")\" || { hdiutil detach \"$MNT\" >/dev/null 2>&1; exit 12; }\n" +
		"hdiutil detach \"$MNT\" >/dev/null 2>&1\n" +
		"open -a \"/Applications/$(basename \"$APP\")\"\n"
}

func selfUpdateApplyMacApp(exePath, dmgPath string) error {
	_ = exePath
	script := selfUpdateMacScript(dmgPath)
	shPath := filepath.Join(os.TempDir(), "relaisdesk-update-"+strconv.Itoa(os.Getpid())+".sh")
	if err := os.WriteFile(shPath, []byte(script), 0700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", shPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = exec.Command("/usr/bin/open", dmgPath).Start()
		_ = os.Remove(shPath)
		return fmt.Errorf("%w : le disque %s est ouvert, glissez l'application vers /Applications (%v %s)",
			ErrSelfUpdateAssisted, filepath.Base(dmgPath), err, strings.TrimSpace(string(out)))
	}
	_ = os.Remove(shPath)
	_ = os.Remove(dmgPath)
	os.Exit(0)
	return nil
}

// SelfUpdateRelaunch starts a new instance of the given executable with the
// current arguments, then terminates this process.
func SelfUpdateRelaunch(exePath string) {
	args := []string{}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}
	if err := selfUpdateSpawnDetached(exePath, args); err != nil {
		return
	}
	os.Exit(0)
}

func copyFile(dest, src string, perm os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, perm)
}
