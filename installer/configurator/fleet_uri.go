package main

import (
	"errors"
	"net/url"
	"regexp"
)

var fleetDevicePattern = regexp.MustCompile(`^DEV-[A-Z0-9]{4}(?:-[A-Z0-9]{4}){1,3}$`)
var pendingFleetDevice string

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
