package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CryptoQuote is a priced payment request for a crypto order. Amounts stay
// exact decimal strings; conversions happen once at quote time.
type CryptoQuote struct {
	ID              int64
	OrderID         string
	Asset           string // 'BTC' or 'XRP'
	AmountCrypto    string
	RateEUR         string
	RateSource      string
	QuotedAt        time.Time
	ExpiresAt       time.Time
	PayAddress      string
	DestTag         *uint32 // XRP only (fixed OKX deposit tag)
	Status          string // 'pending', 'paid', 'expired' or 'cancelled'
	TxID            string
	Sender          string
	Confirmations   int
}

// CryptoPayment is one row of the payments registry (cahier crypto section 6).
type CryptoPayment struct {
	ID            int64
	OrderID       string
	InvoiceNumber string
	Asset         string
	AmountCrypto  string
	RateEUR       string
	RateAt        time.Time
	TxID          string
	Sender        string
	CreatedAt     time.Time
}

// sqlRowScanner abstracts *sql.Row and *sql.Rows (single vs batch scans).
type sqlRowScanner interface {
	Scan(dest ...any) error
}

func scanCryptoQuote(row sqlRowScanner) (*CryptoQuote, error) {
	var q CryptoQuote
	var destTag sql.NullInt64
	var quotedAtRaw, expiresAtRaw any
	err := row.Scan(
		&q.ID, &q.OrderID, &q.Asset, &q.AmountCrypto, &q.RateEUR, &q.RateSource,
		&quotedAtRaw, &expiresAtRaw, &q.PayAddress, &destTag,
		&q.Status, &q.TxID, &q.Sender, &q.Confirmations,
	)
	if err != nil {
		return nil, err
	}
	if q.QuotedAt, err = ParseSQLiteTime(quotedAtRaw); err != nil {
		return nil, err
	}
	if q.ExpiresAt, err = ParseSQLiteTime(expiresAtRaw); err != nil {
		return nil, err
	}
	if destTag.Valid && destTag.Int64 > 0 {
		tag := uint32(destTag.Int64)
		q.DestTag = &tag
	}
	return &q, nil
}

const cryptoQuoteColumns = `
	id, order_id, asset, amount_crypto, rate_eur, rate_source,
	quoted_at, expires_at, pay_address, dest_tag,
	status, txid, sender, confirmations
`

// CreateCryptoQuote stores a pending quote for an order. One live quote per
// order: callers expire or cancel the previous one first.
func CreateCryptoQuote(db *sql.DB, quote *CryptoQuote) error {
	if quote == nil {
		return errors.New("devis crypto requis")
	}
	if quote.Asset != "BTC" && quote.Asset != "XRP" {
		return errors.New("actif crypto invalide")
	}
	if quote.OrderID == "" || quote.AmountCrypto == "" || quote.RateEUR == "" || quote.PayAddress == "" {
		return errors.New("devis crypto incomplet")
	}
	if quote.Asset == "XRP" && quote.DestTag == nil {
		return errors.New("destination tag XRP requis")
	}
	now := time.Now().UTC()
	res, err := db.Exec(`
		INSERT INTO crypto_quotes (
			order_id, asset, amount_crypto, rate_eur, rate_source,
			quoted_at, expires_at, pay_address, dest_tag,
			status, txid, sender, confirmations,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', '', 0, ?, ?)
	`,
		quote.OrderID, quote.Asset, quote.AmountCrypto, quote.RateEUR, "OKX",
		quote.QuotedAt.UTC().Format(time.RFC3339), quote.ExpiresAt.UTC().Format(time.RFC3339),
		quote.PayAddress, destTagValue(quote.DestTag),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("création devis crypto: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	quote.ID = id
	quote.Status = "pending"
	quote.RateSource = "OKX"
	return nil
}

func destTagValue(tag *uint32) any {
	if tag == nil {
		return nil
	}
	return int64(*tag)
}

// GetCryptoQuoteByOrderID returns the latest quote of an order.
func GetCryptoQuoteByOrderID(db *sql.DB, orderID string) (*CryptoQuote, error) {
	return scanCryptoQuote(db.QueryRow(`
		SELECT `+cryptoQuoteColumns+`
		FROM crypto_quotes WHERE order_id = ? ORDER BY id DESC LIMIT 1
	`, orderID))
}

// GetCryptoQuotesByOrderIDs returns the latest quote of each order in a few
// chunked queries instead of one query per order (N+1). Orders without any
// quote are absent from the returned map.
func GetCryptoQuotesByOrderIDs(db *sql.DB, orderIDs []string) (map[string]*CryptoQuote, error) {
	out := make(map[string]*CryptoQuote, len(orderIDs))
	for _, chunk := range chunkStrings(orderIDs, 500) {
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}
		rows, err := db.Query(`SELECT `+cryptoQuoteColumns+` FROM crypto_quotes WHERE order_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY id DESC`, args...)
		if err != nil {
			return nil, fmt.Errorf("requête batch devis crypto: %w", err)
		}
		for rows.Next() {
			q, err := scanCryptoQuote(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if _, seen := out[q.OrderID]; !seen {
				out[q.OrderID] = q
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("requête batch devis crypto: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// GetCryptoQuoteByID returns one quote by row id.
func GetCryptoQuoteByID(db *sql.DB, id int64) (*CryptoQuote, error) {
	if id <= 0 {
		return nil, sql.ErrNoRows
	}
	return scanCryptoQuote(db.QueryRow(`
		SELECT `+cryptoQuoteColumns+`
		FROM crypto_quotes WHERE id = ?
	`, id))
}

// ListOpenCryptoQuotes returns the matchable quotes of an asset for the OKX
// deposit watcher: pending quotes plus recently expired ones (a deposit
// credited shortly after expiry still pays). Pending rows come first,
// oldest first, so the matcher prefers them.
func ListOpenCryptoQuotes(db *sql.DB, asset string, expiredSince time.Time, limit int) ([]CryptoQuote, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT `+cryptoQuoteColumns+`
		FROM crypto_quotes
		WHERE asset = ?
		  AND (status = 'pending' OR (status = 'expired' AND expires_at > ?))
		ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END ASC, id ASC
		LIMIT ?
	`, asset, expiredSince.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CryptoQuote
	for rows.Next() {
		var q CryptoQuote
		var destTag sql.NullInt64
		var quotedAtRaw, expiresAtRaw any
		if err := rows.Scan(
			&q.ID, &q.OrderID, &q.Asset, &q.AmountCrypto, &q.RateEUR, &q.RateSource,
			&quotedAtRaw, &expiresAtRaw, &q.PayAddress, &destTag,
			&q.Status, &q.TxID, &q.Sender, &q.Confirmations,
		); err != nil {
			return nil, err
		}
		if q.QuotedAt, err = ParseSQLiteTime(quotedAtRaw); err != nil {
			return nil, err
		}
		if q.ExpiresAt, err = ParseSQLiteTime(expiresAtRaw); err != nil {
			return nil, err
		}
		if destTag.Valid && destTag.Int64 > 0 {
			tag := uint32(destTag.Int64)
			q.DestTag = &tag
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// MarkCryptoQuotePaidByID flips one open (pending or recently expired) quote
// to paid. It reports whether the quote transitioned (false when already
// settled: safe to call twice, and a second deposit never re-pays).
func MarkCryptoQuotePaidByID(db *sql.DB, quoteID int64, txid, sender string, confirmations int) (bool, error) {
	if quoteID <= 0 {
		return false, errors.New("devis crypto requis")
	}
	res, err := db.Exec(`
		UPDATE crypto_quotes
		SET status = 'paid', txid = ?, sender = ?, confirmations = ?, updated_at = ?
		WHERE id = ? AND status IN ('pending', 'expired')
	`, txid, sender, confirmations, time.Now().UTC().Format(time.RFC3339), quoteID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// ExpireCryptoQuotes expires pending quotes past their deadline.
func ExpireCryptoQuotes(db *sql.DB, now time.Time) (int64, error) {
	res, err := db.Exec(`
		UPDATE crypto_quotes SET status = 'expired', updated_at = ?
		WHERE status = 'pending' AND expires_at <= ?
	`, now.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ExpireCryptoQuotesForOrder expires the pending quotes of one order (used
// before issuing a refreshed quote).
func ExpireCryptoQuotesForOrder(db *sql.DB, orderID string) error {
	_, err := db.Exec(`
		UPDATE crypto_quotes SET status = 'expired', updated_at = ?
		WHERE order_id = ? AND status = 'pending'
	`, time.Now().UTC().Format(time.RFC3339), orderID)
	return err
}

// RecordCryptoPayment appends one row to the payments registry.
func RecordCryptoPayment(db *sql.DB, payment *CryptoPayment) error {
	if payment == nil || payment.OrderID == "" || payment.Asset == "" || payment.AmountCrypto == "" {
		return errors.New("paiement crypto incomplet")
	}
	_, err := db.Exec(`
		INSERT INTO crypto_payments (
			order_id, invoice_number, asset, amount_crypto, rate_eur, rate_at,
			txid, sender, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		payment.OrderID, payment.InvoiceNumber, payment.Asset, payment.AmountCrypto,
		payment.RateEUR, payment.RateAt.UTC().Format(time.RFC3339),
		payment.TxID, payment.Sender, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// ListCryptoPaymentsByOrder returns the registry rows of an order.
func ListCryptoPaymentsByOrder(db *sql.DB, orderID string) ([]CryptoPayment, error) {
	rows, err := db.Query(`
		SELECT id, order_id, invoice_number, asset, amount_crypto, rate_eur, rate_at,
		       txid, sender, created_at
		FROM crypto_payments WHERE order_id = ? ORDER BY id ASC
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CryptoPayment
	for rows.Next() {
		var p CryptoPayment
		var rateAtRaw, createdAtRaw any
		if err := rows.Scan(
			&p.ID, &p.OrderID, &p.InvoiceNumber, &p.Asset, &p.AmountCrypto,
			&p.RateEUR, &rateAtRaw, &p.TxID, &p.Sender, &createdAtRaw,
		); err != nil {
			return nil, err
		}
		var err error
		if p.RateAt, err = ParseSQLiteTime(rateAtRaw); err != nil {
			return nil, err
		}
		if p.CreatedAt, err = ParseSQLiteTime(createdAtRaw); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
