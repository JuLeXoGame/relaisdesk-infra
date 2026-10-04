package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The HTTP status must be honored before JSON decoding: a 429 with an empty
// or non-JSON body must surface the rate-limit error, not a read failure.
func TestValidateLicenseStatusBeforeDecode(t *testing.T) {
	previousURL := APIURL
	t.Cleanup(func() { APIURL = previousURL })

	t.Run("429 empty body reports rate limit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := validateLicense("LICENCE-TEST")
		if err == nil || !strings.Contains(err.Error(), "trop de tentatives") {
			t.Fatalf("429 vide = %v, want rate-limit error", err)
		}
	})

	t.Run("401 empty body reports refusal with status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := validateLicense("LICENCE-TEST")
		if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
			t.Fatalf("401 vide = %v, want refusal with status", err)
		}
	})

	t.Run("200 invalid JSON still reports read error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`pas du json`))
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := validateLicense("LICENCE-TEST")
		if err == nil || !strings.Contains(err.Error(), "reponse du serveur") {
			t.Fatalf("200 garbage = %v, want read error", err)
		}
	})
}
