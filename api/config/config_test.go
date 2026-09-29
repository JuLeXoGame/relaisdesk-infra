package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
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

func TestCryptoAssetConfigured(t *testing.T) {
	valid := &Config{
		CryptoEnabled:        true,
		OKXBaseURL:           "https://www.okx.com",
		OKXAPIKey:            "test-key",
		OKXAPISecret:         "test-secret",
		OKXAPIPassphrase:     "test-passphrase",
		OKXBTCDepositAddress: "bc1qoperator",
		OKXXRPDepositAddress: "rOperatorXRPAddress",
		OKXXRPDepositTag:     "123456",
	}
	if !valid.CryptoAssetConfigured("BTC") || !valid.CryptoAssetConfigured("xrp") {
		t.Fatal("configured crypto assets rejected")
	}
	if valid.CryptoAssetConfigured("ETH") {
		t.Fatal("unknown crypto asset accepted")
	}

	cases := []struct {
		name  string
		apply func(*Config)
		asset string
	}{
		{"globally disabled", func(c *Config) { c.CryptoEnabled = false }, "BTC"},
		{"missing rate source", func(c *Config) { c.OKXBaseURL = "" }, "XRP"},
		{"missing api key", func(c *Config) { c.OKXAPIKey = "" }, "BTC"},
		{"missing api secret", func(c *Config) { c.OKXAPISecret = "" }, "XRP"},
		{"missing api passphrase", func(c *Config) { c.OKXAPIPassphrase = "" }, "BTC"},
		{"missing btc address", func(c *Config) { c.OKXBTCDepositAddress = "" }, "BTC"},
		{"missing xrp address", func(c *Config) { c.OKXXRPDepositAddress = "" }, "XRP"},
		{"missing xrp tag", func(c *Config) { c.OKXXRPDepositTag = "" }, "XRP"},
		{"malformed xrp tag", func(c *Config) { c.OKXXRPDepositTag = "abc" }, "XRP"},
	}
	for _, tc := range cases {
		cfg := *valid
		tc.apply(&cfg)
		if cfg.CryptoAssetConfigured(tc.asset) {
			t.Fatalf("%s accepted for %s", tc.name, tc.asset)
		}
	}
	// Assets are independent: XRP stays available without the BTC address.
	btcDown := *valid
	btcDown.OKXBTCDepositAddress = ""
	if !btcDown.CryptoAssetConfigured("XRP") || btcDown.CryptoAssetConfigured("BTC") {
		t.Fatal("asset independence broken")
	}
	if tag, err := valid.OKXXRPDepositTagValue(); err != nil || tag != 123456 {
		t.Fatalf("tag value = %d, %v", tag, err)
	}
}

func TestCryptoQuoteTTLClamped(t *testing.T) {
	if got := (&Config{CryptoQuoteTTLMinutes: 0}).CryptoQuoteTTL(); got != 5*time.Minute {
		t.Fatalf("zero TTL = %v", got)
	}
	if got := (&Config{CryptoQuoteTTLMinutes: 120}).CryptoQuoteTTL(); got != 30*time.Minute {
		t.Fatalf("oversized TTL = %v", got)
	}
	if got := (&Config{CryptoQuoteTTLMinutes: 15}).CryptoQuoteTTL(); got != 15*time.Minute {
		t.Fatalf("valid TTL = %v", got)
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
