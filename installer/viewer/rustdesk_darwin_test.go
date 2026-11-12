//go:build darwin

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDarwinGeneratedToml(t *testing.T) {
	content := generateViewerRustDesk2Toml(
		"api.relaisdesk.fr:21116",
		"api.relaisdesk.fr",
		"api.relaisdesk.fr",
		"qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74=",
		"/Users/test/Library/Application Support/RelaisDesk/network-token",
		"/Users/test/Library/Application Support/RelaisDesk/proof-key",
	)

	expected := []string{
		"rendezvous_server = 'api.relaisdesk.fr:21116'",
		"nat_type = 1",
		"serial = 0",
		"[options]",
		"custom-rendezvous-server = 'api.relaisdesk.fr'",
		"relay-server = 'api.relaisdesk.fr'",
		"key = 'qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74='",
		"stop-service = 'N'",
		"relaisdesk-token-file = '/Users/test/Library",
		"relaisdesk-proof-key-file = '/Users/test/Library",
	}
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("missing %q in generated config:\n%s", value, content)
		}
	}
}

func TestDarwinTemporaryViewerAnnouncement(t *testing.T) {
	for _, tc := range []struct {
		name      string
		confirmed bool
		status    int
		wantError string
	}{
		{"stale confirmation", false, http.StatusOK, "err_registration_timeout"},
		{"confirmed by server", true, http.StatusOK, ""},
		{"announcement rejected", true, http.StatusUnauthorized, "err_announce_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			previousConfigDir, previousAPI := RUSTDESK_CONFIG_DIR, APIURL
			RUSTDESK_CONFIG_DIR = dir
			t.Cleanup(func() { RUSTDESK_CONFIG_DIR, APIURL = previousConfigDir, previousAPI })
			config := filepath.Join(dir, RUSTDESK_CONFIG_FILE)
			if err := os.WriteFile(config, []byte("key_confirmed = true\n[keys_confirmed]\nold = true\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := resetRustDeskKeyConfirmed(dir); err != nil {
				t.Fatal(err)
			}

			engine := filepath.Join(dir, "test-engine")
			fakeEngine := `#!/bin/sh
case "$1" in
  --server)
    if [ -f "$0.confirm" ]; then
      printf 'key_confirmed = true\n' > "$0.next"
      mv "$0.next" "${0%/*}/RustDesk.toml"
    fi
    exec /bin/sleep 60
    ;;
  --get-id) printf '123456789\n' ;;
  *) exit 1 ;;
esac
`
			if err := os.WriteFile(engine, []byte(fakeEngine), 0700); err != nil {
				t.Fatal(err)
			}
			if tc.confirmed {
				if err := os.WriteFile(engine+".confirm", nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			announced := make(chan map[string]string, 1)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]string
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/viewer/announce" || json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Error("unexpected announcement request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				announced <- payload
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(api.Close)
			APIURL = api.URL
			if err := launchRustDesk(engine); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(terminateRustDesk)
			timeout := 3 * time.Second
			if !tc.confirmed {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err := awaitDarwinViewerAnnouncement(ctx, engine, "ABCD-1234", "device-key", 10*time.Millisecond)
			switch tc.wantError {
			case "":
				if err != nil {
					t.Fatal(err)
				}
			case "err_announce_id":
				if err == nil || err.Error() != TF(tc.wantError, "statut HTTP 401") {
					t.Fatalf("announcement error = %v", err)
				}
			default:
				if err == nil || err.Error() != T(tc.wantError) {
					t.Fatalf("registration error = %v", err)
				}
			}
			select {
			case payload := <-announced:
				if !tc.confirmed || payload["rustdesk_id"] != "123456789" || payload["code"] != "ABCD-1234" || payload["device_public_key"] != "device-key" {
					t.Fatalf("unexpected announcement: %v", payload)
				}
			default:
				if tc.confirmed {
					t.Fatal("confirmed viewer was not announced")
				}
			}
		})
	}
}

func TestDarwinParseNumericRustDeskID(t *testing.T) {
	out := "1248626563\n"
	if id := parseNumericRustDeskID(out); id != "1248626563" {
		t.Fatalf("expected 1248626563, got %q", id)
	}

	invalid := "error: no connection\n"
	if id := parseNumericRustDeskID(invalid); id != "" {
		t.Fatalf("expected empty for invalid output, got %q", id)
	}
}

func TestDarwinCandidateUsable(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "rustdesk")
	payload := []byte("fake-macos-binary")
	if err := os.WriteFile(good, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	pin := hex.EncodeToString(sum[:])
	cases := []struct {
		name      string
		path, sha string
		exists    bool
		override  bool
		want      bool
	}{
		{"matching pin wins anywhere", good, pin, true, false, true},
		{"wrong pin rejects", good, strings.Repeat("0", 64), true, false, false},
		{"missing file rejects", good, pin, false, false, false},
		{"/Applications allowed unpinned", "/Applications/RelaisDesk.app/Contents/MacOS/RelaisDesk", "", true, false, true},
		{"user path rejected unpinned", "/Users/moi/rustdesk", "", true, false, false},
		{"user path allowed with override", "/Users/moi/rustdesk", "", true, true, true},
		{"sibling of Applications rejected", "/ApplicationsX/rustdesk", "", true, false, false},
	}
	for _, tc := range cases {
		if got := darwinCandidateUsable(tc.path, tc.sha, tc.exists, tc.override); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
