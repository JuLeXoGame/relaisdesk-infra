package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type NetworkAuthorization struct {
	DevicePublicKey string
	ProofKeyFile    string
	TokenFile       string
	ExpiresAt       time.Time
}

var (
	viewerRefreshMu           sync.Mutex
	viewerRefreshCancel       chan struct{}
	activeViewerAuthorization *NetworkAuthorization
)

func prepareViewerNetworkAuthorization(code string) (*NetworkAuthorization, error) {
	publicKey, proofKeyFile, tokenFile, err := ensureViewerNetworkIdentity()
	if err != nil {
		return nil, err
	}
	issued, err := getViewerNetworkToken(code, publicKey)
	if err != nil {
		return nil, err
	}
	expiresAt, err := validateViewerNetworkTokenResponse(issued)
	if err != nil {
		return nil, err
	}
	if err := writeViewerSecretAtomically(tokenFile, issued.NetworkToken); err != nil {
		return nil, fmt.Errorf("enregistrement du jeton réseau: %w", err)
	}
	return &NetworkAuthorization{
		DevicePublicKey: publicKey,
		ProofKeyFile:    proofKeyFile,
		TokenFile:       tokenFile,
		ExpiresAt:       expiresAt,
	}, nil
}

func startViewerNetworkRefresh(code string, authorization *NetworkAuthorization) {
	viewerRefreshMu.Lock()
	if viewerRefreshCancel != nil {
		close(viewerRefreshCancel)
	}
	cancel := make(chan struct{})
	viewerRefreshCancel = cancel
	activeViewerAuthorization = authorization
	viewerRefreshMu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancel:
				return
			case <-ticker.C:
				if authorization == nil || time.Until(authorization.ExpiresAt) > 2*time.Minute {
					continue
				}
				issued, err := getViewerNetworkToken(code, authorization.DevicePublicKey)
				if err == nil {
					if expiresAt, validationErr := validateViewerNetworkTokenResponse(issued); validationErr == nil {
						if writeErr := writeViewerSecretAtomically(authorization.TokenFile, issued.NetworkToken); writeErr == nil {
							authorization.ExpiresAt = expiresAt
							continue
						}
					}
				}
				if time.Now().After(authorization.ExpiresAt) {
					_ = os.Remove(authorization.TokenFile)
					cleanupRustDesk2Toml()
					terminateRustDesk()
				}
			}
		}
	}()
}

func stopViewerNetworkRefresh() {
	viewerRefreshMu.Lock()
	if viewerRefreshCancel != nil {
		close(viewerRefreshCancel)
		viewerRefreshCancel = nil
	}
	if activeViewerAuthorization != nil {
		_ = os.Remove(activeViewerAuthorization.TokenFile)
		activeViewerAuthorization = nil
	}
	viewerRefreshMu.Unlock()
}

func ensureViewerNetworkIdentity() (string, string, string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", fmt.Errorf("dossier de configuration utilisateur introuvable: %w", err)
	}
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("LOCALAPPDATA")) != "" {
		configDir = os.Getenv("LOCALAPPDATA")
	}
	dir := filepath.Join(configDir, "RelaisDesk", "authorization")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", "", err
	}
	_ = os.Chmod(dir, 0o700)
	keyFile := filepath.Join(dir, "viewer-proof-key")
	tokenFile := filepath.Join(dir, "viewer-network-token")
	privateKey, err := loadOrCreateViewerProofKey(keyFile)
	if err != nil {
		return "", "", "", err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(publicKey), keyFile, tokenFile, nil
}

func loadOrCreateViewerProofKey(path string) (ed25519.PrivateKey, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 256 {
			return nil, errors.New("fichier de preuve d'appareil invalide")
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if err != nil || len(decoded) != ed25519.PrivateKeySize {
			return nil, errors.New("clé de preuve d'appareil invalide")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
		return ed25519.PrivateKey(decoded), nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(privateKey) + "\n"
	if _, err := file.WriteString(encoded); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return privateKey, nil
}

func writeViewerSecretAtomically(path, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\t ") {
		return errors.New("secret vide ou mal formé")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".network-token-*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.WriteString(value + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}
	// Windows cannot always replace an existing file with Rename. On Unix an
	// error must not delete a previously working token.
	if runtime.GOOS != "windows" {
		return fmt.Errorf("remplacement du jeton impossible: %s", path)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validateViewerNetworkTokenResponse(response *NetworkTokenResponse) (time.Time, error) {
	if response == nil || !response.Valid || !strings.HasPrefix(response.NetworkToken, "rd1.") ||
		strings.Count(response.NetworkToken, ".") != 2 {
		return time.Time{}, errors.New("jeton réseau invalide")
	}
	expiresAt, err := time.Parse(time.RFC3339, response.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().Add(30*time.Second)) {
		return time.Time{}, errors.New("expiration du jeton réseau invalide")
	}
	return expiresAt, nil
}
