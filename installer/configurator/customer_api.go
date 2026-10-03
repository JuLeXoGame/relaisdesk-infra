package main

import (
	"net/http"
)

// Compte client (miroir de l'espace client web) : la session technicien
// (licence) reste le socle des connexions ; la session client est ouverte à
// la demande pour les rubriques du menu web (aperçu, parc, équipe,
// licences, factures, historique, prestations, préférences).

var (
	customerSessionToken      string
	customerSessionEmail      string
	customerSessionCustomerID string
	customerSessionRole       string
	customerDeviceToken       string
)

func clearCustomerSession() {
	customerSessionToken = ""
	customerSessionEmail = ""
	customerSessionCustomerID = ""
	customerSessionRole = ""
}

func customerLoggedIn() bool {
	return customerSessionToken != ""
}

type customerLoginResult struct {
	Requires2FA      bool   `json:"requires_2fa"`
	ChallengeToken   string `json:"challenge_token"`
	EmailCodeAllowed *bool  `json:"email_code_allowed"`
	Token            string `json:"token"`
	CustomerID       string `json:"customer_id"`
	Email            string `json:"email"`
	DeviceToken      string `json:"device_token"`
}

func customerLoginPassword(email, password, deviceToken string) (*customerLoginResult, error) {
	var res customerLoginResult
	err := doTechnicianReq(http.MethodPost, "/api/v1/customer/login", "", map[string]string{
		"email": email, "password": password, "device_token": deviceToken,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func customerLogin2FA(challengeToken, code string, rememberDevice bool, deviceName string) (*customerLoginResult, error) {
	var res customerLoginResult
	err := doTechnicianReq(http.MethodPost, "/api/v1/customer/login/2fa", "", map[string]any{
		"challenge_token": challengeToken, "code": code,
		"remember_device": rememberDevice, "device_name": deviceName,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func customerLoginGoogle(credential string) (*customerLoginResult, error) {
	var res customerLoginResult
	err := doTechnicianReq(http.MethodPost, "/api/v1/customer/login/google", "", map[string]string{
		"credential": credential,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func customerLogout(token string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/customer/logout", token, nil, &res)
}

type customerEmailCodeResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	EmailMasked string `json:"email_masked"`
}

func sendCustomer2FAEmailCode(challengeToken string) (*customerEmailCodeResult, error) {
	var res customerEmailCodeResult
	err := doTechnicianReq(http.MethodPost, "/api/v1/customer/login/2fa/send-email-code", "", map[string]string{
		"challenge_token": challengeToken,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

type customerLicenseView struct {
	LicenseID      string `json:"license_id"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at"`
	MaxConnections int    `json:"max_connections"`
	PendingRenewal bool   `json:"pending_renewal"`
	Renewable      bool   `json:"renewable"`
}

type customerOrderView struct {
	OrderID       string  `json:"order_id"`
	Plan          string  `json:"plan"`
	Technicians   int     `json:"technicians"`
	Price         float64 `json:"price"`
	PaymentMethod string  `json:"payment_method"`
	Status        string  `json:"status"`
	OrderKind     string  `json:"order_kind"`
	LicenseID     string  `json:"license_id"`
	InvoiceNumber string  `json:"invoice_number"`
	CreatedAt     string  `json:"created_at"`
	PaidAt        string  `json:"paid_at"`
}

type customerInvoiceView struct {
	InvoiceNumber string  `json:"invoice_number"`
	OrderID       string  `json:"order_id"`
	Plan          string  `json:"plan"`
	Technicians   int     `json:"technicians"`
	AmountTTC     float64 `json:"amount_ttc"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

type customerInterventionView struct {
	ID              int     `json:"id"`
	InterventionID  string  `json:"intervention_id"`
	LicenseID       string  `json:"license_id"`
	ClientReference string  `json:"client_reference"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	StartedAt       *string `json:"started_at"`
	EndedAt         *string `json:"ended_at"`
	DurationMinutes *int    `json:"duration_minutes"`
	Summary         string  `json:"summary"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type customerTrialView struct {
	ID                  string `json:"id"`
	Plan                string `json:"plan"`
	Technicians         int    `json:"technicians"`
	PriceCents          int64  `json:"price_cents"`
	BillingCycle        string `json:"billing_cycle"`
	State               string `json:"state"`
	LicenseID           string `json:"license_id"`
	TrialStart          int64  `json:"trial_start"`
	TrialEnd            int64  `json:"trial_end"`
	PaidThrough         int64  `json:"paid_through"`
	SubscriptionStatus  string `json:"subscription_status"`
	CancelAtPeriodEnd   bool   `json:"cancel_at_period_end"`
	CancelRequestedAt   int64  `json:"cancel_requested_at"`
	WithdrawalImmediate bool   `json:"withdrawal_immediate"`
}

type customerDashboard struct {
	Email                   string                     `json:"email"`
	CustomerID              string                     `json:"customer_id"`
	CustomerRole            string                     `json:"customer_role"`
	DisplayName             string                     `json:"display_name"`
	BillingEmail            string                     `json:"billing_email"`
	RenewalRemindersEnabled bool                       `json:"renewal_reminders_enabled"`
	Licenses                []customerLicenseView      `json:"licenses"`
	Orders                  []customerOrderView        `json:"orders"`
	Invoices                []customerInvoiceView      `json:"invoices"`
	Interventions           []customerInterventionView `json:"interventions"`
	Subscriptions           []customerTrialView        `json:"subscriptions"`
}

func getCustomerDashboard(token string) (*customerDashboard, error) {
	var res customerDashboard
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/dashboard", token, nil, &res); err != nil {
		return nil, err
	}
	if res.Licenses == nil {
		res.Licenses = []customerLicenseView{}
	}
	if res.Orders == nil {
		res.Orders = []customerOrderView{}
	}
	if res.Invoices == nil {
		res.Invoices = []customerInvoiceView{}
	}
	if res.Interventions == nil {
		res.Interventions = []customerInterventionView{}
	}
	if res.Subscriptions == nil {
		res.Subscriptions = []customerTrialView{}
	}
	return &res, nil
}

func setRenewalReminders(token string, enabled bool) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPut, "/api/v1/customer/preferences", token, map[string]bool{
		"renewal_reminders_enabled": enabled,
	}, &res)
}

func changeCustomerPassword(token, oldPassword, newPassword string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/customer/preferences/password", token, map[string]string{
		"old_password": oldPassword, "new_password": newPassword,
	}, &res)
}

func cancelCustomerSubscription(token, subscriptionID string) (string, error) {
	var res struct {
		Message string `json:"message"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/subscriptions/cancel", token, map[string]string{
		"id": subscriptionID,
	}, &res); err != nil {
		return "", err
	}
	return res.Message, nil
}

type customer2FAStatus struct {
	Enabled                bool   `json:"enabled"`
	ConfirmedAt            string `json:"confirmed_at"`
	RemainingRecoveryCodes int    `json:"remaining_recovery_codes"`
}

func getCustomer2FAStatus(token string) (*customer2FAStatus, error) {
	var res customer2FAStatus
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/2fa/status", token, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type customer2FASetup struct {
	Secret        string   `json:"secret"`
	OTPAuthURL    string   `json:"otpauth_url"`
	RecoveryCodes []string `json:"recovery_codes"`
}

func setupCustomer2FA(token string) (*customer2FASetup, error) {
	var res customer2FASetup
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/2fa/setup", token, map[string]string{}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func enableCustomer2FA(token, password, secret, code string, recoveryCodes []string) (string, error) {
	var res struct {
		Message string `json:"message"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/2fa/enable", token, map[string]any{
		"password": password, "secret": secret, "code": code, "recovery_codes": recoveryCodes,
	}, &res); err != nil {
		return "", err
	}
	return res.Message, nil
}

func disableCustomer2FA(token, password, code string) (string, error) {
	var res struct {
		Message string `json:"message"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/2fa/disable", token, map[string]string{
		"password": password, "code": code,
	}, &res); err != nil {
		return "", err
	}
	return res.Message, nil
}
