package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func mockCustomerAPI(t *testing.T, routes map[string]string) {
	t.Helper()
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://customer.invalid"
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer sess-9" {
			t.Fatal("autorisation manquante pour", r.URL.Path)
		}
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			t.Fatal("route inattendue", r.Method, r.URL.Path)
		}
		code := 200
		if strings.HasPrefix(body, "201:") {
			code = 201
			body = strings.TrimPrefix(body, "201:")
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
}

func TestCustomerTeamFlow(t *testing.T) {
	mockCustomerAPI(t, map[string]string{
		"GET /api/v1/customer/team": `{"licenses":[{"license_id":"LIC-1","plan":"pro","capacity":3,"used":1,"enabled":true}],` +
			`"members":[{"member_id":"MBR-1","license_id":"LIC-1","email":"tech@x.fr","status":"active","folder_ids":["FLD-1"]}],` +
			`"memberships":[]}`,
		"POST /api/v1/customer/team/invitations":          `201:{"message":"Invitation mise en file d'envoi."}`,
		"POST /api/v1/customer/team/members/MBR-1/resend": `{"success":true}`,
		"PUT /api/v1/customer/team/members/MBR-1":         `{"success":true}`,
		"DELETE /api/v1/customer/team/members/MBR-1":      `{"success":true}`,
	})
	team, err := getCustomerTeam("sess-9")
	if err != nil || len(team.Licenses) != 1 || len(team.Members) != 1 || len(team.Memberships) != 0 {
		t.Fatalf("équipe: %+v %v", team, err)
	}
	if team.Licenses[0].Capacity != 3 || team.Members[0].Email != "tech@x.fr" {
		t.Fatalf("contenu équipe: %+v", team)
	}
	msg, err := inviteTeamMember("sess-9", "LIC-1", "n@x.fr", []string{"FLD-1"})
	if err != nil || msg == "" {
		t.Fatal("invitation", msg, err)
	}
	if err := resendTeamInvitation("sess-9", "MBR-1"); err != nil {
		t.Fatal("renvoi", err)
	}
	if err := updateTeamMemberFolders("sess-9", "MBR-1", []string{}); err != nil {
		t.Fatal("dossiers", err)
	}
	if err := revokeTeamMember("sess-9", "MBR-1"); err != nil {
		t.Fatal("révocation", err)
	}
}

func TestRenewLicenseResponses(t *testing.T) {
	mockCustomerAPI(t, map[string]string{
		"POST /api/v1/customer/licenses/LIC-1/renew": `{"order_id":"ORD-9","checkout_url":"https://checkout.stripe.com/pay/cs_9"}`,
	})
	res, err := renewLicense("sess-9", "LIC-1", "stripe", "2026-09-27", false)
	if err != nil || res.OrderID != "ORD-9" || res.CheckoutURL == "" {
		t.Fatalf("stripe: %+v %v", res, err)
	}
	mockCustomerAPI(t, map[string]string{
		"POST /api/v1/customer/licenses/LIC-1/renew": `{"order_id":"ORD-8","instructions":{"amount":118.8,"reference":"ORD-8","beneficiary":"RelaisDesk","iban":"FR00","bic":"ABC"}}`,
	})
	res, err = renewLicense("sess-9", "LIC-1", "bank_transfer", "2026-09-27", false)
	if err != nil || res.IBAN != "FR00" || res.Amount != 118.8 {
		t.Fatalf("virement: %+v %v", res, err)
	}
}

func TestBillingPortalAndInvoiceDownload(t *testing.T) {
	mockCustomerAPI(t, map[string]string{
		"POST /api/v1/customer/billing-portal": `{"url":"https://billing.stripe.com/session/abc"}`,
	})
	url, err := openBillingPortal("sess-9", "TR-1")
	if err != nil || !strings.HasPrefix(url, "https://billing.stripe.com/") {
		t.Fatal("portail", url, err)
	}
	raw := "%PDF-1.4 fake"
	mockCustomerAPI(t, map[string]string{
		"GET /api/v1/customer/invoices/F-1/download": raw,
		"GET /api/v1/customer/invoices/F-1/cii":      `<CrossIndustryInvoice/>`,
	})
	pdf, err := downloadInvoicePDF("sess-9", "F-1")
	if err != nil || string(pdf) != raw {
		t.Fatal("pdf", err)
	}
	cii, err := downloadInvoiceCII("sess-9", "F-1")
	if err != nil || !strings.Contains(string(cii), "CrossIndustryInvoice") {
		t.Fatal("cii", err)
	}
	if _, err := downloadCustomerFile("sess-9", "/api/v1/admin/invoices/upload"); err == nil {
		t.Fatal("chemin hors espace client accepté")
	}
}

func TestCustomerInterventionsFlow(t *testing.T) {
	mockCustomerAPI(t, map[string]string{
		"GET /api/v1/customer/interventions":                 `{"interventions":[{"id":1,"intervention_id":"INT-1","license_id":"LIC-1","title":"Dépannage","status":"completed","created_at":"2026-03-01T00:00:00Z","updated_at":"2026-03-01T01:00:00Z"}]}`,
		"POST /api/v1/customer/interventions":                `201:{"id":2,"intervention_id":"INT-2","license_id":"LIC-1","title":"Install","status":"open","created_at":"","updated_at":""}`,
		"POST /api/v1/customer/interventions/INT-2/start":    `{"id":2,"intervention_id":"INT-2","status":"in_progress"}`,
		"POST /api/v1/customer/interventions/INT-2/complete": `{"id":2,"intervention_id":"INT-2","status":"completed"}`,
		"POST /api/v1/customer/interventions/INT-2/cancel":   `{"id":2,"intervention_id":"INT-2","status":"cancelled"}`,
		"GET /api/v1/customer/interventions/export":          "id;title\n1;Dépannage\n",
	})
	items, err := listCustomerInterventions("sess-9")
	if err != nil || len(items) != 1 || items[0].InterventionID != "INT-1" {
		t.Fatalf("liste: %+v %v", items, err)
	}
	created, err := createCustomerIntervention("sess-9", "LIC-1", "CLI", "Install")
	if err != nil || created.InterventionID != "INT-2" {
		t.Fatal("création", created, err)
	}
	for _, action := range []string{"start", "complete", "cancel"} {
		if _, err := runCustomerInterventionAction("sess-9", "INT-2", action, "", "", ""); err != nil {
			t.Fatal(action, err)
		}
	}
	raw, err := exportCustomerInterventionsCSV("sess-9")
	if err != nil || !strings.Contains(string(raw), "Dépannage") {
		t.Fatal("export", err)
	}
}

func TestCustomerServicesFlow(t *testing.T) {
	mockCustomerAPI(t, map[string]string{
		"GET /api/v1/customer/service-billing": `{"available":true,"active_license":true,` +
			`"merchant":{"enabled":true,"account_id":"acct_1"},` +
			`"rates":[{"id":"RT-1","label":"Forfait","mode":"prepaid","cents":5000,"active":true}],` +
			`"work":[{"id":"W-1","license_id":"LIC-1","label":"Dépannage","mode":"prepaid","rate_cents":5000,"state":"prepared","paid":false,"amount_cents":5000,"created_at":1}],` +
			`"terms":{"version":"2026-09-01","sha256":"abc","url":"https://relaisdesk.fr/cgv","accepted_at":"2026-09-02T00:00:00Z"}}`,
		"POST /api/v1/customer/service-billing/terms/accept":      `{"ok":true}`,
		"PUT /api/v1/customer/service-billing":                    `{"ok":true}`,
		"POST /api/v1/customer/service-billing/onboarding":        `{"url":"https://connect.stripe.com/setup/abc"}`,
		"POST /api/v1/customer/service-billing/rates":             `201:{"id":"RT-2","label":"Heure","mode":"hourly","cents":6000,"active":true}`,
		"DELETE /api/v1/customer/service-billing/rates/RT-2":      `{"ok":true}`,
		"POST /api/v1/customer/service-billing/work/W-1/finish":   `{"id":"W-1","state":"finished"}`,
		"POST /api/v1/customer/service-billing/work/W-1/checkout": `{"id":"W-1","state":"prepared","checkout_url":"https://checkout.stripe.com/pay/cs_w"}`,
	})
	nav, err := getCustomerServiceBilling("sess-9")
	if err != nil || !nav.Available || len(nav.Rates) != 1 || len(nav.Work) != 1 || nav.Terms.Version == "" {
		t.Fatalf("navigation: %+v %v", nav, err)
	}
	if err := acceptCustomerServiceTerms("sess-9", "2026-09-01", "abc"); err != nil {
		t.Fatal("cgv", err)
	}
	if err := setCustomerServiceEnabled("sess-9", true); err != nil {
		t.Fatal("activation", err)
	}
	onboardURL, err := startCustomerServiceOnboarding("sess-9")
	if err != nil || !strings.HasPrefix(onboardURL, "https://connect.stripe.com/") {
		t.Fatal("onboarding", onboardURL, err)
	}
	rate, err := createCustomerServiceRate("sess-9", "Heure", "hourly", 6000)
	if err != nil || rate.ID != "RT-2" {
		t.Fatal("tarif", rate, err)
	}
	if err := deleteCustomerServiceRate("sess-9", "RT-2"); err != nil {
		t.Fatal("suppression tarif", err)
	}
	w, err := runCustomerServiceWork("sess-9", "W-1", "checkout")
	if err != nil || w.CheckoutURL == "" {
		t.Fatal("checkout", w, err)
	}
}
