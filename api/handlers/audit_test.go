package handlers

import (
	"bytes"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAuditLogFormat(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	r := httptest.NewRequest("POST", "/api/v1/admin/licences", nil)
	r.RemoteAddr = "203.0.113.7:1234"
	auditLog(r, "TEST ACTION", "détail sensible")
	out := buf.String()
	if !strings.Contains(out, "[AUDIT TEST ACTION]") || !strings.Contains(out, "détail sensible") {
		t.Fatalf("audit = %q", out)
	}
	if !strings.Contains(out, "203.0.113.7") {
		t.Fatalf("IP appelante absente: %q", out)
	}
}
