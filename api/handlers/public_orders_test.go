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
