package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFleetURIStrictParsing(t *testing.T) {
	id, err := parseFleetURI("relaisdesk://connect/DEV-ABCD-2345-EFGH-6789")
	if err != nil || id != "DEV-ABCD-2345-EFGH-6789" {
		t.Fatal(id, err)
	}
	for _, u := range []string{"relaisdesk://connect/123456789", "relaisdesk://connect/DEV-ABCD-1234?password=secret", "relaisdesk://evil/DEV-ABCD-1234", "relaisdesk://user@connect/DEV-ABCD-1234", "relaisdesk://connect/%2FDEV-ABCD-1234", "relaisdesk://connect/DEV-ABCD-1234#secret", "relaisdesk://connect/--password", "relaisdesk://connect/../DEV-ABCD-1234"} {
		if _, err := parseFleetURI(u); err == nil {
			t.Errorf("accepted %s", u)
		}
	}
}

func TestParseConnectURIArgs(t *testing.T) {
	id, err := parseConnectURIArgs(nil)
	if err != nil || id != "" {
		t.Fatalf("no args: got %q, %v", id, err)
	}
	id, err = parseConnectURIArgs([]string{"--connect-uri", "relaisdesk://connect/DEV-ABCD-2345-EFGH-6789"})
	if err != nil || id != "DEV-ABCD-2345-EFGH-6789" {
		t.Fatalf("valid URI: got %q, %v", id, err)
	}
	for _, args := range [][]string{
		{"--connect-uri"},
		{"--connect-uri", "relaisdesk://connect/DEV-ABCD-2345-EFGH-6789", "extra"},
		{"--unenroll"},
		{"--connect-uri", "relaisdesk://evil/DEV-ABCD-1234"},
		{"--connect-uri", "not a uri"},
	} {
		if _, err := parseConnectURIArgs(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestMoveTechnicianFolderRequest(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://fleet.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/technician/device-folders/FLD-1" {
			t.Fatal("requête inattendue", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer t-9" {
			t.Fatal("autorisation manquante")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"parent_folder_id":"FLD-2"`) {
			t.Fatal("parent manquant:", string(body))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true}`)), Header: http.Header{}}, nil
	})
	if err := moveTechnicianFolder("t-9", "FLD-1", "FLD-2"); err != nil {
		t.Fatal(err)
	}
}

func TestSplitCLIArgs(t *testing.T) {
	cli, rest := splitCLIArgs(nil)
	if cli || len(rest) != 0 {
		t.Fatalf("no args: got cli=%v rest=%v", cli, rest)
	}
	cli, rest = splitCLIArgs([]string{"--cli"})
	if !cli || len(rest) != 0 {
		t.Fatalf("--cli alone: got cli=%v rest=%v", cli, rest)
	}
	cli, rest = splitCLIArgs([]string{"--cli", "--connect-uri", "relaisdesk://connect/DEV-ABCD-1234"})
	if !cli || len(rest) != 2 || rest[0] != "--connect-uri" {
		t.Fatalf("mixed: got cli=%v rest=%v", cli, rest)
	}
	cli, rest = splitCLIArgs([]string{"--connect-uri", "relaisdesk://connect/DEV-ABCD-1234"})
	if cli || len(rest) != 2 {
		t.Fatalf("no flag: got cli=%v rest=%v", cli, rest)
	}
}

func TestFleetProtocolCommand(t *testing.T) {
	got := fleetProtocolCommand(`C:\Program Files\RelaisDesk\configurator.exe`)
	want := `"C:\Program Files\RelaisDesk\configurator.exe" --connect-uri "%1"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
