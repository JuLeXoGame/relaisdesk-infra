package handlers

import (
	"api/middleware"
	"api/networkauth"
	"bytes"
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

func TestTeamAPIEndToEndAuthorization(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "team-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, err := dbpkg.EnsureCustomer(db, "owner@example.test", "business", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	license, err := dbpkg.CreateLicense(db, owner.Email, 365, 5, "Pro")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := dbpkg.CreateDeviceFolder(db, owner.ID, license.LicenseID, "Allowed", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = dbpkg.SetCustomerPassword(db, owner.Email, "StrongPassword!2026"); err != nil {
		t.Fatal(err)
	}
	ownerToken, _, _, err := dbpkg.ValidateCustomerPassword(db, owner.Email, "StrongPassword!2026")
	if err != nil {
		t.Fatal(err)
	}
	call := func(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	body, _ := json.Marshal(teamInvitationRequest{LicenseID: license.LicenseID, Email: "member@example.test", FolderIDs: []string{folder.FolderID}})
	rec := call(middleware.CustomerAuth(db)(CustomerTeamInviteHandler(db)), "POST", "/api/v1/customer/team/invitations", ownerToken, string(body))
	if rec.Code != 201 {
		t.Fatalf("invite %d %s", rec.Code, rec.Body.String())
	}
	members, err := dbpkg.ListOwnedTeamMembers(db, owner.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("members %v", err)
	}
	m := members[0]
	var payload string
	if err = db.QueryRow(`SELECT payload FROM jobs WHERE job_type=?`, jobTeamInvitation).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, m.MemberID) || strings.Contains(payload, "token") {
		t.Fatalf("unexpected mail payload %s", payload)
	}
	invitation, _, err := dbpkg.IssueTeamInvitationToken(db, owner.ID, m.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"token": invitation})
	rec = call(TeamAcceptInvitationHandler(db), "POST", "/api/v1/team/invitations/accept", "", string(body))
	if rec.Code != 200 {
		t.Fatalf("accept %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("token response is cacheable")
	}
	if err = dbpkg.SetCustomerPassword(db, m.Email, "MemberPassword!2026"); err != nil {
		t.Fatal(err)
	}
	customerToken, _, _, err := dbpkg.ValidateCustomerPassword(db, m.Email, "MemberPassword!2026")
	if err != nil {
		t.Fatal(err)
	}
	rec = call(middleware.CustomerAuth(db)(CustomerDashboardHandler(db)), "GET", "/api/v1/customer/dashboard", customerToken, "")
	if rec.Code != 200 || bytes.Contains(rec.Body.Bytes(), []byte(license.LicenseID)) {
		t.Fatalf("commercial data leak: %s", rec.Body.String())
	}
	rec = call(middleware.CustomerAuth(db)(CustomerTeamTechnicianSessionHandler(db)), "POST", "/api/v1/customer/team/technician-session", customerToken, `{"license_id":"`+license.LicenseID+`"}`)
	if rec.Code != 200 {
		t.Fatalf("exchange %s", rec.Body.String())
	}
	var session struct {
		Token      string `json:"token"`
		Restricted bool   `json:"restricted_to_folders"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &session); err != nil || !session.Restricted {
		t.Fatalf("unscoped exchange %v", err)
	}
	scoped := middleware.TechnicianAuth(db)
	for _, path := range []string{"/api/v1/technician/viewer-codes/generate", "/api/v1/technician/devices/enrollment-code", "/api/v1/technician/device-folders", "/api/v1/technician/interventions"} {
		rec = call(scoped(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("restricted mutation reached handler") })), "POST", path, session.Token, `{}`)
		if rec.Code != 403 {
			t.Fatalf("mutation %s: %d", path, rec.Code)
		}
	}
	device, err := dbpkg.CreatePermanentEnrollment(db, owner.ID, license.LicenseID, "PC", "")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/technician/devices/" + device.DeviceID
	rec = call(scoped(TechnicianDeviceActionHandler(db)), "GET", path, session.Token, "")
	if rec.Code != 404 {
		t.Fatal("ungrouped device disclosed")
	}
	db.Exec(`UPDATE devices SET folder_id=? WHERE device_id=?`, folder.FolderID, device.DeviceID)
	rec = call(scoped(TechnicianDeviceActionHandler(db)), "GET", path, session.Token, "")
	if rec.Code != 404 {
		t.Fatal("legacy engine allowed")
	}
	db.Exec(`UPDATE devices SET peer_auth_version=1 WHERE device_id=?`, device.DeviceID)
	rec = call(scoped(TechnicianDeviceActionHandler(db)), "GET", path, session.Token, "")
	if rec.Code != 200 {
		t.Fatalf("allowed device %s", rec.Body.String())
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := networkauth.NewSigner(key, "test", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"device_public_key": base64.RawURLEncoding.EncodeToString(public)})
	rec = call(scoped(TechnicianNetworkTokenHandler(db, signer)), "POST", "/api/v1/technician/network-token", session.Token, string(body))
	if rec.Code != 200 {
		t.Fatalf("network %s", rec.Body.String())
	}
	var issued networkTokenResponse
	json.Unmarshal(rec.Body.Bytes(), &issued)
	claims := decodeUnsignedClaimsForTest(t, issued.Token)
	if claims.Role != "folder_technician" || claims.MaxSessions != 5 || len(claims.FolderAccess) != 1 || claims.ExpiresAt > time.Now().Unix()+60 {
		t.Fatalf("network scope %+v", claims)
	}
	rec = call(middleware.CustomerAuth(db)(CustomerTeamMemberHandler(db)), "DELETE", "/api/v1/customer/team/members/"+m.MemberID, customerToken, "")
	if rec.Code != 403 {
		t.Fatal("member administered his membership")
	}
	if err = dbpkg.RevokeTeamMember(db, owner.ID, m.MemberID); err != nil {
		t.Fatal(err)
	}
	rec = call(scoped(TechnicianDeviceActionHandler(db)), "GET", path, session.Token, "")
	if rec.Code != 401 {
		t.Fatalf("revoked session %d", rec.Code)
	}
}
