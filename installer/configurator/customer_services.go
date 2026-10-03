package main

import (
	"net/http"
	"net/url"
)

// Prestations facturées du propriétaire : marchand Stripe, catalogue,
// conditions et encaissements (miroir du panneau web).

type customerServiceMerchant struct {
	Enabled   bool   `json:"enabled"`
	AccountID string `json:"account_id"`
}

type customerServiceRate struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Mode   string `json:"mode"`
	Cents  int64  `json:"cents"`
	Active bool   `json:"active"`
}

type customerServiceWork struct {
	ID             string `json:"id"`
	LicenseID      string `json:"license_id"`
	TargetKind     string `json:"target_kind"`
	TargetID       string `json:"target_id"`
	PeerID         string `json:"peer_id"`
	Label          string `json:"label"`
	Mode           string `json:"mode"`
	RateCents      int64  `json:"rate_cents"`
	State          string `json:"state"`
	Paid           bool   `json:"paid"`
	ConnectedMS    int64  `json:"connected_ms"`
	AmountCents    int64  `json:"amount_cents"`
	CreatedAt      int64  `json:"created_at"`
	ConnectionOpen bool   `json:"connection_open"`
	CheckoutURL    string `json:"checkout_url,omitempty"`
}

type customerServiceTerms struct {
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	URL        string `json:"url"`
	AcceptedAt string `json:"accepted_at"`
}

type customerServiceBilling struct {
	Available     bool                    `json:"available"`
	ActiveLicense bool                    `json:"active_license"`
	Merchant      customerServiceMerchant `json:"merchant"`
	Rates         []customerServiceRate   `json:"rates"`
	Work          []customerServiceWork   `json:"work"`
	Terms         customerServiceTerms    `json:"terms"`
}

func getCustomerServiceBilling(token string) (*customerServiceBilling, error) {
	var res customerServiceBilling
	if err := doTechnicianReq(http.MethodGet, "/api/v1/customer/service-billing", token, nil, &res); err != nil {
		return nil, err
	}
	if res.Rates == nil {
		res.Rates = []customerServiceRate{}
	}
	if res.Work == nil {
		res.Work = []customerServiceWork{}
	}
	return &res, nil
}

func acceptCustomerServiceTerms(token, version, sha256 string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPost, "/api/v1/customer/service-billing/terms/accept", token, map[string]any{
		"accepted": true, "version": version, "sha256": sha256,
	}, &res)
}

func setCustomerServiceEnabled(token string, enabled bool) error {
	var res map[string]any
	return doTechnicianReq(http.MethodPut, "/api/v1/customer/service-billing", token, map[string]bool{
		"enabled": enabled,
	}, &res)
}

func startCustomerServiceOnboarding(token string) (string, error) {
	var res struct {
		URL     string `json:"url"`
		Message string `json:"message"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/service-billing/onboarding", token, map[string]string{}, &res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func createCustomerServiceRate(token, label, mode string, cents int64) (*customerServiceRate, error) {
	var res customerServiceRate
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/service-billing/rates", token, map[string]any{
		"label": label, "mode": mode, "cents": cents,
	}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func deleteCustomerServiceRate(token, rateID string) error {
	var res map[string]any
	return doTechnicianReq(http.MethodDelete, "/api/v1/customer/service-billing/rates/"+url.PathEscape(rateID), token, nil, &res)
}

func runCustomerServiceWork(token, workID, action string) (*customerServiceWork, error) {
	var res customerServiceWork
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/service-billing/work/"+url.PathEscape(workID)+"/"+action, token, map[string]string{}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
