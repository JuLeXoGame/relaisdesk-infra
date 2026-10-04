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
	t.Run("ordre indifférent", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		if err := selfUpdateVerifyManifest(m, pub); err != nil {
			t.Fatalf("manifeste valide refusé: %v", err)
		}
	})
	t.Run("falsification détectée", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		m.Artifacts[0].URL = "https://evil.example/b.bin"
		if err := selfUpdateVerifyManifest(m, pub); err == nil {
			t.Fatal("manifeste falsifié accepté")
		}
	})
	t.Run("mauvaise clé", func(t *testing.T) {
		m := base()
		signTestManifest(t, m, priv)
		other, _ := testKey(t)
		if err := selfUpdateVerifyManifest(m, other); err == nil {
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
	}
	for name, mutate := range invalids {
		t.Run(name, func(t *testing.T) {
			m := base()
			if mutate != nil {
				mutate(m)
			}
			signTestManifest(t, m, priv)
			key := pub
			if name == "clé vide" {
				key = ""
			}
			if name == "signature vide" {
				m.Signature = ""
			}
			if err := selfUpdateVerifyManifest(m, key); err == nil {
				t.Fatalf("manifeste %q accepté", name)
			}
		})
	}
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

	info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "1.0.0")
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
		info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "9.9.9")
		if err != nil || info.Available {
			t.Fatalf("attendu à-jour: %+v %v", info, err)
		}
	})
	t.Run("anti-downgrade", func(t *testing.T) {
		info, err := SelfUpdateCheck(context.Background(), server.URL, pub, "10.0.0")
		if err != nil || info.Available {
			t.Fatalf("attendu refus silencieux: %+v %v", info, err)
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
	if err := selfUpdateApplyPortable(exe, nouveau); err != nil {
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
	script := selfUpdateWindowsSetupWaiter(`C:\Temp\setup file.exe`, `C:\Program Files\App\app.exe`, 4242)
	for _, want := range []string{"Wait-Process -Id 4242", `"C:\Temp\setup file.exe" /S`, `start "" "C:\Program Files\App\app.exe"`, `del "%~f0"`} {
		if !strings.Contains(script, want) {
			t.Errorf("script dépourvu de %q:\n%s", want, script)
		}
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
	script := selfUpdateMacScript("/tmp/RelaisDesk_Mac.dmg")
	for _, want := range []string{"hdiutil attach", "ditto", "/Applications/", "hdiutil detach", "open -a"} {
		if !strings.Contains(script, want) {
			t.Errorf("script dépourvu de %q:\n%s", want, script)
		}
	}
	quoted := selfUpdateMacScript("/tmp/a'b.dmg")
	if !strings.Contains(quoted, `'/tmp/a'\''b.dmg'`) {
		t.Errorf("quote shell incorrecte:\n%s", quoted)
	}
}
