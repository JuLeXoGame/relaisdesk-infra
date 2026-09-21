package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"
)

func TestFleetUpdateRejectsSignedDowngrades(t *testing.T) {
	for _, test := range []struct {
		current, requested, published string
		allowed                       bool
	}{
		{"1.0.5", "1.0.6", "1.0.6", true},
		{"1.0.9", "1.0.10", "1.0.10", true},
		{"1.0.5", "1.0.6", "1.0.4", false},
		{"1.0.5", "1.0.4", "1.0.4", false},
		{"1.0.5", "1.0.5", "1.0.5", false},
		{"2.0.0", "1.99.99", "1.99.99", false},
		{"1.0.5", "latest", "latest", false},
		{"unknown", "1.0.6", "1.0.6", false},
	} {
		if err := validateFleetUpdateVersion(test.current, test.requested, test.published); (err == nil) != test.allowed {
			t.Errorf("%+v: %v", test, err)
		}
	}
}

func TestVerifyReleaseManifest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubEncoded := base64.RawURLEncoding.EncodeToString(pub)

	manifest := releaseManifest{
		Version:     "1.0.5",
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		KeyID:       "test-key",
		Artifacts: []manifestArtifact{
			{Name: "RelaisDesk_Portable.exe", URL: "https://example.com/p.exe", SHA256: "aabbcc", Size: 100},
			{Name: "RelaisDesk_viewer.deb", URL: "https://example.com/v.deb", SHA256: "ddeeff", Size: 200},
		},
	}

	// Sign payload
	sortedArtifacts := make([]manifestArtifact, len(manifest.Artifacts))
	copy(sortedArtifacts, manifest.Artifacts)
	sort.Slice(sortedArtifacts, func(i, j int) bool { return sortedArtifacts[i].Name < sortedArtifacts[j].Name })
	payload, _ := json.Marshal(manifestPayload{
		Version:     manifest.Version,
		PublishedAt: manifest.PublishedAt,
		KeyID:       manifest.KeyID,
		Artifacts:   sortedArtifacts,
	})
	manifest.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, payload))

	// 1. Verify valid manifest
	if err := verifyReleaseManifest(&manifest, pubEncoded); err != nil {
		t.Fatalf("expected valid manifest, got: %v", err)
	}

	// 2. Verify with wrong public key
	wrongPub, _, _ := ed25519.GenerateKey(rand.Reader)
	wrongPubEncoded := base64.RawURLEncoding.EncodeToString(wrongPub)
	if err := verifyReleaseManifest(&manifest, wrongPubEncoded); err == nil {
		t.Fatal("expected error with wrong public key, got nil")
	}

	// 3. Verify with tampered artifact
	tampered := manifest
	tampered.Artifacts = []manifestArtifact{
		{Name: "RelaisDesk_Portable.exe", URL: "https://example.com/malicious.exe", SHA256: "badbad", Size: 100},
	}
	if err := verifyReleaseManifest(&tampered, pubEncoded); err == nil {
		t.Fatal("expected error with tampered artifacts, got nil")
	}
}

func TestDownloadAndVerifyArtifact(t *testing.T) {
	data := []byte("MZ...test-binary-content-windows...")
	hasher := sha256.New()
	hasher.Write(data)
	correctSHA := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	// 1. Success case
	downloaded, err := downloadAndVerifyArtifact(server.URL, correctSHA, 10000)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if !bytes.Equal(downloaded, data) {
		t.Fatalf("downloaded data does not match")
	}

	// 2. Hash mismatch case
	_, err = downloadAndVerifyArtifact(server.URL, "0000000000000000000000000000000000000000000000000000000000000000", 10000)
	if err == nil {
		t.Fatal("expected SHA mismatch error, got nil")
	}

	// 3. Size limit exceeded case
	_, err = downloadAndVerifyArtifact(server.URL, correctSHA, 10)
	if err == nil {
		t.Fatal("expected size limit exceeded error, got nil")
	}
}

func TestExtractBinaryFromDeb(t *testing.T) {
	elfContent := []byte("\x7fELF\x02\x01\x01\x00custom-elf-binary")

	// Create data.tar.gz containing ./usr/bin/relaisdesk-viewer
	var tarGzBuf bytes.Buffer
	gw := gzip.NewWriter(&tarGzBuf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{
		Name: "./usr/bin/relaisdesk-viewer",
		Mode: 0755,
		Size: int64(len(elfContent)),
	})
	_, _ = tw.Write(elfContent)
	_ = tw.Close()
	_ = gw.Close()

	dataTarGz := tarGzBuf.Bytes()

	// Assemble mock AR file
	var arBuf bytes.Buffer
	arBuf.WriteString("!<arch>\n")

	// control.tar.gz mock
	controlData := []byte("mock-control")
	fmt.Fprintf(&arBuf, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", "control.tar.gz", "0", "0", "0", "100644", len(controlData))
	arBuf.Write(controlData)
	if len(controlData)%2 != 0 {
		arBuf.WriteByte('\n')
	}

	// data.tar.gz mock
	fmt.Fprintf(&arBuf, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", "data.tar.gz", "0", "0", "0", "100644", len(dataTarGz))
	arBuf.Write(dataTarGz)
	if len(dataTarGz)%2 != 0 {
		arBuf.WriteByte('\n')
	}

	debBytes := arBuf.Bytes()

	// 1. Extract from valid .deb
	extracted, err := extractBinaryFromDeb(debBytes, "relaisdesk-viewer")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if !bytes.Equal(extracted, elfContent) {
		t.Fatalf("extracted content mismatch: got %q, want %q", extracted, elfContent)
	}

	// 2. Direct ELF binary should pass through
	passthrough, err := extractBinaryFromDeb(elfContent, "relaisdesk-viewer")
	if err != nil {
		t.Fatalf("passthrough failed: %v", err)
	}
	if !bytes.Equal(passthrough, elfContent) {
		t.Fatalf("passthrough mismatch")
	}

	// 3. Non-ELF non-AR should fail
	_, err = extractBinaryFromDeb([]byte("random-bytes"), "relaisdesk-viewer")
	if err == nil {
		t.Fatal("expected error for invalid archive, got nil")
	}
}
