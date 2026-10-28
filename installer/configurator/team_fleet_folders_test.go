package main

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestCustomerFoldersUseSelectedLicenseAndReloadNames(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	calls := 0
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v1/customer/device-folders" || r.URL.Query().Get("license_id") != "LIC-A&B" || len(r.URL.Query()) != 1 || r.Header.Get("Authorization") != "Bearer sess-9" {
			t.Fatalf("wrong fleet request: %s", r.URL)
		}
		name := "Serveurs"
		if calls > 1 {
			name = "Serveurs renommés"
		}
		body := `{"folders":[{"folder_id":"FLD-A","license_id":"LIC-A&B","name":"` + name + `"}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	for _, expected := range []string{"Serveurs", "Serveurs renommés"} {
		folders, err := listCustomerFolders("sess-9", "LIC-A&B")
		if err != nil || len(folders) != 1 || folders[0].Name != expected {
			t.Fatalf("folders=%+v err=%v", folders, err)
		}
	}
	if _, err := listCustomerFolders("sess-9", ""); err == nil || calls != 2 {
		t.Fatal("unscoped folder request allowed")
	}
}

func TestCustomerFoldersRejectOldServerUnscopedCatalog(t *testing.T) {
	mockCustomerAPI(t, map[string]string{"GET /api/v1/customer/device-folders": `{"folders":[{"folder_id":"OLD","name":"Ancien dossier du compte","license_id":""}]}`})
	if _, err := listCustomerFolders("sess-9", "LIC-A"); err == nil {
		t.Fatal("customer folders accepted as fleet folders")
	}
}

func teamFleetFolderFixture() []customerFolder {
	return []customerFolder{
		{FolderID: "A", LicenseID: "LIC-A", Name: "Paris"},
		{FolderID: "A-SRV", LicenseID: "LIC-A", Name: "Serveurs", ParentFolderID: "A"},
		{FolderID: "B", LicenseID: "LIC-B", Name: "Lyon"},
		{FolderID: "B-SRV", LicenseID: "LIC-B", Name: "Serveurs", ParentFolderID: "B"},
		{FolderID: "OLD", Name: "Ancien dossier du compte"},
	}
}

func TestTeamFolderDialogShowsFleetPathsAndKeepsExistingUnknownGrants(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	SetLang(LangFR)
	defer SetLang(LangFR)
	_, check, ids := teamFoldersDialogContent(customerTeamMember{LicenseID: "LIC-A", Email: "member@example.test", FolderIDs: []string{"A-SRV", "UNAVAILABLE"}}, teamFleetFolderFixture())
	if len(check.Options) != 3 || len(check.Selected) != 2 {
		t.Fatalf("options=%v selected=%v", check.Options, check.Selected)
	}
	var pathFound, oldGrantKept bool
	for _, option := range check.Options {
		if strings.Contains(option, "Lyon") || strings.Contains(option, "Ancien dossier") {
			t.Fatalf("wrong fleet folder: %s", option)
		}
		if strings.Contains(option, "Paris > Serveurs") && ids[option] == "A-SRV" {
			pathFound = true
		}
	}
	for _, option := range check.Selected {
		if ids[option] == "UNAVAILABLE" {
			oldGrantKept = true
		}
	}
	if !pathFound || !oldGrantKept {
		t.Fatalf("fleet path or existing grant lost: %v %v", check.Options, check.Selected)
	}
}

func TestTeamInvitationChangesFolderCatalogWithLicense(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	form := teamInviteForm([]string{"Paris fleet", "Lyon fleet"}, map[string]string{"Paris fleet": "LIC-A", "Lyon fleet": "LIC-B"}, teamFleetFolderFixture(), PanelTeam, func(int) {}).(*fyne.Container)
	var selectLicense *widget.Select
	var check *widget.CheckGroup
	for _, object := range form.Objects {
		if value, ok := object.(*widget.Select); ok {
			selectLicense = value
		}
		if value, ok := object.(*widget.CheckGroup); ok {
			check = value
		}
	}
	if selectLicense == nil || check == nil {
		t.Fatal("folder selector missing")
	}
	if len(check.Options) != 2 || !strings.Contains(strings.Join(check.Options, " "), "Paris > Serveurs") {
		t.Fatalf("initial catalog: %v", check.Options)
	}
	check.SetSelected([]string{check.Options[0]})
	selectLicense.SetSelected("Lyon fleet")
	if len(check.Options) != 2 || len(check.Selected) != 0 || strings.Contains(strings.Join(check.Options, " "), "Paris") || !strings.Contains(strings.Join(check.Options, " "), "Lyon > Serveurs") {
		t.Fatalf("wrong switched catalog: %v %v", check.Options, check.Selected)
	}
}
