package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type SavedCredentials struct {
	Email       string `json:"email,omitempty"`
	Password    string `json:"password,omitempty"`
	LicenseID   string `json:"license_id,omitempty"`
	LicenseKey  string `json:"license_key,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
}

type SavedLicense = SavedCredentials

func getStorageDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Utilise AppData\Roaming\RelaisDesk sur Windows et ~/.config/RelaisDesk sur Linux
	path := filepath.Join(home, ".config", "RelaisDesk")
	if os.Getenv("APPDATA") != "" {
		path = filepath.Join(os.Getenv("APPDATA"), "RelaisDesk")
	}

	os.MkdirAll(path, 0700)
	return path
}

func getLicenseFilePath() string {
	dir := getStorageDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "license.json")
}

func SaveCredentials(email, password string, deviceToken ...string) error {
	dT := ""
	if len(deviceToken) > 0 {
		dT = deviceToken[0]
	}
	return saveCredentials(SavedCredentials{Email: email, Password: password, DeviceToken: dT})
}

func SaveLicense(licenseID, licenseKey string, deviceToken ...string) error {
	dT := ""
	if len(deviceToken) > 0 {
		dT = deviceToken[0]
	}
	return saveCredentials(SavedCredentials{LicenseID: licenseID, LicenseKey: licenseKey, DeviceToken: dT})
}

func LoadCredentials() (*SavedCredentials, error) {
	path := getLicenseFilePath()
	if path == "" {
		return nil, errors.New("dossier personnel introuvable")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024 {
		return nil, errors.New("fichier d'identifiants invalide")
	}
	var envelope credentialEnvelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var credentials SavedCredentials
	if envelope.Version == 2 {
		credentials, err = unprotectCredentials(envelope)
		if err != nil {
			return nil, err
		}
	} else if envelope.Version == 0 {
		if err = json.Unmarshal(data, &credentials); err != nil {
			return nil, err
		}
		// Migrate old plaintext before returning any credentials to the caller.
		if err = saveCredentials(credentials); err != nil {
			return nil, err
		}
		return LoadCredentials()
	} else {
		return nil, errors.New("format d'identifiants non pris en charge")
	}
	return &credentials, nil
}

func LoadLicense() (*SavedLicense, error) {
	return LoadCredentials()
}

func ClearLicense() error {
	return os.Remove(getLicenseFilePath())
}

type credentialEnvelope struct {
	Version     int    `json:"version"`
	Protected   []byte `json:"protected,omitempty"`
	Email       string `json:"email,omitempty"`
	LicenseID   string `json:"license_id,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
}

func saveCredentials(credentials SavedCredentials) error {
	path := getLicenseFilePath()
	if path == "" {
		return errors.New("dossier personnel introuvable")
	}
	envelope, err := protectCredentials(credentials)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
