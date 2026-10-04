package handlers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	dbpkg "database"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api/config"
	"api/releasemanifest"
)

func TestPublicOrderRejectsPlaceholderBankDetailsBeforeCreatingOrder(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body := []byte(`{"email":"client@example.com","plan":"starter","technicians":1,"payment_method":"bank_transfer","name":"Client Test","address":"1 rue du Test","postal_code":"75001","city":"Paris","customer_type":"business","terms_version":"` + publicTermsVersion + `","terms_accepted":true}`)
	handler := PublicOrderHandler(db, &config.Config{
		BankIBAN:   "FR76 0000 0000 0000 0000 0000 000",
		BankBIC:    "BNPAFRPP",
		BankHolder: "RelaisDesk",
	}, nil)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("created %d order(s) with placeholder bank details", count)
	}
}

func TestPublicOrderRequiresCurrentTermsAcceptance(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body := []byte(`{"email":"client@example.com","plan":"starter","technicians":1,"payment_method":"stripe","name":"Client Test","address":"1 rue du Test","postal_code":"75001","city":"Paris","customer_type":"business","terms_version":"` + publicTermsVersion + `","terms_accepted":false}`)
	handler := PublicOrderHandler(db, &config.Config{
		DevHTTP: true, StripeMock: true, B2CSalesEnabled: true,
		LegalPhone: "+33 4 00 00 00 00", ConsumerMediatorName: "Médiateur de test",
		ConsumerMediatorURL: "https://mediateur.example.test",
	}, nil)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("created %d order(s) without terms acceptance", count)
	}
}

func TestConsumerOrderRequiresImmediatePerformanceRequest(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body := []byte(`{"email":"client@example.com","plan":"starter","technicians":1,"payment_method":"stripe","name":"Client Test","address":"1 rue du Test","postal_code":"75001","city":"Paris","customer_type":"consumer","terms_version":"` + publicTermsVersion + `","terms_accepted":true,"immediate_performance_requested":false}`)
	handler := PublicOrderHandler(db, &config.Config{
		DevHTTP: true, StripeMock: true, B2CSalesEnabled: true,
		LegalPhone: "+33 4 00 00 00 00", ConsumerMediatorName: "Médiateur de test",
		ConsumerMediatorURL: "https://mediateur.example.test",
	}, nil)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestDownloadHandlerDoesNotServeAnotherProductAsFallback(t *testing.T) {
	downloadsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(downloadsDir, "viewer.exe"), []byte("viewer"), 0600); err != nil {
		t.Fatal(err)
	}

	handler := DownloadHandler(&config.Config{DownloadsDir: downloadsDir, DevHTTP: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator", nil)
	recorder := httptest.NewRecorder()
	handler(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestDownloadHandlerServesCanonicalProduct(t *testing.T) {
	downloadsDir := t.TempDir()
	content := []byte("configurator")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Technicien_Portable.exe"), content, 0600); err != nil {
		t.Fatal(err)
	}

	handler := DownloadHandler(&config.Config{DownloadsDir: downloadsDir, DevHTTP: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator", nil)
	recorder := httptest.NewRecorder()
	handler(recorder, req)

	if recorder.Code != http.StatusOK || recorder.Body.String() != string(content) {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	sumsContent := []byte("sha256-checksums")
	if err := os.WriteFile(filepath.Join(downloadsDir, "SHA256SUMS.txt"), sumsContent, 0600); err != nil {
		t.Fatal(err)
	}
	reqSums := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/SHA256SUMS.txt", nil)
	recSums := httptest.NewRecorder()
	handler(recSums, reqSums)
	if recSums.Code != http.StatusOK || recSums.Body.String() != string(sumsContent) {
		t.Fatalf("sums status = %d, body = %q", recSums.Code, recSums.Body.String())
	}

	macViewerContent := []byte("mac-viewer-dmg")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Mac.dmg"), macViewerContent, 0600); err != nil {
		t.Fatal(err)
	}
	reqMac := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac", nil)
	recMac := httptest.NewRecorder()
	handler(recMac, reqMac)
	if recMac.Code != http.StatusOK || recMac.Body.String() != string(macViewerContent) {
		t.Fatalf("mac viewer status = %d, body = %q", recMac.Code, recMac.Body.String())
	}

	macTechContent := []byte("mac-tech-dmg")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Technicien_Mac.dmg"), macTechContent, 0600); err != nil {
		t.Fatal(err)
	}
	reqTechMac := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator-mac", nil)
	recTechMac := httptest.NewRecorder()
	handler(recTechMac, reqTechMac)
	if recTechMac.Code != http.StatusOK || recTechMac.Body.String() != string(macTechContent) {
		t.Fatalf("mac tech status = %d, body = %q", recTechMac.Code, recTechMac.Body.String())
	}

	intelViewerContent := []byte("mac-intel-viewer-dmg")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Mac_Intel.dmg"), intelViewerContent, 0600); err != nil {
		t.Fatal(err)
	}
	reqIntel := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/RelaisDesk_Mac_Intel.dmg", nil)
	recIntel := httptest.NewRecorder()
	handler(recIntel, reqIntel)
	if recIntel.Code != http.StatusOK || recIntel.Body.String() != string(intelViewerContent) {
		t.Fatalf("mac intel viewer status = %d, body = %q", recIntel.Code, recIntel.Body.String())
	}

	intelTechContent := []byte("mac-intel-tech-dmg")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Technicien_Mac_Intel.dmg"), intelTechContent, 0600); err != nil {
		t.Fatal(err)
	}
	reqIntelTech := httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator-mac-intel", nil)
	recIntelTech := httptest.NewRecorder()
	handler(recIntelTech, reqIntelTech)
	if recIntelTech.Code != http.StatusOK || recIntelTech.Body.String() != string(intelTechContent) {
		t.Fatalf("mac intel tech status = %d, body = %q", recIntelTech.Code, recIntelTech.Body.String())
	}
}

func TestDownloadHandlerServesEveryManifestArtifact(t *testing.T) {
	// Every signed binary in the shipped manifest must resolve to a
	// servable download: a signed-but-404 artefact is a broken release.
	manifestBytes, err := os.ReadFile("../../relaisdesk/downloads/release-manifest.json")
	if err != nil {
		t.Skipf("shipped manifest unavailable: %v", err)
	}
	var manifest struct {
		Artifacts []struct {
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	downloadsDir := t.TempDir()
	handler := DownloadHandler(&config.Config{DownloadsDir: downloadsDir, DevHTTP: true})
	for _, artifact := range manifest.Artifacts {
		if strings.HasSuffix(artifact.Name, ".txt") || strings.HasSuffix(artifact.Name, ".json") {
			continue
		}
		content := []byte("payload:" + artifact.Name)
		if err := os.WriteFile(filepath.Join(downloadsDir, artifact.Name), content, 0600); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/"+artifact.Name, nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != string(content) {
			t.Errorf("manifest artefact %s: status = %d, want %d", artifact.Name, recorder.Code, http.StatusOK)
		}
	}
}

func TestProductionDownloadRequiresUntamperedSignedArtifact(t *testing.T) {
	downloadsDir := t.TempDir()
	fileName := "RelaisDesk_Technicien_Portable.exe"
	content := []byte("configurator")
	artifactPath := filepath.Join(downloadsDir, fileName)
	if err := os.WriteFile(artifactPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(content)
	manifest := releasemanifest.Manifest{
		Version: "1.0.0", PublishedAt: "2026-08-25T12:00:00Z", KeyID: "release-1",
		Artifacts: []releasemanifest.Artifact{{
			Name: fileName, URL: "https://api.example.test/api/v1/downloads/" + fileName,
			SHA256: hex.EncodeToString(hash[:]), Size: int64(len(content)),
		}},
	}
	if err := releasemanifest.Sign(&manifest, privateKey); err != nil {
		t.Fatal(err)
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(downloadsDir, "release-manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		DownloadsDir: downloadsDir, ClientSourceURL: "https://example.test/client", ServerSourceURL: "https://example.test/server",
		ReleaseManifestPath: manifestPath, ReleasePublicKey: base64.RawURLEncoding.EncodeToString(publicKey),
	}
	handler := DownloadHandler(cfg)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(content) {
		t.Fatalf("signed download status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	if err := os.WriteFile(artifactPath, []byte("XXXXXXXXXXXX"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("tampered download status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestDownloadHandlerMacBetaGate(t *testing.T) {
	downloadsDir := t.TempDir()
	dmgContent := []byte("mac-beta-dmg")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Mac.dmg"), dmgContent, 0600); err != nil {
		t.Fatal(err)
	}
	exeContent := []byte("viewer-exe")
	if err := os.WriteFile(filepath.Join(downloadsDir, "RelaisDesk_Portable.exe"), exeContent, 0600); err != nil {
		t.Fatal(err)
	}
	newCfg := func(token string) *config.Config {
		return &config.Config{
			DownloadsDir: downloadsDir, ClientSourceURL: "https://example.test/client",
			ServerSourceURL: "https://example.test/server", MacBetaToken: token,
		}
	}

	// No token presented: DMG refused, other binaries unaffected.
	handler := DownloadHandler(newCfg("beta-secret-token"))
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("DMG without token: status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(exeContent) {
		t.Fatalf("non-DMG status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	// Wrong token refused.
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac?beta=nope", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("DMG with wrong token: status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	// Correct token served.
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac?beta=beta-secret-token", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(dmgContent) {
		t.Fatalf("DMG with token: status = %d, body = %q", recorder.Code, recorder.Body.String())
	}

	// Unconfigured gate denies everything (fail closed).
	openHandler := DownloadHandler(newCfg(""))
	recorder = httptest.NewRecorder()
	openHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac?beta=anything", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unconfigured gate: status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	// Developer mode stays open for fixtures.
	devHandler := DownloadHandler(&config.Config{DownloadsDir: downloadsDir, DevHTTP: true})
	recorder = httptest.NewRecorder()
	devHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/viewer-mac", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(dmgContent) {
		t.Fatalf("dev DMG: status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestMacBetaTokenOK(t *testing.T) {
	makeReq := func(target string) *http.Request {
		return httptest.NewRequest(http.MethodGet, target, nil)
	}
	if macBetaTokenOK(makeReq("/api/v1/downloads/viewer-mac?beta=s3cret"), "s3cret") {
		// ok
	} else {
		t.Fatal("valid token rejected")
	}
	for name, req := range map[string]*http.Request{
		"missing": makeReq("/api/v1/downloads/viewer-mac"),
		"empty":   makeReq("/api/v1/downloads/viewer-mac?beta="),
		"wrong":   makeReq("/api/v1/downloads/viewer-mac?beta=wrong"),
	} {
		if macBetaTokenOK(req, "s3cret") {
			t.Errorf("%s token accepted", name)
		}
	}
	if macBetaTokenOK(makeReq("/api/v1/downloads/viewer-mac?beta=s3cret"), "") {
		t.Error("token accepted with unconfigured gate")
	}
	if macBetaTokenOK(nil, "s3cret") {
		t.Error("nil request accepted")
	}
}

func TestDownloadHandlerHasNoProductionFallbackDirectory(t *testing.T) {
	handler := DownloadHandler(&config.Config{
		DownloadsDir:    "",
		ClientSourceURL: "https://example.test/client-source",
		ServerSourceURL: "https://example.test/server-source",
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/downloads/configurator", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; production searched an implicit fallback: %q", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}
