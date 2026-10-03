package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCustomerLoginPasswordShapes(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/customer/login" || r.Method != http.MethodPost {
			t.Fatal("requête inattendue", r.Method, r.URL.Path)
		}
		if h := r.Header.Get("Authorization"); h != "" {
			t.Fatal("le login ne doit pas envoyer d'autorisation")
		}
		body := `{"token":"tok-1","customer_id":"CUS-1","email":"a@b.c"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	res, err := customerLoginPassword("a@b.c", "motdepasse123", "")
	if err != nil || res.Token != "tok-1" || res.Requires2FA {
		t.Fatalf("login direct: %+v %v", res, err)
	}
}

func TestCustomerLogin2FAChallenge(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	step := 0
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		step++
		var body string
		switch {
		case step == 1 && r.URL.Path == "/api/v1/customer/login":
			body = `{"requires_2fa":true,"challenge_token":"ch-1","customer_id":"CUS-1","email":"a@b.c"}`
		case step == 2 && r.URL.Path == "/api/v1/customer/login/2fa" && r.Method == http.MethodPost:
			body = `{"token":"tok-2","customer_id":"CUS-1","email":"a@b.c","device_token":"dev-1"}`
		default:
			t.Fatal("séquence inattendue", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	first, err := customerLoginPassword("a@b.c", "motdepasse123", "")
	if err != nil || !first.Requires2FA || first.ChallengeToken != "ch-1" {
		t.Fatalf("challenge attendu: %+v %v", first, err)
	}
	second, err := customerLogin2FA("ch-1", "123456", true, "test")
	if err != nil || second.Token != "tok-2" || second.DeviceToken != "dev-1" {
		t.Fatalf("session 2FA attendue: %+v %v", second, err)
	}
}

func TestCustomerDashboardParsing(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/customer/dashboard" || r.Method != http.MethodGet {
			t.Fatal("requête inattendue", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sess-9" {
			t.Fatal("autorisation manquante")
		}
		body := `{"email":"a@b.c","customer_id":"CUS-9","customer_role":"owner","display_name":"","billing_email":"a@b.c",` +
			`"renewal_reminders_enabled":true,` +
			`"licenses":[{"license_id":"LIC-1","status":"active","created_at":"2026-01-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","max_connections":3,"pending_renewal":false,"renewable":true}],` +
			`"orders":[{"order_id":"ORD-1","plan":"pro","technicians":2,"price":99.0,"payment_method":"stripe","status":"paid","order_kind":"purchase","created_at":"2026-02-01T00:00:00Z"}],` +
			`"invoices":[{"invoice_number":"F-1","order_id":"ORD-1","plan":"pro","technicians":2,"amount_ttc":118.8,"status":"paid","created_at":"2026-02-01T00:00:00Z"}],` +
			`"interventions":[{"id":1,"intervention_id":"INT-1","license_id":"LIC-1","client_reference":"CLI","title":"Dépannage","status":"completed","summary":"ok","created_at":"2026-03-01T00:00:00Z","updated_at":"2026-03-01T01:00:00Z"}],` +
			`"subscriptions":[{"id":"TR-1","plan":"pro","technicians":2,"price_cents":9900,"billing_cycle":"monthly","state":"active","trial_start":1,"trial_end":2,"paid_through":3,"subscription_status":"active","cancel_at_period_end":false,"withdrawal_immediate":false}]` +
			`}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	dash, err := getCustomerDashboard("sess-9")
	if err != nil {
		t.Fatal(err)
	}
	if dash.CustomerID != "CUS-9" || dash.CustomerRole != "owner" || !dash.RenewalRemindersEnabled {
		t.Fatalf("identité: %+v", dash)
	}
	if len(dash.Licenses) != 1 || dash.Licenses[0].LicenseID != "LIC-1" || !dash.Licenses[0].Renewable {
		t.Fatalf("licences: %+v", dash.Licenses)
	}
	if len(dash.Orders) != 1 || dash.Orders[0].Price != 99.0 {
		t.Fatalf("commandes: %+v", dash.Orders)
	}
	if len(dash.Invoices) != 1 || dash.Invoices[0].AmountTTC != 118.8 {
		t.Fatalf("factures: %+v", dash.Invoices)
	}
	if len(dash.Interventions) != 1 || dash.Interventions[0].InterventionID != "INT-1" {
		t.Fatalf("interventions: %+v", dash.Interventions)
	}
	if len(dash.Subscriptions) != 1 || dash.Subscriptions[0].PriceCents != 9900 {
		t.Fatalf("abonnements: %+v", dash.Subscriptions)
	}
}

func TestCustomer2FAManagement(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer sess-9" {
			t.Fatal("autorisation manquante")
		}
		var body string
		switch {
		case r.URL.Path == "/api/v1/customer/2fa/status" && r.Method == http.MethodGet:
			body = `{"enabled":false,"remaining_recovery_codes":0}`
		case r.URL.Path == "/api/v1/customer/2fa/setup" && r.Method == http.MethodPost:
			body = `{"secret":"ABCDEF","otpauth_url":"otpauth://totp/x?secret=ABCDEF","recovery_codes":["r1","r2"]}`
		case r.URL.Path == "/api/v1/customer/2fa/enable" && r.Method == http.MethodPost:
			body = `{"success":true,"message":"Authentification à deux facteurs activée avec succès."}`
		case r.URL.Path == "/api/v1/customer/2fa/disable" && r.Method == http.MethodPost:
			body = `{"success":true,"message":"Authentification à deux facteurs désactivée."}`
		default:
			t.Fatal("requête inattendue", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	status, err := getCustomer2FAStatus("sess-9")
	if err != nil || status.Enabled {
		t.Fatalf("statut: %+v %v", status, err)
	}
	setup, err := setupCustomer2FA("sess-9")
	if err != nil || setup.Secret != "ABCDEF" || len(setup.RecoveryCodes) != 2 {
		t.Fatalf("setup: %+v %v", setup, err)
	}
	msg, err := enableCustomer2FA("sess-9", "motdepasse123", "ABCDEF", "123456", []string{"r1", "r2"})
	if err != nil || msg == "" {
		t.Fatal("activation", msg, err)
	}
	msg, err = disableCustomer2FA("sess-9", "motdepasse123", "123456")
	if err != nil || msg == "" {
		t.Fatal("désactivation", msg, err)
	}
}

func TestCustomerPasswordAndPreferences(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer sess-9" {
			t.Fatal("autorisation manquante")
		}
		switch {
		case r.URL.Path == "/api/v1/customer/preferences/password" && r.Method == http.MethodPost:
			// ok
		case r.URL.Path == "/api/v1/customer/preferences" && r.Method == http.MethodPut:
			// ok
		case r.URL.Path == "/api/v1/customer/logout" && r.Method == http.MethodPost:
			// ok
		default:
			t.Fatal("requête inattendue", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true}`)), Header: http.Header{}}, nil
	})
	if err := changeCustomerPassword("sess-9", "ancien123", "nouveau12345"); err != nil {
		t.Fatal(err)
	}
	if err := setRenewalReminders("sess-9", false); err != nil {
		t.Fatal(err)
	}
	if err := customerLogout("sess-9"); err != nil {
		t.Fatal(err)
	}
}
