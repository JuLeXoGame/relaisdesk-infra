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
