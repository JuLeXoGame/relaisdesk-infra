package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestLoginTechnicianGoogleDispatch(t *testing.T) {
	var receivedBody map[string]string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/technician/login/google" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TechnicianLoginResponse{
				Valid:     true,
				Token:     "google-tech-token-123",
				LicenseID: "MP-GOOGLE-001",
				Email:     "tech@gmail.com",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	origAPI := APIURL
	APIURL = ts.URL
	defer func() { APIURL = origAPI }()

	resp, err := loginTechnicianGoogle("my-google-credential-token", "MP-GOOGLE-001", "device-token-xyz")
	if err != nil {
		t.Fatalf("loginTechnicianGoogle failed: %v", err)
	}
	if !resp.Valid || resp.Token != "google-tech-token-123" || resp.LicenseID != "MP-GOOGLE-001" {
		t.Fatalf("Unexpected login response: %+v", resp)
	}
	if receivedBody["credential"] != "my-google-credential-token" ||
		receivedBody["license_id"] != "MP-GOOGLE-001" ||
		receivedBody["device_token"] != "device-token-xyz" {
		t.Fatalf("Unexpected request received by API: %+v", receivedBody)
	}
}

func TestGoogleOAuthLoopbackCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	authURL, resChan, cleanup, err := startGoogleOAuthLoopback(ctx)
	if err != nil {
		t.Fatalf("startGoogleOAuthLoopback failed: %v", err)
	}
	defer cleanup()

	// Parse authURL to extract port and state
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Failed to parse authURL %s: %v", authURL, err)
	}
	port := u.Query().Get("port")
	state := u.Query().Get("state")
	if port == "" || state == "" {
		t.Fatalf("authURL missing port or state: %s", authURL)
	}

	// 1. Send invalid state -> should return 403
	invalidURL := "http://127.0.0.1:" + port + "/callback?credential=test&state=wrongstate"
	respInvalid, err := http.Get(invalidURL)
	if err != nil {
		t.Fatalf("GET invalid state failed: %v", err)
	}
	respInvalid.Body.Close()
	if respInvalid.StatusCode != http.StatusForbidden {
		t.Fatalf("Expected 403 on invalid state, got %d", respInvalid.StatusCode)
	}

	// 2. Send valid state and credential -> should return 200 and deliver to resChan
	validURL := "http://127.0.0.1:" + port + "/callback?credential=google-cred-token-456&state=" + state
	respValid, err := http.Get(validURL)
	if err != nil {
		t.Fatalf("GET valid callback failed: %v", err)
	}
	respValid.Body.Close()
	if respValid.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 on valid callback, got %d", respValid.StatusCode)
	}

	select {
	case res := <-resChan:
		if res.Error != nil {
			t.Fatalf("resChan returned error: %v", res.Error)
		}
		if res.Credential != "google-cred-token-456" {
			t.Fatalf("Expected credential 'google-cred-token-456', got '%s'", res.Credential)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for result on resChan")
	}
}
