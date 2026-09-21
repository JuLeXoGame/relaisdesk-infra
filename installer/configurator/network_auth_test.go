package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProofKeyPersistsAndHasExpectedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof-key")
	first, err := loadOrCreateProofKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateProofKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("proof key changed between reads")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(string(bytes.TrimSpace(encoded)))
	if err != nil || !bytes.Equal(decoded, first) {
		t.Fatal("stored proof key is not canonical base64url")
	}
}

func TestValidateNetworkTokenResponse(t *testing.T) {
	response := &NetworkTokenResponse{
		Valid:        true,
		NetworkToken: "rd1.payload.signature",
		ExpiresAt:    time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339),
	}
	if _, err := validateNetworkTokenResponse(response); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	response.NetworkToken = "copied-config"
	if _, err := validateNetworkTokenResponse(response); err == nil {
		t.Fatal("malformed network token accepted")
	}
}
