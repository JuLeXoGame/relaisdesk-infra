package database

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func teamFixture(t *testing.T) (*sql.DB, *CustomerIdentity, *License, *DeviceFolder) {
	t.Helper()
	db, err := InitDatabase(filepath.Join(t.TempDir(), "teams.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	owner, err := EnsureCustomer(db, "owner@example.com", "business", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	lic, err := CreateLicense(db, owner.Email, 365, 5, "Pro")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := CreateDeviceFolder(db, owner.ID, lic.LicenseID, "Allowed", "")
	if err != nil {
		t.Fatal(err)
	}
	return db, owner, lic, folder
}

func TestTeamInvitationIsolationAndRevocation(t *testing.T) {
	db, owner, lic, folder := teamFixture(t)
	m, err := CreateTeamInvitation(db, owner.ID, lic.LicenseID, "Member@example.com", []string{folder.FolderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreateTeamInvitation(db, owner.ID, lic.LicenseID, owner.Email, nil); err == nil {
		t.Fatal("owner invited himself")
	}
	if _, _, err = IssueTeamInvitationToken(db, owner.ID+99, m.MemberID); err == nil {
		t.Fatal("foreign owner got token")
	}
	token, _, err := IssueTeamInvitationToken(db, owner.ID, m.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	if err = db.QueryRow(`SELECT invite_hash FROM team_members WHERE member_id=?`, m.MemberID).Scan(&hash); err != nil || hash == token || len(hash) < 32 {
		t.Fatalf("token storage %v", err)
	}
	login, email, err := AcceptTeamInvitation(db, token)
	if err != nil || email != "member@example.com" || login == "" {
		t.Fatalf("accept %s %v", email, err)
	}
	if _, _, err = AcceptTeamInvitation(db, token); err == nil {
		t.Fatal("replayed invitation")
	}
	member, err := GetCustomerIdentityByEmail(db, email)
	if err != nil || member.ID == owner.ID {
		t.Fatalf("not isolated %v", err)
	}
	if err = SetCustomerPassword(db, email, "StrongTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	auth, _, err := ValidateTechnicianEmailPassword(db, email, "StrongTestPassword!2026", "", lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := TeamMemberForSession(db, auth.SessionToken)
	if err != nil || scoped == nil || scoped.MemberID != m.MemberID {
		t.Fatalf("unscoped login %v", err)
	}
	if _, err = db.Exec(`UPDATE licences SET notes='ADMIN' WHERE license_id=?`, lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if err = ValidateAdminSession(db, auth.SessionToken); err == nil {
		t.Fatal("member inherited owner administrator privileges")
	}
	if _, err = CreatePersonalTechnicianSession(db, "outsider@example.com", lic.LicenseID); err == nil {
		t.Fatal("outsider exchanged session")
	}
	if err = RevokeTeamMember(db, owner.ID, m.MemberID); err != nil {
		t.Fatal(err)
	}
	if _, err = ValidateTechnicianSession(db, auth.SessionToken); err == nil {
		t.Fatal("revoked session valid")
	}
	if _, err = CreatePersonalTechnicianSession(db, email, lic.LicenseID); err == nil {
		t.Fatal("revoked member login")
	}
	if _, err = GetCustomerIdentityByEmail(db, email); err != nil {
		t.Fatal("personal account deleted")
	}
}

func TestTeamCapacityPlanAndExpiry(t *testing.T) {
	db, owner, lic, _ := teamFixture(t)
	for i := 0; i < 5; i++ {
		if _, err := CreateTeamInvitation(db, owner.ID, lic.LicenseID, fmt.Sprintf("m%d@example.com", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CreateTeamInvitation(db, owner.ID, lic.LicenseID, "extra@example.com", nil); !errors.Is(err, ErrTeamCapacity) {
		t.Fatalf("quota %v", err)
	}
	members, err := ListOwnedTeamMembers(db, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE team_members SET invite_expires_at='2000-01-01' WHERE member_id=?`, members[0].MemberID)
	if _, _, err = IssueTeamInvitationToken(db, owner.ID, members[0].MemberID); !errors.Is(err, ErrTeamInvite) {
		t.Fatal("expired invitation renewed")
	}
	if _, err = CreateTeamInvitation(db, owner.ID, lic.LicenseID, "extra@example.com", nil); err != nil {
		t.Fatal(err)
	}
	starter, err := CreateLicense(db, owner.Email, 365, 1, "Starter")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreateTeamInvitation(db, owner.ID, starter.LicenseID, "starter@example.com", nil); !errors.Is(err, ErrTeamAccess) {
		t.Fatalf("Starter allowed %v", err)
	}
}

func TestTeamFoldersAndPagination(t *testing.T) {
	db, owner, lic, folder := teamFixture(t)
	child, err := CreateDeviceFolder(db, owner.ID, lic.LicenseID, "Child", folder.FolderID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := CreateDeviceFolder(db, owner.ID, lic.LicenseID, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := CreateTeamInvitation(db, owner.ID, lic.LicenseID, "member@example.com", []string{folder.FolderID})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := IssueTeamInvitationToken(db, owner.ID, m.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = AcceptTeamInvitation(db, token); err != nil {
		t.Fatal(err)
	}
	var allowedID string
	for i, f := range []string{other.FolderID, "", child.FolderID, folder.FolderID} {
		d, e := CreatePermanentEnrollment(db, owner.ID, lic.LicenseID, fmt.Sprint(i), "")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(`UPDATE devices SET folder_id=?,peer_auth_version=1 WHERE device_id=?`, f, d.DeviceID); e != nil {
			t.Fatal(e)
		}
		if i == 2 {
			allowedID = d.DeviceID
		}
	}
	page, err := ListTeamDevicePage(db, m.MemberID, 0, 1)
	if err != nil || len(page) != 1 || page[0].DeviceID != allowedID {
		t.Fatalf("filtered page %+v %v", page, err)
	}
	next, err := ListTeamDevicePage(db, m.MemberID, page[0].ID, 1)
	if err != nil || len(next) != 1 || next[0].FolderID != folder.FolderID {
		t.Fatalf("next %+v %v", next, err)
	}
	current, err := activeTeamMember(db, m.MemberID, "")
	if err != nil {
		t.Fatal(err)
	}
	d, err := GetDeviceByID(db, allowedID)
	if err != nil {
		t.Fatal(err)
	}
	if ok, e := TeamMemberCanAccessDevice(db, current, d); e != nil || !ok {
		t.Fatalf("child denied %v", e)
	}
	if err = UpdateTeamFolders(db, owner.ID, m.MemberID, nil); err != nil {
		t.Fatal(err)
	}
	if ok, _ := TeamMemberCanAccessDevice(db, current, d); ok {
		t.Fatal("stale grants allowed")
	}
	page, err = ListTeamDevicePage(db, m.MemberID, 0, 100)
	if err != nil || len(page) != 0 {
		t.Fatalf("empty grants allowed %v", err)
	}
}
