package database

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var invoiceNumberPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// invoiceSeqMu serializes invoice and credit-note numbering. SQLite defers
// write locks, so two concurrent transactions could compute the same
// FAC-YYYY-NNNN before either inserts; the API is a single process on a
// single SQLite file, so a process-wide mutex closes the race.
var invoiceSeqMu sync.Mutex

// Invoice represents a billing invoice in SQLite.
type Invoice struct {
	ID                 int       `json:"id"`
	CustomerID         int64     `json:"-"`
	InvoiceNumber      string    `json:"invoice_number"`
	OrderID            string    `json:"order_id"`
	CustomerEmail      string    `json:"customer_email"`
	CustomerName       string    `json:"customer_name"`
	CustomerAddress    string    `json:"customer_address"`
	CustomerPostalCode string    `json:"customer_postal_code"`
	CustomerCity       string    `json:"customer_city"`
	CustomerCountry    string    `json:"customer_country"`
	CustomerSIRET      string    `json:"customer_siret"`
	Plan               string    `json:"plan"`
	Technicians        int       `json:"technicians"`
	AmountHT           float64   `json:"amount_ht"`
	AmountTVA          float64   `json:"amount_tva"` // 0.0 for micro-enterprise
	AmountTTC          float64   `json:"amount_ttc"` // Equals AmountHT
	Status             string    `json:"status"`     // 'paid'
	PDFPath            string    `json:"pdf_path"`
	IsManual           bool      `json:"is_manual"`
	CreatedAt          time.Time `json:"created_at"`
	Notes              string    `json:"notes"`
}

// GenerateNextInvoiceNumber generates the next invoice number in format FAC-YYYY-NNNN within a transaction.
func GenerateNextInvoiceNumber(db *sql.DB) (string, error) {
	return nextInvoiceNumber(db, time.Now().UTC().Year())
}

type invoiceQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func nextInvoiceNumber(queryer invoiceQueryer, year int) (string, error) {
	prefix := fmt.Sprintf("FAC-%d-", year)
	rows, err := queryer.Query(`SELECT invoice_number FROM invoices WHERE invoice_number LIKE ?`, prefix+"%")
	if err != nil {
		return "", fmt.Errorf("lecture numéros de facture: %w", err)
	}
	defer rows.Close()

	maxSequence := 0
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return "", fmt.Errorf("lecture numéro de facture: %w", err)
		}
		sequence, err := strconv.Atoi(strings.TrimPrefix(number, prefix))
		if err == nil && sequence > maxSequence {
			maxSequence = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("lecture numéros de facture: %w", err)
	}
	return fmt.Sprintf("%s%04d", prefix, maxSequence+1), nil
}

func nextCreditNoteNumber(queryer invoiceQueryer, year int) (string, error) {
	prefix := fmt.Sprintf("AV-%d-", year)
	rows, err := queryer.Query(`SELECT invoice_number FROM invoices WHERE invoice_number LIKE ?`, prefix+"%")
	if err != nil {
		return "", fmt.Errorf("lecture numéros d'avoir: %w", err)
	}
	defer rows.Close()

	maxSequence := 0
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return "", fmt.Errorf("lecture numéro d'avoir: %w", err)
		}
		sequence, err := strconv.Atoi(strings.TrimPrefix(number, prefix))
		if err == nil && sequence > maxSequence {
			maxSequence = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("lecture numéros d'avoir: %w", err)
	}
	return fmt.Sprintf("%s%04d", prefix, maxSequence+1), nil
}

// GenerateNextCreditNoteNumber generates the next credit note number in format AV-YYYY-NNNN.
func GenerateNextCreditNoteNumber(db *sql.DB) (string, error) {
	return nextCreditNoteNumber(db, time.Now().UTC().Year())
}

// CreateInvoice inserts a new invoice into the database. Numbering is
// serialized process-wide (see invoiceSeqMu) to guarantee sequential
// numbering without gaps and prevent concurrency collisions.
func CreateInvoice(db *sql.DB, inv *Invoice) (*Invoice, error) {
	if inv == nil {
		return nil, fmt.Errorf("facture requise")
	}
	invoiceSeqMu.Lock()
	defer invoiceSeqMu.Unlock()
	if inv.InvoiceNumber != "" && !invoiceNumberPattern.MatchString(inv.InvoiceNumber) {
		return nil, fmt.Errorf("numéro de facture invalide")
	}
	if strings.TrimSpace(inv.CustomerEmail) == "" || strings.TrimSpace(inv.CustomerName) == "" {
		return nil, fmt.Errorf("client et adresse email requis")
	}
	if inv.AmountHT < 0 || inv.AmountTTC < 0 {
		return nil, fmt.Errorf("montant de facture invalide")
	}
	// Begin write transaction with immediate lock
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()
	var customerID int64

	inv.OrderID = strings.TrimSpace(inv.OrderID)
	if inv.OrderID != "" {
		if err := tx.QueryRow(`SELECT customer_id FROM orders WHERE order_id = ?`, inv.OrderID).Scan(&customerID); err != nil {
			return nil, fmt.Errorf("commande de facture introuvable: %w", err)
		}
		existing, lookupErr := scanInvoice(tx.QueryRow(`
			SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
			       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
			       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
			       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
			       pdf_path, is_manual, created_at, COALESCE(notes, '')
			FROM invoices
			WHERE order_id = ?
			LIMIT 1
		`, inv.OrderID))
		if lookupErr == nil {
			return existing, nil
		}
		if lookupErr != sql.ErrNoRows {
			return nil, fmt.Errorf("recherche facture existante: %w", lookupErr)
		}
	}
	if customerID == 0 {
		customer, err := ensureCustomer(tx, inv.CustomerEmail, "business", inv.CustomerName)
		if err != nil {
			return nil, err
		}
		customerID = customer.ID
	}
	inv.CustomerID = customerID

	if inv.InvoiceNumber == "" {
		inv.InvoiceNumber, err = nextInvoiceNumber(tx, time.Now().UTC().Year())
		if err != nil {
			return nil, err
		}
	}

	if inv.CustomerCountry == "" {
		inv.CustomerCountry = "France"
	}
	if inv.Status == "" {
		inv.Status = "paid"
	}
	if inv.AmountTTC == 0 && inv.AmountHT > 0 {
		inv.AmountTTC = inv.AmountHT
	}

	now := time.Now().UTC()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}

	insertQuery := `
		INSERT INTO invoices (
			customer_id, invoice_number, order_id, customer_email, customer_name, customer_address,
			customer_postal_code, customer_city, customer_country, customer_siret,
			plan, technicians, amount_ht, amount_tva, amount_ttc, status,
			pdf_path, is_manual, created_at, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	manualInt := 0
	if inv.IsManual {
		manualInt = 1
	}

	var orderID any
	if inv.OrderID != "" {
		orderID = inv.OrderID
	}
	res, err := tx.Exec(insertQuery,
		customerID, inv.InvoiceNumber, orderID, inv.CustomerEmail, inv.CustomerName, inv.CustomerAddress,
		inv.CustomerPostalCode, inv.CustomerCity, inv.CustomerCountry, inv.CustomerSIRET,
		inv.Plan, inv.Technicians, inv.AmountHT, inv.AmountTVA, inv.AmountTTC, inv.Status,
		inv.PDFPath, manualInt, inv.CreatedAt.Format(time.RFC3339), inv.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("insertion facture sous transaction: %w", err)
	}

	id, _ := res.LastInsertId()
	inv.ID = int(id)

	// If linked to an order, update invoice_number on orders table in same transaction
	if inv.OrderID != "" {
		_, err := tx.Exec(`UPDATE orders SET invoice_number = ? WHERE order_id = ?`, inv.InvoiceNumber, inv.OrderID)
		if err != nil {
			return nil, fmt.Errorf("mise à jour commande avec facture: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction facture: %w", err)
	}

	return inv, nil
}

// GetInvoiceByNumber retrieves an invoice by its invoice_number.
func GetInvoiceByNumber(db *sql.DB, number string) (*Invoice, error) {
	query := `
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices
		WHERE invoice_number = ?
	`
	return scanInvoice(db.QueryRow(query, number))
}

// GetInvoiceByOrderID retrieves an invoice linked to a specific order_id.
func GetInvoiceByOrderID(db *sql.DB, orderID string) (*Invoice, error) {
	query := `
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices
		WHERE order_id = ?
		LIMIT 1
	`
	return scanInvoice(db.QueryRow(query, orderID))
}

// invoiceListClauses builds the shared WHERE clause for invoice listing and counting.
func invoiceListClauses(searchFilter string) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if searchFilter != "" {
		conditions = append(conditions, "(customer_email LIKE ? OR customer_name LIKE ? OR invoice_number LIKE ? OR order_id LIKE ?)")
		pattern := "%" + searchFilter + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

// ListInvoices returns invoices (id DESC) with optional search filter.
// limit <= 0 disables pagination.
func ListInvoices(db *sql.DB, searchFilter string, limit, offset int) ([]Invoice, error) {
	where, args := invoiceListClauses(searchFilter)

	query := fmt.Sprintf(`
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices
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
		return nil, fmt.Errorf("requête list invoices: %w", err)
	}
	defer rows.Close()

	var invoices []Invoice
	for rows.Next() {
		inv, err := scanInvoiceRows(rows)
		if err != nil {
			return nil, err
		}
		invoices = append(invoices, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("requête list invoices: %w", err)
	}
	return invoices, nil
}

// CountInvoices returns the total number of invoices matching the list filters.
func CountInvoices(db *sql.DB, searchFilter string) (int, error) {
	where, args := invoiceListClauses(searchFilter)
	var total int
	err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM invoices %s`, where), args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("requête count invoices: %w", err)
	}
	return total, nil
}

// ListInvoicesByCustomer uses an exact normalized e-mail match. The fuzzy
// administrator search must never be used as an authorization boundary.
func ListInvoicesByCustomer(db *sql.DB, email string) ([]Invoice, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return ListInvoicesByCustomerID(db, identity.ID)
}

func ListInvoicesByCustomerID(db *sql.DB, customerID int64) ([]Invoice, error) {
	rows, err := db.Query(`
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices WHERE customer_id = ? ORDER BY id DESC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Invoice, 0)
	for rows.Next() {
		item, err := scanInvoiceRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func GetInvoiceForCustomer(db *sql.DB, invoiceNumber, email string) (*Invoice, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return GetInvoiceForCustomerID(db, invoiceNumber, identity.ID)
}

func GetInvoiceForCustomerID(db *sql.DB, invoiceNumber string, customerID int64) (*Invoice, error) {
	query := `
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices WHERE invoice_number = ? AND customer_id = ?
	`
	return scanInvoice(db.QueryRow(query, invoiceNumber, customerID))
}

func scanInvoice(row *sql.Row) (*Invoice, error) {
	var inv Invoice
	var createdAtStr string
	var isManualInt int

	err := row.Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.OrderID, &inv.CustomerEmail, &inv.CustomerName,
		&inv.CustomerAddress, &inv.CustomerPostalCode, &inv.CustomerCity,
		&inv.CustomerCountry, &inv.CustomerSIRET,
		&inv.Plan, &inv.Technicians, &inv.AmountHT, &inv.AmountTVA, &inv.AmountTTC, &inv.Status,
		&inv.PDFPath, &isManualInt, &createdAtStr, &inv.Notes,
	)
	if err != nil {
		return nil, err
	}

	inv.IsManual = (isManualInt == 1)
	inv.CreatedAt, _ = ParseSQLiteTime(createdAtStr)
	return &inv, nil
}

func scanInvoiceRows(rows *sql.Rows) (*Invoice, error) {
	var inv Invoice
	var createdAtStr string
	var isManualInt int

	err := rows.Scan(
		&inv.ID, &inv.InvoiceNumber, &inv.OrderID, &inv.CustomerEmail, &inv.CustomerName,
		&inv.CustomerAddress, &inv.CustomerPostalCode, &inv.CustomerCity,
		&inv.CustomerCountry, &inv.CustomerSIRET,
		&inv.Plan, &inv.Technicians, &inv.AmountHT, &inv.AmountTVA, &inv.AmountTTC, &inv.Status,
		&inv.PDFPath, &isManualInt, &createdAtStr, &inv.Notes,
	)
	if err != nil {
		return nil, err
	}

	inv.IsManual = (isManualInt == 1)
	inv.CreatedAt, _ = ParseSQLiteTime(createdAtStr)
	return &inv, nil
}

// DeleteInvoice deletes an invoice from the database, detaches it from any order, and returns its pdf_path for cleanup.
func DeleteInvoice(db *sql.DB, invoiceNumber string) (string, error) {
	invoiceNumber = strings.TrimSpace(invoiceNumber)
	if invoiceNumber == "" {
		return "", fmt.Errorf("numéro de facture invalide")
	}
	// Invoices are only created at payment: every row is an issued fiscal
	// document (chronological, gapless, immutable, 10-year retention).
	// Corrections go through credit notes ("Avoir"), never deletion.
	if _, err := GetInvoiceByNumber(db, invoiceNumber); err != nil {
		return "", fmt.Errorf("facture introuvable")
	}
	return "", fmt.Errorf("suppression de facture interdite : émettez un avoir (document fiscal, conservation 10 ans)")
}

// CreateCreditNote generates an official credit note (avoir) referencing an original invoice.
func CreateCreditNote(db *sql.DB, originalInvoiceNumber, reason string) (*Invoice, error) {
	originalInvoiceNumber = strings.TrimSpace(originalInvoiceNumber)
	if originalInvoiceNumber == "" {
		return nil, fmt.Errorf("numéro de facture d'origine requis")
	}
	invoiceSeqMu.Lock()
	defer invoiceSeqMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	orig, err := scanInvoice(tx.QueryRow(`
		SELECT id, invoice_number, COALESCE(order_id, ''), customer_email, customer_name,
		       COALESCE(customer_address, ''), COALESCE(customer_postal_code, ''), COALESCE(customer_city, ''),
		       COALESCE(customer_country, 'France'), COALESCE(customer_siret, ''),
		       plan, technicians, amount_ht, amount_tva, amount_ttc, status,
		       pdf_path, is_manual, created_at, COALESCE(notes, '')
		FROM invoices
		WHERE invoice_number = ?
	`, originalInvoiceNumber))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("facture d'origine introuvable")
		}
		return nil, fmt.Errorf("recherche facture d'origine: %w", err)
	}

	if strings.HasPrefix(orig.InvoiceNumber, "AV-") || orig.Status == "credit_note" {
		return nil, fmt.Errorf("impossible de générer un avoir sur une facture d'avoir")
	}

	creditNumber, err := nextCreditNoteNumber(tx, time.Now().UTC().Year())
	if err != nil {
		return nil, fmt.Errorf("génération numéro d'avoir: %w", err)
	}

	reason = strings.TrimSpace(reason)
	var creditNotes string
	if reason != "" {
		creditNotes = fmt.Sprintf("Avoir sur facture %s : %s", orig.InvoiceNumber, reason)
	} else {
		creditNotes = fmt.Sprintf("Avoir sur facture %s", orig.InvoiceNumber)
	}

	// Update original invoice notes with reference to this credit note
	mention := fmt.Sprintf("Avoir %s émis", creditNumber)
	if _, err := tx.Exec(`UPDATE invoices SET notes = CASE WHEN COALESCE(notes, '') = '' THEN ? ELSE notes || ' | ' || ? END WHERE invoice_number = ?`, mention, mention, orig.InvoiceNumber); err != nil {
		return nil, fmt.Errorf("mention avoir facture d'origine: %w", err)
	}

	var customerID int64
	_ = tx.QueryRow(`SELECT customer_id FROM invoices WHERE invoice_number = ?`, originalInvoiceNumber).Scan(&customerID)
	if customerID == 0 {
		customer, err := ensureCustomer(tx, orig.CustomerEmail, "business", orig.CustomerName)
		if err != nil {
			return nil, err
		}
		customerID = customer.ID
	}

	now := time.Now().UTC()
	insertQuery := `
		INSERT INTO invoices (
			customer_id, invoice_number, order_id, customer_email, customer_name, customer_address,
			customer_postal_code, customer_city, customer_country, customer_siret,
			plan, technicians, amount_ht, amount_tva, amount_ttc, status,
			pdf_path, is_manual, created_at, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	manualInt := 0
	if orig.IsManual {
		manualInt = 1
	}
	var orderID any
	if orig.OrderID != "" {
		orderID = orig.OrderID
	}
	res, err := tx.Exec(insertQuery,
		customerID, creditNumber, orderID, orig.CustomerEmail, orig.CustomerName, orig.CustomerAddress,
		orig.CustomerPostalCode, orig.CustomerCity, orig.CustomerCountry, orig.CustomerSIRET,
		orig.Plan, orig.Technicians, orig.AmountHT, orig.AmountTVA, orig.AmountTTC, "credit_note",
		"", manualInt, now.Format(time.RFC3339), creditNotes,
	)
	if err != nil {
		return nil, fmt.Errorf("insertion avoir: %w", err)
	}

	id, _ := res.LastInsertId()
	creditInv := &Invoice{
		ID:                 int(id),
		CustomerID:         customerID,
		InvoiceNumber:      creditNumber,
		OrderID:            orig.OrderID,
		CustomerEmail:      orig.CustomerEmail,
		CustomerName:       orig.CustomerName,
		CustomerAddress:    orig.CustomerAddress,
		CustomerPostalCode: orig.CustomerPostalCode,
		CustomerCity:       orig.CustomerCity,
		CustomerCountry:    orig.CustomerCountry,
		CustomerSIRET:      orig.CustomerSIRET,
		Plan:               orig.Plan,
		Technicians:        orig.Technicians,
		AmountHT:           orig.AmountHT,
		AmountTVA:          orig.AmountTVA,
		AmountTTC:          orig.AmountTTC,
		Status:             "credit_note",
		PDFPath:            "",
		IsManual:           orig.IsManual,
		CreatedAt:          now,
		Notes:              creditNotes,
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction avoir: %w", err)
	}
	return creditInv, nil
}

// UpdateInvoicePDFPath updates the PDF path of an existing invoice.
func UpdateInvoicePDFPath(db *sql.DB, invoiceNumber, pdfPath string) error {
	res, err := db.Exec(`UPDATE invoices SET pdf_path = ? WHERE invoice_number = ?`, pdfPath, invoiceNumber)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("facture introuvable (%s)", invoiceNumber)
	}
	return nil
}
