package handlers

import (
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewerActivationReturnsConfiguredPorts(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "viewer-activation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('public-key', 1)`); err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.CreateLicense(db, "viewer-port@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	code, err := dbpkg.CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ViewerActivateHandler(db, ServerSettings{
		ServerIP: "relay.example.com", RendezvousPort: 22116, RelayPort: 22117,
	})(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/viewer/activate", strings.NewReader(`{"code":"`+code.Code+`"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response viewerActivateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RendezvousPort != 22116 || response.RelayPort != 22117 {
		t.Fatalf("unexpected ports: %+v", response)
	}
}
