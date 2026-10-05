package handlers

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	dbpkg "database"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"api/config"
	"api/middleware"
	"api/networkauth"
	"api/releasemanifest"
)

func TestDeviceEnrollAndHeartbeat(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "device-enroll.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('server-public-key-test', 1)`); err != nil {
		t.Fatal(err)
	}

	lic, err := dbpkg.CreateLicense(db, "fleet@example.com", 365, 3, "")
	if err != nil {
		t.Fatal(err)
	}

	dev, err := dbpkg.CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC Atelier", "Secteur A")
	if err != nil {
		t.Fatal(err)
	}

	settings := ServerSettings{
		ServerIP:       "198.51.100.1",
		RendezvousPort: 21116,
		RelayPort:      21117,
	}

	// 1. Enroll
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	_, serverKey, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := networkauth.NewSigner(serverKey, "fleet-test", 5*time.Minute)
	proof := apiFleetTestProof(key, "enroll", dev.PermanentCode, "123456789", "PC-ATELIER-1", "windows", false)
	enrollBody, _ := json.Marshal(map[string]any{
		"permanent_code":    dev.PermanentCode,
		"rustdesk_id":       "123456789",
		"hostname":          "PC-ATELIER-1",
		"os":                "windows",
		"device_public_key": public,
		"timestamp":         proof.Timestamp, "nonce": proof.Nonce, "signature": proof.Signature, "ready": proof.Ready,
	})
	enrollReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/enroll", bytes.NewReader(enrollBody))
	enrollReq.RemoteAddr = "203.0.113.42:54321"
	enrollRec := httptest.NewRecorder()

	DeviceEnrollHandler(db, settings, signer)(enrollRec, enrollReq)
	if enrollRec.Code != http.StatusOK {
		t.Fatalf("enroll status=%d body=%s", enrollRec.Code, enrollRec.Body.String())
	}

	var enrollResp deviceEnrollResponse
	if err := json.Unmarshal(enrollRec.Body.Bytes(), &enrollResp); err != nil {
		t.Fatal(err)
	}
	if !enrollResp.Valid || enrollResp.DeviceID != dev.DeviceID || enrollResp.ServerIP != "198.51.100.1" || enrollResp.NextIntervalSeconds != 45 {
		t.Fatalf("unexpected enroll response: %+v", enrollResp)
	}

	// 2. Heartbeat
	proof = apiFleetTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	hbBody, _ := json.Marshal(map[string]any{
		"device_id": dev.DeviceID,
		"timestamp": proof.Timestamp, "nonce": proof.Nonce, "signature": proof.Signature, "ready": proof.Ready,
	})
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", bytes.NewReader(hbBody))
	hbReq.RemoteAddr = "203.0.113.42:54321"
	hbRec := httptest.NewRecorder()

	DeviceHeartbeatHandler(db, &config.Config{}, signer)(hbRec, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbRec.Code, hbRec.Body.String())
	}
	var issued deviceEnrollResponse
	if err := json.Unmarshal(hbRec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.NetworkToken == "" {
		t.Fatal("missing network authorization")
	}
	if issued.NextIntervalSeconds != 45 {
		t.Fatalf("expected NextIntervalSeconds 45, got %d", issued.NextIntervalSeconds)
	}
	expiry, err := time.Parse(time.RFC3339, issued.ExpiresAt)
	if err != nil || expiry.After(time.Now().Add(5*time.Minute)) {
		t.Fatal("unbounded authorization")
	}
}

func apiFleetTestProof(key ed25519.PrivateKey, action, id, rd, host, osName string, ready bool) dbpkg.DeviceProof {
	nonce := make([]byte, 24)
	_, _ = rand.Read(nonce)
	p := dbpkg.DeviceProof{Timestamp: time.Now().Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ready: ready}
	public := base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	p.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, dbpkg.DeviceProofMessage(action, id, rd, host, osName, public, p)))
	return p
}

func TestFleetRejectsAnonymousHeartbeatAndUntrustedForwarding(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, _ := dbpkg.CreateLicense(db, "fleet-anon@example.invalid", 365, 3, "")
	dev, _ := dbpkg.CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC", "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", bytes.NewBufferString(`{"device_id":"`+dev.DeviceID+`"}`))
	rec := httptest.NewRecorder()
	DeviceHeartbeatHandler(db, &config.Config{})(rec, req)
	if rec.Code != 401 {
		t.Fatalf("anonymous status=%d", rec.Code)
	}
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "192.0.2.10")
	req.Header.Set("CF-Connecting-IP", "192.0.2.99")
	if ip := getDeviceClientIP(req); ip != "192.0.2.10" {
		t.Fatalf("loopback proxy IP=%s", ip)
	}
	req.RemoteAddr = "10.0.0.9:1234"
	if ip := getDeviceClientIP(req); ip != "10.0.0.9" {
		t.Fatalf("untrusted proxy accepted: %s", ip)
	}
}

func TestCustomerDeviceLifecycleAPI(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-devices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := dbpkg.CreateOrderWithBilling(db, "customer-fleet@example.com", "pro", 2, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Fleet Master", Address: "10 Avenue Tech", PostalCode: "75008", City: "Paris", CustomerType: "business",
		TermsVersion: publicTermsVersion, TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}

	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, "customer-fleet@example.com")
	if err != nil || !eligible {
		t.Fatal(err)
	}
	session, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}

	authWrap := func(h http.Handler) http.Handler {
		return middleware.CustomerAuth(db)(h)
	}

	// 1. Create enrollment code
	createBody, _ := json.Marshal(map[string]string{
		"license_id": lic.LicenseID,
		"alias":      "Serveur Principal",
		"notes":      "Salle serveurs",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/devices/enrollment-code", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+session)
	createRec := httptest.NewRecorder()
	authWrap(CustomerCreateDeviceEnrollmentHandler(db)).ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create device status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Device dbpkg.Device `json:"device"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatal(err)
	}
	deviceID := createResp.Device.DeviceID
	if deviceID == "" || createResp.Device.PermanentCode == "" {
		t.Fatalf("invalid device generated: %+v", createResp.Device)
	}

	// 2. List devices
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/customer/devices", nil)
	listReq.Header.Set("Authorization", "Bearer "+session)
	listRec := httptest.NewRecorder()
	authWrap(CustomerListDevicesHandler(db)).ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list devices status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Devices []dbpkg.Device     `json:"devices"`
		Quotas  []dbpkg.FleetQuota `json:"quotas"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Devices) != 1 || listResp.Devices[0].DeviceID != deviceID {
		t.Fatalf("unexpected list response: %+v", listResp)
	}
	if len(listResp.Quotas) != 1 || listResp.Quotas[0].Limit != 1000 || listResp.Quotas[0].Used != 1 || listResp.Quotas[0].Reserved != 1 {
		t.Fatalf("unexpected quota response: %+v", listResp.Quotas)
	}

	// 3. Update device alias
	updateBody, _ := json.Marshal(map[string]string{
		"alias": "Serveur Principal Modifié",
		"notes": "Notes modifiées",
	})
	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/customer/devices/"+deviceID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+session)
	updateRec := httptest.NewRecorder()
	authWrap(CustomerDeviceActionHandler(db)).ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update device status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	// 4. Delete device
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/customer/devices/"+deviceID, nil)
	delReq.Header.Set("Authorization", "Bearer "+session)
	delRec := httptest.NewRecorder()
	authWrap(CustomerDeviceActionHandler(db)).ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete device status=%d body=%s", delRec.Code, delRec.Body.String())
	}

	// 5. Verify list is empty
	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/customer/devices", nil)
	listReq2.Header.Set("Authorization", "Bearer "+session)
	listRec2 := httptest.NewRecorder()
	authWrap(CustomerListDevicesHandler(db)).ServeHTTP(listRec2, listReq2)
	var listResp2 struct {
		Devices []dbpkg.Device `json:"devices"`
	}
	json.Unmarshal(listRec2.Body.Bytes(), &listResp2)
	if len(listResp2.Devices) != 0 {
		t.Fatalf("expected 0 devices after deletion, got %d", len(listResp2.Devices))
	}
}

func TestTechnicianDeviceLifecycleAPI(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-devices.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := dbpkg.CreateLicense(db, "tech@example.com", 30, 2, "Tech License")
	if err != nil {
		t.Fatal(err)
	}

	session, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}

	authWrap := func(h http.Handler) http.Handler {
		return middleware.TechnicianAuth(db)(h)
	}

	// 1. Create enrollment
	createBody, _ := json.Marshal(map[string]string{
		"alias": "Poste Client Dupont",
		"notes": "Support Compta",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/devices/enrollment-code", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+session)
	createRec := httptest.NewRecorder()
	authWrap(TechnicianCreateDeviceEnrollmentHandler(db)).ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("technician create status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	var createResp struct {
		Device dbpkg.Device `json:"device"`
	}
	json.Unmarshal(createRec.Body.Bytes(), &createResp)
	deviceID := createResp.Device.DeviceID

	// 2. List
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/technician/devices", nil)
	listReq.Header.Set("Authorization", "Bearer "+session)
	listRec := httptest.NewRecorder()
	authWrap(TechnicianListDevicesHandler(db)).ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("technician list status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	// 3. Delete
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/technician/devices/"+deviceID, nil)
	delReq.Header.Set("Authorization", "Bearer "+session)
	delRec := httptest.NewRecorder()
	authWrap(TechnicianDeviceActionHandler(db)).ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("technician delete status=%d body=%s", delRec.Code, delRec.Body.String())
	}
}

func TestWakeDeviceEndpoints(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "wake-endpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('server-public-key-test', 1)`); err != nil {
		t.Fatal(err)
	}

	order, err := dbpkg.CreateOrderWithBilling(db, "wake-test@example.com", "pro", 5, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Wake Corp", CustomerType: "business", TermsVersion: publicTermsVersion, TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}

	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, "wake-test@example.com")
	if err != nil || !eligible {
		t.Fatal(err)
	}
	custSession, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}

	_, serverKey, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := networkauth.NewSigner(serverKey, "wake-test", 5*time.Minute)

	// 1. Create target device (sleeping)
	dev1, err := dbpkg.CreatePermanentEnrollment(db, lic.CustomerID, lic.LicenseID, "PC-Cible", "Bureau")
	if err != nil {
		t.Fatalf("dev1: %v", err)
	}
	pub1, key1, _ := ed25519.GenerateKey(rand.Reader)
	public1 := base64.RawURLEncoding.EncodeToString(pub1)
	proof1 := apiFleetTestProof(key1, "enroll", dev1.PermanentCode, "111222333", "PC-CIBLE", "windows", false)
	testMAC := "00:11:22:33:44:55"
	_, err = dbpkg.EnrollDeviceWithNetwork(db, dev1.PermanentCode, "111222333", "PC-CIBLE", "windows", public1, "192.168.1.50", testMAC, "192.168.1.255", proof1)
	if err != nil {
		t.Fatalf("enroll dev1: %v", err)
	}

	// 2. Create online peer
	dev2, err := dbpkg.CreatePermanentEnrollment(db, lic.CustomerID, lic.LicenseID, "PC-Relais", "Accueil")
	if err != nil {
		t.Fatalf("dev2: %v", err)
	}
	pub2, key2, _ := ed25519.GenerateKey(rand.Reader)
	public2 := base64.RawURLEncoding.EncodeToString(pub2)
	proof2 := apiFleetTestProof(key2, "enroll", dev2.PermanentCode, "444555666", "PC-RELAIS", "windows", false)
	_, err = dbpkg.EnrollDeviceWithNetwork(db, dev2.PermanentCode, "444555666", "PC-RELAIS", "windows", public2, "192.168.1.60", "AA:BB:CC:DD:EE:FF", "192.168.1.255", proof2)
	if err != nil {
		t.Fatalf("enroll dev2: %v", err)
	}
	// Make dev2 online
	hbProof2 := apiFleetTestProof(key2, "heartbeat", dev2.DeviceID, "", "", "", true)
	_ = dbpkg.DeviceHeartbeatWithNetwork(db, dev2.DeviceID, "192.168.1.60", "aa:bb:cc:dd:ee:ff", "192.168.1.255", hbProof2)

	// 3. Customer calls POST /api/v1/customer/devices/{id}/wake
	wakeReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/devices/"+dev1.DeviceID+"/wake", nil)
	wakeReq.Header.Set("Authorization", "Bearer "+custSession)
	wakeRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(CustomerDeviceActionHandler(db)).ServeHTTP(wakeRec, wakeReq)
	if wakeRec.Code != http.StatusOK {
		t.Fatalf("customer wake status=%d body=%s", wakeRec.Code, wakeRec.Body.String())
	}
	var wakeResp struct {
		Success          bool   `json:"success"`
		DeviceID         string `json:"device_id"`
		MACAddress       string `json:"mac_address"`
		OnlineRelayPeers int    `json:"online_relay_peers"`
	}
	if err := json.Unmarshal(wakeRec.Body.Bytes(), &wakeResp); err != nil {
		t.Fatalf("unmarshal wakeResp: %v", err)
	}
	if !wakeResp.Success || wakeResp.MACAddress != testMAC || wakeResp.OnlineRelayPeers != 1 {
		t.Fatalf("unexpected wakeResp: %+v", wakeResp)
	}

	// 4. Device 2 heartbeats and receives WakeTargets
	hbProofDev2 := apiFleetTestProof(key2, "heartbeat", dev2.DeviceID, "", "", "", true)
	hbReqBody, _ := json.Marshal(map[string]any{
		"device_id": dev2.DeviceID,
		"timestamp": hbProofDev2.Timestamp, "nonce": hbProofDev2.Nonce, "signature": hbProofDev2.Signature, "ready": hbProofDev2.Ready,
	})
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", bytes.NewReader(hbReqBody))
	hbReq.RemoteAddr = "203.0.113.42:54321"
	hbRec := httptest.NewRecorder()
	DeviceHeartbeatHandler(db, &config.Config{}, signer)(hbRec, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("dev2 heartbeat status=%d body=%s", hbRec.Code, hbRec.Body.String())
	}
	var hbResp deviceEnrollResponse
	if err := json.Unmarshal(hbRec.Body.Bytes(), &hbResp); err != nil {
		t.Fatalf("unmarshal hbResp: %v", err)
	}
	if len(hbResp.WakeTargets) != 1 || hbResp.WakeTargets[0] != testMAC {
		t.Fatalf("expected WakeTargets [%s], got %v", testMAC, hbResp.WakeTargets)
	}
}

func TestDeviceRemoteUpdateAPI(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "update-endpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('server-public-key-test', 1)`); err != nil {
		t.Fatal(err)
	}

	order, err := dbpkg.CreateOrderWithBilling(db, "update-test@example.com", "pro", 5, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Update Corp", CustomerType: "business", TermsVersion: publicTermsVersion, TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}

	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, "update-test@example.com")
	if err != nil || !eligible {
		t.Fatal(err)
	}
	custSession, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}

	_, serverKey, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := networkauth.NewSigner(serverKey, "update-test", 5*time.Minute)

	// 1. Create target device with agent_version 1.0.0
	dev, err := dbpkg.CreatePermanentEnrollment(db, lic.CustomerID, lic.LicenseID, "PC-Update-Target", "Bureau")
	if err != nil {
		t.Fatalf("dev: %v", err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	proof := apiFleetTestProof(key, "enroll", dev.PermanentCode, "999888777", "PC-UPDATE", "windows", false)
	_, err = dbpkg.EnrollDeviceWithFullInfo(db, dev.PermanentCode, "999888777", "PC-UPDATE", "windows", public, "192.168.1.50", "00:11:22:33:44:55", "192.168.1.255", "1.0.0", proof)
	if err != nil {
		t.Fatalf("enroll dev: %v", err)
	}

	// 2. Customer triggers update POST /api/v1/customer/devices/{id}/update
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/devices/"+dev.DeviceID+"/update", bytes.NewReader([]byte(`{"target_version":"1.0.5"}`)))
	updateReq.Header.Set("Authorization", "Bearer "+custSession)
	updateRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(CustomerDeviceActionHandler(db)).ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("customer update status=%d body=%s", updateRec.Code, updateRec.Body.String())
	}

	// 3. Second call should return 409 Conflict
	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/devices/"+dev.DeviceID+"/update", nil)
	dupReq.Header.Set("Authorization", "Bearer "+custSession)
	dupRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(CustomerDeviceActionHandler(db)).ServeHTTP(dupRec, dupReq)
	if dupRec.Code != http.StatusConflict {
		t.Fatalf("duplicate update expected 409, got %d body=%s", dupRec.Code, dupRec.Body.String())
	}

	// 4. Device sends heartbeat -> should receive UpdateTarget
	hbProof := apiFleetTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	hbReqBody, _ := json.Marshal(map[string]any{
		"device_id":     dev.DeviceID,
		"agent_version": "1.0.0",
		"timestamp":     hbProof.Timestamp,
		"nonce":         hbProof.Nonce,
		"signature":     hbProof.Signature,
		"ready":         hbProof.Ready,
	})
	hbReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", bytes.NewReader(hbReqBody))
	hbReq.RemoteAddr = "203.0.113.42:54321"
	hbRec := httptest.NewRecorder()
	DeviceHeartbeatHandler(db, &config.Config{ReleasePublicKey: "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo", ReleaseKeyID: "release-1"}, signer)(hbRec, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hbRec.Code, hbRec.Body.String())
	}
	var hbResp deviceEnrollResponse
	if err := json.Unmarshal(hbRec.Body.Bytes(), &hbResp); err != nil {
		t.Fatalf("unmarshal hbResp: %v", err)
	}
	if hbResp.UpdateTarget == nil {
		t.Fatal("expected non-nil UpdateTarget in heartbeat response")
	}
	if hbResp.UpdateTarget.Version != "1.0.5" {
		t.Fatalf("expected target version 1.0.5, got %s", hbResp.UpdateTarget.Version)
	}
	if hbResp.UpdateTarget.URL == "" {
		t.Fatal("expected non-empty UpdateTarget URL")
	}
	if hbResp.UpdateTarget.StaggerSeconds < 5 || hbResp.UpdateTarget.StaggerSeconds > 60 {
		t.Fatalf("expected StaggerSeconds between 5 and 60, got %d", hbResp.UpdateTarget.StaggerSeconds)
	}

	// 5. Subsequent heartbeat after update applied with agent_version 1.0.5 -> UpdateTarget should be nil
	hbProof2 := apiFleetTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	hbReqBody2, _ := json.Marshal(map[string]any{
		"device_id":     dev.DeviceID,
		"agent_version": "1.0.5",
		"timestamp":     hbProof2.Timestamp,
		"nonce":         hbProof2.Nonce,
		"signature":     hbProof2.Signature,
		"ready":         hbProof2.Ready,
	})
	hbReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/devices/heartbeat", bytes.NewReader(hbReqBody2))
	hbReq2.RemoteAddr = "203.0.113.42:54321"
	hbRec2 := httptest.NewRecorder()
	DeviceHeartbeatHandler(db, &config.Config{ReleasePublicKey: "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo", ReleaseKeyID: "release-1"}, signer)(hbRec2, hbReq2)
	if hbRec2.Code != http.StatusOK {
		t.Fatalf("heartbeat 2 status=%d body=%s", hbRec2.Code, hbRec2.Body.String())
	}
	var hbResp2 deviceEnrollResponse
	if err := json.Unmarshal(hbRec2.Body.Bytes(), &hbResp2); err != nil {
		t.Fatalf("unmarshal hbResp2: %v", err)
	}
	if hbResp2.UpdateTarget != nil {
		t.Fatalf("expected nil UpdateTarget after update completed, got %+v", hbResp2.UpdateTarget)
	}

	// 6. Test Technician Update trigger
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	techReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/devices/"+dev.DeviceID+"/update", bytes.NewReader([]byte(`{"target_version":"1.0.6"}`)))
	techReq.Header.Set("Authorization", "Bearer "+techSession)
	techRec := httptest.NewRecorder()
	middleware.TechnicianAuth(db)(TechnicianDeviceActionHandler(db)).ServeHTTP(techRec, techReq)
	if techRec.Code != http.StatusOK {
		t.Fatalf("technician update status=%d body=%s", techRec.Code, techRec.Body.String())
	}
}

func TestDeviceConnectionInterventionTracking(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "device-connect-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := dbpkg.CreateOrderWithBilling(db, "customer-connect@example.com", "pro", 2, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Fleet Master", Address: "10 Avenue Tech", PostalCode: "75008", City: "Paris", CustomerType: "business",
		TermsVersion: publicTermsVersion, TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}

	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, "customer-connect@example.com")
	if err != nil || !eligible {
		t.Fatal(err)
	}
	sessionToken, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}

	cust, err := dbpkg.GetCustomerIdentityByEmail(db, "customer-connect@example.com")
	if err != nil {
		t.Fatal(err)
	}

	dev, err := dbpkg.CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "Serveur Principal", "Notes test")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Customer calls POST /api/v1/customer/devices/{id}/connect
	connectReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/devices/"+dev.DeviceID+"/connect", nil)
	connectReq.Header.Set("Authorization", "Bearer "+sessionToken)
	connectRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(CustomerDeviceActionHandler(db)).ServeHTTP(connectRec, connectReq)
	if connectRec.Code != http.StatusOK {
		t.Fatalf("customer connect status=%d body=%s", connectRec.Code, connectRec.Body.String())
	}

	var connectResp struct {
		Success      bool               `json:"success"`
		DeviceID     string             `json:"device_id"`
		Alias        string             `json:"alias"`
		Intervention dbpkg.Intervention `json:"intervention"`
	}
	if err := json.Unmarshal(connectRec.Body.Bytes(), &connectResp); err != nil {
		t.Fatalf("unmarshal connectResp: %v", err)
	}
	if !connectResp.Success || connectResp.Intervention.InterventionID == "" {
		t.Fatalf("invalid connect response: %+v", connectResp)
	}
	if connectResp.Intervention.Status != "in_progress" {
		t.Fatalf("expected in_progress status, got %s", connectResp.Intervention.Status)
	}
	if connectResp.Intervention.StartedAt == nil {
		t.Fatal("expected non-nil started_at")
	}

	// 2. Technician completes intervention
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/interventions/"+connectResp.Intervention.InterventionID+"/complete", bytes.NewReader([]byte(`{"summary":"Intervention test terminee"}`)))
	completeReq.Header.Set("Authorization", "Bearer "+techSession)
	completeRec := httptest.NewRecorder()
	middleware.TechnicianAuth(db)(TechnicianInterventionActionHandler(db)).ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("technician complete status=%d body=%s", completeRec.Code, completeRec.Body.String())
	}

	var completedItem dbpkg.Intervention
	if err := json.Unmarshal(completeRec.Body.Bytes(), &completedItem); err != nil {
		t.Fatalf("unmarshal completedItem: %v", err)
	}
	if completedItem.Status != "completed" {
		t.Fatalf("expected completed status, got %s", completedItem.Status)
	}
	if completedItem.DurationMinutes == nil || *completedItem.DurationMinutes < 1 {
		t.Fatalf("expected duration >= 1 min, got %+v", completedItem.DurationMinutes)
	}

	// 3. Customer lists interventions and finds the newly completed one
	list, err := dbpkg.ListInterventionsByCustomerID(db, cust.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 intervention in history, got %d", len(list))
	}
	if list[0].InterventionID != connectResp.Intervention.InterventionID {
		t.Fatalf("expected intervention ID %s, got %s", connectResp.Intervention.InterventionID, list[0].InterventionID)
	}
	if list[0].Status != "completed" {
		t.Fatalf("expected list item status completed, got %s", list[0].Status)
	}
}

func TestViewerCodeConnectInterventionTracking(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "viewer-connect-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := dbpkg.CreateLicense(db, "tech-viewer@example.com", 365, 3, "")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create viewer code -> intervention created in 'planned' state
	vc, err := dbpkg.CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Client validates viewer code -> intervention becomes 'client_ready'
	if err := dbpkg.MarkViewerInterventionReady(db, vc.ID); err != nil {
		t.Fatal(err)
	}

	// 3. Technician calls POST /api/v1/technician/viewer-codes/{code}/connect -> intervention becomes 'in_progress'
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	connectReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/viewer-codes/"+vc.Code+"/connect", nil)
	connectReq.Header.Set("Authorization", "Bearer "+techSession)
	connectRec := httptest.NewRecorder()
	middleware.TechnicianAuth(db)(TechnicianConnectCodeHandler(db)).ServeHTTP(connectRec, connectReq)
	if connectRec.Code != http.StatusOK {
		t.Fatalf("technician viewer connect status=%d body=%s", connectRec.Code, connectRec.Body.String())
	}

	var connectResp struct {
		Success      bool               `json:"success"`
		Code         string             `json:"code"`
		Intervention dbpkg.Intervention `json:"intervention"`
	}
	if err := json.Unmarshal(connectRec.Body.Bytes(), &connectResp); err != nil {
		t.Fatalf("unmarshal connectResp: %v", err)
	}
	if !connectResp.Success || connectResp.Intervention.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %+v", connectResp)
	}
	if connectResp.Intervention.StartedAt == nil {
		t.Fatal("expected non-nil started_at")
	}

	// 4. Technician completes intervention
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/interventions/"+connectResp.Intervention.InterventionID+"/complete", bytes.NewReader([]byte(`{"summary":"Assistance client terminee"}`)))
	completeReq.Header.Set("Authorization", "Bearer "+techSession)
	completeRec := httptest.NewRecorder()
	middleware.TechnicianAuth(db)(TechnicianInterventionActionHandler(db)).ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("technician complete status=%d body=%s", completeRec.Code, completeRec.Body.String())
	}

	var completedItem dbpkg.Intervention
	if err := json.Unmarshal(completeRec.Body.Bytes(), &completedItem); err != nil {
		t.Fatalf("unmarshal completedItem: %v", err)
	}
	if completedItem.Status != "completed" {
		t.Fatalf("expected completed, got %s", completedItem.Status)
	}
	if completedItem.DurationMinutes == nil || *completedItem.DurationMinutes < 1 {
		t.Fatalf("expected duration >= 1, got %+v", completedItem.DurationMinutes)
	}
}

func TestViewerCodeReconnectAfterCompleteOpensNewIntervention(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "viewer-reconnect-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := dbpkg.CreateLicense(db, "tech-reconnect@example.com", 365, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	vc, err := dbpkg.CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.MarkViewerInterventionReady(db, vc.ID); err != nil {
		t.Fatal(err)
	}
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	connect := func() dbpkg.Intervention {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/viewer-codes/"+vc.Code+"/connect", nil)
		req.Header.Set("Authorization", "Bearer "+techSession)
		rec := httptest.NewRecorder()
		middleware.TechnicianAuth(db)(TechnicianConnectCodeHandler(db)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Success      bool               `json:"success"`
			Intervention dbpkg.Intervention `json:"intervention"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal connect: %v", err)
		}
		if !resp.Success || resp.Intervention.Status != "in_progress" {
			t.Fatalf("expected in_progress, got %+v", resp)
		}
		return resp.Intervention
	}
	complete := func(id string) dbpkg.Intervention {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/interventions/"+id+"/complete", bytes.NewReader([]byte(`{"summary":"Fin de session"}`)))
		req.Header.Set("Authorization", "Bearer "+techSession)
		rec := httptest.NewRecorder()
		middleware.TechnicianAuth(db)(TechnicianInterventionActionHandler(db)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
		}
		var item dbpkg.Intervention
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatalf("unmarshal complete: %v", err)
		}
		if item.Status != "completed" {
			t.Fatalf("expected completed, got %s", item.Status)
		}
		return item
	}

	first := connect()
	complete(first.InterventionID)
	// Reconnecting after the closure opens a new fiche instead of failing.
	second := connect()
	if second.InterventionID == first.InterventionID {
		t.Fatalf("expected a new intervention, got the same %s", first.InterventionID)
	}
	complete(second.InterventionID)
}

func TestCompleteInterventionRefreshesEndOnRepeat(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "viewer-recomplete-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := dbpkg.CreateLicense(db, "tech-recomplete@example.com", 365, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	vc, err := dbpkg.CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.MarkViewerInterventionReady(db, vc.ID); err != nil {
		t.Fatal(err)
	}
	techSession, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	complete := func(id string) dbpkg.Intervention {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/interventions/"+id+"/complete", bytes.NewReader([]byte(`{"summary":"Fin de session"}`)))
		req.Header.Set("Authorization", "Bearer "+techSession)
		rec := httptest.NewRecorder()
		middleware.TechnicianAuth(db)(TechnicianInterventionActionHandler(db)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
		}
		var item dbpkg.Intervention
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatalf("unmarshal complete: %v", err)
		}
		return item
	}
	// First completion goes through Start to reach a normal in_progress fiche.
	connectReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/viewer-codes/"+vc.Code+"/connect", nil)
	connectReq.Header.Set("Authorization", "Bearer "+techSession)
	connectRec := httptest.NewRecorder()
	middleware.TechnicianAuth(db)(TechnicianConnectCodeHandler(db)).ServeHTTP(connectRec, connectReq)
	if connectRec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", connectRec.Code, connectRec.Body.String())
	}
	var connectResp struct {
		Intervention dbpkg.Intervention `json:"intervention"`
	}
	if err := json.Unmarshal(connectRec.Body.Bytes(), &connectResp); err != nil {
		t.Fatalf("unmarshal connect: %v", err)
	}
	first := complete(connectResp.Intervention.InterventionID)
	if first.EndedAt == nil {
		t.Fatal("expected ended_at after completion")
	}
	time.Sleep(1200 * time.Millisecond)
	second := complete(connectResp.Intervention.InterventionID)
	if second.EndedAt == nil || !second.EndedAt.After(*first.EndedAt) {
		t.Fatalf("expected refreshed ended_at, got first=%v second=%v", first.EndedAt, second.EndedAt)
	}
}

func TestUpdateTargetFromManifest(t *testing.T) {
	if got := updateTargetFromManifest(nil, "windows", "1.0.5", 15); got != nil {
		t.Fatalf("nil manifest = %+v, want nil", got)
	}
	empty := &releasemanifest.Manifest{Version: "1.0.0"}
	if got := updateTargetFromManifest(empty, "windows", "1.0.5", 15); got != nil {
		t.Fatalf("missing artifact = %+v, want nil", got)
	}
	full := &releasemanifest.Manifest{Version: "1.0.0", Artifacts: []releasemanifest.Artifact{
		{Name: "RelaisDesk_Portable.exe", URL: "https://example.com/p.exe", SHA256: "abc123"},
		{Name: "RelaisDesk_viewer.deb", URL: "https://example.com/v.deb", SHA256: "def456"},
	}}
	win := updateTargetFromManifest(full, "windows", "1.0.5", 15)
	if win == nil || win.SHA256 != "abc123" || win.Version != "1.0.5" || win.StaggerSeconds != 15 {
		t.Fatalf("windows target = %+v", win)
	}
	lin := updateTargetFromManifest(full, "linux", "", 15)
	if lin == nil || lin.SHA256 != "def456" || lin.Version != "1.0.0" {
		t.Fatalf("linux target = %+v", lin)
	}
}

func TestTechnicianParkTokenAPI(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-park.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := dbpkg.CreateLicense(db, "park-tech@example.com", 30, 2, "Park")
	if err != nil {
		t.Fatal(err)
	}
	session, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	authWrap := func(h http.Handler) http.Handler {
		return middleware.TechnicianAuth(db)(h)
	}
	do := func(h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+session)
		rec := httptest.NewRecorder()
		authWrap(h).ServeHTTP(rec, req)
		return rec
	}

	// Create.
	rec := do(TechnicianCreateParkTokenHandler(db), http.MethodPost, "/api/v1/technician/device-park-tokens", map[string]any{
		"label": "Vague 1", "max_uses": 50, "ttl_days": 7,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		Token     string `json:"token"`
		ParkToken struct {
			ID      int64  `json:"id"`
			MaxUses int    `json:"max_uses"`
			Label   string `json:"label"`
		} `json:"park_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Token) < 20 || created.ParkToken.MaxUses != 50 {
		t.Fatalf("created = %+v", created)
	}

	// List (metadata only, no plaintext).
	rec = do(TechnicianListParkTokensHandler(db), http.MethodGet, "/api/v1/technician/device-park-tokens", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	var listed struct {
		ParkTokens []map[string]any `json:"park_tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.ParkTokens) != 1 {
		t.Fatalf("tokens = %d, want 1", len(listed.ParkTokens))
	}
	if _, hasToken := listed.ParkTokens[0]["token"]; hasToken {
		t.Fatal("le clair fuite dans le list")
	}

	// Revoke.
	revokePath := "/api/v1/technician/device-park-tokens/" + strconv.FormatInt(created.ParkToken.ID, 10) + "/revoke"
	rec = do(TechnicianRevokeParkTokenHandler(db), http.MethodPut, revokePath, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = do(TechnicianListParkTokensHandler(db), http.MethodGet, "/api/v1/technician/device-park-tokens", nil)
	var listed2 struct {
		ParkTokens []struct {
			IsActive bool `json:"is_active"`
		} `json:"park_tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed2); err != nil {
		t.Fatal(err)
	}
	if len(listed2.ParkTokens) != 1 || listed2.ParkTokens[0].IsActive {
		t.Fatalf("après revoke: %+v", listed2.ParkTokens)
	}
}

// A technician session is scoped to its own licence: a foreign license_id in
// the body or query string must never mint or list another licence's tokens.
func TestTechnicianParkTokenIgnoresForeignLicense(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-park-scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	licA, err := dbpkg.CreateLicense(db, "scope-a@example.com", 30, 2, "A")
	if err != nil {
		t.Fatal(err)
	}
	licB, err := dbpkg.CreateLicense(db, "scope-b@example.com", 30, 2, "B")
	if err != nil {
		t.Fatal(err)
	}
	session, err := dbpkg.CreateTechnicianSession(db, licA.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	do := func(h http.HandlerFunc, method, path string, body any) *httptest.ResponseRecorder {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+session)
		rec := httptest.NewRecorder()
		middleware.TechnicianAuth(db)(h).ServeHTTP(rec, req)
		return rec
	}
	rec := do(TechnicianCreateParkTokenHandler(db), http.MethodPost, "/api/v1/technician/device-park-tokens", map[string]any{
		"license_id": licB.LicenseID, "label": "tentative", "max_uses": 5, "ttl_days": 7,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d (%s)", rec.Code, rec.Body.String())
	}
	var created struct {
		ParkToken struct {
			LicenseID string `json:"license_id"`
		} `json:"park_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ParkToken.LicenseID != licA.LicenseID {
		t.Fatalf("token créé pour %q, want %q (contexte technicien)", created.ParkToken.LicenseID, licA.LicenseID)
	}
	rec = do(TechnicianListParkTokensHandler(db), http.MethodGet, "/api/v1/technician/device-park-tokens?license_id="+licB.LicenseID, nil)
	var listed struct {
		ParkTokens []struct {
			LicenseID string `json:"license_id"`
		} `json:"park_tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.ParkTokens) != 1 || listed.ParkTokens[0].LicenseID != licA.LicenseID {
		t.Fatalf("list avec license_id étrangère = %+v, want les tokens de %q", listed.ParkTokens, licA.LicenseID)
	}
}
