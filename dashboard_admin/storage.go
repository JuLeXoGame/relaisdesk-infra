package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type AdminStoredSession struct {
	Token     string `json:"token"`
	LicenseID string `json:"license_id"`
	Email     string `json:"email"`
}

func getSessionFilePath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	dir := filepath.Join(appData, "RelaisDesk")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "admin_session.json"), nil
}

// adminSessionEnvelope mirrors the configurator credential envelope: version 2
// carries a DPAPI-protected blob on Windows, version 0 is the legacy plaintext
// format migrated on first read.
type adminSessionEnvelope struct {
	Version   int    `json:"version"`
	Protected []byte `json:"protected,omitempty"`
	Email     string `json:"email,omitempty"`
	LicenseID string `json:"license_id,omitempty"`
}

func SaveAdminSession(session AdminStoredSession) error {
	filePath, err := getSessionFilePath()
	if err != nil {
		return err
	}
	envelope, err := protectAdminSession(session)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(filePath), ".admin-session-*")
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
	return os.Rename(tmp.Name(), filePath)
}

func LoadAdminSession() (*AdminStoredSession, error) {
	filePath, err := getSessionFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024 {
		return nil, errors.New("fichier de session invalide")
	}
	var envelope adminSessionEnvelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	var session AdminStoredSession
	if envelope.Version == 2 {
		session, err = unprotectAdminSession(envelope)
		if err != nil {
			return nil, err
		}
	} else if envelope.Version == 0 {
		if err = json.Unmarshal(data, &session); err != nil {
			return nil, err
		}
		// Migrate old plaintext before returning any credentials to the caller.
		if err = SaveAdminSession(session); err != nil {
			return nil, err
		}
		return LoadAdminSession()
	} else {
		return nil, errors.New("format de session non pris en charge")
	}
	return &session, nil
}

func ClearAdminSession() error {
	filePath, err := getSessionFilePath()
	if err != nil {
		return err
	}
	_ = os.Remove(filePath)
	return nil
}
