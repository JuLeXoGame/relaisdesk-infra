package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fleetTestTransport func(*http.Request) (*http.Response, error)

func (fn fleetTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestFleetRejectsPlaceholderAndHTTPFailures(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	previousURL, previousTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = previousURL; http.DefaultTransport = previousTransport })
	APIURL = "https://audit.invalid"
	requests := 0
	http.DefaultTransport = fleetTestTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "audit.invalid" {
			t.Fatalf("unexpected target %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 404, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"revoked"}`))}, nil
	})
	if _, err := enrollFleet(context.Background(), "PERM-AUDT-0001", "AUTO-ID", key); err == nil {
		t.Fatal("placeholder accepted")
	}
	if requests != 0 {
		t.Fatal("invalid ID sent to network")
	}
	if _, err := refreshFleet(context.Background(), &fleetState{DeviceID: "DEV-AUDT-0001"}, key, true); err != errFleetRevoked {
		t.Fatalf("revocation ignored: %v", err)
	}
}
func TestFleetSignatureBindsEveryField(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	p, err := signFleetRequest(key, "enroll", "PERM-AUDT-0001", "123456789", "PC", "windows", false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal([]any{"relaisdesk-fleet-v1", "enroll", "PERM-AUDT-0001", "123456789", "PC", "windows", base64.RawURLEncoding.EncodeToString(pub), false, p.Timestamp, p.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(p.Signature)
	if !ed25519.Verify(pub, raw, sig) {
		t.Fatal("wire protocol signature mismatch")
	}
	p.Ready = true
	if ed25519.Verify(pub, fleetProofMessage("enroll", "PERM-AUDT-0001", "123456789", "PC", "windows", base64.RawURLEncoding.EncodeToString(pub), p), sig) {
		t.Fatal("mutable readiness outside signature")
	}
}
