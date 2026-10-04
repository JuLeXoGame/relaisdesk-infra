package database

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const TrialDays = 30
const TrialTermsVersion = "2026-09-24-fleet-v3"

var ErrTrialUsed = errors.New("essai déjà utilisé ou compte déjà client")
var ErrTrialUnavailable = errors.New("demande d'essai invalide ou expirée")

type Trial struct {
	ID                    string         `json:"id"`
	Email                 string         `json:"-"`
	CustomerID            int64          `json:"-"`
	Plan                  string         `json:"plan"`
	Technicians           int            `json:"technicians"`
	PriceCents            int64          `json:"price_cents"`
	BillingCycle          string         `json:"billing_cycle"`
	Billing               BillingDetails `json:"-"`
	CreatedAt             int64          `json:"-"`
	ExpiresAt             int64          `json:"-"`
	VerifiedAt            int64          `json:"-"`
	State                 string         `json:"state"`
	StripeCustomerID      string         `json:"-"`
	CheckoutSessionID     string         `json:"-"`
	SubscriptionID        string         `json:"-"`
	LicenseID             string         `json:"license_id,omitempty"`
	TrialStart            int64          `json:"trial_start"`
	TrialEnd              int64          `json:"trial_end"`
	PaidThrough           int64          `json:"paid_through"`
	StripeStatus          string         `json:"subscription_status"`
	CancelAtPeriodEnd     bool           `json:"cancel_at_period_end"`
	CancelRequestedAt     int64          `json:"cancel_requested_at,omitempty"`
	WithdrawalRequestedAt int64          `json:"withdrawal_requested_at,omitempty"`
	WithdrawalImmediate   bool           `json:"withdrawal_immediate"`
}

const trialColumns = `id,email,COALESCE(customer_id,0),plan,technicians,price_cents,billing_cycle,billing_json,
 created_at,expires_at,COALESCE(verified_at,0),state,COALESCE(stripe_customer_id,''),COALESCE(checkout_session_id,''),
 COALESCE(subscription_id,''),COALESCE(license_id,''),trial_start,trial_end,paid_through,stripe_status,cancel_at_period_end,COALESCE(cancel_requested_at,0),
 COALESCE((SELECT requested_at FROM trial_withdrawals WHERE application_id=trial_applications.id),0),
 COALESCE((SELECT immediate FROM trial_withdrawals WHERE application_id=trial_applications.id),0)`

func scanTrial(row interface{ Scan(...any) error }) (*Trial, error) {
	t := &Trial{}
	var billing string
	err := row.Scan(&t.ID, &t.Email, &t.CustomerID, &t.Plan, &t.Technicians, &t.PriceCents, &t.BillingCycle, &billing,
		&t.CreatedAt, &t.ExpiresAt, &t.VerifiedAt, &t.State, &t.StripeCustomerID, &t.CheckoutSessionID, &t.SubscriptionID,
		&t.LicenseID, &t.TrialStart, &t.TrialEnd, &t.PaidThrough, &t.StripeStatus, &t.CancelAtPeriodEnd, &t.CancelRequestedAt, &t.WithdrawalRequestedAt, &t.WithdrawalImmediate)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(billing), &t.Billing); err != nil {
		return nil, err
	}
	return t, nil
}
func GetTrial(db *sql.DB, id string) (*Trial, error) {
	return scanTrial(db.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE id=?`, id))
}
func GetTrialBySubscription(db *sql.DB, id string) (*Trial, error) {
	return scanTrial(db.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE subscription_id=?`, id))
}
func ListCustomerTrials(db *sql.DB, customerID int64) ([]Trial, error) {
	rows, err := db.Query(`SELECT `+trialColumns+` FROM trial_applications WHERE customer_id=? AND state IN ('provisioning','active') ORDER BY created_at DESC`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trial{}
	for rows.Next() {
		t, err := scanTrial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Domain separation prevents correlating different types of fraud markers.
// No card number, CVC, IBAN or raw provider fingerprint is persisted here.
func TrialMarker(secret []byte, kind, value string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(kind + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func CreateTrialApplication(db *sql.DB, email, plan string, techs int, b BillingDetails, now time.Time) (*Trial, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if b.TermsVersion != TrialTermsVersion || !b.TermsAccepted {
		return nil, ErrTrialUnavailable
	}
	if b.BillingCycle != "monthly" && b.BillingCycle != "annual" {
		return nil, ErrTrialUnavailable
	}
	if strings.EqualFold(plan, "pro") {
		techs = 5
	}
	price, techs, plan, err := CalculateServerPrice(plan, techs, b.BillingCycle)
	if err != nil {
		return nil, err
	}
	id, err := generateSecureToken()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	// Rate-limit by destination as well as HTTP source; no address enumeration.
	var recent int
	if err = db.QueryRow(`SELECT COUNT(*) FROM trial_applications WHERE email=? AND created_at>?`, email, now.Add(-time.Hour).Unix()).Scan(&recent); err != nil {
		return nil, err
	}
	if recent >= 3 {
		return nil, ErrTrialUnavailable
	}
	_, err = db.Exec(`INSERT INTO trial_applications(id,email,plan,technicians,price_cents,billing_cycle,billing_json,created_at,expires_at)
 VALUES(?,?,?,?,?,?,?,?,?)`, id, email, plan, techs, int64(price*100+0.5), b.BillingCycle, string(encoded), now.Unix(), now.Add(time.Hour).Unix())
	if err != nil {
		return nil, err
	}
	return GetTrial(db, id)
}

func CreateTrialEmailToken(db *sql.DB, id string, now time.Time) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	res, err := db.Exec(`UPDATE trial_applications SET token_hash=?,token_expires_at=? WHERE id=? AND state='pending' AND expires_at>?`, hashSessionToken(token), now.Add(15*time.Minute).Unix(), id, now.Unix())
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return "", ErrTrialUnavailable
	}
	return token, nil
}

func VerifyTrialEmail(db *sql.DB, token string, now time.Time) (*Trial, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := scanTrial(tx.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE token_hash=? AND token_expires_at>? AND expires_at>? AND state='pending'`, hashSessionToken(token), now.Unix(), now.Unix()))
	if err != nil {
		return nil, ErrTrialUnavailable
	}
	customer, err := ensureCustomer(tx, t.Email, t.Billing.CustomerType, t.Billing.Name)
	if err != nil {
		return nil, err
	}
	res, err := tx.Exec(`UPDATE trial_applications SET state='verified',verified_at=?,customer_id=?,token_hash=NULL WHERE id=? AND state='pending'`, now.Unix(), customer.ID, t.ID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, ErrTrialUnavailable
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetTrial(db, t.ID)
}

// ReserveTrial atomically consumes the offer, across accounts/plans/concurrent
// requests. Markers outlive expiry, cancellation and non-renewal of the licence.
func ReserveTrial(db *sql.DB, id string, secret []byte, fingerprint string, now time.Time) (*Trial, error) {
	if len(secret) != 32 || fingerprint == "" || len(fingerprint) > 255 {
		return nil, ErrTrialUnavailable
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Accidentally rotating the HMAC secret must not erase card history.
	digest := TrialMarker(secret, "key-check", "relaisdesk-trials-v1")
	if _, err = tx.Exec(`INSERT INTO trial_hmac_check(singleton,digest) VALUES(1,?) ON CONFLICT(singleton) DO NOTHING`, digest); err != nil {
		return nil, err
	}
	var previous string
	if err = tx.QueryRow(`SELECT digest FROM trial_hmac_check WHERE singleton=1`).Scan(&previous); err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(previous), []byte(digest)) {
		return nil, errors.New("TRIAL_FINGERPRINT_KEY modifiée : restauration ou migration des marqueurs requise")
	}
	t, err := scanTrial(tx.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if t.State == "provisioning" || t.State == "active" {
		return t, nil
	}
	if t.State != "verified" || t.CustomerID == 0 || t.ExpiresAt <= now.Unix() {
		return nil, ErrTrialUnavailable
	}
	var used int
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM licences WHERE customer_id=? OR email=? COLLATE NOCASE)`, t.CustomerID, t.Email).Scan(&used); err != nil {
		return nil, err
	}
	if used != 0 {
		return nil, ErrTrialUsed
	}
	markers := map[string]string{"email": t.Email, "customer": fmt.Sprint(t.CustomerID), "card": fingerprint}
	// A self-declared SIRET does not prove ownership: do not allow an attacker
	// to burn another business's eligibility merely by quoting its public SIRET.
	for kind, value := range markers {
		hash := TrialMarker(secret, kind, value)
		if err = tx.QueryRow(`SELECT COUNT(*) FROM trial_claims WHERE kind=? AND key_hash=?`, kind, hash).Scan(&used); err != nil {
			return nil, err
		}
		if used != 0 {
			return nil, ErrTrialUsed
		}
		if _, err = tx.Exec(`INSERT INTO trial_claims(kind,key_hash,application_id,created_at) VALUES(?,?,?,?)`, kind, hash, id, now.Unix()); err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(`UPDATE trial_applications SET state='provisioning',trial_start=?,trial_end=? WHERE id=?`, now.Unix(), now.Add(TrialDays*24*time.Hour).Unix(), id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetTrial(db, id)
}

// ActivateTrial provisions the trial license and returns the trial plus the
// plaintext license key for one-time delivery ("" when the trial was already
// active: the key was delivered by the first activation).
func ActivateTrial(db *sql.DB, id, subscriptionID string, trialEnd int64) (*Trial, string, error) {
	if subscriptionID == "" {
		return nil, "", ErrTrialUnavailable
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	t, err := scanTrial(tx.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE id=?`, id))
	if err != nil {
		return nil, "", err
	}
	if t.State == "active" && t.SubscriptionID == subscriptionID {
		return t, "", nil
	}
	if t.State != "provisioning" || t.TrialEnd != trialEnd {
		return nil, "", ErrTrialUnavailable
	}
	licenseID, err := generateLicenseID()
	if err != nil {
		return nil, "", err
	}
	key, err := generateLicenseKey()
	if err != nil {
		return nil, "", err
	}
	_, err = tx.Exec(`INSERT INTO licences(customer_id,license_id,email,license_key,key_hint,expires_at,max_connections,notes) VALUES(?,?,?,?,?,?,?,?)`,
		t.CustomerID, licenseID, t.Email, HashLicenseKey(key), LicenseKeyHint(key), time.Unix(trialEnd, 0).UTC().Format(time.RFC3339), t.Technicians, "Essai 30 jours ; abonnement Stripe "+subscriptionID)
	if err != nil {
		return nil, "", err
	}
	_, err = tx.Exec(`UPDATE trial_applications SET state='active',subscription_id=?,license_id=?,stripe_status='trialing' WHERE id=?`, subscriptionID, licenseID, id)
	if err != nil {
		return nil, "", err
	}
	if err = tx.Commit(); err != nil {
		return nil, "", err
	}
	trial, err := GetTrial(db, id)
	if err != nil {
		return nil, "", err
	}
	return trial, key, nil
}

// RecordSubscriptionPayment uses the actual paid period, never now+30: replay,
// out-of-order delivery and late payment cannot grant an extra free month.
func RecordSubscriptionPayment(db *sql.DB, id, invoiceID string, amount, start, end int64) (*Order, error) {
	if invoiceID == "" || len(invoiceID) > 200 || start <= 0 || end <= start {
		return nil, errors.New("période de facture invalide")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := scanTrial(tx.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if t.State != "active" || amount != t.PriceCents || start < t.TrialEnd {
		return nil, errors.New("facture hors contrat")
	}
	orderID := "SUB-" + invoiceID
	var existing string
	err = tx.QueryRow(`SELECT order_id FROM subscription_payments WHERE stripe_invoice_id=? AND application_id=?`, invoiceID, id).Scan(&existing)
	if err == nil {
		tx.Rollback()
		return GetOrderByID(db, existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	b := t.Billing
	_, err = tx.Exec(`INSERT INTO orders(customer_id,order_id,email,plan,technicians,price,payment_method,status,license_id,
 billing_name,billing_address,billing_postal_code,billing_city,billing_country,billing_siret,customer_type,terms_version,
 terms_accepted_at,immediate_performance_requested,billing_cycle,order_kind,renewal_license_id,paid_at,notes)
 VALUES(?,?,?,?,?,?,'stripe','paid',?,?,?,?,?,?,?,?,?,?,?,?, 'renewal',?,?,?)`,
		t.CustomerID, orderID, t.Email, t.Plan, t.Technicians, float64(amount)/100, t.LicenseID,
		b.Name, b.Address, b.PostalCode, b.City, b.Country, b.SIRET, b.CustomerType, b.TermsVersion, time.Unix(t.CreatedAt, 0).UTC().Format(time.RFC3339), b.ImmediatePerformanceRequested, t.BillingCycle,
		t.LicenseID, time.Now().UTC().Format(time.RFC3339), "Abonnement Stripe "+t.SubscriptionID+" / "+invoiceID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`INSERT INTO subscription_payments VALUES(?,?,?,?,?)`, invoiceID, id, orderID, start, end)
	if err != nil {
		return nil, err
	}
	// A payment cannot silently undo an administrator's security revocation.
	_, err = tx.Exec(`UPDATE licences SET expires_at=?,status='active' WHERE license_id=? AND status!='revoked' AND datetime(expires_at)<datetime(?)
        AND NOT EXISTS (SELECT 1 FROM trial_withdrawals WHERE application_id=? AND immediate=1)`,
		time.Unix(end, 0).UTC().Format(time.RFC3339), t.LicenseID, time.Unix(end, 0).UTC().Format(time.RFC3339), t.ID)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`UPDATE trial_applications SET paid_through=MAX(paid_through,?) WHERE id=?`, end, id)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE trial_withdrawals SET status='refund_review' WHERE application_id=? AND immediate=1`, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetOrderByID(db, orderID)
}

func LicenseHasSubscription(db *sql.DB, licenseID string) (bool, error) {
	var exists int
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM trial_applications WHERE license_id=? AND state='active' AND stripe_status NOT IN ('canceled','incomplete_expired'))`, licenseID).Scan(&exists)
	return exists == 1, err
}
