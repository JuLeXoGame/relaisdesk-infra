package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBankTransferConfigured(t *testing.T) {
	valid := &Config{
		BankIBAN:   "FR76 3000 6000 0112 3456 7890 189",
		BankBIC:    "AGRIFRPP",
		BankHolder: "RelaisDesk",
	}
	if !valid.BankTransferConfigured() {
		t.Fatal("valid bank details rejected")
	}

	tests := []Config{
		{BankIBAN: "FR76 0000 0000 0000 0000 0000 000", BankBIC: "BNPAFRPP", BankHolder: "RelaisDesk"},
		{BankIBAN: "FR76 3000 6000 0112 3456 7890 180", BankBIC: "AGRIFRPP", BankHolder: "RelaisDesk"},
		{BankIBAN: valid.BankIBAN, BankBIC: "invalid", BankHolder: "RelaisDesk"},
		{BankIBAN: valid.BankIBAN, BankBIC: valid.BankBIC, BankHolder: ""},
	}
	for _, cfg := range tests {
		if cfg.BankTransferConfigured() {
			t.Fatalf("invalid bank details accepted: %#v", cfg)
		}
	}
}

func TestValidateServerSettings(t *testing.T) {
	cfg := &Config{
		APIPort:                   "8443",
		ServerIP:                  "api.relaisdesk.fr",
		RendezvousPort:            21116,
		RelayPort:                 21117,
		NetworkAuthPrivateKeyFile: "/run/secrets/relaisdesk-network-auth",
		NetworkAuthKeyID:          "relaisdesk-1",
		NetworkTokenTTLSeconds:    300,
		ClientSourceURL:           "https://github.com/example/relaisdesk-client",
		ServerSourceURL:           "https://github.com/example/relaisdesk-server",
		LegalPhone:                "+33 4 00 00 00 00",
		PublicWebsiteURL:          "https://relaisdesk.example",
		StripeSuccessURL:          "https://relaisdesk.example/commande?result=success",
		StripeCancelURL:           "https://relaisdesk.example/commande?result=cancel",
		ReleaseManifestPath:       "/opt/relaisdesk/downloads/release-manifest.json",
		ReleasePublicKey:          base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
	}
	if err := cfg.ValidateServerSettings(); err != nil {
		t.Fatalf("valid server settings rejected: %v", err)
	}

	cfg.APIBind = "0.0.0.0"
	if err := cfg.ValidateServerSettings(); err == nil {
		t.Fatal("public API bind accepted in production")
	}
	cfg.APIBind = "127.0.0.1"

	invalidHosts := []string{"", "https://api.example.fr", "api.example.fr:21116", "api.example.fr'\nkey='evil", "-api.example.fr"}
	for _, host := range invalidHosts {
		cfg.ServerIP = host
		if err := cfg.ValidateServerSettings(); err == nil {
			t.Fatalf("invalid server host accepted: %q", host)
		}
	}

	cfg.ServerIP = "api.relaisdesk.fr"
	cfg.RendezvousPort = 0
	if err := cfg.ValidateServerSettings(); err == nil {
		t.Fatal("invalid rendezvous port accepted")
	}

	cfg.RendezvousPort = 21116
	cfg.StripeSuccessURL = "https://phishing.example/commande"
	if err := cfg.ValidateServerSettings(); err == nil {
		t.Fatal("cross-origin Stripe return URL accepted")
	}
}

func TestSourceDistributionReadyRequiresTwoHTTPSURLs(t *testing.T) {
	cfg := &Config{ClientSourceURL: "https://example.test/client", ServerSourceURL: "https://example.test/server"}
	if !cfg.SourceDistributionReady() {
		t.Fatal("valid public source URLs rejected")
	}
	cfg.ServerSourceURL = "http://example.test/server"
	if cfg.SourceDistributionReady() {
		t.Fatal("non-HTTPS source URL accepted")
	}
}

func TestProductionEmailAuthenticationSettings(t *testing.T) {
	cfg := &Config{
		APIPort:                   "8443",
		ServerIP:                  "api.relaisdesk.fr",
		RendezvousPort:            21116,
		RelayPort:                 21117,
		NetworkAuthPrivateKeyFile: "/run/secrets/relaisdesk-network-auth",
		NetworkAuthKeyID:          "relaisdesk-1",
		NetworkTokenTTLSeconds:    300,
		ClientSourceURL:           "https://github.com/example/relaisdesk-client",
		ServerSourceURL:           "https://github.com/example/relaisdesk-server",
		LegalPhone:                "+33 4 00 00 00 00",
		PublicWebsiteURL:          "https://relaisdesk.example",
		StripeSuccessURL:          "https://relaisdesk.example/commande?result=success",
		StripeCancelURL:           "https://relaisdesk.example/commande?result=cancel",
		ReleaseManifestPath:       "/opt/relaisdesk/downloads/release-manifest.json",
		ReleasePublicKey:          base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32))),
		SMTPHost:                  "smtp.example.test",
		SMTPFrom:                  "RelaisDesk <contact@relaisdesk.fr>",
		EmailDomain:               "relaisdesk.fr",
		SMTPDKIMSelector:          "mail-2026",
		EmailRequireDNSAuth:       true,
	}
	if err := cfg.ValidateServerSettings(); err != nil {
		t.Fatalf("valid authenticated e-mail configuration rejected: %v", err)
	}

	cfg.SMTPFrom = "RelaisDesk <contact@example.net>"
	if err := cfg.ValidateServerSettings(); err == nil {
		t.Fatal("cross-domain SMTP_FROM accepted")
	}
	cfg.SMTPFrom = "RelaisDesk <contact@relaisdesk.fr>"
	cfg.SMTPDKIMSelector = "selector with spaces"
	if err := cfg.ValidateServerSettings(); err == nil {
		t.Fatal("invalid DKIM selector accepted")
	}
}

func TestB2CSalesRemainClosedUntilLegalDetailsAreComplete(t *testing.T) {
	cfg := &Config{B2CSalesEnabled: true}
	if cfg.B2CSalesReady() {
		t.Fatal("ventes B2C ouvertes sans mentions obligatoires")
	}
	cfg.LegalPhone = "+33 4 00 00 00 00"
	cfg.ConsumerMediatorName = "Médiateur de test"
	cfg.ConsumerMediatorURL = "https://mediateur.example.test"
	if !cfg.B2CSalesReady() {
		t.Fatal("coordonnées B2C complètes rejetées")
	}
	cfg.ConsumerMediatorURL = "http://mediateur.example.test"
	if cfg.B2CSalesReady() {
		t.Fatal("URL non HTTPS du médiateur acceptée")
	}
}
