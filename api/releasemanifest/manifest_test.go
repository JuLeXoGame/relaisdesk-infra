package releasemanifest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestSignatureRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Version: "1.2.3", PublishedAt: "2026-08-25T12:00:00Z", KeyID: "release-1",
		Artifacts: []Artifact{{Name: "RelaisDesk.exe", URL: "https://api.example.test/api/v1/downloads/RelaisDesk.exe", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42}},
	}
	if err := Sign(&manifest, privateKey); err != nil {
		t.Fatal(err)
	}
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(publicKey)
	if err := Verify(manifest, encodedPublicKey, "release-1"); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Artifacts[0].Size++
	if err := Verify(manifest, encodedPublicKey, "release-1"); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}

func TestManifestSignatureRejectsUnexpectedKeyID(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Version: "1.2.3", PublishedAt: "2026-08-25T12:00:00Z", KeyID: "release-2",
		Artifacts: []Artifact{{Name: "RelaisDesk.exe", URL: "https://api.example.test/api/v1/downloads/RelaisDesk.exe", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42}},
	}
	if err := Sign(&manifest, privateKey); err != nil {
		t.Fatal(err)
	}
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(publicKey)
	// Valid signature, but from an unlisted key: must still be rejected.
	if err := Verify(manifest, encodedPublicKey, "release-1"); err == nil {
		t.Fatal("manifest from unlisted key accepted")
	}
}

func TestLoadVerifiedManifest(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Version: "1.0.0", PublishedAt: "2026-10-28T12:00:00Z", KeyID: "test-release",
		Artifacts: []Artifact{{Name: "RelaisDesk.exe", URL: "https://example.test/RelaisDesk.exe", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42}},
	}
	if err := Sign(&manifest, privateKey); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "release-manifest.json")
	writeManifest := func() {
		t.Helper()
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest()
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(publicKey)
	loaded, err := LoadVerified(path, encodedPublicKey, manifest.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != manifest.Version || len(loaded.Artifacts) != 1 || loaded.Artifacts[0] != manifest.Artifacts[0] {
		t.Fatalf("unexpected loaded manifest: %+v", loaded)
	}
	manifest.Artifacts[0].Size++
	writeManifest()
	if _, err := LoadVerified(path, encodedPublicKey, manifest.KeyID); err == nil {
		t.Fatal("loaded a manifest with a tampered artifact")
	}
}
