package handlers

import (
	"api/middleware"
	"api/networkauth"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	dbpkg "database"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetworkTokenHandlersBindRolesAndHonorRevocation(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "network-tokens.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('rustdesk-public-key', 1)`); err != nil {
		t.Fatal(err)
	}
	license, err := dbpkg.CreateLicense(db, "tech@example.test", 30, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := dbpkg.CreateViewerCode(db, license.LicenseID, "viewer@example.test")
	if err != nil {
		t.Fatal(err)
	}
	_, signerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := networkauth.NewSigner(signerPrivate, "test-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	devicePublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	device := base64.RawURLEncoding.EncodeToString(devicePublic)

	technicianBody, _ := json.Marshal(map[string]string{"device_public_key": device})
	technicianRequest := httptest.NewRequest(http.MethodPost, "/api/v1/technician/network-token", bytes.NewReader(technicianBody))
	technicianRequest = technicianRequest.WithContext(context.WithValue(
		technicianRequest.Context(),
		middleware.TechnicianLicenseContextKey,
		license.LicenseID,
	))
	technicianRecorder := httptest.NewRecorder()
	TechnicianNetworkTokenHandler(db, signer)(technicianRecorder, technicianRequest)
	if technicianRecorder.Code != http.StatusOK {
		t.Fatalf("technician token status=%d body=%s", technicianRecorder.Code, technicianRecorder.Body.String())
	}
	var technicianResponse networkTokenResponse
	if err := json.Unmarshal(technicianRecorder.Body.Bytes(), &technicianResponse); err != nil {
		t.Fatal(err)
	}
	technicianClaims := decodeUnsignedClaimsForTest(t, technicianResponse.Token)
	if technicianClaims.Role != "technician" || technicianClaims.Tenant != license.LicenseID || technicianClaims.MaxSessions != 3 {
		t.Fatalf("unexpected technician claims: %+v", technicianClaims)
	}

	viewerBody, _ := json.Marshal(map[string]string{"code": viewer.Code, "device_public_key": device})
	viewerRecorder := httptest.NewRecorder()
	ViewerNetworkTokenHandler(db, signer)(
		viewerRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/network-token", bytes.NewReader(viewerBody)),
	)
	if viewerRecorder.Code != http.StatusOK {
		t.Fatalf("viewer token status=%d body=%s", viewerRecorder.Code, viewerRecorder.Body.String())
	}
	var viewerResponse networkTokenResponse
	if err := json.Unmarshal(viewerRecorder.Body.Bytes(), &viewerResponse); err != nil {
		t.Fatal(err)
	}
	viewerClaims := decodeUnsignedClaimsForTest(t, viewerResponse.Token)
	if viewerClaims.Role != "viewer" || viewerClaims.Tenant != license.LicenseID || !strings.HasPrefix(viewerClaims.Subject, "viewer:") {
		t.Fatalf("unexpected viewer claims: %+v", viewerClaims)
	}
	otherDevicePublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherDevice := base64.RawURLEncoding.EncodeToString(otherDevicePublic)
	spoofedAnnounceBody, _ := json.Marshal(map[string]string{
		"code": viewer.Code, "rustdesk_id": "987654321", "device_public_key": otherDevice,
	})
	spoofedAnnounceRecorder := httptest.NewRecorder()
	ViewerAnnounceHandler(db)(
		spoofedAnnounceRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/announce", bytes.NewReader(spoofedAnnounceBody)),
	)
	if spoofedAnnounceRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed announcement status=%d body=%s", spoofedAnnounceRecorder.Code, spoofedAnnounceRecorder.Body.String())
	}
	validAnnounceBody, _ := json.Marshal(map[string]string{
		"code": viewer.Code, "rustdesk_id": "123456789", "device_public_key": device,
	})
	validAnnounceRecorder := httptest.NewRecorder()
	ViewerAnnounceHandler(db)(
		validAnnounceRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/announce", bytes.NewReader(validAnnounceBody)),
	)
	if validAnnounceRecorder.Code != http.StatusOK {
		t.Fatalf("valid announcement status=%d body=%s", validAnnounceRecorder.Code, validAnnounceRecorder.Body.String())
	}

	// Token renewal is allowed from the device that first claimed the code.
	sameDeviceRecorder := httptest.NewRecorder()
	ViewerNetworkTokenHandler(db, signer)(
		sameDeviceRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/network-token", bytes.NewReader(viewerBody)),
	)
	if sameDeviceRecorder.Code != http.StatusOK {
		t.Fatalf("same-device renewal status=%d body=%s", sameDeviceRecorder.Code, sameDeviceRecorder.Body.String())
	}

	otherDeviceBody, _ := json.Marshal(map[string]string{
		"code":              viewer.Code,
		"device_public_key": otherDevice,
	})
	otherDeviceRecorder := httptest.NewRecorder()
	ViewerNetworkTokenHandler(db, signer)(
		otherDeviceRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/network-token", bytes.NewReader(otherDeviceBody)),
	)
	if otherDeviceRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("cross-device renewal status=%d body=%s", otherDeviceRecorder.Code, otherDeviceRecorder.Body.String())
	}

	if err := dbpkg.RevokeViewerCode(db, viewer.Code); err != nil {
		t.Fatal(err)
	}
	revokedRecorder := httptest.NewRecorder()
	ViewerNetworkTokenHandler(db, signer)(
		revokedRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/viewer/network-token", bytes.NewReader(viewerBody)),
	)
	if revokedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked viewer token status=%d body=%s", revokedRecorder.Code, revokedRecorder.Body.String())
	}
}

func decodeUnsignedClaimsForTest(t *testing.T, token string) networkauth.Claims {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("invalid token format: %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims networkauth.Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}
