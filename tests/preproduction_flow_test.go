package tests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/config"
	"api/handlers"
	"api/middleware"
	"api/networkauth"
	dbpkg "database"
)

// TestPreproductionPurchaseActivationAndConnectionAuthorization covers the
// commercial path up to the signed authorizations consumed by the RustDesk
// forks. The fork's own tests cover enforcement of those authorizations and
// of the simultaneous-session quota.
func TestPreproductionPurchaseActivationAndConnectionAuthorization(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "preproduction.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO server_keys (public_key, is_active) VALUES (?, 1)", strings.Repeat("R", 43)); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		BankIBAN: "FR76 3000 6000 0112 3456 7890 189", BankBIC: "AGRIFRPP", BankHolder: "RelaisDesk",
		DevHTTP: true, InvoicesDir: t.TempDir(),
	}
	settings := handlers.ServerSettings{ServerIP: "api.example.test", RendezvousPort: 21116, RelayPort: 21117}

	orderResponse := postPreproductionJSON(t, handlers.PublicOrderHandler(db, cfg, nil), "/api/v1/public/order", map[string]any{
		"email": "preprod@example.test", "plan": "pro", "technicians": 3, "payment_method": "bank_transfer",
		"name": "Client Préproduction", "address": "1 rue du Test", "postal_code": "75001", "city": "Paris",
		"customer_type": "business", "terms_version": "2026-09-27", "terms_accepted": true,
	}, "")
	if orderResponse.Code != http.StatusCreated {
		t.Fatalf("purchase status = %d: %s", orderResponse.Code, orderResponse.Body.String())
	}
	var order struct {
		OrderID string `json:"order_id"`
	}
	decodePreproductionResponse(t, orderResponse, &order)

	paidRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+order.OrderID+"/mark-paid", nil)
	paidResponse := httptest.NewRecorder()
	handlers.AdminMarkOrderPaidHandler(db, cfg, nil)(paidResponse, paidRequest)
	if paidResponse.Code != http.StatusOK {
		t.Fatalf("payment fulfillment status = %d: %s", paidResponse.Code, paidResponse.Body.String())
	}
	var paid struct {
		LicenseID  string `json:"license_id"`
		LicenseKey string `json:"license_key"`
	}
	decodePreproductionResponse(t, paidResponse, &paid)
	ordersResponse := httptest.NewRecorder()
	handlers.AdminListOrdersHandler(db)(ordersResponse, httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil))
	if ordersResponse.Code != http.StatusOK {
		t.Fatalf("order tracking status = %d: %s", ordersResponse.Code, ordersResponse.Body.String())
	}
	var tracked struct {
		Orders []struct {
			PaymentComplete  bool `json:"payment_complete"`
			LicenseCreated   bool `json:"license_created"`
			InvoiceCreated   bool `json:"invoice_created"`
			LicenseEmailSent bool `json:"license_email_sent"`
			InvoiceEmailSent bool `json:"invoice_email_sent"`
			FulfillmentDone  bool `json:"fulfillment_done"`
		} `json:"orders"`
	}
	decodePreproductionResponse(t, ordersResponse, &tracked)
	if len(tracked.Orders) != 1 || !tracked.Orders[0].PaymentComplete || !tracked.Orders[0].LicenseCreated ||
		!tracked.Orders[0].InvoiceCreated || tracked.Orders[0].LicenseEmailSent || tracked.Orders[0].InvoiceEmailSent || tracked.Orders[0].FulfillmentDone {
		t.Fatalf("unexpected fulfillment stages: %+v", tracked.Orders)
	}
	retryResponse := httptest.NewRecorder()
	handlers.AdminMarkOrderPaidHandler(db, cfg, nil)(retryResponse,
		httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+order.OrderID+"/retry-fulfillment", nil))
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("idempotent fulfillment retry status = %d: %s", retryResponse.Code, retryResponse.Body.String())
	}
	invoices, err := dbpkg.ListInvoices(db, order.OrderID, 0, 0)
	if err != nil || len(invoices) != 1 {
		t.Fatalf("invoice count after retry = %d, %v; want exactly one", len(invoices), err)
	}

	loginResponse := postPreproductionJSON(t, handlers.TechnicianLoginHandler(db, settings), "/api/v1/technician/login", map[string]any{
		"license_id": paid.LicenseID, "license_key": paid.LicenseKey,
	}, "")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("technician activation status = %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	var login struct {
		Token string `json:"token"`
	}
	decodePreproductionResponse(t, loginResponse, &login)

	generateHandler := middleware.TechnicianAuth(db)(handlers.TechnicianGenerateCodeHandler(db))
	viewerCodeResponse := postPreproductionJSON(t, generateHandler.ServeHTTP, "/api/v1/technician/viewer-codes/generate", map[string]any{
		"client_email": "viewer@example.test",
	}, login.Token)
	if viewerCodeResponse.Code != http.StatusOK {
		t.Fatalf("viewer code status = %d: %s", viewerCodeResponse.Code, viewerCodeResponse.Body.String())
	}
	var viewerCode struct {
		Code string `json:"code"`
	}
	decodePreproductionResponse(t, viewerCodeResponse, &viewerCode)

	viewerActivation := postPreproductionJSON(t, handlers.ViewerActivateHandler(db, settings), "/api/v1/viewer/activate", map[string]any{"code": viewerCode.Code}, "")
	if viewerActivation.Code != http.StatusOK {
		t.Fatalf("viewer activation status = %d: %s", viewerActivation.Code, viewerActivation.Body.String())
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := networkauth.NewSigner(privateKey, "preprod-1", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	devicePublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encodedDeviceKey := base64.RawURLEncoding.EncodeToString(devicePublicKey)
	technicianNetworkHandler := middleware.TechnicianAuth(db)(handlers.TechnicianNetworkTokenHandler(db, signer))
	technicianNetworkResponse := postPreproductionJSON(t, technicianNetworkHandler.ServeHTTP, "/api/v1/technician/network-token", map[string]any{
		"device_public_key": encodedDeviceKey,
	}, login.Token)
	if technicianNetworkResponse.Code != http.StatusOK {
		t.Fatalf("technician network authorization status = %d: %s", technicianNetworkResponse.Code, technicianNetworkResponse.Body.String())
	}
	assertSignedNetworkToken(t, technicianNetworkResponse, publicKey, 3, "technician")

	viewerNetworkResponse := postPreproductionJSON(t, handlers.ViewerNetworkTokenHandler(db, signer), "/api/v1/viewer/network-token", map[string]any{
		"code": viewerCode.Code, "device_public_key": encodedDeviceKey,
	}, "")
	if viewerNetworkResponse.Code != http.StatusOK {
		t.Fatalf("viewer network authorization status = %d: %s", viewerNetworkResponse.Code, viewerNetworkResponse.Body.String())
	}
	assertSignedNetworkToken(t, viewerNetworkResponse, publicKey, 1, "viewer")
}

func postPreproductionJSON(t *testing.T, handler http.HandlerFunc, path string, payload any, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func decodePreproductionResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatal(err)
	}
}

func assertSignedNetworkToken(t *testing.T, response *httptest.ResponseRecorder, publicKey ed25519.PublicKey, maxSessions int, role string) {
	t.Helper()
	var tokenResponse struct {
		Token string `json:"network_token"`
	}
	decodePreproductionResponse(t, response, &tokenResponse)
	parts := strings.Split(tokenResponse.Token, ".")
	if len(parts) != 3 || parts[0] != networkauth.TokenPrefix {
		t.Fatalf("invalid network token format")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		t.Fatal("network token signature verification failed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims networkauth.Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Role != role || claims.MaxSessions != maxSessions {
		t.Fatalf("claims role/max_sessions = %s/%d, want %s/%d", claims.Role, claims.MaxSessions, role, maxSessions)
	}
}
