package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const maxOrderTechnicians = 10000

// Catalog prices apply to new orders and renewals, never to stored order amounts.
const (
	ProMonthlyPrice       = 110.00
	ProAnnualPrice        = 10 * ProMonthlyPrice
	UltraBaseMonthlyPrice = 199.00
	UltraBaseAnnualPrice  = 10 * UltraBaseMonthlyPrice
)

// Order represents a subscription order in SQLite.
type Order struct {
	ID                            int        `json:"id"`
	CustomerID                    int64      `json:"-"`
	OrderID                       string     `json:"order_id"`
	Email                         string     `json:"email"`
	Plan                          string     `json:"plan"`
	Technicians                   int        `json:"technicians"`
	Price                         float64    `json:"price"`
	PaymentMethod                 string     `json:"payment_method"`
	Status                        string     `json:"status"`
	StripeSessionID               string     `json:"stripe_session_id"`
	LicenseID                     string     `json:"license_id"`
	BillingName                   string     `json:"billing_name"`
	BillingAddress                string     `json:"billing_address"`
	BillingPostalCode             string     `json:"billing_postal_code"`
	BillingCity                   string     `json:"billing_city"`
	BillingCountry                string     `json:"billing_country"`
	BillingSIRET                  string     `json:"billing_siret"`
	InvoiceNumber                 string     `json:"invoice_number"`
	CustomerType                  string     `json:"customer_type"`
	TermsVersion                  string     `json:"terms_version"`
	TermsAcceptedAt               *time.Time `json:"terms_accepted_at,omitempty"`
	ImmediatePerformanceRequested bool       `json:"immediate_performance_requested"`
	OrderKind                     string     `json:"order_kind"`
	RenewalLicenseID              string     `json:"renewal_license_id,omitempty"`
	BillingCycle                  string     `json:"billing_cycle"`
	CreatedAt                     time.Time  `json:"created_at"`
	PaidAt                        *time.Time `json:"paid_at"`
	ExpiresAt                     *time.Time `json:"expires_at"`
	Notes                         string     `json:"notes"`
}

// BillingDetails holds buyer invoicing information.
type BillingDetails struct {
	Name                          string `json:"name"`
	Address                       string `json:"address"`
	PostalCode                    string `json:"postal_code"`
	City                          string `json:"city"`
	Country                       string `json:"country"`
	SIRET                         string `json:"siret"`
	CustomerType                  string `json:"customer_type"`
	TermsVersion                  string `json:"terms_version"`
	TermsAccepted                 bool   `json:"terms_accepted"`
	ImmediatePerformanceRequested bool   `json:"immediate_performance_requested"`
	BillingCycle                  string `json:"billing_cycle,omitempty"`
}

// ValidPaymentMethod reports whether a payment method is accepted for orders.
func ValidPaymentMethod(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "stripe", "bank_transfer", "crypto_btc", "crypto_xrp":
		return true
	default:
		return false
	}
}

// CalculateServerPrice calculates the official price and technician count on the server side.
func CalculateServerPrice(plan string, requestedTechs int, billingCycle ...string) (price float64, technicians int, planName string, err error) {
	cycle := "monthly"
	if len(billingCycle) > 0 && strings.ToLower(strings.TrimSpace(billingCycle[0])) == "annual" {
		cycle = "annual"
	}

	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "starter":
		if cycle == "annual" {
			return 239.00, 1, "Starter", nil
		}
		return 24.90, 1, "Starter", nil

	case "pro":
		techs := requestedTechs
		if techs < 1 || techs > 5 {
			techs = 5
		}
		if cycle == "annual" {
			return ProAnnualPrice, techs, "Pro", nil
		}
		return ProMonthlyPrice, techs, "Pro", nil

	case "ultra", "custom", "personnalise", "personnalisé":
		techs := requestedTechs
		if techs < 10 {
			techs = 10
		}
		if techs > 500 {
			return 0, 0, "", fmt.Errorf("nombre de techniciens limité à %d", 500)
		}

		// Bareme mensuel :
		// Base 10 techs : 199.00€
		// 10 à 50 techs : +14.00€ / tech (+140€ / tranche de 10)
		// 50 à 100 techs : +10.00€ / tech (+100€ / tranche de 10)
		// 100 à 500 techs : +7.00€ / tech (+70€ / tranche de 10)
		var monthlyPrice float64
		if techs <= 10 {
			monthlyPrice = UltraBaseMonthlyPrice
		} else if techs <= 50 {
			extra := techs - 10
			monthlyPrice = UltraBaseMonthlyPrice + float64(extra)*14.00
		} else if techs <= 100 {
			base50 := UltraBaseMonthlyPrice + 40.0*14.00 // 759.00€
			extra := techs - 50
			monthlyPrice = base50 + float64(extra)*10.00
		} else {
			base100 := UltraBaseMonthlyPrice + 40.0*14.00 + 50.0*10.00 // 1259.00€
			extra := techs - 100
			monthlyPrice = base100 + float64(extra)*7.00
		}

		if cycle == "annual" {
			annualPrice := math.Round((monthlyPrice*10)*100) / 100
			return annualPrice, techs, "Ultra", nil
		}

		rounded := math.Round(monthlyPrice*100) / 100
		return rounded, techs, "Ultra", nil

	default:
		return 0, 0, "", fmt.Errorf("plan inconnu %q (choix: starter, pro, ultra)", plan)
	}
}

// GenerateOrderID generates an unpredictable order ID (64-bit crypto
// randomness, format ORD-XXXXXXXX-XXXXXXXX). It fails closed: there is no
// predictable time-based fallback.
func GenerateOrderID() (string, error) {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("génération identifiant commande: %w", err)
	}
	hexStr := strings.ToUpper(hex.EncodeToString(entropy[:]))
	return fmt.Sprintf("ORD-%s-%s", hexStr[:8], hexStr[8:]), nil
}

// CreateOrder inserts a new order with pending status and 48-hour expiration.
func CreateOrder(db *sql.DB, email, plan string, requestedTechs int, paymentMethod, notes, stripeSessionID string) (*Order, error) {
	return CreateOrderWithBilling(db, email, plan, requestedTechs, paymentMethod, notes, stripeSessionID, nil)
}

// CreateOrderWithBilling inserts a new order with optional billing details.
func CreateOrderWithBilling(db *sql.DB, email, plan string, requestedTechs int, paymentMethod, notes, stripeSessionID string, billing *BillingDetails) (*Order, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, fmt.Errorf("email requis")
	}

	billingCycle := "monthly"
	if billing != nil && strings.ToLower(strings.TrimSpace(billing.BillingCycle)) == "annual" {
		billingCycle = "annual"
	}

	price, techs, canonicalPlan, err := CalculateServerPrice(plan, requestedTechs, billingCycle)
	if err != nil {
		return nil, err
	}

	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	if !ValidPaymentMethod(paymentMethod) {
		return nil, fmt.Errorf("méthode de paiement invalide (choix: stripe, bank_transfer, crypto_btc, crypto_xrp)")
	}

	var bName, bAddr, bZip, bCity, bCountry, bSiret string
	customerType := "business"
	termsVersion := "legacy"
	var termsAcceptedAt interface{}
	var termsAcceptedAtTime *time.Time
	immediatePerformanceRequested := false
	if billing != nil {
		bName = strings.TrimSpace(billing.Name)
		bAddr = strings.TrimSpace(billing.Address)
		bZip = strings.TrimSpace(billing.PostalCode)
		bCity = strings.TrimSpace(billing.City)
		bCountry = strings.TrimSpace(billing.Country)
		if bCountry == "" {
			bCountry = "France"
		}
		bSiret = strings.TrimSpace(billing.SIRET)
		customerType = strings.ToLower(strings.TrimSpace(billing.CustomerType))
		if customerType == "" {
			customerType = "business"
		}
		if customerType != "business" && customerType != "consumer" {
			return nil, fmt.Errorf("type de client invalide")
		}
		termsVersion = strings.TrimSpace(billing.TermsVersion)
		if termsVersion == "" {
			termsVersion = "legacy"
		}
		if billing.TermsAccepted {
			acceptedAt := time.Now().UTC()
			termsAcceptedAt = acceptedAt.Format(time.RFC3339)
			termsAcceptedAtTime = &acceptedAt
		}
		immediatePerformanceRequested = billing.ImmediatePerformanceRequested
	}
	customer, err := EnsureCustomer(db, email, customerType, bName)
	if err != nil {
		return nil, err
	}

	orderID, err := GenerateOrderID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(48 * time.Hour)

	query := `
		INSERT INTO orders (
			customer_id, order_id, email, plan, technicians, price, payment_method, status,
			stripe_session_id, billing_name, billing_address, billing_postal_code,
			billing_city, billing_country, billing_siret, customer_type, terms_version,
			terms_accepted_at, immediate_performance_requested, billing_cycle, created_at, expires_at, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.Exec(query,
		customer.ID, orderID, email, canonicalPlan, techs, price, paymentMethod,
		stripeSessionID, bName, bAddr, bZip, bCity, bCountry, bSiret,
		customerType, termsVersion, termsAcceptedAt, immediatePerformanceRequested,
		billingCycle, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339), notes,
	)
	if err != nil {
		return nil, fmt.Errorf("création commande: %w", err)
	}

	return &Order{
		CustomerID:                    customer.ID,
		OrderID:                       orderID,
		Email:                         email,
		Plan:                          canonicalPlan,
		Technicians:                   techs,
		Price:                         price,
		PaymentMethod:                 paymentMethod,
		Status:                        "pending",
		StripeSessionID:               stripeSessionID,
		BillingName:                   bName,
		BillingAddress:                bAddr,
		BillingPostalCode:             bZip,
		BillingCity:                   bCity,
		BillingCountry:                bCountry,
		BillingSIRET:                  bSiret,
		CustomerType:                  customerType,
		TermsVersion:                  termsVersion,
		TermsAcceptedAt:               termsAcceptedAtTime,
		ImmediatePerformanceRequested: immediatePerformanceRequested,
		OrderKind:                     "initial",
		BillingCycle:                  billingCycle,
		CreatedAt:                     now,
		ExpiresAt:                     &expiresAt,
		Notes:                         notes,
	}, nil
}

// CreateRenewalOrder creates a payment order tied to an existing licence. It
// copies the latest verified billing identity but records a fresh acceptance of
// the current terms supplied by the customer portal.
func CreateRenewalOrder(db *sql.DB, email, licenseID, paymentMethod, termsVersion string, termsAccepted, immediatePerformanceRequested bool) (*Order, error) {
	if recurring, err := LicenseHasSubscription(db, licenseID); err != nil {
		return nil, err
	} else if recurring {
		return nil, errors.New("cet abonnement se renouvelle via Stripe ; gérez sa reconduction dans l'espace client")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	identity, identityErr := GetCustomerIdentityByEmail(db, email)
	if identityErr != nil {
		return nil, errors.New("compte client introuvable")
	}
	licenseID = strings.TrimSpace(licenseID)
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	termsVersion = strings.TrimSpace(termsVersion)
	if !ValidPaymentMethod(paymentMethod) {
		return nil, errors.New("méthode de paiement invalide (choix: stripe, bank_transfer, crypto_btc, crypto_xrp)")
	}
	if !termsAccepted || termsVersion == "" {
		return nil, errors.New("acceptation des conditions requise")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("début transaction renouvellement: %w", err)
	}
	defer tx.Rollback()

	var licenseEmail, licenseStatus string
	var customerID int64
	var maxConnections int
	if err := tx.QueryRow(`
		SELECT email, status, max_connections, customer_id FROM licences WHERE license_id = ?
	`, licenseID).Scan(&licenseEmail, &licenseStatus, &maxConnections, &customerID); err != nil || customerID != identity.ID {
		return nil, errors.New("licence introuvable")
	}
	email = identity.BillingEmail
	if licenseStatus == "revoked" {
		return nil, errors.New("une licence révoquée ne peut pas être renouvelée")
	}

	query := `
		SELECT id, order_id, email, plan, technicians, price, payment_method, status,
		       COALESCE(stripe_session_id, ''), COALESCE(license_id, ''),
		       COALESCE(billing_name, ''), COALESCE(billing_address, ''),
		       COALESCE(billing_postal_code, ''), COALESCE(billing_city, ''),
		       COALESCE(billing_country, 'France'), COALESCE(billing_siret, ''),
		       COALESCE(invoice_number, ''),
		       COALESCE(customer_type, 'business'), COALESCE(terms_version, 'legacy'),
		       terms_accepted_at, COALESCE(immediate_performance_requested, 0),
		       COALESCE(order_kind, 'initial'), COALESCE(renewal_license_id, ''),
		       COALESCE(billing_cycle, 'monthly'),
		       created_at, paid_at, expires_at, COALESCE(notes, '')
		FROM orders
		WHERE status = 'paid' AND (license_id = ? OR renewal_license_id = ?)
		ORDER BY COALESCE(paid_at, created_at) DESC LIMIT 1
	`
	source, err := scanOrder(tx.QueryRow(query, licenseID, licenseID))
	if err != nil {
		return nil, errors.New("commande d'origine introuvable")
	}
	if source.CustomerType == "consumer" && !immediatePerformanceRequested {
		return nil, errors.New("demande expresse de renouvellement immédiat requise")
	}
	billingCycle := source.BillingCycle
	if billingCycle == "" {
		billingCycle = "monthly"
	}
	price, technicians, canonicalPlan, err := CalculateServerPrice(source.Plan, maxConnections, billingCycle)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(48 * time.Hour)
	// Expired attempts no longer reserve the licence. The partial unique index
	// below the schema still protects against concurrent open renewals.
	if _, err := tx.Exec(`
		UPDATE orders SET status = 'cancelled'
		WHERE renewal_license_id = ? AND order_kind = 'renewal' AND status = 'pending'
		  AND datetime(expires_at) <= datetime(?)
	`, licenseID, now.Format(time.RFC3339)); err != nil {
		return nil, err
	}
	var openRenewal bool
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM orders WHERE renewal_license_id = ?
			  AND order_kind = 'renewal' AND status IN ('pending', 'processing')
		)
	`, licenseID).Scan(&openRenewal); err != nil {
		return nil, err
	}
	if openRenewal {
		return nil, errors.New("un renouvellement est déjà en attente pour cette licence")
	}

	orderID, err := GenerateOrderID()
	if err != nil {
		return nil, err
	}
	notes := fmt.Sprintf("Renouvellement licence %s", licenseID)
	_, err = tx.Exec(`
		INSERT INTO orders (
			customer_id, order_id, email, plan, technicians, price, payment_method, status,
			billing_name, billing_address, billing_postal_code, billing_city,
			billing_country, billing_siret, customer_type, terms_version,
			terms_accepted_at, immediate_performance_requested, order_kind,
			renewal_license_id, billing_cycle, created_at, expires_at, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		          'renewal', ?, ?, ?, ?, ?)
	`, customerID, orderID, email, canonicalPlan, technicians, price, paymentMethod,
		source.BillingName, source.BillingAddress, source.BillingPostalCode, source.BillingCity,
		source.BillingCountry, source.BillingSIRET, source.CustomerType, termsVersion,
		now.Format(time.RFC3339), immediatePerformanceRequested, licenseID,
		billingCycle, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339), notes)
	if err != nil {
		return nil, fmt.Errorf("création renouvellement: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	acceptedAt := now
	return &Order{
		CustomerID: customerID, OrderID: orderID, Email: email, Plan: canonicalPlan, Technicians: technicians,
		Price: price, PaymentMethod: paymentMethod, Status: "pending",
		BillingName: source.BillingName, BillingAddress: source.BillingAddress,
		BillingPostalCode: source.BillingPostalCode, BillingCity: source.BillingCity,
		BillingCountry: source.BillingCountry, BillingSIRET: source.BillingSIRET,
		CustomerType: source.CustomerType, TermsVersion: termsVersion,
		TermsAcceptedAt: &acceptedAt, ImmediatePerformanceRequested: immediatePerformanceRequested,
		OrderKind: "renewal", RenewalLicenseID: licenseID, BillingCycle: billingCycle, CreatedAt: now,
		ExpiresAt: &expiresAt, Notes: notes,
	}, nil
}

// UpdateOrderBillingDetails updates billing information on an order.
func UpdateOrderBillingDetails(db *sql.DB, orderID string, billing *BillingDetails) error {
	if billing == nil {
		return nil
	}
	country := strings.TrimSpace(billing.Country)
	if country == "" {
		country = "France"
	}
	query := `
		UPDATE orders
		SET billing_name = ?, billing_address = ?, billing_postal_code = ?,
		    billing_city = ?, billing_country = ?, billing_siret = ?
		WHERE order_id = ?
	`
	_, err := db.Exec(query,
		strings.TrimSpace(billing.Name),
		strings.TrimSpace(billing.Address),
		strings.TrimSpace(billing.PostalCode),
		strings.TrimSpace(billing.City),
		country,
		strings.TrimSpace(billing.SIRET),
		orderID,
	)
	return err
}

// LinkOrderInvoice associates an invoice number with an order.
func LinkOrderInvoice(db *sql.DB, orderID, invoiceNumber string) error {
	query := `UPDATE orders SET invoice_number = ? WHERE order_id = ?`
	_, err := db.Exec(query, invoiceNumber, orderID)
	return err
}

// OrderEmailStatus records which transactional messages were durably accepted
// by the SMTP layer. A failed delivery remains retryable on the next webhook or
// explicit mark-paid attempt.
type OrderEmailStatus struct {
	LicenseSent bool
	InvoiceSent bool
}

func GetOrderEmailStatus(db *sql.DB, orderID string) (OrderEmailStatus, error) {
	var status OrderEmailStatus
	err := db.QueryRow(`
		SELECT license_email_sent_at IS NOT NULL, invoice_email_sent_at IS NOT NULL
		FROM orders WHERE order_id = ?
	`, orderID).Scan(&status.LicenseSent, &status.InvoiceSent)
	return status, err
}

// GetOrderEmailStatusBatch returns email delivery flags for many orders in a
// few chunked queries instead of one query per order (N+1). Unknown IDs are
// simply absent from the returned map.
func GetOrderEmailStatusBatch(db *sql.DB, orderIDs []string) (map[string]OrderEmailStatus, error) {
	out := make(map[string]OrderEmailStatus, len(orderIDs))
	for _, chunk := range chunkStrings(orderIDs, 500) {
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		rows, err := db.Query(fmt.Sprintf(`
			SELECT order_id, license_email_sent_at IS NOT NULL, invoice_email_sent_at IS NOT NULL
			FROM orders WHERE order_id IN (%s)
		`, strings.Join(placeholders, ",")), args...)
		if err != nil {
			return nil, fmt.Errorf("requête batch statuts email: %w", err)
		}
		for rows.Next() {
			var id string
			var st OrderEmailStatus
			if err := rows.Scan(&id, &st.LicenseSent, &st.InvoiceSent); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = st
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("requête batch statuts email: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

func MarkOrderEmailSent(db *sql.DB, orderID, messageType string) error {
	var query string
	switch messageType {
	case "license":
		query = `UPDATE orders SET license_email_sent_at = COALESCE(license_email_sent_at, ?) WHERE order_id = ?`
	case "invoice":
		query = `UPDATE orders SET invoice_email_sent_at = COALESCE(invoice_email_sent_at, ?) WHERE order_id = ?`
	default:
		return fmt.Errorf("type de notification inconnu")
	}
	result, err := db.Exec(query, time.Now().UTC().Format(time.RFC3339), orderID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("commande introuvable (%s)", orderID)
	}
	return nil
}

// GetOrderByID retrieves an order by its order_id.
func GetOrderByID(db *sql.DB, orderID string) (*Order, error) {
	query := `
		SELECT id, order_id, email, plan, technicians, price, payment_method, status,
		       COALESCE(stripe_session_id, ''), COALESCE(license_id, ''),
		       COALESCE(billing_name, ''), COALESCE(billing_address, ''),
		       COALESCE(billing_postal_code, ''), COALESCE(billing_city, ''),
		       COALESCE(billing_country, 'France'), COALESCE(billing_siret, ''),
		       COALESCE(invoice_number, ''),
		       COALESCE(customer_type, 'business'), COALESCE(terms_version, 'legacy'),
		       terms_accepted_at, COALESCE(immediate_performance_requested, 0),
		       COALESCE(order_kind, 'initial'), COALESCE(renewal_license_id, ''),
		       COALESCE(billing_cycle, 'monthly'),
		       created_at, paid_at, expires_at, COALESCE(notes, '')
		FROM orders
		WHERE order_id = ?
	`
	return scanOrder(db.QueryRow(query, orderID))
}

// GetOrderByStripeSessionID retrieves an order by its Stripe checkout session ID.
func GetOrderByStripeSessionID(db *sql.DB, sessionID string) (*Order, error) {
	query := `
		SELECT id, order_id, email, plan, technicians, price, payment_method, status,
		       COALESCE(stripe_session_id, ''), COALESCE(license_id, ''),
		       COALESCE(billing_name, ''), COALESCE(billing_address, ''),
		       COALESCE(billing_postal_code, ''), COALESCE(billing_city, ''),
		       COALESCE(billing_country, 'France'), COALESCE(billing_siret, ''),
		       COALESCE(invoice_number, ''),
		       COALESCE(customer_type, 'business'), COALESCE(terms_version, 'legacy'),
		       terms_accepted_at, COALESCE(immediate_performance_requested, 0),
		       COALESCE(order_kind, 'initial'), COALESCE(renewal_license_id, ''),
		       COALESCE(billing_cycle, 'monthly'),
		       created_at, paid_at, expires_at, COALESCE(notes, '')
		FROM orders
		WHERE stripe_session_id = ?
	`
	return scanOrder(db.QueryRow(query, sessionID))
}

// UpdateOrderStripeSessionID updates the stripe_session_id of an order.
func UpdateOrderStripeSessionID(db *sql.DB, orderID, sessionID string) error {
	query := `UPDATE orders SET stripe_session_id = ? WHERE order_id = ?`
	_, err := db.Exec(query, sessionID, orderID)
	return err
}

// MarkOrderPaid updates an order to paid and associates it with a license.
func MarkOrderPaid(db *sql.DB, orderID, licenseID string) error {
	now := time.Now().UTC()
	query := `
		UPDATE orders
		SET status = 'paid', paid_at = ?, license_id = ?
		WHERE order_id = ? AND status = 'pending'
	`
	res, err := db.Exec(query, now.Format(time.RFC3339), licenseID, orderID)
	if err != nil {
		return fmt.Errorf("mise à jour commande payée: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("commande introuvable ou déjà traitée (%s)", orderID)
	}
	return nil
}

// FulfillPendingOrder atomically claims a pending order, creates its license and marks it paid.
func FulfillPendingOrder(db *sql.DB, orderID, notes string) (*License, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("début transaction paiement: %w", err)
	}
	defer tx.Rollback()

	claimed, err := tx.Exec(`
		UPDATE orders SET status = 'processing'
		WHERE order_id = ? AND status = 'pending'
		  AND COALESCE(order_kind, 'initial') = 'initial'
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("verrouillage commande: %w", err)
	}
	count, err := claimed.RowsAffected()
	if err != nil || count != 1 {
		return nil, fmt.Errorf("commande introuvable ou déjà traitée (%s)", orderID)
	}

	var email, billingCycle string
	var customerID int64
	var technicians int
	if err := tx.QueryRow(`SELECT email, technicians, customer_id, COALESCE(billing_cycle, 'monthly') FROM orders WHERE order_id = ?`, orderID).Scan(&email, &technicians, &customerID, &billingCycle); err != nil {
		return nil, fmt.Errorf("lecture commande: %w", err)
	}

	now := time.Now().UTC()
	duration := 30 * 24 * time.Hour
	if strings.ToLower(billingCycle) == "annual" {
		duration = 365 * 24 * time.Hour
	}
	expiresAt := now.Add(duration)
	var lic *License
	for attempt := 0; attempt < 5; attempt++ {
		licenseID, genErr := generateLicenseID()
		if genErr != nil {
			return nil, genErr
		}
		licenseKey, genErr := generateLicenseKey()
		if genErr != nil {
			return nil, genErr
		}
		result, insertErr := tx.Exec(`
			INSERT INTO licences (customer_id, license_id, email, license_key, expires_at, max_connections, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, customerID, licenseID, email, licenseKey, expiresAt.Format("2006-01-02 15:04:05"), technicians, notes)
		if insertErr != nil {
			continue
		}
		rowID, _ := result.LastInsertId()
		lic = &License{
			ID: int(rowID), CustomerID: customerID, LicenseID: licenseID, Email: email, LicenseKey: licenseKey,
			Status: "active", CreatedAt: now, ExpiresAt: expiresAt, MaxConnections: technicians,
		}
		break
	}
	if lic == nil {
		return nil, errors.New("impossible de générer une licence unique")
	}

	paidAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE orders SET status = 'paid', paid_at = ?, license_id = ? WHERE order_id = ? AND status = 'processing'`, paidAt, lic.LicenseID, orderID); err != nil {
		return nil, fmt.Errorf("finalisation commande: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("validation paiement: %w", err)
	}
	return lic, nil
}

// FulfillPendingRenewalOrder atomically extends the existing licence instead
// of creating a second set of technician credentials.
func FulfillPendingRenewalOrder(db *sql.DB, orderID, _ string) (*License, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("début transaction renouvellement: %w", err)
	}
	defer tx.Rollback()

	claimed, err := tx.Exec(`
		UPDATE orders SET status = 'processing'
		WHERE order_id = ? AND status = 'pending' AND order_kind = 'renewal'
	`, orderID)
	if err != nil {
		return nil, err
	}
	count, err := claimed.RowsAffected()
	if err != nil || count != 1 {
		return nil, fmt.Errorf("renouvellement introuvable ou déjà traité (%s)", orderID)
	}

	var licenseID, orderEmail, billingCycle string
	var technicians int
	if err := tx.QueryRow(`
		SELECT renewal_license_id, email, technicians, COALESCE(billing_cycle, 'monthly') FROM orders WHERE order_id = ?
	`, orderID).Scan(&licenseID, &orderEmail, &technicians, &billingCycle); err != nil {
		return nil, err
	}
	var status, licenseEmail, key, createdRaw, expiresRaw, existingNotes string
	var id int
	if err := tx.QueryRow(`
		SELECT id, status, email, license_key, created_at, expires_at, COALESCE(notes, '')
		FROM licences WHERE license_id = ?
	`, licenseID).Scan(&id, &status, &licenseEmail, &key, &createdRaw, &expiresRaw, &existingNotes); err != nil {
		return nil, errors.New("licence à renouveler introuvable")
	}
	if status == "revoked" || !strings.EqualFold(orderEmail, licenseEmail) {
		return nil, errors.New("licence non renouvelable")
	}
	expiresAt, err := ParseSQLiteTime(expiresRaw)
	if err != nil {
		return nil, err
	}
	base := expiresAt
	now := time.Now().UTC()
	if base.Before(now) {
		base = now
	}
	duration := 30 * 24 * time.Hour
	if strings.ToLower(billingCycle) == "annual" {
		duration = 365 * 24 * time.Hour
	}
	newExpiry := base.Add(duration)
	if _, err := tx.Exec(`
		UPDATE licences
		SET status = 'active', expires_at = ?, max_connections = ?
		WHERE license_id = ? AND status != 'revoked'
	`, newExpiry.Format(time.RFC3339), technicians, licenseID); err != nil {
		return nil, err
	}
	paidAt := now.Format(time.RFC3339)
	if _, err := tx.Exec(`
		UPDATE orders SET status = 'paid', paid_at = ?, license_id = ?
		WHERE order_id = ? AND status = 'processing'
	`, paidAt, licenseID, orderID); err != nil {
		return nil, err
	}
	createdAt, _ := ParseSQLiteTime(createdRaw)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &License{
		ID: id, LicenseID: licenseID, Email: licenseEmail, LicenseKey: key, Status: "active",
		CreatedAt: createdAt, ExpiresAt: newExpiry, MaxConnections: technicians, Notes: existingNotes,
	}, nil
}

func ListOrdersByCustomer(db *sql.DB, email string) ([]Order, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return ListOrdersByCustomerID(db, identity.ID)
}

func ListOrdersByCustomerID(db *sql.DB, customerID int64) ([]Order, error) {
	rows, err := db.Query(`
		SELECT id, order_id, email, plan, technicians, price, payment_method, status,
		       COALESCE(stripe_session_id, ''), COALESCE(license_id, ''),
		       COALESCE(billing_name, ''), COALESCE(billing_address, ''),
		       COALESCE(billing_postal_code, ''), COALESCE(billing_city, ''),
		       COALESCE(billing_country, 'France'), COALESCE(billing_siret, ''),
		       COALESCE(invoice_number, ''),
		       COALESCE(customer_type, 'business'), COALESCE(terms_version, 'legacy'),
		       terms_accepted_at, COALESCE(immediate_performance_requested, 0),
		       COALESCE(order_kind, 'initial'), COALESCE(renewal_license_id, ''),
		       COALESCE(billing_cycle, 'monthly'),
		       created_at, paid_at, expires_at, COALESCE(notes, '')
		FROM orders WHERE customer_id = ? ORDER BY id DESC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Order, 0)
	for rows.Next() {
		item, err := scanOrderRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

// CancelExpiredOrders cancels all pending orders whose expires_at is in the past.
func CancelExpiredOrders(db *sql.DB) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		UPDATE orders
		SET status = 'cancelled'
		WHERE status = 'pending' AND expires_at < ?
	`
	res, err := db.Exec(query, now)
	if err != nil {
		return 0, fmt.Errorf("annulation commandes expirées: %w", err)
	}
	return res.RowsAffected()
}

// orderSelectColumns is the column list scanned by scanOrderRows. The COALESCE
// wrappers are required: the scan targets non-nullable Go values.
const orderSelectColumns = `id, order_id, email, plan, technicians, price, payment_method, status,
       COALESCE(stripe_session_id, ''), COALESCE(license_id, ''),
       COALESCE(billing_name, ''), COALESCE(billing_address, ''),
       COALESCE(billing_postal_code, ''), COALESCE(billing_city, ''),
       COALESCE(billing_country, 'France'), COALESCE(billing_siret, ''),
       COALESCE(invoice_number, ''),
       COALESCE(customer_type, 'business'), COALESCE(terms_version, 'legacy'),
       terms_accepted_at, COALESCE(immediate_performance_requested, 0),
       COALESCE(order_kind, 'initial'), COALESCE(renewal_license_id, ''),
       COALESCE(billing_cycle, 'monthly'),
       created_at, paid_at, expires_at, COALESCE(notes, '')`

// orderListClauses builds the shared WHERE clause for order listing and counting.
func orderListClauses(statusFilter, emailFilter string) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if statusFilter != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, statusFilter)
	}
	if emailFilter != "" {
		conditions = append(conditions, "(email LIKE ? OR billing_name LIKE ? OR order_id LIKE ?)")
		pattern := "%" + emailFilter + "%"
		args = append(args, pattern, pattern, pattern)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

// ListOrders returns orders (id DESC) with optional status and email filters.
// limit <= 0 disables pagination.
func ListOrders(db *sql.DB, statusFilter, emailFilter string, limit, offset int) ([]Order, error) {
	where, args := orderListClauses(statusFilter, emailFilter)

	query := fmt.Sprintf(`
		SELECT `+orderSelectColumns+`
		FROM orders
		%s
		ORDER BY id DESC
	`, where)
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("requête list orders: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		order, err := scanOrderRows(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("requête list orders: %w", err)
	}
	return orders, nil
}

// CountOrders returns the total number of orders matching the list filters.
func CountOrders(db *sql.DB, statusFilter, emailFilter string) (int, error) {
	where, args := orderListClauses(statusFilter, emailFilter)
	var total int
	err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM orders %s`, where), args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("requête count orders: %w", err)
	}
	return total, nil
}

// ListAbandonedCarts returns unpaid one-shot Stripe orders created in
// [from, to) and not yet expired (cart reminder candidates). Times are
// compared as RFC3339 strings like the order reaper does.
func ListAbandonedCarts(db *sql.DB, from, to time.Time) ([]Order, error) {
	rows, err := db.Query(`
		SELECT `+orderSelectColumns+`
		FROM orders
		WHERE payment_method = 'stripe'
		  AND status IN ('pending', 'processing')
		  AND created_at >= ? AND created_at < ?
		  AND expires_at > ?
		ORDER BY id ASC
	`, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("requête paniers abandonnés: %w", err)
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		order, err := scanOrderRows(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("requête paniers abandonnés: %w", err)
	}
	return orders, nil
}

func scanOrder(row *sql.Row) (*Order, error) {
	var o Order
	var createdAtStr string
	var paidAtStr sql.NullString
	var expiresAtStr sql.NullString
	var termsAcceptedAtStr sql.NullString

	err := row.Scan(
		&o.ID, &o.OrderID, &o.Email, &o.Plan, &o.Technicians, &o.Price,
		&o.PaymentMethod, &o.Status, &o.StripeSessionID, &o.LicenseID,
		&o.BillingName, &o.BillingAddress, &o.BillingPostalCode, &o.BillingCity,
		&o.BillingCountry, &o.BillingSIRET, &o.InvoiceNumber,
		&o.CustomerType, &o.TermsVersion, &termsAcceptedAtStr, &o.ImmediatePerformanceRequested,
		&o.OrderKind, &o.RenewalLicenseID, &o.BillingCycle,
		&createdAtStr, &paidAtStr, &expiresAtStr, &o.Notes,
	)
	if err != nil {
		return nil, err
	}

	o.CreatedAt, _ = ParseSQLiteTime(createdAtStr)
	if termsAcceptedAtStr.Valid {
		if acceptedAt, err := ParseSQLiteTime(termsAcceptedAtStr.String); err == nil {
			o.TermsAcceptedAt = &acceptedAt
		}
	}
	if paidAtStr.Valid {
		t, err := ParseSQLiteTime(paidAtStr.String)
		if err == nil {
			o.PaidAt = &t
		}
	}
	if expiresAtStr.Valid {
		t, err := ParseSQLiteTime(expiresAtStr.String)
		if err == nil {
			o.ExpiresAt = &t
		}
	}
	return &o, nil
}

func scanOrderRows(rows *sql.Rows) (*Order, error) {
	var o Order
	var createdAtStr string
	var paidAtStr sql.NullString
	var expiresAtStr sql.NullString
	var termsAcceptedAtStr sql.NullString

	err := rows.Scan(
		&o.ID, &o.OrderID, &o.Email, &o.Plan, &o.Technicians, &o.Price,
		&o.PaymentMethod, &o.Status, &o.StripeSessionID, &o.LicenseID,
		&o.BillingName, &o.BillingAddress, &o.BillingPostalCode, &o.BillingCity,
		&o.BillingCountry, &o.BillingSIRET, &o.InvoiceNumber,
		&o.CustomerType, &o.TermsVersion, &termsAcceptedAtStr, &o.ImmediatePerformanceRequested,
		&o.OrderKind, &o.RenewalLicenseID, &o.BillingCycle,
		&createdAtStr, &paidAtStr, &expiresAtStr, &o.Notes,
	)
	if err != nil {
		return nil, err
	}

	o.CreatedAt, _ = ParseSQLiteTime(createdAtStr)
	if termsAcceptedAtStr.Valid {
		if acceptedAt, err := ParseSQLiteTime(termsAcceptedAtStr.String); err == nil {
			o.TermsAcceptedAt = &acceptedAt
		}
	}
	if paidAtStr.Valid {
		t, err := ParseSQLiteTime(paidAtStr.String)
		if err == nil {
			o.PaidAt = &t
		}
	}
	if expiresAtStr.Valid {
		t, err := ParseSQLiteTime(expiresAtStr.String)
		if err == nil {
			o.ExpiresAt = &t
		}
	}
	return &o, nil
}

// DeleteOrder permanently deletes an order and disassociates any related records.
func DeleteOrder(db *sql.DB, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("identifiant de commande invalide")
	}
	order, err := GetOrderByID(db, orderID)
	if err != nil || order == nil {
		return fmt.Errorf("commande introuvable")
	}
	// Paid orders back licences, invoices and withdrawal records: deleting
	// them would destroy the accounting and legal trail. Only abandoned
	// (pending/cancelled/expired) orders may be cleaned up.
	if order.Status == "paid" {
		return fmt.Errorf("commande payée : suppression interdite (pièce comptable, conservation 10 ans)")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Detach or cleanup related records before deleting the order
	_, _ = tx.Exec(`UPDATE invoices SET order_id = NULL WHERE order_id = ?`, orderID)
	_, _ = tx.Exec(`DELETE FROM withdrawal_requests WHERE order_id = ?`, orderID)

	res, err := tx.Exec(`DELETE FROM orders WHERE order_id = ?`, orderID)
	if err != nil {
		return fmt.Errorf("erreur suppression commande: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("commande introuvable")
	}

	return tx.Commit()
}
