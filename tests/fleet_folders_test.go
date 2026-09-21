package tests

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"api/handlers"
	"api/middleware"
)

func TestFleetFoldersAPI(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "fleet_folders_api.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	settings := handlers.ServerSettings{
		ServerIP:       "127.0.0.1",
		RendezvousPort: 21116,
		RelayPort:      21117,
	}

	// 1. Setup license & technician session
	lic, err := dbpkg.CreateLicense(db, "tech-folders@example.com", 30, 2, "Pro")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatalf("CreateTechnicianSession failed: %v", err)
	}

	mux := http.NewServeMux()
	techAuth := middleware.TechnicianAuth(db)

	mux.Handle("/api/v1/technician/devices", techAuth(http.HandlerFunc(handlers.TechnicianListDevicesHandler(db))))
	mux.Handle("/api/v1/technician/devices/enrollment-code", techAuth(http.HandlerFunc(handlers.TechnicianCreateDeviceEnrollmentHandler(db))))
	mux.Handle("/api/v1/technician/devices/", techAuth(http.HandlerFunc(handlers.TechnicianDeviceActionHandler(db))))
	mux.Handle("/api/v1/technician/device-folders", techAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.TechnicianListFoldersHandler(db)(w, r)
			return
		}
		if r.Method == http.MethodPost {
			handlers.TechnicianCreateFolderHandler(db)(w, r)
			return
		}
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})))
	mux.Handle("/api/v1/technician/device-folders/", techAuth(http.HandlerFunc(handlers.TechnicianFolderActionHandler(db))))

	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := ts.Client()

	// Helper for technician authenticated requests
	authReq := func(method, path string, body any) *http.Response {
		var reqBody []byte
		if body != nil {
			reqBody, _ = json.Marshal(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer "+techSession)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		return resp
	}

	// 2. Create Root Folder "Bureau Principal"
	resp := authReq(http.MethodPost, "/api/v1/technician/device-folders", map[string]string{
		"name": "Bureau Principal",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for root folder, got %d", resp.StatusCode)
	}
	var createRootResp struct {
		Folder dbpkg.DeviceFolder `json:"folder"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createRootResp)
	rootFolderID := createRootResp.Folder.FolderID
	if rootFolderID == "" || createRootResp.Folder.Name != "Bureau Principal" {
		t.Fatalf("Unexpected root folder created: %+v", createRootResp.Folder)
	}

	// 3. Create Subfolder "Serveurs & Baies" under root folder
	resp = authReq(http.MethodPost, "/api/v1/technician/device-folders", map[string]string{
		"name":             "Serveurs & Baies",
		"parent_folder_id": rootFolderID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for subfolder, got %d", resp.StatusCode)
	}
	var createSubResp struct {
		Folder dbpkg.DeviceFolder `json:"folder"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createSubResp)
	subFolderID := createSubResp.Folder.FolderID
	if subFolderID == "" || createSubResp.Folder.ParentFolderID != rootFolderID {
		t.Fatalf("Unexpected subfolder: %+v", createSubResp.Folder)
	}

	// 4. List Folders
	resp = authReq(http.MethodGet, "/api/v1/technician/device-folders", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK listing folders, got %d", resp.StatusCode)
	}
	var listFoldersResp struct {
		Folders []dbpkg.DeviceFolder `json:"folders"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listFoldersResp)
	if len(listFoldersResp.Folders) != 2 {
		t.Fatalf("Expected 2 folders, got %d", len(listFoldersResp.Folders))
	}

	// 5. Enroll a permanent device
	resp = authReq(http.MethodPost, "/api/v1/technician/devices/enrollment-code", map[string]string{
		"alias": "SRV-WEB-01",
		"notes": "Serveur de production",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 Created for device enrollment, got %d", resp.StatusCode)
	}
	var enrollDevResp struct {
		Device dbpkg.Device `json:"device"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&enrollDevResp)
	deviceID := enrollDevResp.Device.DeviceID

	// 6. Assign device to subfolder
	resp = authReq(http.MethodPut, "/api/v1/technician/devices/"+deviceID, map[string]string{
		"alias":     "SRV-WEB-01-PROD",
		"notes":     "Serveur de production web",
		"folder_id": subFolderID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK updating device folder, got %d", resp.StatusCode)
	}

	// 7. Get device page (should include devices with folder_id and folders list)
	resp = authReq(http.MethodGet, "/api/v1/technician/devices", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK getting devices page, got %d", resp.StatusCode)
	}
	var devPageResp struct {
		Devices []dbpkg.Device       `json:"devices"`
		Folders []dbpkg.DeviceFolder `json:"folders"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&devPageResp)
	if len(devPageResp.Devices) != 1 || devPageResp.Devices[0].FolderID != subFolderID {
		t.Fatalf("Device folder_id mismatch in page: %+v", devPageResp.Devices)
	}
	if len(devPageResp.Folders) != 2 {
		t.Fatalf("Folders array missing or incomplete in devices page: %+v", devPageResp.Folders)
	}

	// 8. Rename subfolder
	resp = authReq(http.MethodPut, "/api/v1/technician/device-folders/"+subFolderID, map[string]string{
		"name": "Datacenter & Baies",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK renaming folder, got %d", resp.StatusCode)
	}

	// 9. Delete Root Folder -> must delete root and subfolder, and device's folder_id must revert to ""
	resp = authReq(http.MethodDelete, "/api/v1/technician/device-folders/"+rootFolderID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK deleting root folder, got %d", resp.StatusCode)
	}

	// Check folders list is empty
	resp = authReq(http.MethodGet, "/api/v1/technician/device-folders", nil)
	var emptyFoldersResp struct {
		Folders []dbpkg.DeviceFolder `json:"folders"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&emptyFoldersResp)
	if len(emptyFoldersResp.Folders) != 0 {
		t.Fatalf("Expected 0 folders remaining after cascade delete, got %d", len(emptyFoldersResp.Folders))
	}

	// Check device folder_id is reset to ""
	resp = authReq(http.MethodGet, "/api/v1/technician/devices", nil)
	var finalPageResp struct {
		Devices []dbpkg.Device `json:"devices"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&finalPageResp)
	if len(finalPageResp.Devices) != 1 || finalPageResp.Devices[0].FolderID != "" {
		t.Fatalf("Device folder_id should have been reset to empty string, got: %+v", finalPageResp.Devices)
	}

	_ = settings
	_ = time.Now()
}
