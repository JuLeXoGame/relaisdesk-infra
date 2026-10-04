package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The HTTP status must be honored before JSON decoding: a 429/401 with an
// empty or non-JSON body must surface its dedicated error, not a generic
// decode failure.
func TestActivateViewerCodeStatusBeforeDecode(t *testing.T) {
	previousURL := APIURL
	t.Cleanup(func() { APIURL = previousURL })

	t.Run("429 empty body reports rate limit", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := activateViewerCode("ABCD-1234")
		if err == nil || !strings.Contains(err.Error(), "trop de tentatives") {
			t.Fatalf("429 vide = %v, want rate-limit error", err)
		}
	})

	t.Run("401 empty body reports expired code", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := activateViewerCode("ABCD-1234")
		if err == nil || !strings.Contains(err.Error(), "code expiré ou invalide") {
			t.Fatalf("401 vide = %v, want expired-code error", err)
		}
	})

	t.Run("401 JSON error is passed through", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"session révoquée"}`))
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := activateViewerCode("ABCD-1234")
		if err == nil || !strings.Contains(err.Error(), "session révoquée") {
			t.Fatalf("401 JSON = %v, want server error passthrough", err)
		}
	})

	t.Run("200 invalid JSON still reports bad response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`pas du json`))
		}))
		defer srv.Close()
		APIURL = srv.URL
		_, err := activateViewerCode("ABCD-1234")
		if err == nil || !strings.Contains(err.Error(), "réponse serveur invalide") {
			t.Fatalf("200 garbage = %v, want bad-response error", err)
		}
	})
}
