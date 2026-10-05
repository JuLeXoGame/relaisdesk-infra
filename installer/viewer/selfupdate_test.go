package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestSelfUpdateCompareVersions(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
		wantErr     bool
	}{
		{"1.0.0", "1.0.0", 0, false},
		{"1.0.1", "1.0.0", 1, false},
		{"1.0.0", "1.0.1", -1, false},
		{"1.10.0", "1.9.9", 1, false},
		{"v2.0.0", "1.9.9", 1, false},
		{" 1.0.1 ", "1.0.0", 1, false},
		{"", "1.0.0", 0, true},
		{"1.0", "1.0.0", 0, true},
		{"1.0.0.0", "1.0.0", 0, true},
		{"1.0.a", "1.0.0", 0, true},
		{"1.0.-1", "1.0.0", 0, true},
		{"1.0.+1", "1.0.0", 0, true},
	}
	for _, c := range cases {
		got, err := selfUpdateCompareVersions(c.left, c.right)
		if c.wantErr {
			if err == nil {
				t.Errorf("compare(%q,%q): erreur attendue", c.left, c.right)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("compare(%q,%q) = %d,%v, want %d", c.left, c.right, got, err, c.want)
		}
	}
}

func TestSelfUpdateProductValid(t *testing.T) {
	if selfUpdateProduct != "viewer" && selfUpdateProduct != "technician" {
		t.Fatalf("selfUpdateProduct inattendu: %q", selfUpdateProduct)
	}
}

func TestSelfUpdateArtifactName(t *testing.T) {
	cases := []struct {
		product, goos, goarch string
		kind                  selfUpdateKind
		want                  string
	}{
		{"technician", "windows", "amd64", kindPortable, "RelaisDesk_Technicien_Portable.exe"},
		{"technician", "windows", "amd64", kindWindowsSetup, "RelaisDesk_Technicien_Setup_1.0.0.exe"},
		{"technician", "linux", "amd64", kindPortable, "RelaisDesk_Technicien_Linux"},
		{"technician", "linux", "amd64", kindLinuxDeb, "RelaisDesk_Technicien.deb"},
		{"technician", "darwin", "arm64", kindMacApp, "RelaisDesk_Technicien_Mac.dmg"},
		{"technician", "darwin", "amd64", kindMacApp, "RelaisDesk_Technicien_Mac_Intel.dmg"},
		{"viewer", "windows", "amd64", kindPortable, "RelaisDesk_Portable.exe"},
		{"viewer", "windows", "amd64", kindWindowsSetup, "RelaisDesk_Setup.exe"},
		{"viewer", "linux", "amd64", kindPortable, "RelaisDesk_Viewer_Linux"},
		{"viewer", "linux", "amd64", kindLinuxDeb, "RelaisDesk_viewer.deb"},
		{"viewer", "darwin", "arm64", kindMacApp, "RelaisDesk_Mac.dmg"},
		{"viewer", "darwin", "amd64", kindMacApp, "RelaisDesk_Mac_Intel.dmg"},
		{"viewer", "darwin", "arm64", kindPortable, "RelaisDesk_Mac.dmg"},
		{"viewer", "plan9", "amd64", kindPortable, ""},
		{"viewer", "windows", "amd64", kindUnsupported, ""},
		{"viewer", "windows", "amd64", kindLinuxDeb, ""},
		{"intrus", "windows", "amd64", kindPortable, ""},
	}
	for _, c := range cases {
		if got := selfUpdateArtifactName(c.product, c.goos, c.goarch, c.kind); got != c.want {
			t.Errorf("artifact(%s,%s,%s,%d) = %q, want %q", c.product, c.goos, c.goarch, c.kind, got, c.want)
		}
	}
}

func TestClassifyInstallKind(t *testing.T) {
	progFiles := []string{`C:\Program Files`, `C:\Program Files (x86)`}
	cases := []struct {
		path, goos string
		want       selfUpdateKind
	}{
		{`C:\Program Files\RelaisDesk\configurator.exe`, "windows", kindWindowsSetup},
		{`c:\program files (x86)\RelaisDesk\viewer.exe`, "windows", kindWindowsSetup},
		{`C:\Users\moi\RelaisDesk_Technicien_Portable.exe`, "windows", kindPortable},
		{`C:\Program FilesFake\app.exe`, "windows", kindPortable},
		{"/usr/bin/relaisdesk-viewer", "linux", kindLinuxDeb},
		{"/usr/local/bin/app", "linux", kindLinuxDeb},
		{"/opt/relaisdesk/app", "linux", kindLinuxDeb},
		{"/home/moi/RelaisDesk_Viewer_Linux", "linux", kindPortable},
		{"/Applications/RelaisDesk Viewer.app/Contents/MacOS/viewer", "darwin", kindMacApp},
		{"/Volumes/RelaisDesk/app.app/Contents/MacOS/app", "darwin", kindUnsupported},
		{"/Users/moi/viewer", "darwin", kindPortable},
		{"/x/app", "plan9", kindUnsupported},
	}
	for _, c := range cases {
		if got := ClassifyInstallKind(c.path, c.goos, progFiles); got != c.want {
			t.Errorf("classify(%q,%s) = %d, want %d", c.path, c.goos, got, c.want)
		}
	}
}

func TestSelfUpdateValidateURL(t *testing.T) {
	valid := []string{
		"https://api.relaisdesk.fr/api/v1/downloads/a.exe",
		"http://localhost:8080/a.exe",
		"http://127.0.0.1/a.exe",
		"http://[::1]/a.exe",
	}
	for _, u := range valid {
		if err := selfUpdateValidateURL(u); err != nil {
			t.Errorf("URL %q refusée: %v", u, err)
		}
	}
	invalid := []string{
		"",
		"http://api.relaisdesk.fr/a.exe",
		"ftp://x.example/a",
		"https://user@api.relaisdesk.fr/a.exe",
		"https://api.relaisdesk.fr/a.exe#frag",
		"https:///sans-hote",
		"http://localhost.evil.com/a.exe",
	}
	for _, u := range invalid {
		if err := selfUpdateValidateURL(u); err == nil {
			t.Errorf("URL %q acceptée, want refus", u)
		}
	}
}

// signTestManifest signe un manifeste avec la forme canonique du moteur.
func signTestManifest(t *testing.T, m *selfUpdateManifest, priv ed25519.PrivateKey) {
	t.Helper()
	sorted := append([]selfUpdateArtifact(nil), m.Artifacts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	payload, err := json.Marshal(struct {
		Version     string               `json:"version"`
		PublishedAt string               `json:"published_at"`
		KeyID       string               `json:"key_id"`
		Artifacts   []selfUpdateArtifact `json:"artifacts"`
	}{m.Version, m.PublishedAt, m.KeyID, sorted})
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, payload))
}

func testKey(t *testing.T) (pub string, priv ed25519.PrivateKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(pubKey), privKey
}

func TestSelfUpdateVerifyManifest(t *testing.T) {
	pub, priv := testKey(t)
	base := func() *selfUpdateManifest {
		return &selfUpdateManifest{
			Version: "1.1.0", PublishedAt: "2026-10-04T00:00:00Z", KeyID: "test-1",
			Artifacts: []selfUpdateArtifact{
				{Name: "b.bin", URL: "https://cdn.example/b.bin", SHA256: strings.Repeat("a", 64), Size: 10},
				{Name: "a.bin", URL: "https://cdn.example/a.bin", SHA256: strings.Repeat("b", 64), Size: 20},
			},
		}
	}
	ring := map[string]string{"test-1": pub}
	t.Run("ordre indifférent", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		if err := selfUpdateVerifyManifest(m, ring); err != nil {
			t.Fatalf("manifeste valide refusé: %v", err)
		}
	})
	t.Run("falsification détectée", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		m.Artifacts[0].URL = "https://evil.example/b.bin"
		if err := selfUpdateVerifyManifest(m, ring); err == nil {
			t.Fatal("manifeste falsifié accepté")
		}
	})
	t.Run("mauvaise clé", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		other, _ := testKey(t)
		if err := selfUpdateVerifyManifest(m, map[string]string{"test-1": other}); err == nil {
			t.Fatal("manifeste accepté avec une autre clé")
		}
	})
	invalids := map[string]func(m *selfUpdateManifest){
		"sans artefacts": func(m *selfUpdateManifest) { m.Artifacts = nil },
		"mauvaise date":  func(m *selfUpdateManifest) { m.PublishedAt = "demain" },
		"nom séparateur": func(m *selfUpdateManifest) { m.Artifacts[0].Name = "../x" },
		"url http":       func(m *selfUpdateManifest) { m.Artifacts[0].URL = "http://cdn.example/b.bin" },
		"sha courte":     func(m *selfUpdateManifest) { m.Artifacts[0].SHA256 = "abcd" },
		"taille nulle":   func(m *selfUpdateManifest) { m.Artifacts[0].Size = 0 },
		"taille énorme":  func(m *selfUpdateManifest) { m.Artifacts[0].Size = selfUpdateDownloadCap + 1 },
		"signature vide": func(m *selfUpdateManifest) { m.Signature = "" },
		"clé vide":       nil,
		"clé inconnue":   nil,
		"key_id vide":    nil,
	}
	for name, mutate := range invalids {
		t.Run(name, func(t *testing.T) {
			m := base()
			if mutate != nil {
				mutate(m)
			}
			if name == "clé inconnue" {
				m.KeyID = "release-2"
			}
			if name == "key_id vide" {
				m.KeyID = ""
			}
			signTestManifest(t, m, priv)
			keys := ring
			if name == "clé vide" {
				keys = map[string]string{"test-1": ""}
			}
			if name == "signature vide" {
				m.Signature = ""
			}
			if err := selfUpdateVerifyManifest(m, keys); err == nil {
				t.Fatalf("manifeste %q accepté", name)
			}
		})
	}
	t.Run("rotation accepte les deux clés", func(t *testing.T) {
		pub2, priv2 := testKey(t)
		keys := map[string]string{"test-1": pub, "test-2": pub2}
		m1 := base()
		signTestManifest(t, m1, priv)
		if err := selfUpdateVerifyManifest(m1, keys); err != nil {
			t.Fatalf("ancienne clé refusée: %v", err)
		}
		m2 := base()
		m2.KeyID = "test-2"
		signTestManifest(t, m2, priv2)
		if err := selfUpdateVerifyManifest(m2, keys); err != nil {
			t.Fatalf("nouvelle clé refusée: %v", err)
		}
		// ... mais jamais une clé hors trousseau, même bien signée.
		pub3, priv3 := testKey(t)
		_ = pub3
		m3 := base()
		m3.KeyID = "test-3"
		signTestManifest(t, m3, priv3)
		if err := selfUpdateVerifyManifest(m3, keys); err == nil {
			t.Fatal("clé hors trousseau acceptée")
		}
	})
}

func TestSelfUpdateCheckEndToEnd(t *testing.T) {
	pub, priv := testKey(t)
	payload := []byte("faux-binaire-de-test")
	sum := sha256.Sum256(payload)
	// The test binary runs from a temp dir, hence kindPortable on every OS.
	artifactName := selfUpdateArtifactName(selfUpdateProduct, runtime.GOOS, runtime.GOARCH, kindPortable)
	if artifactName == "" {
		t.Skipf("aucun artefact portable pour %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/public/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		m := &selfUpdateManifest{
			Version: "9.9.9", PublishedAt: "2026-10-04T00:00:00Z", KeyID: "test-1",
			Artifacts: []selfUpdateArtifact{
				{Name: artifactName, URL: "http://" + r.Host + "/dl.bin", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload))},
			},
		}
		signTestManifest(t, m, priv)
		_ = json.NewEncoder(w).Encode(m)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "test-1", "1.0.0")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !info.Available || info.LatestVersion != "9.9.9" || info.ArtifactName != artifactName {
		t.Fatalf("info inattendue: %+v", info)
	}
	if info.ArtifactSize != int64(len(payload)) {
		t.Fatalf("taille inattendue: %d", info.ArtifactSize)
	}

	t.Run("à jour", func(t *testing.T) {
		info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "test-1", "9.9.9")
		if err != nil || info.Available {
			t.Fatalf("attendu à-jour: %+v %v", info, err)
		}
	})
	t.Run("anti-downgrade", func(t *testing.T) {
		info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "test-1", "10.0.0")
		if err != nil || info.Available {
			t.Fatalf("attendu refus silencieux: %+v %v", info, err)
		}
	})
	t.Run("mauvais key_id refusé", func(t *testing.T) {
		if _, err := SelfUpdateCheck(context.Background(), server.URL, pub, "release-1", "1.0.0"); err == nil {
			t.Fatal("manifeste accepté avec un key_id inattendu")
		}
	})
}

func TestSelfUpdateDownload(t *testing.T) {
	payload := []byte("contenu-vérifié-0123456789")
	sum := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		info := &SelfUpdateInfo{Available: true, ArtifactURL: server.URL, ArtifactSHA256: hex.EncodeToString(sum[:]), ArtifactSize: int64(len(payload))}
		path, err := SelfUpdateDownload(ctx, info)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		defer os.Remove(path)
		got, _ := os.ReadFile(path)
		if string(got) != string(payload) {
			t.Fatal("contenu altéré")
		}
	})
	t.Run("sha rejetée", func(t *testing.T) {
		info := &SelfUpdateInfo{Available: true, ArtifactURL: server.URL, ArtifactSHA256: strings.Repeat("0", 64), ArtifactSize: int64(len(payload))}
		if _, err := SelfUpdateDownload(ctx, info); err == nil {
			t.Fatal("mauvaise empreinte acceptée")
		}
	})
	t.Run("taille rejetée", func(t *testing.T) {
		info := &SelfUpdateInfo{Available: true, ArtifactURL: server.URL, ArtifactSHA256: hex.EncodeToString(sum[:]), ArtifactSize: int64(len(payload) + 1)}
		if _, err := SelfUpdateDownload(ctx, info); err == nil {
			t.Fatal("mauvaise taille acceptée")
		}
	})
	t.Run("sans info", func(t *testing.T) {
		if _, err := SelfUpdateDownload(ctx, nil); err == nil {
			t.Fatal("téléchargement sans info accepté")
		}
	})
}

func TestSelfUpdateDownloadMacBetaToken(t *testing.T) {
	payload := []byte("dmg-beta-0123456789")
	sum := sha256.Sum256(payload)
	var gotBeta string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.URL.Query().Get("beta")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	ctx := context.Background()
	sha := hex.EncodeToString(sum[:])

	// .dmg downloads carry the beta token.
	info := &SelfUpdateInfo{Available: true, ArtifactName: "RelaisDesk_Mac.dmg", ArtifactURL: server.URL + "/a.dmg", ArtifactSHA256: sha, ArtifactSize: int64(len(payload))}
	path, err := selfUpdateDownloadWithBetaToken(ctx, info, "tok-bêta")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer os.Remove(path)
	if gotBeta != "tok-bêta" {
		t.Fatalf("beta=%q, want jeton transmis", gotBeta)
	}

	// Other artifacts never carry it, even when a token is configured.
	gotBeta = "sentinel"
	exe := &SelfUpdateInfo{Available: true, ArtifactName: "RelaisDesk_Portable.exe", ArtifactURL: server.URL + "/a.exe", ArtifactSHA256: sha, ArtifactSize: int64(len(payload))}
	path, err = selfUpdateDownloadWithBetaToken(ctx, exe, "tok-bêta")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer os.Remove(path)
	if gotBeta != "" {
		t.Fatalf("beta=%q joint à un non-dmg", gotBeta)
	}

	// No token configured: plain URL.
	gotBeta = "sentinel"
	path, err = selfUpdateDownloadWithBetaToken(ctx, info, "")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	defer os.Remove(path)
	if gotBeta != "" {
		t.Fatalf("beta=%q sans jeton configuré", gotBeta)
	}
}

func TestSelfUpdateApplyPortable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app")
	if err := os.WriteFile(exe, []byte("vieux"), 0755); err != nil {
		t.Fatal(err)
	}
	nouveau := filepath.Join(dir, "nouveau")
	if err := os.WriteFile(nouveau, []byte("neuf"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("neuf"))
	// Swapped bytes (same path, wrong hash) must be rejected.
	if err := selfUpdateApplyPortable(exe, nouveau, strings.Repeat("0", 64)); err == nil {
		t.Fatal("payload au hash non conforme accepté")
	}
	if err := selfUpdateApplyPortable(exe, nouveau, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "neuf" {
		t.Fatalf("binaire non remplacé: %q", got)
	}
	old, _ := os.ReadFile(exe + ".old")
	if string(old) != "vieux" {
		t.Fatal("sauvegarde .old manquante")
	}
	st, _ := os.Stat(exe)
	wantPerm := os.FileMode(0755)
	if runtime.GOOS == "windows" {
		wantPerm = 0666 // Windows only tracks the read-only bit
	}
	if st.Mode().Perm() != wantPerm {
		t.Fatalf("permissions perdues: %o", st.Mode().Perm())
	}
	SelfUpdateCleanupPending(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old non nettoyé")
	}
}

func TestIsMachOMagic(t *testing.T) {
	if !isMachOMagic([]byte{0xcf, 0xfa, 0xed, 0xfe}) {
		t.Error("MH_MAGIC_64 refusé")
	}
	if !isMachOMagic([]byte{0xca, 0xfe, 0xba, 0xbe}) {
		t.Error("FAT_MAGIC refusé")
	}
	if isMachOMagic([]byte("MZ..")) || isMachOMagic([]byte{0x7f, 'E', 'L', 'F'}) || isMachOMagic(nil) {
		t.Error("faux positif mach-o")
	}
}

func TestSelfUpdateCheckMagic(t *testing.T) {
	write := func(t *testing.T, data []byte) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "f.bin")
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := selfUpdateCheckMagic(kindPortable, "windows", write(t, []byte("MZ..."))); err != nil {
		t.Errorf("MZ refusé: %v", err)
	}
	if err := selfUpdateCheckMagic(kindPortable, "windows", write(t, []byte("ELF."))); err == nil {
		t.Error("non-MZ accepté pour windows")
	}
	if err := selfUpdateCheckMagic(kindPortable, "linux", write(t, []byte("\x7fELF...."))); err != nil {
		t.Errorf("ELF refusé: %v", err)
	}
	if err := selfUpdateCheckMagic(kindLinuxDeb, "linux", write(t, []byte("!<arch>\n......"))); err != nil {
		t.Errorf("deb refusé: %v", err)
	}
	dmg := make([]byte, 600)
	copy(dmg[596:], []byte("koly"))
	if err := selfUpdateCheckMagic(kindMacApp, "darwin", write(t, dmg)); err != nil {
		t.Errorf("dmg refusé: %v", err)
	}
	if err := selfUpdateCheckMagic(kindMacApp, "darwin", write(t, make([]byte, 600))); err == nil {
		t.Error("faux dmg accepté")
	}
}

func TestSelfUpdateWindowsSetupWaiter(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	script := selfUpdateWindowsSetupWaiter(`C:\Temp\setup file.exe`, `C:\Program Files\App\app.exe`, 4242, sha)
	for _, want := range []string{"Wait-Process -Id 4242", `"C:\Temp\setup file.exe" /S`, `start "" "C:\Program Files\App\app.exe"`, `del "%~f0"`,
		"Get-FileHash -Algorithm SHA256", sha, "if errorlevel 1 exit /b 1"} {
		if !strings.Contains(script, want) {
			t.Errorf("script dépourvu de %q:\n%s", want, script)
		}
	}
	pct := selfUpdateWindowsSetupWaiter(`C:\Temp\100%ok\setup.exe`, `C:\App\app.exe`, 1, sha)
	if !strings.Contains(pct, `"C:\Temp\100%%ok\setup.exe"`) {
		t.Errorf("pourcent non échappé pour cmd:\n%s", pct)
	}
}

func TestSelfUpdateDebPlan(t *testing.T) {
	args, assisted := selfUpdateDebPlan("/tmp/a.deb", true)
	if assisted || len(args) != 4 || args[0] != "pkexec" || args[3] != "/tmp/a.deb" {
		t.Fatalf("plan pkexec inattendu: %v %v", args, assisted)
	}
	args, assisted = selfUpdateDebPlan("/tmp/a.deb", false)
	if !assisted || args != nil {
		t.Fatalf("repli assisté inattendu: %v %v", args, assisted)
	}
}

func TestSelfUpdateMacScript(t *testing.T) {
	sha := strings.Repeat("cd", 32)
	script := selfUpdateMacScript("/tmp/RelaisDesk_Mac.dmg", sha)
	for _, want := range []string{"hdiutil attach", "ditto", "/Applications/", "hdiutil detach", "open -a",
		"shasum -a 256", sha, "exit 9"} {
		if !strings.Contains(script, want) {
			t.Errorf("script dépourvu de %q:\n%s", want, script)
		}
	}
	quoted := selfUpdateMacScript("/tmp/a'b.dmg", sha)
	if !strings.Contains(quoted, `'/tmp/a'\''b.dmg'`) {
		t.Errorf("quote shell incorrecte:\n%s", quoted)
	}
}

func TestSelfUpdateVerifyFile(t *testing.T) {
	payload := []byte("\x7fELF-faux-binaire")
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])
	p := filepath.Join(t.TempDir(), "f.bin")
	if err := os.WriteFile(p, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := selfUpdateVerifyFile(p, sha, int64(len(payload))); err != nil {
		t.Errorf("payload conforme rejeté: %v", err)
	}
	if err := selfUpdateVerifyFile(p, strings.Repeat("0", 64), int64(len(payload))); err == nil {
		t.Error("hash non conforme accepté")
	}
	if err := selfUpdateVerifyFile(p, sha, int64(len(payload)+1)); err == nil {
		t.Error("taille non conforme acceptée")
	}
	if err := selfUpdateVerifyFile(p+"-absent", sha, int64(len(payload))); err == nil {
		t.Error("fichier absent accepté")
	}
}

func TestSelfUpdateApplyRejectsSwappedFile(t *testing.T) {
	dir := t.TempDir()
	staged := filepath.Join(dir, "staged")
	// Magic bytes must match the platform: SelfUpdateApply checks the
	// executable format before the hash.
	original := []byte("\x7fELF-version-originale")
	swapped := []byte("\x7fELF-version-substitue")
	if runtime.GOOS == "windows" {
		original = []byte("MZ-version-originale-win")
		swapped = []byte("MZ-version-substitue-win")
	}
	sum := sha256.Sum256(original)
	info := &SelfUpdateInfo{
		Available:      true,
		Kind:           kindPortable,
		ArtifactSHA256: hex.EncodeToString(sum[:]),
		ArtifactSize:   int64(len(original)),
	}
	// Swap between download and apply: same size, valid magic, wrong bytes.
	if len(swapped) != len(original) {
		t.Fatalf("préparation du test incohérente: %d vs %d", len(swapped), len(original))
	}
	if err := os.WriteFile(staged, swapped, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SelfUpdateApply(info, staged); err == nil {
		t.Fatal("fichier substitué accepté à l'installation")
	} else if !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("erreur inattendue: %v", err)
	}
}

func TestCopyFileStreamsContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	payload := []byte("contenu-copié-en-streaming-0123456789")
	if err := os.WriteFile(src, payload, 0600); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.bin")
	if err := copyFile(dst, src, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("copie = %q, want %q", got, payload)
	}
	if err := copyFile(filepath.Join(dir, "nope.bin"), filepath.Join(dir, "absent.bin"), 0600); err == nil {
		t.Fatal("source absente acceptée")
	}
}

// A .deb swapped after the apply-time check must be refused before pkexec:
// the hash is re-verified milliseconds before handing the file to root.
func TestSelfUpdateApplyLinuxDebReverifiesHash(t *testing.T) {
	dir := t.TempDir()
	deb := filepath.Join(dir, "pkg.deb")
	original := []byte("faux-paquets-de-meme-taille-1234")
	if err := os.WriteFile(deb, original, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(original)
	sha := hex.EncodeToString(sum[:])
	size := int64(len(original))
	// Tamper after "download": same size, different bytes.
	tampered := append([]byte(nil), original...)
	tampered[0] ^= 0xff
	if err := os.WriteFile(deb, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("exécutable introuvable: %v", err)
	}
	if err := selfUpdateApplyLinuxDeb(exe, deb, sha, size); err == nil {
		t.Fatal("paquet substitué accepté avant pkexec")
	} else if !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("erreur inattendue: %v", err)
	}
}
