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
	"sync"
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
	raw, err := os.ReadFile(bridge.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	// A live session protects the bridge: another fiche, or a duplicate
	// launch of the same one, must be refused while connected.
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
	for _, id := range []string{"INT-REBINDOTHER", "INT-REBINDACTIVE"} {
		if _, err := startInterventionBridge("technician-session", tokenFile, "666777888", id); err == nil || !strings.Contains(err.Error(), "déjà suivie") {
			t.Fatalf("expected already-tracked error for %s, got %v", id, err)
		}
	}
}

func waitForBridgeCondition(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func sendBridgeEvent(t *testing.T, binding interventionBinding, event string) int {
	t.Helper()
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

func readBridgeBinding(t *testing.T, bridge *interventionBridge) interventionBinding {
	t.Helper()
	raw, err := os.ReadFile(bridge.path)
	if err != nil {
		t.Fatal(err)
	}
	var binding interventionBinding
	if err = json.Unmarshal(raw, &binding); err != nil {
		t.Fatal(err)
	}
	return binding
}

// Retrying a failed launch reuses the same fiche (the device endpoint
// deduplicates in-progress fiches within 5 minutes): the unsettled bridge
// must accept it without cancelling its own fiche.
func TestInterventionBridgeAllowsSameFicheRetryWhileUnconnected(t *testing.T) {
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
	first, err := startInterventionBridge("technician-session", tokenFile, "777888999", "INT-RETRYSAME")
	if err != nil {
		t.Fatal(err)
	}
	defer first.stop()
	second, err := startInterventionBridge("technician-session", tokenFile, "777888999", "INT-RETRYSAME")
	if err != nil {
		t.Fatalf("same-fiche retry rejected: %v", err)
	}
	if second != first {
		t.Fatal("expected bridge reuse on same-fiche retry")
	}
	if len(calls) != 0 {
		t.Fatalf("same-fiche retry must not settle anything: %v", calls)
	}
	binding := readBridgeBinding(t, first)
	if got := sendBridgeEvent(t, binding, "connected"); got != http.StatusOK {
		t.Fatalf("connected event rejected: %d", got)
	}
	if got := sendBridgeEvent(t, binding, "closed"); got != http.StatusOK {
		t.Fatalf("closed event rejected: %d", got)
	}
	if len(calls) != 1 || calls[0] != "/api/v1/technician/interventions/INT-RETRYSAME/complete" {
		t.Fatalf("unexpected completion calls: %v", calls)
	}
}

// A bridge that never reported "connected" holds no live session (the engine
// reports nothing for sessions that never connect): rebinding to another
// fiche evicts the abandoned one instead of blocking the retry forever.
func TestInterventionBridgeEvictsUnconnectedBridgeOnRebind(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
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
	first, err := startInterventionBridge("technician-session", tokenFile, "888999000", "INT-EVICTOLD")
	if err != nil {
		t.Fatal(err)
	}
	defer first.stop()
	second, err := startInterventionBridge("technician-session", tokenFile, "888999000", "INT-EVICTNEW")
	if err != nil {
		t.Fatalf("rebind of unconnected bridge rejected: %v", err)
	}
	if second != first {
		t.Fatal("expected bridge reuse on evicting rebind")
	}
	logPath := filepath.Join(filepath.Dir(first.path), "888999000.log")
	waitForBridgeCondition(t, "eviction cancel of INT-EVICTOLD", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calls) == 1 && calls[0] == "/api/v1/technician/interventions/INT-EVICTOLD/cancel"
	})
	waitForBridgeCondition(t, "eviction log confirmation", func() bool {
		logged, err := os.ReadFile(logPath)
		return err == nil && strings.Contains(string(logged), "rebind evict: cancelled INT-EVICTOLD")
	})
	binding := readBridgeBinding(t, first)
	if got := sendBridgeEvent(t, binding, "connected"); got != http.StatusOK {
		t.Fatalf("connected event rejected: %d", got)
	}
	if got := sendBridgeEvent(t, binding, "closed"); got != http.StatusOK {
		t.Fatalf("closed event rejected: %d", got)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"/api/v1/technician/interventions/INT-EVICTOLD/cancel",
		"/api/v1/technician/interventions/INT-EVICTNEW/complete",
	}
	if len(calls) != len(want) {
		t.Fatalf("unexpected settlement calls: %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("unexpected settlement calls: %v", calls)
		}
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
