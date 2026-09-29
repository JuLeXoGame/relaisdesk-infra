package main

import "testing"

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

func TestFleetProtocolCommand(t *testing.T) {
	got := fleetProtocolCommand(`C:\Program Files\RelaisDesk\configurator.exe`)
	want := `"C:\Program Files\RelaisDesk\configurator.exe" --connect-uri "%1"`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
