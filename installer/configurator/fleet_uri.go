package main

import (
	"errors"
	"net/url"
	"regexp"
)

var fleetDevicePattern = regexp.MustCompile(`^DEV-[A-Z0-9]{4}(?:-[A-Z0-9]{4}){1,3}$`)
var pendingFleetDevice string
var pendingFleetError string

// splitCLIArgs extracts the Linux-only --cli flag (terminal mode instead of
// the graphical interface) and returns the remaining arguments untouched.
func splitCLIArgs(args []string) (cli bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "--cli" {
			cli = true
			continue
		}
		rest = append(rest, a)
	}
	return cli, rest
}

// parseConnectURIArgs interprets CLI args. Empty args mean a normal launch.
func parseConnectURIArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) != 2 || args[0] != "--connect-uri" {
		return "", errors.New("Arguments non reconnus")
	}
	return parseFleetURI(args[1])
}

// fleetProtocolCommand builds the shell open command for relaisdesk:// links.
func fleetProtocolCommand(exe string) string {
	return "\"" + exe + "\" --connect-uri \"%1\""
}

func parseFleetURI(raw string) (string, error) {
	if len(raw) > 180 {
		return "", errors.New("lien de parc invalide")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "relaisdesk" || u.Host != "connect" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || len(u.Path) < 2 || !fleetDevicePattern.MatchString(u.Path[1:]) {
		return "", errors.New("lien de parc invalide")
	}
	return u.Path[1:], nil
}
func getFleetConnectionTarget(token, id string) (*DeviceItem, error) {
	if !fleetDevicePattern.MatchString(id) {
		return nil, errors.New("identifiant de parc invalide")
	}
	var response struct {
		Device DeviceItem `json:"device"`
	}
	if err := doTechnicianReq("GET", "/api/v1/technician/devices/"+id, token, nil, &response); err != nil {
		return nil, err
	}
	if response.Device.DeviceID != id || response.Device.EnrollmentState != "enrolled" || !numericFleetTarget(response.Device.RustDeskID) {
		return nil, errors.New("poste non enrôlé ou identité invalide")
	}
	return &response.Device, nil
}
func numericFleetTarget(id string) bool {
	if len(id) < 6 || len(id) > 16 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
