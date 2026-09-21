package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var APIURL = "https://api.relaisdesk.fr"

type AdminLoginResponse struct {
	Valid          bool   `json:"valid"`
	Token          string `json:"token"`
	Role           string `json:"role"`
	Email          string `json:"email"`
	LicenseID      string `json:"license_id"`
	Requires2FA    bool   `json:"requires_2fa"`
	ChallengeToken string `json:"challenge_token"`
	Message        string `json:"message"`
	Error          string `json:"error"`
}

type AdminStats struct {
	TotalLicences            int  `json:"total_licences"`
	ActiveLicences           int  `json:"active_licences"`
	ExpiredLicences          int  `json:"expired_licences"`
	RevokedLicences          int  `json:"revoked_licences"`
	CurrentConnections       int  `json:"current_connections"`
	ConnectionCountAvailable bool `json:"connection_count_available"`
}

type SystemComponentStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
}

type AdminSystemStatus struct {
	Status     string                  `json:"status"`
	CheckedAt  string                  `json:"checked_at"`
	Components []SystemComponentStatus `json:"components"`
}

type MonthlyRevenueItem struct {
	Month       string  `json:"month"`
	Label       string  `json:"label"`
	Revenue     float64 `json:"revenue"`
	OrdersCount int     `json:"orders_count"`
}

type AdminFinancials struct {
	TotalRevenue            float64              `json:"total_revenue"`
	PaidOrdersCount         int                  `json:"paid_orders_count"`
	MonthlyRecurringRevenue float64              `json:"monthly_recurring_revenue"`
	ActiveSubscribersCount  int                  `json:"active_subscribers_count"`
	MonthlyHistory          []MonthlyRevenueItem `json:"monthly_history"`
}

type AdminLicense struct {
	LicenseID                string `json:"license_id"`
	Email                    string `json:"email"`
	LicenseKey               string `json:"license_key"`
	Status                   string `json:"status"`
	CreatedAt                string `json:"created_at"`
	ExpiresAt                string `json:"expires_at"`
	MaxConnections           int    `json:"max_connections"`
	CurrentConnections       int    `json:"current_connections"`
	ConnectionCountAvailable bool   `json:"connection_count_available"`
	Notes                    string `json:"notes"`
	RevokedReason            string `json:"revoked_reason"`
}

type CreateLicenseReq struct {
	Email          string `json:"email"`
	Plan           string `json:"plan"`
	Technicians    int    `json:"technicians"`
	Days           int    `json:"days"`
	MaxConnections int    `json:"max_connections"`
	Notes          string `json:"notes"`
}

type AdminOrder struct {
	OrderID          string  `json:"order_id"`
	Email            string  `json:"email"`
	Plan             string  `json:"plan"`
	Technicians      int     `json:"technicians"`
	Price            float64 `json:"price"`
	PaymentMethod    string  `json:"payment_method"`
	Status           string  `json:"status"`
	LicenseID        string  `json:"license_id"`
	CreatedAt        string  `json:"created_at"`
	PaymentComplete  bool    `json:"payment_complete"`
	LicenseCreated   bool    `json:"license_created"`
	InvoiceCreated   bool    `json:"invoice_created"`
	LicenseEmailSent bool    `json:"license_email_sent"`
	InvoiceEmailSent bool    `json:"invoice_email_sent"`
	FulfillmentDone  bool    `json:"fulfillment_done"`
}

type AdminInvoice struct {
	InvoiceNumber string  `json:"invoice_number"`
	OrderID       string  `json:"order_id"`
	CustomerEmail string  `json:"customer_email"`
	CustomerName  string  `json:"customer_name"`
	Plan          string  `json:"plan"`
	Technicians   int     `json:"technicians"`
	AmountHT      float64 `json:"amount_ht"`
	AmountTTC     float64 `json:"amount_ttc"`
	Status        string  `json:"status"`
	IsManual      bool    `json:"is_manual"`
	CreatedAt     string  `json:"created_at"`
	Notes         string  `json:"notes"`
}

type AdminAlert struct {
	ID        int    `json:"id"`
	Type      string `json:"type"`
	Code      string `json:"code"`
	FirstIP   string `json:"first_ip"`
	SecondIP  string `json:"second_ip"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func getHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}
}

func doAdminReq(method, path, token string, reqBody any, respObj any) error {
	var bodyBuffer *bytes.Buffer
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("erreur sérialisation requête: %w", err)
		}
		bodyBuffer = bytes.NewBuffer(jsonData)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	client := getHTTPClient()
	req, err := http.NewRequest(method, APIURL+path, bodyBuffer)
	if err != nil {
		return fmt.Errorf("erreur création requête: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("impossible de contacter le serveur API (%s) : %w", APIURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if respObj != nil {
			_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(respObj)
		}
		return fmt.Errorf("accès non autorisé (identifiants incorrects ou droits insuffisants)")
	}

	if respObj != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(respObj); err != nil {
			return fmt.Errorf("erreur lecture réponse JSON (HTTP %d): %w", resp.StatusCode, err)
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("erreur serveur (HTTP %d)", resp.StatusCode)
	}

	return nil
}

func AdminLogin(licenseID, licenseKey, secretToken string) (*AdminLoginResponse, error) {
	payload := map[string]string{}
	if secretToken != "" {
		payload["admin_token"] = secretToken
	} else {
		payload["license_id"] = licenseID
		payload["license_key"] = licenseKey
	}

	var resp AdminLoginResponse
	err := doAdminReq(http.MethodPost, "/api/v1/admin/login", "", payload, &resp)
	if err != nil {
		return nil, err
	}
	if !resp.Valid {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("identifiants administrateur invalides")
	}
	return &resp, nil
}

func AdminLoginWithEmail(email, password string) (*AdminLoginResponse, error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
	}

	var resp AdminLoginResponse
	err := doAdminReq(http.MethodPost, "/api/v1/admin/login", "", payload, &resp)
	if resp.Requires2FA {
		return &resp, nil
	}
	if err != nil {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, err
	}
	if !resp.Valid {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("identifiants administrateur invalides")
	}
	return &resp, nil
}

func AdminVerify2FA(challengeToken, code string) (*AdminLoginResponse, error) {
	payload := map[string]string{
		"challenge_token": challengeToken,
		"code":            code,
	}

	var resp AdminLoginResponse
	err := doAdminReq(http.MethodPost, "/api/v1/admin/login", "", payload, &resp)
	if err != nil {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, err
	}
	if !resp.Valid {
		if resp.Error != "" {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return nil, fmt.Errorf("code 2FA invalide")
	}
	return &resp, nil
}

func AdminLogout(token string) error {
	return doAdminReq(http.MethodPost, "/api/v1/admin/logout", token, nil, nil)
}

func GetStats(token string) (*AdminStats, error) {
	var stats AdminStats
	err := doAdminReq(http.MethodGet, "/api/v1/admin/stats", token, nil, &stats)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

func GetSystemStatus(token string) (*AdminSystemStatus, error) {
	var status AdminSystemStatus
	err := doAdminReq(http.MethodGet, "/api/v1/admin/system-status", token, nil, &status)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func GetFinancials(token string) (*AdminFinancials, error) {
	var fin AdminFinancials
	err := doAdminReq(http.MethodGet, "/api/v1/admin/financials", token, nil, &fin)
	if err != nil {
		return nil, err
	}
	return &fin, nil
}

func ListLicenses(token string, unmask bool, status, email string) ([]AdminLicense, error) {
	path := fmt.Sprintf("/api/v1/admin/licences?unmask=%t&limit=500", unmask)
	if status != "" {
		path += "&status=" + status
	}
	if email != "" {
		path += "&email=" + email
	}

	var resp struct {
		Licences []AdminLicense `json:"licences"`
	}
	err := doAdminReq(http.MethodGet, path, token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Licences, nil
}

func CreateLicense(token string, req CreateLicenseReq) (*AdminLicense, error) {
	var lic AdminLicense
	err := doAdminReq(http.MethodPost, "/api/v1/admin/licences", token, req, &lic)
	if err != nil {
		return nil, err
	}
	return &lic, nil
}

func ExtendLicense(token string, licenseID string, days int) error {
	payload := map[string]int{"days": days}
	return doAdminReq(http.MethodPost, "/api/v1/admin/licences/"+licenseID+"/extend", token, payload, nil)
}

func RevokeLicense(token string, licenseID string, reason string) error {
	payload := map[string]string{"reason": reason}
	return doAdminReq(http.MethodPost, "/api/v1/admin/licences/"+licenseID+"/revoke", token, payload, nil)
}

func ListOrders(token string) ([]AdminOrder, error) {
	var resp struct {
		Orders []AdminOrder `json:"orders"`
	}
	err := doAdminReq(http.MethodGet, "/api/v1/admin/orders", token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Orders, nil
}

func MarkOrderPaid(token string, orderID string) error {
	return doAdminReq(http.MethodPost, "/api/v1/admin/orders/"+orderID+"/mark-paid", token, nil, nil)
}

func RetryOrderFulfillment(token string, orderID string) error {
	return doAdminReq(http.MethodPost, "/api/v1/admin/orders/"+orderID+"/retry-fulfillment", token, nil, nil)
}

func DeleteOrder(token string, orderID string) error {
	return doAdminReq(http.MethodDelete, "/api/v1/admin/orders/"+orderID, token, nil, nil)
}

func ListAlerts(token string) ([]AdminAlert, error) {
	var resp struct {
		Alerts []AdminAlert `json:"alerts"`
	}
	err := doAdminReq(http.MethodGet, "/api/v1/admin/alerts", token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Alerts, nil
}

func ListInvoices(token string) ([]AdminInvoice, error) {
	var resp struct {
		Invoices []AdminInvoice `json:"invoices"`
	}
	err := doAdminReq(http.MethodGet, "/api/v1/admin/invoices", token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Invoices, nil
}

func DeleteInvoice(token string, invoiceNumber string) error {
	return doAdminReq(http.MethodDelete, "/api/v1/admin/invoices/"+invoiceNumber, token, nil, nil)
}

func CreateCreditNote(token string, invoiceNumber string, reason string) error {
	payload := map[string]string{"reason": reason}
	return doAdminReq(http.MethodPost, "/api/v1/admin/invoices/"+invoiceNumber+"/credit-note", token, payload, nil)
}

type AdminDeviceItem struct {
	ID            int64      `json:"id"`
	DeviceID      string     `json:"device_id"`
	LicenseID     string     `json:"license_id"`
	FolderID      string     `json:"folder_id"`
	PermanentCode string     `json:"permanent_code"`
	RustDeskID    string     `json:"rustdesk_id"`
	Alias         string     `json:"alias"`
	Hostname      string     `json:"hostname"`
	OS            string     `json:"os"`
	Status        string     `json:"status"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	LastIP        string     `json:"last_ip"`
	Notes         string     `json:"notes"`
	IsActive      bool       `json:"is_active"`
}

type AdminFolderItem struct {
	ID             int    `json:"id,omitempty"`
	FolderID       string `json:"folder_id"`
	ParentFolderID string `json:"parent_folder_id"`
	Name           string `json:"name"`
	CustomerID     any    `json:"customer_id,omitempty"`
	LicenseID      string `json:"license_id,omitempty"`
	CreatedAt      any    `json:"created_at,omitempty"`
	UpdatedAt      any    `json:"updated_at,omitempty"`
}

func ListAdminDevices(token string) ([]AdminDeviceItem, error) {
	var resp struct {
		Devices []AdminDeviceItem `json:"devices"`
	}
	err := doAdminReq(http.MethodGet, "/api/v1/technician/devices", token, nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Devices, nil
}

func ListAdminFolders(token string) ([]AdminFolderItem, error) {
	var resp struct {
		Folders       []AdminFolderItem `json:"folders"`
		DeviceFolders []AdminFolderItem `json:"device_folders"`
	}
	err := doAdminReq(http.MethodGet, "/api/v1/technician/device-folders", token, nil, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Folders) > 0 {
		return resp.Folders, nil
	}
	return resp.DeviceFolders, nil
}

func CreateAdminFolder(token, name, parentFolderID string) (*AdminFolderItem, error) {
	payload := map[string]string{
		"name":             strings.TrimSpace(name),
		"parent_folder_id": strings.TrimSpace(parentFolderID),
	}
	var resp struct {
		Folder AdminFolderItem `json:"folder"`
	}
	err := doAdminReq(http.MethodPost, "/api/v1/technician/device-folders", token, payload, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Folder, nil
}

func UpdateAdminFolder(token, folderID, name string) error {
	payload := map[string]string{
		"name": strings.TrimSpace(name),
	}
	return doAdminReq(http.MethodPut, "/api/v1/technician/device-folders/"+folderID, token, payload, nil)
}

func DeleteAdminFolder(token, folderID string) error {
	return doAdminReq(http.MethodDelete, "/api/v1/technician/device-folders/"+folderID, token, nil, nil)
}

func UpdateAdminDeviceFolder(token, deviceID, folderID string) error {
	payload := map[string]string{
		"folder_id": strings.TrimSpace(folderID),
	}
	return doAdminReq(http.MethodPut, "/api/v1/technician/devices/"+deviceID, token, payload, nil)
}

func GenerateAdminPermanentCode(token, alias, notes string) (string, error) {
	payload := map[string]string{
		"alias": strings.TrimSpace(alias),
		"notes": strings.TrimSpace(notes),
	}
	var resp struct {
		Device struct {
			PermanentCode string `json:"permanent_code"`
		} `json:"device"`
	}
	err := doAdminReq(http.MethodPost, "/api/v1/technician/devices/enrollment-code", token, payload, &resp)
	if err != nil {
		return "", err
	}
	return resp.Device.PermanentCode, nil
}

func DeleteAdminDevice(token, deviceID string) error {
	return doAdminReq(http.MethodDelete, "/api/v1/technician/devices/"+deviceID, token, nil, nil)
}


