package networkauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIssueProducesVerifiableBoundToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, devicePrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(privateKey, "test-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Unix(1_800_000_000, 0).UTC()
	signer.now = func() time.Time { return fixedNow }
	devicePublicKey := devicePrivateKey.Public().(ed25519.PublicKey)
	issued, err := signer.Issue(IssueRequest{
		Subject:         "LIC-123",
		Tenant:          "LIC-123",
		Role:            "technician",
		DevicePublicKey: base64.RawURLEncoding.EncodeToString(devicePublicKey),
		MaxSessions:     3,
		EntitlementEnds: fixedNow.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(issued.Token, ".")
	if len(parts) != 3 || parts[0] != TokenPrefix {
		t.Fatalf("unexpected token format: %q", issued.Token)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		t.Fatal("token signature is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.DevicePublicKey != base64.RawURLEncoding.EncodeToString(devicePublicKey) || claims.MaxSessions != 3 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestNormalizeDevicePublicKeyRejectsWrongSize(t *testing.T) {
	if _, err := NormalizeDevicePublicKey(base64.RawURLEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected an invalid key length error")
	}
}
