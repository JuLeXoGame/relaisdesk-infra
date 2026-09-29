//go:build !windows

package main

// ensureFleetProtocol is a no-op outside Windows. Linux and macOS register
// URL handlers through desktop files / bundles at install time.
func ensureFleetProtocol() {}
