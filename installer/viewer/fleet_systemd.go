package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const linuxFleetDir = "/var/lib/relaisdesk-fleet"
const linuxFleetRunDir = "/run/relaisdesk-fleet"
const linuxFleetSocket = linuxFleetRunDir + "/proof.sock"
const linuxFleetUnitPath = "/etc/systemd/system/relaisdesk-fleet.service"
const linuxFleetDropinPath = "/etc/systemd/system/rustdesk.service.d/90-relaisdesk-fleet.conf"
const linuxFleetProtocol = "relaisdesk-fleet-proof-v1"

const linuxFleetUnit = `# Managed by RelaisDesk Viewer --enroll
[Unit]
Description=RelaisDesk permanent device authorization
Wants=network-online.target
After=network-online.target
Before=rustdesk.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=root
Group=root
ExecStart=/var/lib/relaisdesk-fleet/viewer-agent --fleet-service
Restart=on-failure
RestartSec=10
TimeoutStopSec=45
UMask=0077
RuntimeDirectory=relaisdesk-fleet
RuntimeDirectoryMode=0755
Environment=HOME=/root XDG_CONFIG_HOME=/root/.config
WorkingDirectory=/var/lib/relaisdesk-fleet
PrivateTmp=yes
ProtectSystem=full
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6

[Install]
WantedBy=multi-user.target
`

// RustDesk may only run while the authorization service is alive. Do not inherit
// the upstream broad pkill command, which would affect unrelated RustDesk tasks.
const linuxFleetDropin = `# Managed by RelaisDesk Viewer --enroll
[Unit]
BindsTo=relaisdesk-fleet.service
After=relaisdesk-fleet.service

[Service]
ExecStart=
ExecStart=/usr/bin/rustdesk --service
ExecStartPre=/var/lib/relaisdesk-fleet/viewer-agent --fleet-check
ExecStop=
KillMode=control-group
Environment=HOME=/root XDG_CONFIG_HOME=/root/.config
`

func linuxFleetConfig(previous []byte, a ActivationResponse) ([]byte, error) {
	if err := validateRendezvousHost(a.ServerIP); err != nil {
		return nil, err
	}
	if err := validateRendezvousPort(a.RendezvousPort); err != nil {
		return nil, err
	}
	if err := validateRendezvousPort(a.RelayPort); err != nil {
		return nil, err
	}
	if err := validateRustDeskPublicKey(a.PublicKey); err != nil {
		return nil, err
	}
	var cfg map[string]any
	if _, err := toml.Decode(string(previous), &cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	opts, ok := cfg["options"].(map[string]any)
	if !ok && cfg["options"] != nil {
		return nil, errors.New("options RustDesk invalides")
	}
	if opts == nil {
		opts = make(map[string]any)
	}
	cfg["options"] = opts
	cfg["rendezvous_server"] = net.JoinHostPort(a.ServerIP, fmt.Sprint(a.RendezvousPort))
	relay := a.ServerIP
	if a.RelayPort != 21117 {
		relay = net.JoinHostPort(a.ServerIP, fmt.Sprint(a.RelayPort))
	}
	delete(opts, "relaisdesk-token-file")
	delete(opts, "relaisdesk-proof-key-file")
	for k, v := range map[string]string{
		"custom-rendezvous-server": a.ServerIP, "relay-server": relay,
		"api-server": APIURL, "key": a.PublicKey, "stop-service": "N",
		"allow-websocket": "N", "verification-method": "use-permanent-password",
		"approve-mode": "password", "allow-hide-cm": "N",
		"relaisdesk-proof-socket": linuxFleetSocket,
	} {
		opts[k] = v
	}
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(cfg)
	return buf.Bytes(), err
}

type fleetSocketRequest struct {
	Protocol string `json:"protocol"`
	Action   string `json:"action"`
}
type fleetSocketProof struct {
	Token     string `json:"token"`
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

// The token is read only from the root-owned authorization directory. The hbbs/hbbr
// still verify its issuer signature; this local check prevents signing expired tokens.
func fleetTokenLive(token string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "rd1" || len(token) > 8192 {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Expires int64 `json:"exp"`
	}
	return err == nil && json.Unmarshal(b, &claims) == nil && claims.Expires > now.Unix()
}

func makeFleetSocketProof(req fleetSocketRequest, token string, key ed25519.PrivateKey, now time.Time) (*fleetSocketProof, error) {
	if req.Protocol != linuxFleetProtocol || len(req.Action) == 0 || len(req.Action) > 512 ||
		strings.ContainsAny(req.Action, "\r\n\x00") || len(key) != ed25519.PrivateKeySize || !fleetTokenLive(token, now) {
		return nil, errors.New("demande de preuve refusée")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	p := &fleetSocketProof{Token: token, Timestamp: now.Unix(), Nonce: hex.EncodeToString(nonce)}
	message := fmt.Sprintf("relaisdesk-proof-v1\n%s\n%d\n%s\n%s", req.Action, p.Timestamp, p.Nonce, token)
	p.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(message)))
	return p, nil
}
