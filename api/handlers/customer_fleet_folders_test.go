package handlers

import (
	"api/middleware"
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestCustomerLicenseFoldersMatchTechnicianFleet(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "fleet-folder-catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	owner, err := dbpkg.EnsureCustomer(db, "fleet-owner@example.test", "business", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	makeLicense := func(email string) *dbpkg.License {
		t.Helper()
		license, err := dbpkg.CreateLicense(db, email, 365, 5, "Pro")
		if err != nil {
			t.Fatal(err)
		}
		return license
	}
	license := makeLicense(owner.Email)
	otherOwned := makeLicense(owner.Email)
	foreign := makeLicense("another-owner@example.test")
	makeFolder := func(customer int64, licenseID, name, parent string) *dbpkg.DeviceFolder {
		t.Helper()
		folder, err := dbpkg.CreateDeviceFolder(db, customer, licenseID, name, parent)
		if err != nil {
			t.Fatal(err)
		}
		return folder
	}
	root := makeFolder(0, license.LicenseID, "Siège Paris", "")
	child := makeFolder(0, license.LicenseID, "Serveurs", root.FolderID)
	makeFolder(0, otherOwned.LicenseID, "Autre parc du propriétaire", "")
	makeFolder(0, foreign.LicenseID, "Parc confidentiel tiers", "")
	personal := makeFolder(owner.ID, "", "Ancien dossier du compte", "")
	login, eligible, err := dbpkg.CreateCustomerLoginToken(db, owner.Email)
	if err != nil || !eligible {
		t.Fatalf("login: %v", err)
	}
	session, _, err := dbpkg.ConsumeCustomerLoginToken(db, login)
	if err != nil {
		t.Fatal(err)
	}
	techSession, err := dbpkg.CreateTechnicianSession(db, license.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	call := func(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}
	customer := middleware.CustomerAuth(db)(CustomerListFoldersHandler(db))
	technician := middleware.TechnicianAuth(db)(TechnicianListFoldersHandler(db))
	decode := func(rec *httptest.ResponseRecorder) []dbpkg.DeviceFolder {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Folders []dbpkg.DeviceFolder `json:"folders"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Folders
	}
	path := "/api/v1/customer/device-folders?license_id=" + url.QueryEscape(license.LicenseID)
	assertSameFleet := func(expectedChildName string) {
		t.Helper()
		folders := decode(call(customer, "GET", path, session, ""))
		fleet := decode(call(technician, "GET", "/api/v1/technician/device-folders", techSession, ""))
		if len(folders) != 2 || len(fleet) != 2 {
			t.Fatalf("catalog differs from fleet: %v / %v", folders, fleet)
		}
		for i, folder := range folders {
			if folder.FolderID != fleet[i].FolderID || folder.Name != fleet[i].Name || folder.ParentFolderID != fleet[i].ParentFolderID || folder.LicenseID != license.LicenseID {
				t.Fatalf("wrong fleet folder: %+v / %+v", folder, fleet[i])
			}
			if folder.FolderID == child.FolderID && folder.Name != expectedChildName {
				t.Fatalf("stale name: %s", folder.Name)
			}
		}
	}
	assertSameFleet("Serveurs")
	rename := middleware.TechnicianAuth(db)(TechnicianFolderActionHandler(db))
	rec := call(rename, "PUT", "/api/v1/technician/device-folders/"+child.FolderID, techSession, `{"name":"Serveurs renommés"}`)
	if rec.Code != 200 {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	assertSameFleet("Serveurs renommés")
	legacy := decode(call(customer, "GET", "/api/v1/customer/device-folders", session, ""))
	if len(legacy) != 1 || legacy[0].FolderID != personal.FolderID {
		t.Fatalf("legacy customer catalog changed: %+v", legacy)
	}
	for _, tc := range []struct {
		query, token string
		status       int
	}{
		{"?license_id=" + foreign.LicenseID, session, 404},
		{"?license_id=LIC-UNKNOWN", session, 404},
		{"?license_id=", session, 400},
		{"?license_id=" + license.LicenseID + "&license_id=" + otherOwned.LicenseID, session, 400},
		{"?license_id=" + license.LicenseID, "", 401},
	} {
		if rec := call(customer, "GET", "/api/v1/customer/device-folders"+tc.query, tc.token, ""); rec.Code != tc.status {
			t.Errorf("query=%s status=%d, want %d", tc.query, rec.Code, tc.status)
		}
	}
}
