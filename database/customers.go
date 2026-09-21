package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

type CustomerIdentity struct {
	ID           int64  `json:"-"`
	PublicID     string `json:"customer_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	CustomerType string `json:"customer_type"`
	DisplayName  string `json:"display_name"`
	BillingEmail string `json:"billing_email"`
}

type customerQueryer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func generateCustomerPublicID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "CUS-" + strings.ToUpper(hex.EncodeToString(value)), nil
}

// EnsureCustomer gives an e-mail a stable tenant identity. E-mail remains a
// login address, never an authorization boundary.
func EnsureCustomer(db *sql.DB, email, customerType, displayName string) (*CustomerIdentity, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	identity, err := ensureCustomer(tx, email, customerType, displayName)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return identity, nil
}

func ensureCustomer(queryer customerQueryer, email, customerType, displayName string) (*CustomerIdentity, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	customerType = strings.ToLower(strings.TrimSpace(customerType))
	displayName = strings.TrimSpace(displayName)
	if email == "" {
		return nil, errors.New("adresse e-mail client requise")
	}
	if customerType != "consumer" {
		customerType = "business"
	}
	if existing, err := customerIdentityByEmail(queryer, email); err == nil {
		if displayName != "" {
			_, _ = queryer.Exec(`UPDATE customers SET display_name = COALESCE(NULLIF(display_name, ''), ?), updated_at = CURRENT_TIMESTAMP WHERE id = ?`, displayName, existing.ID)
			existing.DisplayName = displayName
		}
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		publicID, err := generateCustomerPublicID()
		if err != nil {
			return nil, err
		}
		result, err := queryer.Exec(`
			INSERT INTO customers (public_id, customer_type, display_name, billing_email)
			VALUES (?, ?, ?, ?)
		`, publicID, customerType, displayName, email)
		if err != nil {
			lastErr = err
			if existing, lookupErr := customerIdentityByEmail(queryer, email); lookupErr == nil {
				return existing, nil
			}
			continue
		}
		id, _ := result.LastInsertId()
		if _, err := queryer.Exec(`INSERT INTO customer_users (customer_id, email, role) VALUES (?, ?, 'owner')`, id, email); err != nil {
			return nil, err
		}
		if _, err := queryer.Exec(`
			INSERT INTO customer_accounts (email, customer_id) VALUES (?, ?)
			ON CONFLICT(email) DO UPDATE SET customer_id = excluded.customer_id
		`, email, id); err != nil {
			return nil, err
		}
		return &CustomerIdentity{ID: id, PublicID: publicID, Email: email, Role: "owner", CustomerType: customerType, DisplayName: displayName, BillingEmail: email}, nil
	}
	return nil, fmt.Errorf("création identité client: %w", lastErr)
}

func customerIdentityByEmail(queryer customerQueryer, email string) (*CustomerIdentity, error) {
	var identity CustomerIdentity
	err := queryer.QueryRow(`
		SELECT c.id, c.public_id, u.email, u.role, c.customer_type,
		       COALESCE(c.display_name, ''), c.billing_email
		FROM customer_users u JOIN customers c ON c.id = u.customer_id
		WHERE LOWER(u.email) = LOWER(?)
	`, strings.TrimSpace(email)).Scan(&identity.ID, &identity.PublicID, &identity.Email, &identity.Role,
		&identity.CustomerType, &identity.DisplayName, &identity.BillingEmail)
	return &identity, err
}

func GetCustomerIdentityByEmail(db *sql.DB, email string) (*CustomerIdentity, error) {
	return customerIdentityByEmail(db, email)
}

func GetCustomerIdentityByID(db *sql.DB, customerID int64, email string) (*CustomerIdentity, error) {
	var identity CustomerIdentity
	err := db.QueryRow(`
		SELECT c.id, c.public_id, u.email, u.role, c.customer_type,
		       COALESCE(c.display_name, ''), c.billing_email
		FROM customers c JOIN customer_users u ON u.customer_id = c.id
		WHERE c.id = ? AND LOWER(u.email) = LOWER(?)
	`, customerID, strings.TrimSpace(email)).Scan(&identity.ID, &identity.PublicID, &identity.Email, &identity.Role,
		&identity.CustomerType, &identity.DisplayName, &identity.BillingEmail)
	return &identity, err
}
