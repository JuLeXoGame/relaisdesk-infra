package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type fleetAPITransport func(*http.Request) (*http.Response, error)

func (fn fleetAPITransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestFleetAPIEnvelopesAndPagination(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://fleet.invalid"
	calls := 0
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "fleet.invalid" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("unexpected request target or authentication")
		}
		calls++
		body := ""
		switch {
		case r.Method == "POST":
			body = `{"device":{"device_id":"DEV-ABCD-1234","permanent_code":"PERM-ABCD-1234","alias":"PC"}}`
		case r.URL.Path == "/api/v1/technician/devices/DEV-ABCD-1234":
			body = `{"device":{"device_id":"DEV-ABCD-1234","rustdesk_id":"123456789","enrollment_state":"enrolled"}}`
		case r.URL.Query().Get("after") == "0":
			body = `{"devices":[{"id":1,"device_id":"DEV-ABCD-1234"}],"next_cursor":1}`
		case r.URL.Query().Get("after") == "1":
			body = `{"devices":[{"id":2,"device_id":"DEV-EFGH-5678"}],"next_cursor":0}`
		default:
			t.Fatal("unexpected fleet route")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	devices, err := listTechnicianDevices("test-token")
	if err != nil || len(devices) != 2 {
		t.Fatal(devices, err)
	}
	code, err := generateTechnicianPermanentCode("test-token", "PC", "")
	if err != nil || code.PermanentCode != "PERM-ABCD-1234" {
		t.Fatal(code, err)
	}
	target, err := getFleetConnectionTarget("test-token", "DEV-ABCD-1234")
	if err != nil || target.RustDeskID != "123456789" {
		t.Fatal(target, err)
	}
	if calls != 4 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestFleetFoldersAPI(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://fleet.invalid"

	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "fleet.invalid" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("unexpected request target or authentication")
		}
		body := "{}"
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/technician/device-folders":
			body = `{"device_folders":[{"folder_id":"FLD-001","name":"Agence Paris","parent_folder_id":""}]}`
		case r.Method == "POST" && r.URL.Path == "/api/v1/technician/device-folders":
			body = `{"folder":{"folder_id":"FLD-002","name":"Compta","parent_folder_id":"FLD-001"}}`
		case r.Method == "PUT" && r.URL.Path == "/api/v1/technician/device-folders/FLD-002":
			body = `{"folder":{"folder_id":"FLD-002","name":"Comptabilité"}}`
		case r.Method == "DELETE" && r.URL.Path == "/api/v1/technician/device-folders/FLD-002":
			body = `{"deleted":true}`
		case r.Method == "PUT" && r.URL.Path == "/api/v1/technician/devices/DEV-001":
			body = `{"device":{"device_id":"DEV-001","folder_id":"FLD-001"}}`
		default:
			t.Fatalf("unexpected folder route: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	folders, err := listTechnicianFolders("test-token")
	if err != nil || len(folders) != 1 || folders[0].Name != "Agence Paris" {
		t.Fatalf("listTechnicianFolders failed: %v, %v", folders, err)
	}

	newFolder, err := createTechnicianFolder("test-token", "Compta", "FLD-001")
	if err != nil || newFolder.FolderID != "FLD-002" {
		t.Fatalf("createTechnicianFolder failed: %v, %v", newFolder, err)
	}

	if err := updateTechnicianFolder("test-token", "FLD-002", "Comptabilité"); err != nil {
		t.Fatalf("updateTechnicianFolder failed: %v", err)
	}

	if err := deleteTechnicianFolder("test-token", "FLD-002"); err != nil {
		t.Fatalf("deleteTechnicianFolder failed: %v", err)
	}

	if err := updateTechnicianDeviceFolder("test-token", "DEV-001", "FLD-001"); err != nil {
		t.Fatalf("updateTechnicianDeviceFolder failed: %v", err)
	}

	if err := updateTechnicianDevice("test-token", "DEV-001", "Poste Accueil", "Salle 1", "FLD-001"); err != nil {
		t.Fatalf("updateTechnicianDevice failed: %v", err)
	}
}
