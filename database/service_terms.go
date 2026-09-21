package database

import (
	"database/sql"
	"errors"
	"time"
)

func ServiceTermsAccepted(db *sql.DB, customer int64, version, hash string) (int64, error) {
	var at int64
	err := db.QueryRow(`SELECT accepted_at FROM service_terms_acceptances WHERE customer_id=? AND version=? AND document_hash=?`, customer, version, hash).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return at, err
}

func AcceptServiceTerms(db *sql.DB, customer int64, version, hash string) error {
	_, err := db.Exec(`INSERT INTO service_terms_acceptances(customer_id,version,document_hash,accepted_at) VALUES(?,?,?,?) ON CONFLICT(customer_id,version) DO NOTHING`, customer, version, hash, time.Now().Unix())
	if err != nil {
		return err
	}
	at, err := ServiceTermsAccepted(db, customer, version, hash)
	if err == nil && at == 0 {
		return ErrServiceState
	}
	return err
}
