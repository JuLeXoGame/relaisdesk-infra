package main

// Regression test: the SHA-256 pins compiled into the technician and viewer
// apps must match the RustDesk binaries actually shipped in embedded/.
// A stale pin blocks the app right after login ("empreinte SHA-256 invalide").
//
// Layout notes:
//   - embedded/*.deb files are tracked in git: their checks always run.
//   - embedded/*.exe files are git-ignored: exe checks run when present and
//     skip on fresh checkouts (CI), but pin *format* is always validated.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	goPinAny = map[string]*regexp.Regexp{
		"RUSTDESK_EXPECTED_SHA256":         regexp.MustCompile(`RUSTDESK_EXPECTED_SHA256\s*=\s*"([^"]*)"`),
		"RUSTDESK_PACKAGE_EXPECTED_SHA256": regexp.MustCompile(`RUSTDESK_PACKAGE_EXPECTED_SHA256\s*=\s*"([^"]*)"`),
		"RUSTDESK_SO_EXPECTED_SHA256":      regexp.MustCompile(`RUSTDESK_SO_EXPECTED_SHA256\s*=\s*"([^"]*)"`),
		"RUSTDESK_SERVICE_EXPECTED_SHA256": regexp.MustCompile(`RUSTDESK_SERVICE_EXPECTED_SHA256\s*=\s*"([^"]*)"`),
	}
)

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lecture %s: %v", path, err)
	}
	return string(data)
}

func extractGoPin(t *testing.T, source, file, name string) string {
	t.Helper()
	re, ok := goPinAny[name]
	if !ok {
		t.Fatalf("pin inconnue %s", name)
	}
	m := re.FindStringSubmatch(source)
	if m == nil {
		t.Fatalf("pin %s introuvable dans %s (renommer le test si le code a change)", name, file)
	}
	return m[1]
}

func extractPs1Pin(t *testing.T, source, file, name string) string {
	t.Helper()
	re := regexp.MustCompile(`\$` + regexp.QuoteMeta(name) + `\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(source)
	if m == nil {
		t.Fatalf("variable $%s introuvable dans %s (renommer le test si le script a change)", name, file)
	}
	return m[1]
}

func validHex64(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lecture %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// debInnerSHA256 extracts one file from a .deb (ar archive) and hashes it.
// ok=false means the extraction toolchain is unavailable: caller must Skip.
func debInnerSHA256(debPath, innerPath string) (sum string, ok bool) {
	if _, err := exec.LookPath("tar"); err != nil {
		return "", false
	}
	var dataTar []byte
	// dpkg-deb outputs an uncompressed tar and handles xz/gzip/zstd
	// internally (no external zstd binary needed).
	if _, err := exec.LookPath("dpkg-deb"); err == nil {
		if out, err := exec.Command("dpkg-deb", "--fsys-tarfile", debPath).Output(); err == nil && len(out) > 0 {
			dataTar = out
		}
	}
	if dataTar == nil {
		if _, err := exec.LookPath("ar"); err != nil {
			return "", false
		}
		for _, member := range []string{"data.tar.xz", "data.tar.gz", "data.tar.zst"} {
			out, err := exec.Command("ar", "p", debPath, member).Output()
			if err == nil && len(out) > 0 {
				dataTar = out
				break
			}
		}
	}
	if dataTar == nil {
		return "", false
	}
	for _, candidate := range []string{innerPath, "./" + innerPath, strings.TrimPrefix(innerPath, "./")} {
		// No decompression flag: GNU/BSD tar autodetects xz/gzip/zstd.
		tar := exec.Command("tar", "-xOf", "-", candidate)
		tar.Stdin = bytes.NewReader(dataTar)
		var out bytes.Buffer
		tar.Stdout = &out
		if err := tar.Run(); err != nil || out.Len() == 0 {
			continue
		}
		sum := sha256.Sum256(out.Bytes())
		return hex.EncodeToString(sum[:]), true
	}
	return "", false
}

func checkExePin(t *testing.T, exePath, goFile, goVar, ps1File, ps1Var string) {
	t.Helper()
	goSrc := readTestFile(t, goFile)
	ps1Src := readTestFile(t, ps1File)
	goPin := extractGoPin(t, goSrc, goFile, goVar)
	ps1Pin := extractPs1Pin(t, ps1Src, ps1File, ps1Var)
	if !validHex64(goPin) {
		t.Errorf("%s: pin %q invalide (64 hex attendus)", goFile, goPin)
	}
	if !validHex64(ps1Pin) {
		t.Errorf("%s: $%s %q invalide (64 hex attendus)", ps1File, ps1Var, ps1Pin)
	}
	if !strings.EqualFold(goPin, ps1Pin) {
		t.Errorf("pin desaccord: %s=%s mais %s $%s=%s", goFile, goPin, ps1File, ps1Var, ps1Pin)
	}
	if _, err := os.Stat(exePath); err != nil {
		t.Skipf("%s absent (git-ignored), comparaison contournee", exePath)
	}
	actual := sha256OfFile(t, exePath)
	if !strings.EqualFold(actual, goPin) {
		t.Errorf("%s sha256=%s ne correspond pas au pin %s (%s)", exePath, actual, goPin, goFile)
	}
}

func checkDebPins(t *testing.T, debPath, goFile, ps1File, ps1Elf, ps1Deb, ps1So, elfInner, soInner string) {
	t.Helper()
	goSrc := readTestFile(t, goFile)
	ps1Src := readTestFile(t, ps1File)
	elfPin := extractGoPin(t, goSrc, goFile, "RUSTDESK_EXPECTED_SHA256")
	debPin := extractGoPin(t, goSrc, goFile, "RUSTDESK_PACKAGE_EXPECTED_SHA256")
	soPin := extractGoPin(t, goSrc, goFile, "RUSTDESK_SO_EXPECTED_SHA256")
	for name, pin := range map[string]string{"ELF": elfPin, "DEB": debPin, "SO": soPin} {
		if !validHex64(pin) {
			t.Errorf("%s: pin %s %q invalide (64 hex attendus)", goFile, name, pin)
		}
	}
	pairs := map[string][2]string{
		"ELF": {elfPin, extractPs1Pin(t, ps1Src, ps1File, ps1Elf)},
		"DEB": {debPin, extractPs1Pin(t, ps1Src, ps1File, ps1Deb)},
		"SO":  {soPin, extractPs1Pin(t, ps1Src, ps1File, ps1So)},
	}
	for name, p := range pairs {
		if !strings.EqualFold(p[0], p[1]) {
			t.Errorf("pin %s desaccord: %s=%s mais %s=%s", name, goFile, p[0], ps1File, p[1])
		}
	}
	if _, err := os.Stat(debPath); err != nil {
		t.Skipf("%s absent, comparaison contournee", debPath)
	}
	if actual := sha256OfFile(t, debPath); !strings.EqualFold(actual, debPin) {
		t.Errorf("%s sha256=%s ne correspond pas au pin %s", debPath, actual, debPin)
	}
	if actual, ok := debInnerSHA256(debPath, elfInner); !ok {
		t.Skipf("extraction %s impossible ici (outil manquant ou membre inconnu), ELF/SO non verifies", debPath)
	} else if !strings.EqualFold(actual, elfPin) {
		t.Errorf("%s:%s sha256=%s ne correspond pas au pin ELF %s", debPath, elfInner, actual, elfPin)
	}
	if actual, ok := debInnerSHA256(debPath, soInner); !ok {
		t.Skipf("extraction %s impossible ici (outil manquant ou membre inconnu), SO non verifie", debPath)
	} else if !strings.EqualFold(actual, soPin) {
		t.Errorf("%s:%s sha256=%s ne correspond pas au pin SO %s", debPath, soInner, actual, soPin)
	}
}

func TestWindowsPinMatchesEmbeddedTechnician(t *testing.T) {
	checkExePin(t,
		filepath.Join("embedded", "rustdesk.exe"),
		"config_windows.go", "RUSTDESK_EXPECTED_SHA256",
		filepath.Join("..", "build_technicien_portable.ps1"), "windowsHash",
	)
}

func TestWindowsPinMatchesEmbeddedViewer(t *testing.T) {
	checkExePin(t,
		filepath.Join("..", "viewer", "embedded", "rustdesk.exe"),
		filepath.Join("..", "viewer", "config_windows.go"), "RUSTDESK_EXPECTED_SHA256",
		filepath.Join("..", "build_viewer_windows.ps1"), "forkWindowsSha256",
	)
}

func TestWindowsServicePinMatchesEmbeddedViewer(t *testing.T) {
	checkExePin(t,
		filepath.Join("..", "viewer", "embedded", "fleet", "rustdesk.exe"),
		filepath.Join("..", "viewer", "config_windows.go"), "RUSTDESK_SERVICE_EXPECTED_SHA256",
		filepath.Join("..", "build_viewer_windows.ps1"), "forkWindowsServiceSha256",
	)
}

func TestLinuxPinsMatchEmbeddedTechnician(t *testing.T) {
	checkDebPins(t,
		filepath.Join("embedded", "rustdesk.deb"),
		"config_linux.go",
		filepath.Join("..", "build_linux_technicien.ps1"),
		"rustdeskElfSha", "rustdeskDebSha", "rustdeskSoSha",
		"usr/share/rustdesk/rustdesk", "usr/share/rustdesk/lib/librustdesk.so",
	)
}

func TestLinuxPinsMatchEmbeddedViewer(t *testing.T) {
	checkDebPins(t,
		filepath.Join("..", "viewer", "embedded", "rustdesk.deb"),
		filepath.Join("..", "viewer", "config_linux.go"),
		filepath.Join("..", "build_linux_viewer.ps1"),
		"rustdeskElfSha", "rustdeskDebSha", "rustdeskSoSha",
		"usr/share/rustdesk/rustdesk", "usr/share/rustdesk/lib/librustdesk.so",
	)
}
