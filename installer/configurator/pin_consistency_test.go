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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
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
// ok=false means extraction failed everywhere: caller must Skip.
func debInnerSHA256(debPath, innerPath string) (sum string, ok bool) {
	// Pure Go first: Windows/macOS runners have no dpkg-deb/ar, and the
	// ELF/SO pins must verify there too instead of silently skipping.
	if sum, err := debInnerSHA256Go(debPath, innerPath); err == nil {
		return sum, true
	}
	return debInnerSHA256Exec(debPath, innerPath)
}

// debInnerSHA256Go extracts one member from a .deb without external tools:
// GNU ar container, data.tar.{gz,xz,zst} by magic bytes, tar walk.
func debInnerSHA256Go(debPath, innerPath string) (string, error) {
	raw, err := os.ReadFile(debPath)
	if err != nil {
		return "", err
	}
	dataTar, err := debDataTar(raw)
	if err != nil {
		return "", err
	}
	stream, err := debDecompress(dataTar)
	if err != nil {
		return "", err
	}
	want := map[string]bool{innerPath: true, "./" + innerPath: true, strings.TrimPrefix(innerPath, "./"): true}
	tr := tar.NewReader(bytes.NewReader(stream))
	for {
		hdr, err := tr.Next()
		if err != nil {
			return "", errors.New("membre introuvable dans data.tar")
		}
		if !want[hdr.Name] || !hdr.FileInfo().Mode().IsRegular() || hdr.Size == 0 {
			continue
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, tr); err != nil {
			return "", err
		}
		return hex.EncodeToString(hash.Sum(nil)), nil
	}
}

// debDataTar returns the data.tar payload of a GNU ar archive.
func debDataTar(raw []byte) ([]byte, error) {
	if len(raw) < 8 || string(raw[:8]) != "!<arch>\n" {
		return nil, errors.New("pas une archive ar")
	}
	off := 8
	for off+60 <= len(raw) {
		name := strings.TrimSuffix(strings.TrimSpace(string(raw[off:off+16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(raw[off+48 : off+58])))
		if err != nil || size < 0 || string(raw[off+58:off+60]) != "`\n" {
			return nil, errors.New("membre ar invalide")
		}
		off += 60
		if off+size > len(raw) {
			return nil, errors.New("membre ar tronqué")
		}
		data := raw[off : off+size]
		off += size + size%2
		if strings.HasPrefix(name, "#1/") {
			// BSD long name: the name extension prefixes the data.
			ext, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(name, "#1/")))
			if err != nil || ext < 0 || ext > len(data) {
				return nil, errors.New("nom BSD invalide")
			}
			name = string(data[:ext])
			data = data[ext:]
		}
		if strings.HasPrefix(name, "data.tar.") {
			out := make([]byte, len(data))
			copy(out, data)
			return out, nil
		}
	}
	return nil, errors.New("data.tar introuvable")
}

// debDecompress expands a data.tar payload selected by magic bytes.
func debDecompress(data []byte) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	if len(data) >= 6 && bytes.Equal(data[:6], []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}) {
		r, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	}
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	// Assume an uncompressed tar.
	return data, nil
}

func debInnerSHA256Exec(debPath, innerPath string) (sum string, ok bool) {
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

func writeTestArMember(buf *bytes.Buffer, name string, data []byte) {
	header := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n", name, 0, 0, 0, 0o644, len(data))
	buf.WriteString(header)
	buf.Write(data)
	if len(data)%2 == 1 {
		buf.WriteByte('\n')
	}
}

func testDeb(t *testing.T, compress string, files map[string][]byte) string {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var payload []byte
	member := "data.tar"
	switch compress {
	case "gz":
		var gzBuf bytes.Buffer
		gz := gzip.NewWriter(&gzBuf)
		if _, err := gz.Write(tarBuf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		payload, member = gzBuf.Bytes(), "data.tar.gz"
	case "xz":
		var xzBuf bytes.Buffer
		xzw, err := xz.NewWriter(&xzBuf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := xzw.Write(tarBuf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := xzw.Close(); err != nil {
			t.Fatal(err)
		}
		payload, member = xzBuf.Bytes(), "data.tar.xz"
	case "zst":
		var zstBuf bytes.Buffer
		zstw, err := zstd.NewWriter(&zstBuf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := zstw.Write(tarBuf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if err := zstw.Close(); err != nil {
			t.Fatal(err)
		}
		payload, member = zstBuf.Bytes(), "data.tar.zst"
	default:
		t.Fatalf("compression inconnue %q", compress)
	}
	var arBuf bytes.Buffer
	arBuf.WriteString("!<arch>\n")
	writeTestArMember(&arBuf, "debian-binary", []byte("2.0\n"))
	writeTestArMember(&arBuf, member, payload)
	path := filepath.Join(t.TempDir(), "test.deb")
	if err := os.WriteFile(path, arBuf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDebInnerSHA256Go(t *testing.T) {
	elf := []byte("ELF-mock-binary-content")
	so := []byte("SO-mock-library-content")
	elfSum := sha256.Sum256(elf)
	soSum := sha256.Sum256(so)
	files := map[string][]byte{
		"usr/share/rustdesk/rustdesk":           elf,
		"usr/share/rustdesk/lib/librustdesk.so": so,
	}
	for _, compress := range []string{"gz", "xz", "zst"} {
		deb := testDeb(t, compress, files)
		got, err := debInnerSHA256Go(deb, "usr/share/rustdesk/rustdesk")
		if err != nil {
			t.Fatalf("%s: %v", compress, err)
		}
		if got != hex.EncodeToString(elfSum[:]) {
			t.Fatalf("%s: ELF=%s, want %x", compress, got, elfSum)
		}
		got, err = debInnerSHA256Go(deb, "./usr/share/rustdesk/lib/librustdesk.so")
		if err != nil {
			t.Fatalf("%s: %v", compress, err)
		}
		if got != hex.EncodeToString(soSum[:]) {
			t.Fatalf("%s: SO=%s, want %x", compress, got, soSum)
		}
	}
	// Missing member and truncated archives fail instead of hashing garbage.
	deb := testDeb(t, "gz", files)
	if _, err := debInnerSHA256Go(deb, "usr/share/rustdesk/absent"); err == nil {
		t.Fatal("membre absent accepté")
	}
	broken := filepath.Join(t.TempDir(), "broken.deb")
	if err := os.WriteFile(broken, []byte("!<arch>\ndebian-binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := debInnerSHA256Go(broken, "x"); err == nil {
		t.Fatal("archive tronquée acceptée")
	}
}

// Regression: the fork links media codecs statically since the 2026-10-08
// nightly, so libvpx.so.7, libaom.so.3, libyuv.so.0 and libjpeg.so.8 no
// longer exist in the shipped lib/ dir. A stale hardcoded list broke
// enrollment on a healthy install ("bibliothèque ... requise absente
// (libvpx.so.7)"). The viewer manifest must exactly match the lib/ payload
// of its pinned embedded deb; update both together in config_linux.go.
func TestViewerBundledLibManifest(t *testing.T) {
	const goFile = "../viewer/config_linux.go"
	src := readTestFile(t, goFile)
	re := regexp.MustCompile(`RUSTDESK_BUNDLED_LIBS\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("RUSTDESK_BUNDLED_LIBS introuvable dans %s (renommer le test si le code a change)", goFile)
	}
	manifest := strings.Fields(m[1])
	if len(manifest) == 0 {
		t.Fatalf("%s: RUSTDESK_BUNDLED_LIBS vide", goFile)
	}
	if !slices.IsSorted(manifest) {
		t.Errorf("%s: RUSTDESK_BUNDLED_LIBS doit rester trié: %q", goFile, manifest)
	}
	if deduped := slices.Compact(slices.Clone(manifest)); len(deduped) != len(manifest) {
		t.Errorf("%s: RUSTDESK_BUNDLED_LIBS contient des doublons: %q", goFile, manifest)
	}
	for _, name := range manifest {
		if strings.ContainsAny(name, "/\\") {
			t.Errorf("%s: entrée inattendue %q dans RUSTDESK_BUNDLED_LIBS (noms de fichiers seuls)", goFile, name)
		}
	}
	const debPath = "../viewer/embedded/rustdesk.deb"
	raw, err := os.ReadFile(debPath)
	if err != nil {
		t.Skipf("%s absent, comparaison contournee", debPath)
	}
	dataTar, err := debDataTar(raw)
	if err != nil {
		t.Fatalf("extraction %s: %v", debPath, err)
	}
	stream, err := debDecompress(dataTar)
	if err != nil {
		t.Fatalf("décompression %s: %v", debPath, err)
	}
	var payload []string
	tr := tar.NewReader(bytes.NewReader(stream))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("lecture tar %s: %v", debPath, err)
		}
		rest, ok := strings.CutPrefix(strings.TrimPrefix(hdr.Name, "./"), "usr/share/rustdesk/lib/")
		if !ok || rest == "" || strings.Contains(rest, "/") || !hdr.FileInfo().Mode().IsRegular() {
			continue
		}
		payload = append(payload, rest)
	}
	if len(payload) == 0 {
		t.Fatalf("aucune bibliothèque lib/ dans %s", debPath)
	}
	slices.Sort(payload)
	want := slices.Clone(manifest)
	slices.Sort(want)
	if !slices.Equal(want, payload) {
		t.Fatalf("RUSTDESK_BUNDLED_LIBS ne correspond pas au payload lib/ de %s.\nmanifeste: %q\npayload:   %q", debPath, want, payload)
	}
}
