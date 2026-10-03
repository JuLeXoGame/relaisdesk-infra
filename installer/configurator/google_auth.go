package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type googleOAuthResult struct {
	Credential string
	Error      error
}

func generateSecureOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openBrowserCrossPlatform(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	return cmd.Start()
}

// startGoogleOAuthLoopback starts an ephemeral HTTP server on 127.0.0.1 to capture the Google OAuth callback.
func startGoogleOAuthLoopback(ctx context.Context) (authURL string, resultChan <-chan googleOAuthResult, cancelFunc func(), err error) {
	state, err := generateSecureOAuthState()
	if err != nil {
		return "", nil, nil, fmt.Errorf("erreur génération état OAuth: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, fmt.Errorf("impossible d'ouvrir le port d'authentification local: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	resCh := make(chan googleOAuthResult, 1)

	var closeOnce sync.Once
	cleanup := func() {
		closeOnce.Do(func() {
			_ = listener.Close()
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		reqState := r.URL.Query().Get("state")
		if reqState == "" || reqState != state {
			http.Error(w, "Requête non autorisée (jeton d'état invalide)", http.StatusForbidden)
			return
		}

		credential := strings.TrimSpace(r.URL.Query().Get("credential"))
		if credential == "" {
			http.Error(w, "Jeton Google manquant", http.StatusBadRequest)
			select {
			case resCh <- googleOAuthResult{Error: errors.New("jeton Google manquant dans la réponse")}:
			default:
			}
			return
		}

		// Return success HTML page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="fr">
<head>
  <meta charset="utf-8">
  <title>Connexion Réussie — RelaisDesk Technicien</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0b0f19; color: #f8fafc; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .card { background: #1e293b; border-radius: 12px; padding: 2.5rem; text-align: center; max-width: 440px; box-shadow: 0 16px 36px rgba(0,0,0,0.6); border: 1px solid #334155; }
    .badge { display: inline-block; background: rgba(34, 197, 94, 0.2); color: #4ade80; border: 1px solid rgba(34, 197, 94, 0.4); padding: 6px 14px; border-radius: 9999px; font-weight: 600; font-size: 0.85rem; margin-bottom: 1rem; }
    h2 { color: #38bdf8; margin: 0 0 10px 0; font-size: 1.4rem; }
    p { color: #94a3b8; font-size: 0.95rem; line-height: 1.5; margin: 0 0 1.2rem 0; }
    .btn { display: inline-block; background: #2563eb; color: #fff; padding: 8px 18px; border-radius: 8px; text-decoration: none; font-size: 0.9rem; font-weight: 600; cursor: pointer; border: none; }
  </style>
</head>
<body>
  <div class="card">
    <div class="badge">✓ Authentification confirmée</div>
    <h2>Connexion réussie !</h2>
    <p>Votre compte Google a été vérifié avec succès.<br>Vous pouvez maintenant fermer cet onglet et revenir dans votre application <strong>RelaisDesk Technicien</strong>.</p>
    <button class="btn" onclick="window.close()">Fermer cet onglet</button>
  </div>
  <script>
    setTimeout(function() { window.close(); }, 3000);
  </script>
</body>
</html>`)

		select {
		case resCh <- googleOAuthResult{Credential: credential}:
		default:
		}

		go func() {
			time.Sleep(1 * time.Second)
			cleanup()
		}()
	})

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	go func() {
		<-ctx.Done()
		cleanup()
		_ = server.Shutdown(context.Background())
	}()

	authURL = fmt.Sprintf("https://relaisdesk.fr/client/login-desktop.html?port=%d&state=%s", port, state)
	return authURL, resCh, cleanup, nil
}

// performGoogleOAuthFlowCredential executes the full OAuth loopback flow,
// authenticates the technician and returns the raw Google credential so the
// caller can also open the customer session with it (verification rejouable).
func performGoogleOAuthFlowCredential(ctx context.Context, deviceToken ...string) (*TechnicianLoginResponse, string, error) {
	flowCtx, flowCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer flowCancel()

	authURL, resChan, cleanup, err := startGoogleOAuthLoopback(flowCtx)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()

	// Open user's default browser
	if err := openBrowserCrossPlatform(authURL); err != nil {
		return nil, "", fmt.Errorf("impossible d'ouvrir le navigateur : %w", err)
	}

	select {
	case res := <-resChan:
		if res.Error != nil {
			return nil, "", res.Error
		}
		dT := ""
		if len(deviceToken) > 0 {
			dT = deviceToken[0]
		}
		resp, err := loginTechnicianGoogle(res.Credential, "", dT)
		if err != nil {
			return nil, "", err
		}
		return resp, res.Credential, nil

	case <-flowCtx.Done():
		if errors.Is(flowCtx.Err(), context.DeadlineExceeded) {
			return nil, "", errors.New("délai de connexion Google dépassé (3 minutes)")
		}
		return nil, "", errors.New("connexion Google annulée")
	}
}

// performGoogleOAuthFlow executes the full OAuth loopback flow and authenticates the technician.
func performGoogleOAuthFlow(ctx context.Context, deviceToken ...string) (*TechnicianLoginResponse, error) {
	resp, _, err := performGoogleOAuthFlowCredential(ctx, deviceToken...)
	return resp, err
}
