package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// WithdrawalRequest records a consumer's unambiguous withdrawal notice.
type WithdrawalRequest struct {
	ID                  int        `json:"id"`
	RequestID           string     `json:"request_id"`
	OrderID             string     `json:"order_id"`
	Email               string     `json:"email"`
	CustomerName        string     `json:"customer_name"`
	RequestedAt         time.Time  `json:"requested_at"`
	Status              string     `json:"status"`
	ProcessedAt         *time.Time `json:"processed_at,omitempty"`
	CustomerEmailSentAt *time.Time `json:"customer_email_sent_at,omitempty"`
	AdminEmailSentAt    *time.Time `json:"admin_email_sent_at,omitempty"`
	Notes               string     `json:"notes"`
}

func generateWithdrawalRequestID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("génération référence rétractation: %w", err)
	}
	return "RET-" + strings.ToUpper(hex.EncodeToString(value)), nil
}

// CreateWithdrawalRequest stores a notice after matching the order reference and email.
func CreateWithdrawalRequest(db *sql.DB, orderID, email, customerName string) (*WithdrawalRequest, error) {
	orderID = strings.TrimSpace(orderID)
	email = strings.ToLower(strings.TrimSpace(email))
	customerName = strings.TrimSpace(customerName)
	if orderID == "" || email == "" {
		return nil, errors.New("référence de commande et adresse e-mail requises")
	}
	if len(orderID) > 255 || len(email) > 254 || len(customerName) > 200 {
		return nil, errors.New("informations de rétractation trop longues")
	}

	var matchedOrderID string
	if err := db.QueryRow(`SELECT order_id FROM orders WHERE order_id = ? COLLATE NOCASE AND LOWER(email) = ?`, orderID, email).Scan(&matchedOrderID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("commande introuvable avec cette adresse e-mail")
		}
		return nil, fmt.Errorf("vérification commande: %w", err)
	}

	now := time.Now().UTC()
	for attempt := 0; attempt < 5; attempt++ {
		requestID, err := generateWithdrawalRequestID()
		if err != nil {
			return nil, err
		}
		result, err := db.Exec(`
			INSERT INTO withdrawal_requests (request_id, order_id, email, customer_name, requested_at)
			VALUES (?, ?, ?, ?, ?)
		`, requestID, matchedOrderID, email, customerName, now.Format(time.RFC3339))
		if err != nil {
			continue
		}
		id, _ := result.LastInsertId()
		return &WithdrawalRequest{
			ID: int(id), RequestID: requestID, OrderID: matchedOrderID, Email: email,
			CustomerName: customerName, RequestedAt: now, Status: "received",
		}, nil
	}
	return nil, errors.New("impossible de générer une référence de rétractation unique")
}

// ListWithdrawalRequests returns notices ordered from newest to oldest.
func ListWithdrawalRequests(db *sql.DB, status string) ([]WithdrawalRequest, error) {
	query := `
		SELECT id, request_id, order_id, email, COALESCE(customer_name, ''), requested_at,
		       status, processed_at, customer_email_sent_at, admin_email_sent_at,
		       COALESCE(notes, '')
		FROM withdrawal_requests
	`
	var args []interface{}
	if strings.TrimSpace(status) != "" {
		query += " WHERE status = ?"
		args = append(args, strings.TrimSpace(status))
	}
	query += " ORDER BY id DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("liste rétractations: %w", err)
	}
	defer rows.Close()

	requests := make([]WithdrawalRequest, 0)
	for rows.Next() {
		var item WithdrawalRequest
		var requestedAt string
		var processedAt, customerEmailSentAt, adminEmailSentAt sql.NullString
		if err := rows.Scan(&item.ID, &item.RequestID, &item.OrderID, &item.Email, &item.CustomerName,
			&requestedAt, &item.Status, &processedAt, &customerEmailSentAt, &adminEmailSentAt, &item.Notes); err != nil {
			return nil, err
		}
		item.RequestedAt, _ = ParseSQLiteTime(requestedAt)
		if processedAt.Valid {
			if parsed, err := ParseSQLiteTime(processedAt.String); err == nil {
				item.ProcessedAt = &parsed
			}
		}
		if customerEmailSentAt.Valid {
			if parsed, err := ParseSQLiteTime(customerEmailSentAt.String); err == nil {
				item.CustomerEmailSentAt = &parsed
			}
		}
		if adminEmailSentAt.Valid {
			if parsed, err := ParseSQLiteTime(adminEmailSentAt.String); err == nil {
				item.AdminEmailSentAt = &parsed
			}
		}
		requests = append(requests, item)
	}
	return requests, rows.Err()
}

func GetWithdrawalRequest(db *sql.DB, requestID string) (*WithdrawalRequest, error) {
	var item WithdrawalRequest
	var requestedRaw, processedRaw, customerEmailSentRaw, adminEmailSentRaw any
	err := db.QueryRow(`
		SELECT id, request_id, order_id, email, COALESCE(customer_name, ''), requested_at,
		       status, processed_at, customer_email_sent_at, admin_email_sent_at,
		       COALESCE(notes, '')
		FROM withdrawal_requests WHERE request_id = ?
	`, strings.TrimSpace(requestID)).Scan(&item.ID, &item.RequestID, &item.OrderID, &item.Email,
		&item.CustomerName, &requestedRaw, &item.Status, &processedRaw,
		&customerEmailSentRaw, &adminEmailSentRaw, &item.Notes)
	if err != nil {
		return nil, err
	}
	item.RequestedAt, _ = ParseSQLiteTime(requestedRaw)
	if processedRaw != nil {
		if value, parseErr := ParseSQLiteTime(processedRaw); parseErr == nil {
			item.ProcessedAt = &value
		}
	}
	if customerEmailSentRaw != nil {
		if value, parseErr := ParseSQLiteTime(customerEmailSentRaw); parseErr == nil {
			item.CustomerEmailSentAt = &value
		}
	}
	if adminEmailSentRaw != nil {
		if value, parseErr := ParseSQLiteTime(adminEmailSentRaw); parseErr == nil {
			item.AdminEmailSentAt = &value
		}
	}
	return &item, nil
}

// MarkWithdrawalEmailSent records each delivery stage independently so a
// retry after a later SMTP failure does not resend an already delivered mail.
func MarkWithdrawalEmailSent(db *sql.DB, requestID, recipient string, sentAt time.Time) error {
	column := ""
	switch recipient {
	case "customer":
		column = "customer_email_sent_at"
	case "admin":
		column = "admin_email_sent_at"
	default:
		return errors.New("destinataire de rétractation invalide")
	}
	result, err := db.Exec(`UPDATE withdrawal_requests SET `+column+` = COALESCE(`+column+`, ?) WHERE request_id = ?`,
		sentAt.UTC().Format(time.RFC3339), strings.TrimSpace(requestID))
	if err != nil {
		return fmt.Errorf("marquage e-mail rétractation: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}
