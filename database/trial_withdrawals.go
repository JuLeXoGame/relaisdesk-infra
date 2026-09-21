package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type TrialWithdrawal struct {
	ApplicationID       string `json:"application_id"`
	RequestID           string `json:"request_id"`
	RequestedAt         int64  `json:"requested_at"`
	CustomerName        string `json:"customer_name"`
	Immediate           bool   `json:"immediate"`
	ProcessedAt         int64  `json:"processed_at,omitempty"`
	CustomerEmailSentAt int64  `json:"-"`
	AdminEmailSentAt    int64  `json:"-"`
	Status              string `json:"status"`
}

func GetTrialWithdrawal(db *sql.DB, id string) (*TrialWithdrawal, error) {
	w := &TrialWithdrawal{}
	err := db.QueryRow(`SELECT application_id,request_id,requested_at,customer_name,immediate,
        COALESCE(processed_at,0),COALESCE(customer_email_sent_at,0),COALESCE(admin_email_sent_at,0),status
        FROM trial_withdrawals WHERE application_id=?`, id).Scan(&w.ApplicationID, &w.RequestID, &w.RequestedAt, &w.CustomerName, &w.Immediate, &w.ProcessedAt, &w.CustomerEmailSentAt, &w.AdminEmailSentAt, &w.Status)
	return w, err
}

// A notification is accepted even outside the normal deadline: extensions and
// more favourable rights require human assessment, never a silent rejection.
// During the free trial we contractually accept withdrawal without charge.
func RequestTrialWithdrawal(db *sql.DB, id, email, name string, now time.Time) (*TrialWithdrawal, error) {
	id, email, name = strings.TrimSpace(id), strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(name)
	if id == "" || len(id) > 128 || email == "" || len(email) > 254 || len(name) > 200 {
		return nil, errors.New("référence du contrat et adresse e-mail valides requises")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := scanTrial(tx.QueryRow(`SELECT `+trialColumns+` FROM trial_applications WHERE id=? AND email=? COLLATE NOCASE`, id, email))
	if err != nil {
		return nil, errors.New("contrat introuvable avec cette adresse e-mail")
	}
	if t.VerifiedAt == 0 {
		return nil, errors.New("demande non confirmée : aucun contrat activé ; contactez-nous si nécessaire")
	}
	if t.WithdrawalRequestedAt != 0 {
		tx.Rollback()
		return GetTrialWithdrawal(db, id)
	}
	requestID, err := generateWithdrawalRequestID()
	if err != nil {
		return nil, err
	}
	immediate := t.Billing.CustomerType == "consumer" && (t.TrialStart == 0 || (t.TrialEnd > now.Unix() && t.PaidThrough == 0))
	if _, err = tx.Exec(`INSERT INTO trial_withdrawals(application_id,request_id,requested_at,customer_name,immediate) VALUES(?,?,?,?,?)`, id, requestID, now.Unix(), name, immediate); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE trial_applications SET cancel_requested_at=COALESCE(cancel_requested_at,?) WHERE id=?`, now.Unix(), id); err != nil {
		return nil, err
	}
	if immediate && t.LicenseID != "" {
		if _, err = tx.Exec(`UPDATE licences SET status='revoked',revoked_at=?,revoke_reason='Rétractation pendant essai gratuit' WHERE license_id=?`, now.UTC().Format(time.RFC3339), t.LicenseID); err != nil {
			return nil, err
		}
	}
	// Evidence and retry are committed atomically; a crash cannot lose notice.
	payload, _ := json.Marshal(map[string]string{"value": id})
	for _, kind := range []string{"trial_withdrawal_process", "trial_withdrawal_email"} {
		if _, err = tx.Exec(`INSERT INTO jobs(job_type,payload,unique_key,max_attempts) VALUES(?,?,?,30) ON CONFLICT(unique_key) DO NOTHING`, kind, string(payload), kind+":"+id); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetTrialWithdrawal(db, id)
}

func ListTrialWithdrawals(db *sql.DB) ([]TrialWithdrawal, error) {
	rows, err := db.Query(`SELECT application_id FROM trial_withdrawals ORDER BY requested_at DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []TrialWithdrawal{}
	for _, id := range ids {
		w, e := GetTrialWithdrawal(db, id)
		if e != nil {
			return nil, e
		}
		out = append(out, *w)
	}
	return out, nil
}
