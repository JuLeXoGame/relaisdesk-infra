package main

import (
	"testing"
	"time"
)

const testRustDeskPublicKey = "qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74="

func TestRustDeskConfigurationValidation(t *testing.T) {
	for _, host := range []string{"api.relaisdesk.fr", "203.0.113.10", "2001:db8::1"} {
		if err := validateRendezvousHost(host); err != nil {
			t.Fatalf("valid host %q rejected: %v", host, err)
		}
	}
	for _, host := range []string{"", "bad host", "server.example'\nkey = 'attacker", "https://example.com", "-invalid.example"} {
		if err := validateRendezvousHost(host); err == nil {
			t.Fatalf("unsafe host %q accepted", host)
		}
	}
	for _, key := range []string{"", "too-short", testRustDeskPublicKey + "\nrelay-server = 'attacker'", "'" + testRustDeskPublicKey} {
		if err := validateRustDeskPublicKey(key); err == nil {
			t.Fatalf("unsafe public key %q accepted", key)
		}
	}
	if err := validateRustDeskPublicKey(testRustDeskPublicKey); err != nil {
		t.Fatalf("valid public key rejected: %v", err)
	}
}

func TestCorruptedRustDeskConfigDetectionIsTargeted(t *testing.T) {
	if !isCorruptedRustDeskConfig([]byte("key_pair = [[], []]\nkey_confirmed = true\n")) {
		t.Fatal("empty key pair was not detected")
	}
	if isCorruptedRustDeskConfig([]byte("key_pair = [[1], [2]]\nkey_confirmed = false\n")) {
		t.Fatal("unconfirmed key must not be treated as corrupted")
	}
	if isCorruptedRustDeskConfig([]byte("allowed_values = [[], [1]]\nkey_pair = [[1], [2]]\nkey_confirmed = true\n")) {
		t.Fatal("an unrelated empty array caused a valid configuration to be classified as corrupted")
	}
}

func TestValidateActivationResponse(t *testing.T) {
	valid := &ActivationResponse{
		ServerIP:       "203.0.113.10",
		RendezvousPort: 21116,
		RelayPort:      21117,
		PublicKey:      testRustDeskPublicKey,
		ExpiresAt:      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	if err := validateActivationResponse(valid); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}

	invalid := *valid
	invalid.PublicKey += "\nrelay-server = 'attacker'"
	if err := validateActivationResponse(&invalid); err == nil {
		t.Fatal("malicious response accepted")
	}

	expired := *valid
	expired.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if err := validateActivationResponse(&expired); err == nil {
		t.Fatal("expired response accepted")
	}
}
