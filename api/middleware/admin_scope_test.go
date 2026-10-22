package middleware

import (
	dbpkg "database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAdminAuthDoesNotPromoteTeamMember(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "admin-scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, err := dbpkg.EnsureCustomer(db, "admin@example.invalid", "business", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.CreateLicense(db, owner.Email, 30, 5, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	member, err := dbpkg.CreateTeamInvitation(db, owner.ID, lic.LicenseID, "member@example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	invite, _, err := dbpkg.IssueTeamInvitationToken(db, owner.ID, member.MemberID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = dbpkg.AcceptTeamInvitation(db, invite); err != nil {
		t.Fatal(err)
	}
	memberToken, err := dbpkg.CreatePersonalTechnicianSession(db, member.Email, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dbpkg.ValidateTechnicianSession(db, memberToken); err != nil {
		t.Fatalf("team session must remain usable as technician: %v", err)
	}
	if err = dbpkg.ValidateAdminSession(db, memberToken); err == nil {
		t.Fatal("team session accepted by central admin validation")
	}
	ownerToken, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	masterSession, err := dbpkg.CreateAdminSession(db)
	if err != nil {
		t.Fatal(err)
	}
	h := AdminAuth("master-secret", db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name, token string
		status      int
	}{
		{"team member", memberToken, http.StatusUnauthorized},
		{"license owner", ownerToken, http.StatusNoContent},
		{"master session", masterSession, http.StatusNoContent},
		{"raw master secret", "master-secret", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/licences", nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
		})
	}
}
