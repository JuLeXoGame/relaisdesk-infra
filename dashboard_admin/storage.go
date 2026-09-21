package main

import (
	"encoding/json"
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

func SaveAdminSession(session AdminStoredSession) error {
	filePath, err := getSessionFilePath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0600)
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
	var session AdminStoredSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
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
