package servicelegal

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPublishedServiceTermsMatchAcceptedDocument(t *testing.T) {
	b, err := os.ReadFile("../../relaisdesk/prestations/conditions-2026-09-19.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte(Document)) {
		t.Fatal("published copy differs from accepted document")
	}
	js, err := os.ReadFile("../../relaisdesk/client/services.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), Hash()) || !strings.Contains(string(js), Version) {
		t.Fatal("portal must pin the terms version and digest")
	}
}
