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
	"time"
)

type NetworkAuthorization struct {
	DevicePublicKey string
	ProofKeyFile    string
	TokenFile       string
	ExpiresAt       time.Time
}

func prepareTechnicianNetworkAuthorization(sessionToken string) (*NetworkAuthorization, error) {
	publicKey, proofKeyFile, tokenFile, err := ensureNetworkIdentity("technician")
	if err != nil {
		return nil, err
	}
	issued, err := getTechnicianNetworkToken(sessionToken, publicKey)
	if err != nil {
		return nil, err
	}
	expiresAt, err := validateNetworkTokenResponse(issued)
	if err != nil {
		return nil, err
	}
	if err := writeSecretAtomically(tokenFile, issued.NetworkToken); err != nil {
		return nil, fmt.Errorf("enregistrement du jeton réseau: %w", err)
	}
	cleanupAbandonedServiceBindings(tokenFile)
	cleanupAbandonedInterventionBindings(tokenFile)
	return &NetworkAuthorization{
		DevicePublicKey: publicKey,
		ProofKeyFile:    proofKeyFile,
		TokenFile:       tokenFile,
		ExpiresAt:       expiresAt,
	}, nil
}

func refreshTechnicianNetworkAuthorization(sessionToken string, authorization *NetworkAuthorization) error {
	if authorization == nil {
		return errors.New("autorisation réseau absente")
	}
	issued, err := getTechnicianNetworkToken(sessionToken, authorization.DevicePublicKey)
	if err != nil {
		return err
	}
	expiresAt, err := validateNetworkTokenResponse(issued)
	if err != nil {
		return err
	}
	if err := writeSecretAtomically(authorization.TokenFile, issued.NetworkToken); err != nil {
		return err
	}
	authorization.ExpiresAt = expiresAt
	return nil
}

func ensureNetworkIdentity(role string) (string, string, string, error) {
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
	keyFile := filepath.Join(dir, role+"-proof-key")
	tokenFile := filepath.Join(dir, role+"-network-token")

	privateKey, err := loadOrCreateProofKey(keyFile)
	if err != nil {
		return "", "", "", err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(publicKey), keyFile, tokenFile, nil
}

func loadOrCreateProofKey(path string) (ed25519.PrivateKey, error) {
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
		_ = os.Chmod(path, 0o600)
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

func writeSecretAtomically(path, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\t ") {
		return errors.New("secret vide ou mal formé")
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".network-token-*")
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
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validateNetworkTokenResponse(response *NetworkTokenResponse) (time.Time, error) {
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

func removeNetworkToken(authorization *NetworkAuthorization) {
	if authorization != nil && authorization.TokenFile != "" {
		stopServiceBridges(authorization.TokenFile)
		stopInterventionBridges(authorization.TokenFile)
		closeServiceBillingUI()
		_ = os.Remove(authorization.TokenFile)
	}
}
