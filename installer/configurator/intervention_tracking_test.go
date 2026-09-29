package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInterventionBridgeTracksOnlyLocalEngineEvents(t *testing.T) {
	var calls []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer technician-session" {
			t.Error("missing technician session")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" && !strings.Contains(string(body), "automatiquement") {
			t.Errorf("unexpected completion payload: %s", body)
		}
		calls = append(calls, r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	oldURL := APIURL
	APIURL = api.URL
	defer func() { APIURL = oldURL }()

	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge, err := startInterventionBridge("technician-session", tokenFile, "123456789", "INT-ABCDEF123456")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.stop()
	raw, err := os.ReadFile(bridge.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	send := func(secret, event string) int {
		req, err := http.NewRequest(http.MethodPost, binding.Endpoint, bytes.NewBufferString(`{"event":"`+event+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+secret)
		res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if got := send("wrong", "connected"); got != http.StatusForbidden {
		t.Fatalf("wrong local secret accepted: %d", got)
	}
	if got := send(binding.Secret, "connected"); got != http.StatusOK {
		t.Fatalf("connected event rejected: %d", got)
	}
	if len(calls) != 0 {
		t.Fatalf("connected event must not complete an intervention: %v", calls)
	}
	if got := send(binding.Secret, "closed"); got != http.StatusOK {
		t.Fatalf("closed event rejected: %d", got)
	}
	if len(calls) != 1 || calls[0] != "/api/v1/technician/interventions/INT-ABCDEF123456/complete" {
		t.Fatalf("unexpected completion calls: %v", calls)
	}
}

func TestInterventionBridgeCancelsWhenNoSessionWasEstablished(t *testing.T) {
	var path string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	oldURL := APIURL
	APIURL = api.URL
	defer func() { APIURL = oldURL }()
	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge, err := startInterventionBridge("technician-session", tokenFile, "987654321", "INT-ABCDEF654321")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.stop()
	raw, err := os.ReadFile(bridge.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, binding.Endpoint, bytes.NewBufferString(`{"event":"closed"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+binding.Secret)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unestablished closure rejected: %d", res.StatusCode)
	}
	if path != "/api/v1/technician/interventions/INT-ABCDEF654321/cancel" {
		t.Fatalf("expected cancellation, got %q", path)
	}
}

func TestInterventionBridgeCompletesConnectedSessionOnStop(t *testing.T) {
	var calls []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	oldURL := APIURL
	APIURL = api.URL
	defer func() { APIURL = oldURL }()

	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge, err := startInterventionBridge("technician-session", tokenFile, "111222333", "INT-STOPCOMPLETE")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(bridge.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, binding.Endpoint, bytes.NewBufferString(`{"event":"connected"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+binding.Secret)
	res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("connected event rejected: %d", res.StatusCode)
	}
	// The engine exits without delivering "closed" (or the application
	// quits): stopping the bridge must still complete the fiche.
	bridge.stop()
	if len(calls) != 1 || calls[0] != "/api/v1/technician/interventions/INT-STOPCOMPLETE/complete" {
		t.Fatalf("expected single completion on stop, got %v", calls)
	}
	logged, err := os.ReadFile(filepath.Join(filepath.Dir(bridge.path), "111222333.log"))
	if err != nil {
		t.Fatalf("missing bridge log: %v", err)
	}
	for _, want := range []string{"bridge started", "event connected", "stop: settled connected=true"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("bridge log missing %q:\n%s", want, logged)
		}
	}
}

func TestInterventionBridgeRebindsAfterFinalize(t *testing.T) {
	var calls []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	oldURL := APIURL
	APIURL = api.URL
	defer func() { APIURL = oldURL }()

	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := startInterventionBridge("technician-session", tokenFile, "555666777", "INT-REBINDFIRST")
	if err != nil {
		t.Fatal(err)
	}
	defer first.stop()
	raw, err := os.ReadFile(first.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	send := func(event string) int {
		req, err := http.NewRequest(http.MethodPost, binding.Endpoint, bytes.NewBufferString(`{"event":"`+event+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+binding.Secret)
		res, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if got := send("connected"); got != http.StatusOK {
		t.Fatalf("connected event rejected: %d", got)
	}
	if got := send("closed"); got != http.StatusOK {
		t.Fatalf("closed event rejected: %d", got)
	}
	// Reconnecting to the same peer reuses the settled bridge for a new fiche.
	second, err := startInterventionBridge("technician-session", tokenFile, "555666777", "INT-REBINDSECOND")
	if err != nil {
		t.Fatalf("rebind rejected: %v", err)
	}
	if second != first {
		t.Fatal("expected bridge reuse on rebind")
	}
	if got := send("connected"); got != http.StatusOK {
		t.Fatalf("second connected event rejected: %d", got)
	}
	if got := send("closed"); got != http.StatusOK {
		t.Fatalf("second closed event rejected: %d", got)
	}
	want := []string{
		"/api/v1/technician/interventions/INT-REBINDFIRST/complete",
		"/api/v1/technician/interventions/INT-REBINDSECOND/complete",
	}
	if len(calls) != len(want) {
		t.Fatalf("unexpected completion calls: %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("unexpected completion calls: %v", calls)
		}
	}
}

func TestInterventionBridgeRefusesRebindWhileActive(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge, err := startInterventionBridge("technician-session", tokenFile, "666777888", "INT-REBINDACTIVE")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.stop()
	if _, err := startInterventionBridge("technician-session", tokenFile, "666777888", "INT-REBINDOTHER"); err == nil || !strings.Contains(err.Error(), "déjà suivie") {
		t.Fatalf("expected already-tracked error, got %v", err)
	}
}

func TestInterventionBridgeCancelsUnestablishedSessionOnStop(t *testing.T) {
	var calls []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer api.Close()
	oldURL := APIURL
	APIURL = api.URL
	defer func() { APIURL = oldURL }()

	tokenFile := filepath.Join(t.TempDir(), "technician-network-token")
	if err := os.WriteFile(tokenFile, []byte("rd1.test.signature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bridge, err := startInterventionBridge("technician-session", tokenFile, "444555666", "INT-STOPCANCEL")
	if err != nil {
		t.Fatal(err)
	}
	bridge.stop()
	if len(calls) != 1 || calls[0] != "/api/v1/technician/interventions/INT-STOPCANCEL/cancel" {
		t.Fatalf("expected single cancellation on stop, got %v", calls)
	}
}
