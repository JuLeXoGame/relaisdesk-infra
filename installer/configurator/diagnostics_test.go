package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConnectivityDiagnosticsSuccessAndPrivacy(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer api.Close()

	rendezvous := listenForDiagnosticTest(t)
	defer rendezvous.Close()
	relay := listenForDiagnosticTest(t)
	defer relay.Close()

	activation := &ActivationResponse{
		ServerIP:       "127.0.0.1",
		RendezvousPort: rendezvous.Addr().(*net.TCPAddr).Port,
		RelayPort:      relay.Addr().(*net.TCPAddr).Port,
		PublicKey:      strings.Repeat("A", 43),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report := runConnectivityDiagnostics(ctx, api.URL, activation)
	if !report.OK() {
		t.Fatalf("diagnostic should succeed: %s", report.String())
	}
	text := report.String()
	for _, secret := range []string{"license_key", "network_token", "private_key"} {
		if strings.Contains(text, secret) {
			t.Fatalf("diagnostic report leaks %q", secret)
		}
	}
}

func TestConnectivityDiagnosticsFailsClosedWithoutConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	report := runConnectivityDiagnostics(ctx, "://bad", nil)
	if report.OK() {
		t.Fatal("diagnostic without server configuration should fail")
	}
}

func listenForDiagnosticTest(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}
