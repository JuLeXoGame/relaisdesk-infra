package releasemanifest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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
	if err := Verify(manifest, encodedPublicKey); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Artifacts[0].Size++
	if err := Verify(manifest, encodedPublicKey); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}

func TestCheckProductionManifest(t *testing.T) {
	manifest, err := LoadVerified("../../relaisdesk/downloads/release-manifest.json", "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo")
	if err != nil {
		t.Fatalf("Verification failed: %v", err)
	} else {
		t.Logf("Verification succeeded: version %s", manifest.Version)
	}
}

