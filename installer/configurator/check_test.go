package main

import (
	"os"
	"testing"
)

func TestCheckServerVersion(t *testing.T) {
	if os.Getenv("RELAISDESK_RUN_LIVE_TESTS") != "1" {
		t.Skip("live account access requires RELAISDESK_RUN_LIVE_TESTS=1")
	}
	creds, err := LoadCredentials()
	if err != nil || creds == nil {
		t.Skip("no creds")
	}
	ident := creds.Email
	secret := creds.Password
	if ident == "" {
		ident = creds.LicenseID
		secret = creds.LicenseKey
	}
	loginResp, err := loginTechnician(ident, secret, "", creds.DeviceToken)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	devices, err := listTechnicianDevices(loginResp.Token)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, d := range devices {
		t.Logf("Device: ID=%s Alias=%s Host=%s OS=%s Status=%s", d.DeviceID, d.Alias, d.Hostname, d.OS, d.Status)
	}
}
