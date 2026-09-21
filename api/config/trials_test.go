package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTrialsRequireSeparateKeyStripeAndUsableSMTP(t *testing.T) {
	good := Config{TrialsEnabled: true, TrialFingerprintKey: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32))), StripeSecretKey: "sk_test_fixture", StripeWebhookSecret: "whsec_fixture", SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPFrom: "test@example.com"}
	if !good.TrialsReady() {
		t.Fatal("valid config rejected")
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.TrialsEnabled = false }, func(c *Config) { c.TrialFingerprintKey = "" }, func(c *Config) { c.TrialFingerprintKey = "not-a-key" },
		func(c *Config) { c.StripeSecretKey = "" }, func(c *Config) { c.StripeWebhookSecret = "" }, func(c *Config) { c.SMTPHost = "" }, func(c *Config) { c.SMTPHost = "https://smtp.example.com" }, func(c *Config) { c.SMTPPort = 25 }, func(c *Config) { c.SMTPFrom = "invalid" }, func(c *Config) { c.SMTPUser = "user"; c.SMTPPass = "" },
	} {
		test := good
		mutate(&test)
		if test.TrialsReady() {
			t.Fatal("incomplete trial configuration enabled")
		}
	}
}
