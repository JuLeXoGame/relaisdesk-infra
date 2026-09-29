//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// ensureFleetProtocol makes sure relaisdesk:// links open this application.
// The installer registers HKLM, but this per-user fallback (no admin rights
// needed) also covers portable builds, moved executables and broken machine
// registrations. Best effort, never fatal. Idempotent.
func ensureFleetProtocol() {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return
	}
	_ = registerFleetProtocol(registry.CURRENT_USER, `Software\Classes\relaisdesk`, fleetProtocolCommand(exe))
}

// registerFleetProtocol writes the URL protocol handler under the given key.
// Exported within the package so tests can target a temporary key.
func registerFleetProtocol(root registry.Key, path, command string) error {
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("", "URL:RelaisDesk"); err != nil {
		return err
	}
	if err := key.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	cmdKey, _, err := registry.CreateKey(root, path+`\shell\open\command`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer cmdKey.Close()
	if current, _, err := cmdKey.GetStringValue(""); err == nil && current == command {
		return nil
	}
	return cmdKey.SetStringValue("", command)
}
