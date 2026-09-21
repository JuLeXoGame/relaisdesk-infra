package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifyReleaseManifestRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	previous := RELEASE_PUBLIC_KEY
	RELEASE_PUBLIC_KEY = base64.RawURLEncoding.EncodeToString(publicKey)
	defer func() { RELEASE_PUBLIC_KEY = previous }()

	manifest := releaseManifest{
		Version: "1.2.3", PublishedAt: "2026-08-25T12:00:00Z", KeyID: "release-1",
		Artifacts: []releaseArtifact{{Name: "RelaisDesk_Technicien_Portable.exe", URL: "https://api.example.test/download", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42}},
	}
	payload, err := json.Marshal(releasePayload{Version: manifest.Version, PublishedAt: manifest.PublishedAt, KeyID: manifest.KeyID, Artifacts: manifest.Artifacts})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	if err := verifyReleaseManifest(manifest); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	manifest.Artifacts[0].Size++
	if err := verifyReleaseManifest(manifest); err == nil {
		t.Fatal("tampered update accepted")
	}
}

func TestCompareVersions(t *testing.T) {
	comparison, err := compareVersions("1.10.0", "1.9.9")
	if err != nil || comparison != 1 {
		t.Fatalf("comparison = %d, %v", comparison, err)
	}
}

func TestDefaultReleasePublicKeyIsValid(t *testing.T) {
	if len(RELEASE_PUBLIC_KEY) == 0 {
		t.Fatal("RELEASE_PUBLIC_KEY must not be empty by default")
	}
	key, err := base64.RawURLEncoding.DecodeString(RELEASE_PUBLIC_KEY)
	if err != nil {
		t.Fatalf("RELEASE_PUBLIC_KEY is not valid base64url: %v", err)
	}
	if len(key) != ed25519.PublicKeySize {
		t.Fatalf("expected public key length %d, got %d", ed25519.PublicKeySize, len(key))
	}
}

func TestLiveUpdateCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := checkForUpdate(ctx, "https://api.relaisdesk.fr")
	if err != nil {
		t.Skipf("checkForUpdate skipped (network unreachable or timeout): %v", err)
		return
	}
	if info == nil {
		t.Fatal("expected non-nil UpdateInfo")
	}
	if info.CurrentVersion != APP_VERSION {
		t.Fatalf("expected current version %s, got %s", APP_VERSION, info.CurrentVersion)
	}
}

func TestIsDeviceUpdateAvailable(t *testing.T) {
	// Baseline: target is 1.0.3
	if IsDeviceUpdateAvailable("", "1.0.3") {
		t.Errorf("empty version should NOT report update available")
	}
	if !IsDeviceUpdateAvailable("1.0.0", "1.0.3") {
		t.Errorf("1.0.0 vs 1.0.3 should report update available")
	}
	if !IsDeviceUpdateAvailable("1.0.2", "1.0.3") {
		t.Errorf("1.0.2 vs 1.0.3 should report update available")
	}
	if !IsDeviceUpdateAvailable("v1.0.2", "1.0.3") {
		t.Errorf("v1.0.2 vs 1.0.3 should report update available")
	}
	if IsDeviceUpdateAvailable("1.0.3", "1.0.3") {
		t.Errorf("1.0.3 vs 1.0.3 should NOT report update available")
	}
	if IsDeviceUpdateAvailable("v1.0.3", "1.0.3") {
		t.Errorf("v1.0.3 vs 1.0.3 should NOT report update available")
	}
	if IsDeviceUpdateAvailable("1.0.4", "1.0.3") {
		t.Errorf("1.0.4 vs 1.0.3 should NOT report update available")
	}
	if IsDeviceUpdateAvailable("1.1.0", "1.0.3") {
		t.Errorf("1.1.0 vs 1.0.3 should NOT report update available")
	}
	// Fallback to APP_VERSION
	if APP_VERSION == "1.0.0" {
		if IsDeviceUpdateAvailable("") {
			t.Errorf("empty vs default APP_VERSION should NOT report update available")
		}
		if !IsDeviceUpdateAvailable("0.9.9") {
			t.Errorf("0.9.9 vs default APP_VERSION should report update available")
		}
		if IsDeviceUpdateAvailable("1.0.0") {
			t.Errorf("1.0.0 vs default APP_VERSION should NOT report update available")
		}
	}
}



