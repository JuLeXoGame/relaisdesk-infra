//go:build !windows && !linux

package main

import "errors"

func fleetPeerAuthVersion() int { return 0 }

func fleetUnsupported() error {
	return errors.New("l'accès permanent nécessite Windows ou Linux avec systemd et le service RustDesk du fork ; les sessions temporaires restent disponibles sur ce système")
}
func installFleet(string) (*DeviceEnrollResponse, error) { return nil, fleetUnsupported() }
func uninstallFleet() error                              { return fleetUnsupported() }
func runFleetService() error                             { return fleetUnsupported() }
func fleetServiceExists() bool                           { return false }
func fleetCheckAuthorization() error                     { return fleetUnsupported() }
func writeFleetConfiguration(*fleetState) error          { return fleetUnsupported() }
func fleetRustDeskRunning() bool                         { return false }
func startFleetRustDesk() error                          { return fleetUnsupported() }
func stopFleetRustDesk() error                           { return nil }
func isElevated() bool                                   { return false }
func relaunchElevated([]string) error                    { return fleetUnsupported() }
func rustDeskServiceExists() bool                        { return false }
func ensureRustDeskServiceInstalled() (string, error)    { return "", fleetUnsupported() }
func hasFleetPermanentPassword() (bool, error)           { return false, fleetUnsupported() }
func setFleetPermanentPassword(string) error             { return fleetUnsupported() }
func openRustDeskSettings() error                        { return fleetUnsupported() }
func applyServiceUpdate(string, string) error            { return fleetUnsupported() }
