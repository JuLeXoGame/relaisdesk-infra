//go:build windows

package main

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Uses a temporary key, cleaned up afterwards. Never touches the real
// relaisdesk registration and never calls ensureFleetProtocol (which would
// point the protocol at the test binary).
func TestRegisterFleetProtocolTempKey(t *testing.T) {
	path := fmt.Sprintf(`Software\Classes\relaisdesk-test-%d`, os.Getpid())
	defer registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open\command`)
	defer registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open`)
	defer registry.DeleteKey(registry.CURRENT_USER, path+`\shell`)
	defer registry.DeleteKey(registry.CURRENT_USER, path)

	command := `"C:\Temp\configurator.exe" --connect-uri "%1"`
	if err := registerFleetProtocol(registry.CURRENT_USER, path, command); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := registerFleetProtocol(registry.CURRENT_USER, path, command); err != nil {
		t.Fatalf("register idempotent: %v", err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer key.Close()
	if v, _, err := key.GetStringValue(""); err != nil || v != "URL:RelaisDesk" {
		t.Errorf("default value: got %q, %v", v, err)
	}
	if v, _, err := key.GetStringValue("URL Protocol"); err != nil || v != "" {
		t.Errorf("URL Protocol: got %q, %v", v, err)
	}
	cmdKey, err := registry.OpenKey(registry.CURRENT_USER, path+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open command: %v", err)
	}
	defer cmdKey.Close()
	if v, _, err := cmdKey.GetStringValue(""); err != nil || v != command {
		t.Errorf("command: got %q, %v", v, err)
	}

	other := `"D:\Other\configurator.exe" --connect-uri "%1"`
	if err := registerFleetProtocol(registry.CURRENT_USER, path, other); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if v, _, err := cmdKey.GetStringValue(""); err != nil || v != other {
		t.Errorf("command after move: got %q, %v", v, err)
	}
}
