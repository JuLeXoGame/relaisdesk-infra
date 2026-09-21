package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCustomerMagicLinkIsSingleUseAndHashed(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "customer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order, err := CreateOrderWithBilling(db, "owner@example.com", "starter", 1, "stripe", "", "", &BillingDetails{
		Name: "Owner", Address: "1 rue Test", PostalCode: "75001", City: "Paris", CustomerType: "business",
		TermsVersion: "2026-08-25", TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FulfillPendingOrder(db, order.OrderID, "test"); err != nil {
		t.Fatal(err)
	}

	token, eligible, err := CreateCustomerLoginToken(db, "OWNER@example.com")
	if err != nil || !eligible || token == "" {
		t.Fatalf("magic link = %q eligible=%v err=%v", token, eligible, err)
	}
	var plaintextCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM customer_login_tokens WHERE token_hash = ?`, token).Scan(&plaintextCount); err != nil {
		t.Fatal(err)
	}
	if plaintextCount != 0 {
		t.Fatal("magic-link token was stored in plaintext")
	}
	session, email, err := ConsumeCustomerLoginToken(db, token)
	if err != nil || session == "" || email != "owner@example.com" {
		t.Fatalf("consume failed: session=%q email=%q err=%v", session, email, err)
	}
	if _, _, err := ConsumeCustomerLoginToken(db, token); err == nil {
		t.Fatal("single-use magic link was accepted twice")
	}
	if got, err := ValidateCustomerSession(db, session); err != nil || got != email {
		t.Fatalf("session validation = %q, %v", got, err)
	}
	if _, eligible, err := CreateCustomerLoginToken(db, "unknown@example.com"); err != nil || eligible {
		t.Fatalf("unknown account eligibility=%v err=%v", eligible, err)
	}
}

func TestRenewalExtendsExistingLicenseWithoutChangingCredentials(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "renewal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	initial, err := CreateOrderWithBilling(db, "owner@example.com", "starter", 1, "stripe", "", "", &BillingDetails{
		Name: "Owner", Address: "1 rue Test", PostalCode: "75001", City: "Paris", CustomerType: "business",
		TermsVersion: "2026-08-25", TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := FulfillPendingOrder(db, initial.OrderID, "initial")
	if err != nil {
		t.Fatal(err)
	}
	originalExpiry, originalKey := lic.ExpiresAt, lic.LicenseKey
	renewal, err := CreateRenewalOrder(db, lic.Email, lic.LicenseID, "stripe", "2026-08-25", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if renewal.OrderKind != "renewal" || renewal.RenewalLicenseID != lic.LicenseID {
		t.Fatalf("invalid renewal order: %+v", renewal)
	}
	if _, err := CreateRenewalOrder(db, lic.Email, lic.LicenseID, "stripe", "2026-08-25", true, false); err == nil {
		t.Fatal("a second open renewal was created for the same licence")
	}
	if _, err := FulfillPendingOrder(db, renewal.OrderID, "wrong fulfillment path"); err == nil {
		t.Fatal("renewal was accepted by the initial-order fulfillment path")
	}
	renewed, err := FulfillPendingRenewalOrder(db, renewal.OrderID, "renewal")
	if err != nil {
		t.Fatal(err)
	}
	if renewed.LicenseID != lic.LicenseID || renewed.LicenseKey != originalKey {
		t.Fatal("renewal replaced existing licence credentials")
	}
	if renewed.Notes != "initial" {
		t.Fatalf("renewal replaced internal licence notes: %q", renewed.Notes)
	}
	if renewed.ExpiresAt.Sub(originalExpiry.Add(30*24*time.Hour)).Abs() > time.Second {
		t.Fatalf("expiry=%v want=%v", renewed.ExpiresAt, originalExpiry.Add(30*24*time.Hour))
	}
	if _, err := FulfillPendingRenewalOrder(db, renewal.OrderID, "duplicate"); err == nil {
		t.Fatal("renewal fulfillment was not idempotently rejected")
	}
}

func TestViewerCodeCreatesProfessionalInterventionLifecycle(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "interventions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "owner@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	code, err := CreateViewerCode(db, lic.LicenseID, "=cmd|client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('test-public-key', 1)`); err != nil {
		t.Fatal(err)
	}
	items, err := ListInterventionsByLicense(db, lic.LicenseID, 10)
	if err != nil || len(items) != 1 || items[0].ViewerCodeID == nil || *items[0].ViewerCodeID != code.ID {
		t.Fatalf("automatic intervention missing: %+v err=%v", items, err)
	}
	if _, err := ValidateViewerCode(db, code.Code); err != nil {
		t.Fatal(err)
	}
	items, _ = ListInterventionsByLicense(db, lic.LicenseID, 10)
	if items[0].Status != "client_ready" {
		t.Fatalf("status=%q want client_ready", items[0].Status)
	}
	started, err := StartIntervention(db, lic.LicenseID, items[0].InterventionID)
	if err != nil || started.Status != "in_progress" {
		t.Fatalf("start failed: %+v %v", started, err)
	}
	completed, err := CompleteIntervention(db, lic.LicenseID, items[0].InterventionID, "=SUM(A1:A2)", "Maintenance", "Mise à jour terminée")
	if err != nil || completed.Status != "completed" || completed.EndedAt == nil || completed.DurationMinutes == nil {
		t.Fatalf("completion failed: %+v %v", completed, err)
	}
	csv := InterventionCSV([]Intervention{*completed})
	if !strings.Contains(csv, `"'=SUM(A1:A2)"`) {
		t.Fatalf("CSV formula was not neutralized: %s", csv)
	}
}

func TestRenewalReminderClaimIsDeduplicated(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "reminders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "owner@example.com", 7, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateCustomerLoginToken(db, lic.Email); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	candidates, err := ListRenewalReminderCandidates(db, now)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%+v err=%v", candidates, err)
	}
	claimed, err := ClaimRenewalReminder(db, candidates[0], now)
	if err != nil || !claimed {
		t.Fatalf("first claim=%v err=%v", claimed, err)
	}
	if err := FinishRenewalReminder(db, candidates[0], nil); err != nil {
		t.Fatal(err)
	}
	claimed, err = ClaimRenewalReminder(db, candidates[0], now.Add(2*time.Hour))
	if err != nil || claimed {
		t.Fatalf("sent reminder reclaimed=%v err=%v", claimed, err)
	}
}

func TestStableCustomerIDAllowsAnotherAuthorizedEmail(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "customer-id.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "owner@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := GetCustomerIdentityByEmail(db, "owner@example.com")
	if err != nil || owner.ID == 0 || owner.PublicID == "" {
		t.Fatalf("owner=%+v err=%v", owner, err)
	}
	if _, err := db.Exec(`INSERT INTO customer_users (customer_id, email, role) VALUES (?, ?, 'billing')`, owner.ID, "billing@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customer_accounts (email, customer_id) VALUES (?, ?)`, "billing@example.com", owner.ID); err != nil {
		t.Fatal(err)
	}
	items, err := ListLicensesByCustomer(db, "billing@example.com")
	if err != nil || len(items) != 1 || items[0].LicenseID != lic.LicenseID {
		t.Fatalf("delegated licences=%+v err=%v", items, err)
	}
	if _, eligible, err := CreateCustomerLoginToken(db, "billing@example.com"); err != nil || !eligible {
		t.Fatalf("delegated login eligible=%v err=%v", eligible, err)
	}
}
