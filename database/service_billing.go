package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var ErrServiceState = errors.New("prestation indisponible ou état incompatible")

type ServiceMerchant struct {
	CustomerID   int64  `json:"-"`
	Enabled      bool   `json:"enabled"`
	AccountID    string `json:"account_id"`
	SetupStarted int64  `json:"-"`
}
type ServiceRate struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Mode   string `json:"mode"`
	Cents  int64  `json:"cents"`
	Active bool   `json:"active"`
}
type ServiceWork struct {
	ID              string `json:"id"`
	CustomerID      int64  `json:"-"`
	LicenseID       string `json:"license_id"`
	MemberID        string `json:"-"`
	TargetKind      string `json:"target_kind"`
	TargetID        string `json:"target_id"`
	PeerID          string `json:"peer_id"`
	Label           string `json:"label"`
	Mode            string `json:"mode"`
	RateCents       int64  `json:"rate_cents"`
	AccountID       string `json:"-"`
	State           string `json:"state"`
	Paid            bool   `json:"paid"`
	ConnectedMS     int64  `json:"connected_ms"`
	AmountCents     int64  `json:"amount_cents"`
	CreatedAt       int64  `json:"created_at"`
	ConnectionID    string `json:"-"`
	Sequence        int64  `json:"-"`
	CumulativeMS    int64  `json:"-"`
	LastSeenMS      int64  `json:"-"`
	ConnectionOpen  bool   `json:"connection_open"`
	CheckoutID      string `json:"-"`
	CheckoutURL     string `json:"checkout_url,omitempty"`
	CheckoutStarted int64  `json:"-"`
}

func ServiceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "SVC-" + hex.EncodeToString(b[:]), nil
}
func GetServiceMerchant(db *sql.DB, customer int64) (*ServiceMerchant, error) {
	m := &ServiceMerchant{CustomerID: customer}
	err := db.QueryRow(`SELECT enabled,account_id,setup_started FROM service_merchants WHERE customer_id=?`, customer).Scan(&m.Enabled, &m.AccountID, &m.SetupStarted)
	if errors.Is(err, sql.ErrNoRows) {
		return m, nil
	}
	return m, err
}
func EnsureServiceMerchant(db *sql.DB, customer int64) (*ServiceMerchant, error) {
	_, err := db.Exec(`INSERT INTO service_merchants(customer_id,setup_started) VALUES(?,?) ON CONFLICT(customer_id) DO NOTHING`, customer, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	return GetServiceMerchant(db, customer)
}
func ListServiceRates(db *sql.DB, customer int64) ([]ServiceRate, error) {
	rows, err := db.Query(`SELECT id,label,mode,cents,active FROM service_rates WHERE customer_id=? ORDER BY active DESC,rowid DESC LIMIT 200`, customer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceRate{}
	for rows.Next() {
		var r ServiceRate
		if err = rows.Scan(&r.ID, &r.Label, &r.Mode, &r.Cents, &r.Active); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func CreateServiceRate(db *sql.DB, customer int64, r ServiceRate) (*ServiceRate, error) {
	r.Label = strings.TrimSpace(r.Label)
	if r.Label == "" || len(r.Label) > 160 || strings.ContainsAny(r.Label, "\x00\r\n") || (r.Mode != "prepaid" && r.Mode != "hourly") || r.Cents < 50 || r.Cents > 1000000 {
		return nil, ErrServiceState
	}
	var err error
	r.ID, err = ServiceID()
	if err != nil {
		return nil, err
	}
	r.Active = true
	result, err := db.Exec(`INSERT INTO service_rates(id,customer_id,label,mode,cents) SELECT ?,?,?,?,? WHERE (SELECT COUNT(*) FROM service_rates WHERE customer_id=? AND active=1)<100`, r.ID, customer, r.Label, r.Mode, r.Cents, customer)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, ErrServiceState
	}
	return &r, nil
}

const serviceColumns = `id,customer_id,license_id,member_id,target_kind,target_id,peer_id,label,mode,rate_cents,account_id,state,paid,connected_ms,amount_cents,created_at,connection_id,sequence,cumulative_ms,last_seen_ms,connection_open,checkout_id,checkout_url,checkout_started`

type serviceScanner interface{ Scan(...any) error }

func scanService(row serviceScanner) (*ServiceWork, error) {
	var w ServiceWork
	err := row.Scan(&w.ID, &w.CustomerID, &w.LicenseID, &w.MemberID, &w.TargetKind, &w.TargetID, &w.PeerID, &w.Label, &w.Mode, &w.RateCents, &w.AccountID, &w.State, &w.Paid, &w.ConnectedMS, &w.AmountCents, &w.CreatedAt, &w.ConnectionID, &w.Sequence, &w.CumulativeMS, &w.LastSeenMS, &w.ConnectionOpen, &w.CheckoutID, &w.CheckoutURL, &w.CheckoutStarted)
	return &w, err
}
func GetServiceWork(db *sql.DB, id string) (*ServiceWork, error) {
	return scanService(db.QueryRow(`SELECT `+serviceColumns+` FROM service_work WHERE id=?`, id))
}
func ListServiceWork(db *sql.DB, customer int64, license, member string) ([]ServiceWork, error) {
	rows, err := db.Query(`SELECT `+serviceColumns+` FROM service_work WHERE customer_id=? AND (?='' OR license_id=?) AND (?='' OR member_id=?) ORDER BY created_at DESC LIMIT 100`, customer, license, license, member, member)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceWork{}
	for rows.Next() {
		w, e := scanService(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}
func CreateServiceWork(db *sql.DB, customer int64, license, member, kind, target, peer, rate string) (*ServiceWork, error) {
	id, err := ServiceID()
	if err != nil {
		return nil, err
	}
	result, err := db.Exec(`INSERT INTO service_work(id,customer_id,license_id,member_id,target_kind,target_id,peer_id,label,mode,rate_cents,account_id,amount_cents,created_at)
 SELECT ?,r.customer_id,?,?,?,?,?,r.label,r.mode,r.cents,m.account_id,CASE WHEN r.mode='prepaid' THEN r.cents ELSE 0 END,?
 FROM service_rates r JOIN service_merchants m ON m.customer_id=r.customer_id WHERE r.id=? AND r.customer_id=? AND r.active=1 AND m.enabled=1 AND m.account_id<>'' AND EXISTS(SELECT 1 FROM licences WHERE license_id=? AND customer_id=r.customer_id)`, id, license, member, kind, target, peer, time.Now().Unix(), rate, customer, license)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return nil, ErrServiceState
	}
	return GetServiceWork(db, id)
}

// Only bounded, consecutive confirmations contribute time; silence never does.
func RecordServicePulse(db *sql.DB, id, connection string, seq, cumulative int64, closed bool, now time.Time) (*ServiceWork, error) {
	if len(connection) != 32 || seq < 1 || cumulative < 0 || cumulative > 7*24*3600000 {
		return nil, ErrServiceState
	}
	if _, err := hex.DecodeString(connection); err != nil {
		return nil, ErrServiceState
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	w, err := scanService(tx.QueryRow(`SELECT `+serviceColumns+` FROM service_work WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if w.State == "finished" || w.State == "cancelled" || (w.Mode == "prepaid" && !w.Paid) {
		return nil, ErrServiceState
	}
	at := now.UnixMilli()
	if w.ConnectionID == connection {
		if seq <= w.Sequence {
			return w, nil
		}
		if !w.ConnectionOpen || cumulative < w.CumulativeMS {
			return nil, ErrServiceState
		}
		delta := cumulative - w.CumulativeMS
		gap := at - w.LastSeenMS
		if gap < 0 || delta > gap+1000 {
			return nil, ErrServiceState
		}
		if gap <= 6000 && seq == w.Sequence+1 {
			w.ConnectedMS += delta
		}
	} else {
		if (w.ConnectionOpen && at-w.LastSeenMS < 10000) || seq != 1 || cumulative != 0 || closed {
			return nil, ErrServiceState
		}
	}
	if w.ConnectedMS > 7*24*3600000 {
		return nil, ErrServiceState
	}
	_, err = tx.Exec(`UPDATE service_work SET state='open',connected_ms=?,connection_id=?,sequence=?,cumulative_ms=?,last_seen_ms=?,connection_open=? WHERE id=?`, w.ConnectedMS, connection, seq, cumulative, at, !closed, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetServiceWork(db, id)
}
func ServiceAmount(cents, ms int64) (int64, error) {
	if cents < 50 || cents > 1000000 || ms < 0 || ms > 7*24*3600000 {
		return 0, ErrServiceState
	}
	return (cents*ms + 1800000) / 3600000, nil
}
func FinishServiceWork(db *sql.DB, id string, cancel bool, now time.Time) (*ServiceWork, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	w, err := scanService(tx.QueryRow(`SELECT `+serviceColumns+` FROM service_work WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	if w.State == "finished" || w.State == "cancelled" {
		return w, nil
	}
	if !cancel && w.Mode == "prepaid" && !w.Paid {
		return nil, errors.New("forfait non payé : annulez-le ou attendez la confirmation du paiement")
	}
	if w.ConnectionOpen && now.UnixMilli()-w.LastSeenMS < 10000 {
		return nil, errors.New("fermez la prise en main avant de terminer la prestation")
	}
	state := "finished"
	amount := w.AmountCents
	if cancel {
		if w.CheckoutStarted != 0 || w.Paid {
			return nil, errors.New("un paiement existe : gérer le remboursement depuis Stripe")
		}
		state = "cancelled"
	} else if w.Mode == "hourly" {
		amount, err = ServiceAmount(w.RateCents, w.ConnectedMS)
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.Exec(`UPDATE service_work SET state=?,connection_open=0,amount_cents=? WHERE id=?`, state, amount, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetServiceWork(db, id)
}
