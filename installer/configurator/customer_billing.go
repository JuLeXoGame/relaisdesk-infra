package main

import (
	"net/http"
)

// Licences et factures : renouvellement, portail Stripe et téléchargements.

type renewLicenseResult struct {
	OrderID       string `json:"order_id"`
	CheckoutURL   string `json:"checkout_url"`
	BankDetails   string `json:"-"`
	Beneficiary   string `json:"-"`
	IBAN          string `json:"-"`
	BIC           string `json:"-"`
	Amount        float64
	Reference     string
	CryptoAddress string `json:"-"`
	CryptoAmount  string `json:"-"`
	CryptoAsset   string `json:"-"`
}

func renewLicense(token, licenseID, paymentMethod, termsVersion string, immediatePerformance bool) (*renewLicenseResult, error) {
	var raw map[string]any
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/licenses/"+licenseID+"/renew", token, map[string]any{
		"payment_method": paymentMethod, "terms_version": termsVersion,
		"terms_accepted": true, "immediate_performance_requested": immediatePerformance,
	}, &raw); err != nil {
		return nil, err
	}
	res := &renewLicenseResult{}
	if v, _ := raw["order_id"].(string); v != "" {
		res.OrderID = v
	}
	if v, _ := raw["checkout_url"].(string); v != "" {
		res.CheckoutURL = v
	}
	if inst, _ := raw["instructions"].(map[string]any); inst != nil {
		res.Beneficiary, _ = inst["beneficiary"].(string)
		res.IBAN, _ = inst["iban"].(string)
		res.BIC, _ = inst["bic"].(string)
		res.Reference, _ = inst["reference"].(string)
		if f, ok := inst["amount"].(float64); ok {
			res.Amount = f
		}
	}
	for _, key := range []string{"address", "deposit_address", "payment_address"} {
		if v, _ := raw[key].(string); v != "" {
			res.CryptoAddress = v
			break
		}
	}
	for _, key := range []string{"amount", "amount_requested", "total"} {
		switch v := raw[key].(type) {
		case string:
			if v != "" {
				res.CryptoAmount = v
			}
		case float64:
			if v != 0 {
				res.CryptoAmount = formatEUR(v)
			}
		}
		if res.CryptoAmount != "" {
			break
		}
	}
	if v, _ := raw["asset"].(string); v != "" {
		res.CryptoAsset = v
	} else if v, _ := raw["currency"].(string); v != "" {
		res.CryptoAsset = v
	}
	return res, nil
}

func openBillingPortal(token, subscriptionID string) (string, error) {
	var res struct {
		URL string `json:"url"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/customer/billing-portal", token, map[string]string{
		"id": subscriptionID,
	}, &res); err != nil {
		return "", err
	}
	return res.URL, nil
}

func downloadInvoicePDF(token, invoiceNumber string) ([]byte, error) {
	return downloadCustomerFile(token, "/api/v1/customer/invoices/"+invoiceNumber+"/download")
}

func downloadInvoiceCII(token, invoiceNumber string) ([]byte, error) {
	return downloadCustomerFile(token, "/api/v1/customer/invoices/"+invoiceNumber+"/cii")
}
