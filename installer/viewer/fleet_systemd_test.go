package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func TestLinuxFleetConfigPreservesSettingsAndNeverExportsKeyPaths(t *testing.T) {
	a := ActivationResponse{ServerIP: "2001:db8::1", RendezvousPort: 21116, RelayPort: 21117, PublicKey: strings.Repeat("a", 43)}
	b, err := linuxFleetConfig([]byte("serial = 17\n[options]\nenable-file-transfer = 'N'\napprove-mode = 'click'\nallow-hide-cm = 'Y'\nrelaisdesk-proof-key-file = '/private'\nrelaisdesk-token-file = '/old'\n"), a)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Serial     int
		Rendezvous string `toml:"rendezvous_server"`
		Options    map[string]string
	}
	if _, err = toml.Decode(string(b), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Serial != 17 || cfg.Options["enable-file-transfer"] != "N" || cfg.Rendezvous != "[2001:db8::1]:21116" {
		t.Fatalf("settings lost: %+v", cfg)
	}
	if cfg.Options["relaisdesk-proof-socket"] != linuxFleetSocket || cfg.Options["verification-method"] != "use-permanent-password" {
		t.Fatal("missing secure options")
	}
	if cfg.Options["approve-mode"] != "password" || cfg.Options["allow-hide-cm"] != "N" {
		t.Fatal("permanent password access or visible connection manager not enforced")
	}
	if bytes.Contains(b, []byte("/private")) || bytes.Contains(b, []byte("/old")) {
		t.Fatal("private paths exported to desktop config")
	}
	if cfg.Options["relaisdesk-token-file"] != "" || cfg.Options["relaisdesk-proof-key-file"] != "" {
		t.Fatal("desktop configuration must use the broker, not private files")
	}
	a.ServerIP = "host'\n[malicious]"
	if _, err = linuxFleetConfig(nil, a); err == nil {
		t.Fatal("TOML injection accepted")
	}
	if _, err = linuxFleetConfig([]byte("malformed = ["), ActivationResponse{ServerIP: "test.invalid", RendezvousPort: 1, RelayPort: 2, PublicKey: strings.Repeat("a", 43)}); err == nil {
		t.Fatal("corrupt user config overwritten")
	}
}

func fleetTestToken(now time.Time) string {
	b, _ := json.Marshal(map[string]any{"exp": now.Unix()})
	return "rd1." + base64.RawURLEncoding.EncodeToString(b) + ".test-signature"
}
func TestLinuxFleetSocketProofProtocol(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	req := fleetSocketRequest{Protocol: linuxFleetProtocol, Action: "register:123456789"}
	token := fleetTestToken(now.Add(time.Minute))
	p, err := makeFleetSocketProof(req, token, key, now)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(p.Signature)
	if err != nil {
		t.Fatal(err)
	}
	msg := fmt.Sprintf("relaisdesk-proof-v1\n%s\n%d\n%s\n%s", req.Action, p.Timestamp, p.Nonce, p.Token)
	if !ed25519.Verify(pub, []byte(msg), sig) {
		t.Fatal("RustDesk proof protocol mismatch")
	}
	if ed25519.Verify(pub, []byte(strings.Replace(msg, "register:", "punch:", 1)), sig) {
		t.Fatal("signature does not bind action")
	}
	other, _ := makeFleetSocketProof(req, token, key, now)
	if p.Nonce == other.Nonce {
		t.Fatal("nonce reused")
	}
	for _, invalid := range []string{fleetTestToken(now), fleetTestToken(now.Add(-time.Second)), "rd1.not-json.sig", ""} {
		if _, err := makeFleetSocketProof(req, invalid, key, now); err == nil {
			t.Fatalf("invalid/expired token accepted: %s", invalid)
		}
	}
	for _, action := range []string{"", "register:x\nother", "nul\x00", strings.Repeat("a", 513)} {
		req.Action = action
		if _, err := makeFleetSocketProof(req, token, key, now); err == nil {
			t.Fatal("invalid action accepted")
		}
	}
	req = fleetSocketRequest{Protocol: "wrong", Action: "register:123456789"}
	if _, err := makeFleetSocketProof(req, token, key, now); err == nil {
		t.Fatal("unknown protocol accepted")
	}
}

func TestLinuxFleetUnitsFailClosedAndContainNoCredentials(t *testing.T) {
	for _, s := range []string{"BindsTo=relaisdesk-fleet.service", "After=relaisdesk-fleet.service", "--fleet-check", "KillMode=control-group", "ExecStop=\n"} {
		if !strings.Contains(linuxFleetDropin, s) {
			t.Fatalf("missing %q", s)
		}
	}
	for _, s := range []string{"User=root", "UMask=0077", "Restart=on-failure", "RuntimeDirectoryMode=0755", "WantedBy=multi-user.target"} {
		if !strings.Contains(linuxFleetUnit, s) {
			t.Fatalf("missing %q", s)
		}
	}
	for _, forbidden := range []string{"PERM-", "--password", "pkill", "proof-key", "network-token"} {
		if strings.Contains(linuxFleetUnit+linuxFleetDropin, forbidden) {
			t.Fatalf("unsafe unit: %s", forbidden)
		}
	}
}

func TestFleetAgentLifecycle(t *testing.T) {
	for _, scenario := range []string{"revoked", "offline-at-start", "start-failed", "configure-failed", "cancel", "missing-key"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			if err := saveFleetState(dir, &fleetState{DeviceID: "DEV-TEST-1234", RustDeskID: "123456789"}); err != nil {
				t.Fatal(err)
			}
			if _, err := loadOrCreateViewerProofKey(filepath.Join(dir, "proof-key")); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-key" {
				if err := os.Remove(filepath.Join(dir, "proof-key")); err != nil {
					t.Fatal(err)
				}
			}
			_ = os.WriteFile(filepath.Join(dir, "network-token"), []byte("stale-token"), 0600)
			_ = os.WriteFile(filepath.Join(dir, "ready"), []byte("stale-ready"), 0600)
			oldURL, oldTransport := APIURL, http.DefaultTransport
			t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
			APIURL = "https://fleet-test.invalid"
			requests, starts, stops, waits := 0, 0, 0, 0
			running := false
			http.DefaultTransport = fleetTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "fleet-test.invalid" {
					t.Fatal("external request")
				}
				requests++
				if scenario == "offline-at-start" {
					return nil, errors.New("offline")
				}
				if scenario == "revoked" && requests > 1 {
					return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
				}
				b, _ := json.Marshal(DeviceEnrollResponse{Valid: true, NetworkToken: fleetTestToken(time.Now().Add(time.Minute)), ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339)})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
			})
			hooks := fleetAgentHooks{
				configure: func(*fleetState) error {
					if scenario == "configure-failed" {
						return errors.New("configuration error")
					}
					return nil
				},
				running: func() bool { return running },
				start: func() error {
					starts++
					if scenario == "start-failed" {
						return errors.New("start error")
					}
					running = true
					return nil
				},
				stop: func() error { stops++; running = false; return nil },
				wait: func(context.Context, time.Duration) bool { waits++; return scenario == "revoked" && waits < 2 },
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel" {
				cancel()
			}
			err := runFleetAgentWithHooks(ctx, dir, hooks)
			if (scenario == "start-failed" || scenario == "configure-failed" || scenario == "missing-key") != (err != nil) {
				t.Fatalf("unexpected error: %v", err)
			}
			if stops == 0 || running {
				t.Fatal("service not stopped on exit")
			}
			if _, err := os.Stat(filepath.Join(dir, "network-token")); !os.IsNotExist(err) {
				t.Fatal("authorization left behind")
			}
			if _, err := os.Stat(filepath.Join(dir, "ready")); !os.IsNotExist(err) {
				t.Fatal("stale ready marker")
			}
			if scenario == "offline-at-start" && starts != 0 {
				t.Fatal("offline service started using stale authorization")
			}
			if scenario == "revoked" && (requests != 2 || starts != 1 || stops < 2) {
				t.Fatal("revocation ignored")
			}
			if scenario == "missing-key" {
				if _, err := os.Lstat(filepath.Join(dir, "proof-key")); !os.IsNotExist(err) {
					t.Fatal("missing identity was regenerated")
				}
				if requests != 0 {
					t.Fatal("missing identity contacted API")
				}
			}
			if scenario == "cancel" && (starts != 0 || requests != 0) {
				t.Fatal("cancelled service contacted API or started RustDesk")
			}
		})
	}
}
