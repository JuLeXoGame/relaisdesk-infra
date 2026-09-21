package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestGoogleClaimsSecurity(t *testing.T) {
	now := time.Now()
	valid := googleTokenInfo{Iss: "https://accounts.google.com", Sub: "subject", Aud: "our-client", Azp: "our-client", Email: "owner@gmail.com", EmailVerified: true, Exp: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)}
	if err := validateGoogleClaims(&valid, "our-client", now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*googleTokenInfo){
		"audience":          func(c *googleTokenInfo) { c.Aud = "another-app" },
		"presenter":         func(c *googleTokenInfo) { c.Azp = "another-app" },
		"issuer":            func(c *googleTokenInfo) { c.Iss = "evil.invalid" },
		"expired":           func(c *googleTokenInfo) { c.Exp = strconv.FormatInt(now.Unix(), 10) },
		"missing expiry":    func(c *googleTokenInfo) { c.Exp = "" },
		"missing subject":   func(c *googleTokenInfo) { c.Sub = "" },
		"unverified":        func(c *googleTokenInfo) { c.EmailVerified = false },
		"external mailbox":  func(c *googleTokenInfo) { c.Email = "owner@example.com" },
		"malformed mailbox": func(c *googleTokenInfo) { c.Email = "User <owner@gmail.com>" },
	} {
		t.Run(name, func(t *testing.T) {
			claims := valid
			mutate(&claims)
			if validateGoogleClaims(&claims, "our-client", now) == nil {
				t.Fatal("invalid Google identity accepted")
			}
		})
	}
	if validateGoogleClaims(&valid, "", now) == nil {
		t.Fatal("blank client ID accepted")
	}
	workspace := valid
	workspace.Email, workspace.HostedDomain = "owner@example.com", "example.com"
	if err := validateGoogleClaims(&workspace, "our-client", now); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyGoogleIDToken("do-not-send", ""); err == nil {
		t.Fatal("unconfigured login accepted")
	}
}

func TestGoogleHandlerRequiresLocalMFA(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "google-handler.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const email = "mfa-handler@gmail.com"
	if _, err := dbpkg.CreateLicense(db, email, 30, 5, "Pro"); err != nil {
		t.Fatal(err)
	}
	secret, _, recovery, err := dbpkg.SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatal(err)
	}
	code, err := dbpkg.CalculateTOTP(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = dbpkg.EnableCustomerTOTP(db, email, secret, code, recovery); err != nil {
		t.Fatal(err)
	}
	original := verifyGoogleToken
	defer func() { verifyGoogleToken = original }()
	verifyGoogleToken = func(_, _ string) (*googleTokenInfo, error) { return &googleTokenInfo{Email: email}, nil }
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/google", bytes.NewBufferString(`{"credential":"valid"}`))
	CustomerGoogleLoginHandler(db, "client").ServeHTTP(rec, req)
	var result struct {
		Token        string
		Requires2FA  bool   `json:"requires_2fa"`
		Challenge    string `json:"challenge_token"`
		EmailAllowed bool   `json:"email_code_allowed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !result.Requires2FA || result.Challenge == "" || result.Token != "" || result.EmailAllowed {
		t.Fatalf("unsafe response: %s", rec.Body)
	}
}
