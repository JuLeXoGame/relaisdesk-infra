package database

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql indexes.sql service_billing.sql
var schemaFS embed.FS

// License represents a license in the SQLite database.
type License struct {
	ID                 int
	CustomerID         int64
	LicenseID          string
	Email              string
	LicenseKey         string
	Status             string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	MaxConnections     int
	CurrentConnections int
	LastConnectionAt   *time.Time
	Notes              string
	RevokedAt          *time.Time
	RevokeReason       string
}

// Stats contains aggregate license counters.
type Stats struct {
	Total              int
	Active             int
	Expired            int
	Revoked            int
	CurrentConnections int
}

// InitDatabase opens or creates the SQLite database and applies schema.sql.
func InitDatabase(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		return nil, errors.New("database path is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	if err := os.Chmod(dbPath, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to secure database permissions: %w", err)
	}

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read embedded schema: %w", err)
	}

	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	if err := applyMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := applyAuthSecurityMigration(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("authentication security migration: %w", err)
	}
	billingSchema, err := schemaFS.ReadFile("service_billing.sql")
	if err == nil {
		_, err = db.Exec(string(billingSchema))
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("service billing migration: %w", err)
	}

	// Indexes that reference columns added by applyMigrations must be created
	// only after legacy databases have received those columns.
	indexes, err := schemaFS.ReadFile("indexes.sql")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read embedded indexes: %w", err)
	}
	if _, err := db.Exec(string(indexes)); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	return db, nil
}

func applyMigrations(db *sql.DB) error {
	migrations := []struct {
		table      string
		column     string
		definition string
	}{
		{"devices", "enrollment_version", "INTEGER NOT NULL DEFAULT 0"},
		{"devices", "peer_auth_version", "INTEGER NOT NULL DEFAULT 0"},
		{"technician_sessions", "last_used_at", "DATETIME"},
		{"technician_sessions", "team_member_id", "TEXT"},
		{"viewer_codes", "max_connections", "INTEGER NOT NULL DEFAULT 1"},
		{"viewer_codes", "current_connections", "INTEGER NOT NULL DEFAULT 0"},
		{"viewer_codes", "first_client_ip", "TEXT"},
		{"viewer_codes", "last_connection_at", "DATETIME"},
		{"viewer_codes", "revoked_reason", "TEXT"},
		{"viewer_codes", "client_rustdesk_id", "TEXT DEFAULT ''"},
		{"viewer_codes", "network_device_public_key", "TEXT"},
		{"orders", "billing_name", "TEXT"},
		{"orders", "billing_address", "TEXT"},
		{"orders", "billing_postal_code", "TEXT"},
		{"orders", "billing_city", "TEXT"},
		{"orders", "billing_country", "TEXT DEFAULT 'France'"},
		{"orders", "billing_siret", "TEXT"},
		{"orders", "invoice_number", "TEXT"},
		{"orders", "customer_type", "TEXT NOT NULL DEFAULT 'business'"},
		{"orders", "terms_version", "TEXT NOT NULL DEFAULT 'legacy'"},
		{"orders", "terms_accepted_at", "DATETIME"},
		{"orders", "immediate_performance_requested", "INTEGER NOT NULL DEFAULT 0"},
		{"orders", "license_email_sent_at", "DATETIME"},
		{"orders", "invoice_email_sent_at", "DATETIME"},
		{"orders", "order_kind", "TEXT NOT NULL DEFAULT 'initial'"},
		{"orders", "renewal_license_id", "TEXT"},
		{"orders", "billing_cycle", "TEXT NOT NULL DEFAULT 'monthly'"},
		{"licences", "customer_id", "INTEGER"},
		{"orders", "customer_id", "INTEGER"},
		{"invoices", "customer_id", "INTEGER"},
		{"customer_accounts", "customer_id", "INTEGER"},
		{"customer_login_tokens", "customer_id", "INTEGER"},
		{"customer_login_tokens", "mfa_attempts", "INTEGER NOT NULL DEFAULT 0"},
		{"customer_sessions", "customer_id", "INTEGER"},
		{"customer_accounts", "password_hash", "TEXT"},
		{"customer_accounts", "totp_enabled", "INTEGER NOT NULL DEFAULT 0"},
		{"customer_accounts", "totp_secret", "TEXT"},
		{"customer_accounts", "totp_recovery_codes", "TEXT"},
		{"customer_accounts", "totp_confirmed_at", "DATETIME"},
		{"licences", "totp_enabled", "INTEGER NOT NULL DEFAULT 0"},
		{"licences", "totp_secret", "TEXT"},
		{"licences", "totp_recovery_codes", "TEXT"},
		{"licences", "totp_confirmed_at", "DATETIME"},
		{"withdrawal_requests", "customer_email_sent_at", "DATETIME"},
		{"withdrawal_requests", "admin_email_sent_at", "DATETIME"},
		{"devices", "folder_id", "TEXT NOT NULL DEFAULT ''"},
		{"devices", "mac_address", "TEXT NOT NULL DEFAULT ''"},
		{"devices", "subnet_broadcast", "TEXT NOT NULL DEFAULT ''"},
		{"devices", "agent_version", "TEXT NOT NULL DEFAULT ''"},
	}

	for _, migration := range migrations {
		if err := ensureColumn(db, migration.table, migration.column, migration.definition); err != nil {
			return fmt.Errorf("failed to migrate %s.%s: %w", migration.table, migration.column, err)
		}
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS device_folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_id TEXT NOT NULL UNIQUE,
			customer_id INTEGER,
			license_id TEXT,
			parent_folder_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (customer_id) REFERENCES customers(id),
			FOREIGN KEY (license_id) REFERENCES licences(license_id)
		);
		CREATE INDEX IF NOT EXISTS idx_device_folders_customer ON device_folders(customer_id);
		CREATE INDEX IF NOT EXISTS idx_device_folders_license ON device_folders(license_id);
		CREATE INDEX IF NOT EXISTS idx_device_folders_parent ON device_folders(parent_folder_id);
		CREATE INDEX IF NOT EXISTS idx_devices_folder ON devices(folder_id);

		CREATE TABLE IF NOT EXISTS device_wake_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_device_id TEXT NOT NULL,
			license_id TEXT NOT NULL,
			mac_address TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			dispatched_at DATETIME,
			FOREIGN KEY (target_device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_device_wake_license ON device_wake_requests(license_id, status);

		CREATE TABLE IF NOT EXISTS device_update_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			target_version TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			dispatched_at DATETIME,
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_device_update_req ON device_update_requests(device_id, status);


		CREATE TABLE IF NOT EXISTS security_alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			code TEXT,
			first_ip TEXT,
			second_ip TEXT,
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			resolved_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_viewer_codes_expires ON viewer_codes(expires_at);
		CREATE INDEX IF NOT EXISTS idx_security_alerts_type ON security_alerts(type);
		CREATE INDEX IF NOT EXISTS idx_security_alerts_code ON security_alerts(code);
		CREATE INDEX IF NOT EXISTS idx_security_alerts_created_at ON security_alerts(created_at);

		CREATE TABLE IF NOT EXISTS admin_sessions (
			token_hash TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			last_used_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires ON admin_sessions(expires_at);

		CREATE TABLE IF NOT EXISTS invoices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_number TEXT NOT NULL UNIQUE,
			order_id TEXT,
			customer_email TEXT NOT NULL,
			customer_name TEXT NOT NULL,
			customer_address TEXT,
			customer_postal_code TEXT,
			customer_city TEXT,
			customer_country TEXT DEFAULT 'France',
			customer_siret TEXT,
			plan TEXT NOT NULL,
			technicians INTEGER NOT NULL DEFAULT 1,
			amount_ht REAL NOT NULL,
			amount_tva REAL NOT NULL DEFAULT 0.0,
			amount_ttc REAL NOT NULL,
			status TEXT NOT NULL DEFAULT 'paid',
			pdf_path TEXT NOT NULL,
			is_manual INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			notes TEXT,
			FOREIGN KEY (order_id) REFERENCES orders(order_id)
		);
		CREATE INDEX IF NOT EXISTS idx_invoices_number ON invoices(invoice_number);
		CREATE INDEX IF NOT EXISTS idx_invoices_order_id ON invoices(order_id);
		CREATE INDEX IF NOT EXISTS idx_invoices_email ON invoices(customer_email);
		CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices(created_at);

		CREATE TABLE IF NOT EXISTS withdrawal_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_id TEXT NOT NULL UNIQUE,
			order_id TEXT NOT NULL,
			email TEXT NOT NULL,
			customer_name TEXT,
			requested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			status TEXT NOT NULL DEFAULT 'received',
			processed_at DATETIME,
			customer_email_sent_at DATETIME,
			admin_email_sent_at DATETIME,
			notes TEXT,
			FOREIGN KEY (order_id) REFERENCES orders(order_id)
		);
		CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_order ON withdrawal_requests(order_id);
		CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_status ON withdrawal_requests(status);
		CREATE INDEX IF NOT EXISTS idx_withdrawal_requests_requested_at ON withdrawal_requests(requested_at);

		CREATE TABLE IF NOT EXISTS customer_accounts (
			email TEXT PRIMARY KEY COLLATE NOCASE,
			renewal_reminders_enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_login_at DATETIME
		);
		CREATE TABLE IF NOT EXISTS customer_login_tokens (
			token_hash TEXT PRIMARY KEY,
			email TEXT NOT NULL COLLATE NOCASE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			consumed_at DATETIME,
			FOREIGN KEY (email) REFERENCES customer_accounts(email)
		);
		CREATE INDEX IF NOT EXISTS idx_customer_login_tokens_email ON customer_login_tokens(email);
		CREATE INDEX IF NOT EXISTS idx_customer_login_tokens_expires ON customer_login_tokens(expires_at);
		CREATE TABLE IF NOT EXISTS customer_sessions (
			token_hash TEXT PRIMARY KEY,
			email TEXT NOT NULL COLLATE NOCASE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			last_used_at DATETIME NOT NULL,
			FOREIGN KEY (email) REFERENCES customer_accounts(email)
		);
		CREATE INDEX IF NOT EXISTS idx_customer_sessions_email ON customer_sessions(email);
		CREATE INDEX IF NOT EXISTS idx_customer_sessions_expires ON customer_sessions(expires_at);

		CREATE TABLE IF NOT EXISTS customer_2fa_challenges (
			challenge_token TEXT PRIMARY KEY,
			email TEXT NOT NULL COLLATE NOCASE,
			customer_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			email_code_hash TEXT,
			email_code_expires_at DATETIME,
			email_code_sent_at DATETIME,
			FOREIGN KEY (email) REFERENCES customer_accounts(email)
		);
		CREATE INDEX IF NOT EXISTS idx_customer_2fa_challenges_email ON customer_2fa_challenges(email);
		CREATE INDEX IF NOT EXISTS idx_customer_2fa_challenges_expires ON customer_2fa_challenges(expires_at);

		CREATE TABLE IF NOT EXISTS customer_trusted_devices (
			device_token_hash TEXT PRIMARY KEY,
			email TEXT NOT NULL COLLATE NOCASE,
			customer_id INTEGER NOT NULL,
			device_name TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			last_used_at DATETIME NOT NULL,
			FOREIGN KEY (email) REFERENCES customer_accounts(email)
		);
		CREATE INDEX IF NOT EXISTS idx_customer_trusted_devices_email ON customer_trusted_devices(email);
		CREATE INDEX IF NOT EXISTS idx_customer_trusted_devices_expires ON customer_trusted_devices(expires_at);

		CREATE TABLE IF NOT EXISTS technician_2fa_challenges (
			challenge_token TEXT PRIMARY KEY,
			license_id TEXT NOT NULL COLLATE NOCASE,
			email TEXT NOT NULL COLLATE NOCASE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			team_member_id TEXT,
			email_code_hash TEXT,
			email_code_expires_at DATETIME,
			email_code_sent_at DATETIME,
			FOREIGN KEY (license_id) REFERENCES licences(license_id)
		);
		CREATE INDEX IF NOT EXISTS idx_technician_2fa_challenges_license ON technician_2fa_challenges(license_id);
		CREATE INDEX IF NOT EXISTS idx_technician_2fa_challenges_expires ON technician_2fa_challenges(expires_at);
		CREATE TABLE IF NOT EXISTS renewal_reminders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			license_id TEXT NOT NULL,
			reminder_type TEXT NOT NULL,
			cycle_expires_at DATETIME NOT NULL,
			claimed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			sent_at DATETIME,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			UNIQUE (license_id, reminder_type, cycle_expires_at),
			FOREIGN KEY (license_id) REFERENCES licences(license_id)
		);
		CREATE INDEX IF NOT EXISTS idx_renewal_reminders_license ON renewal_reminders(license_id);
		CREATE TABLE IF NOT EXISTS interventions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			intervention_id TEXT NOT NULL UNIQUE,
			license_id TEXT NOT NULL,
			viewer_code_id INTEGER,
			client_reference TEXT,
			title TEXT NOT NULL DEFAULT 'Assistance à distance',
			status TEXT NOT NULL DEFAULT 'planned',
			started_at DATETIME,
			ended_at DATETIME,
			duration_minutes INTEGER,
			summary TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (license_id) REFERENCES licences(license_id),
			FOREIGN KEY (viewer_code_id) REFERENCES viewer_codes(id) ON DELETE SET NULL
		);
		CREATE INDEX IF NOT EXISTS idx_interventions_license ON interventions(license_id);
		CREATE INDEX IF NOT EXISTS idx_interventions_viewer_code ON interventions(viewer_code_id);
		CREATE INDEX IF NOT EXISTS idx_interventions_started ON interventions(started_at);
		CREATE INDEX IF NOT EXISTS idx_orders_renewal_license ON orders(renewal_license_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_open_renewal ON orders(renewal_license_id)
			WHERE order_kind = 'renewal' AND status IN ('pending', 'processing');
		CREATE INDEX IF NOT EXISTS idx_licences_customer ON licences(customer_id);
		CREATE INDEX IF NOT EXISTS idx_orders_customer ON orders(customer_id);
		CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id);
		CREATE INDEX IF NOT EXISTS idx_customer_users_customer ON customer_users(customer_id);
		CREATE INDEX IF NOT EXISTS idx_customer_users_email ON customer_users(email);
		CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_type TEXT NOT NULL,
			payload TEXT NOT NULL,
			unique_key TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 12,
			available_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			locked_at DATETIME,
			completed_at DATETIME,
			last_error TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_ready ON jobs(status, available_at);

		CREATE TABLE IF NOT EXISTS auth_bans (
			ip TEXT PRIMARY KEY,
			failed_count INTEGER NOT NULL DEFAULT 0,
			banned_until DATETIME NOT NULL,
			last_attempt_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			reason TEXT NOT NULL DEFAULT 'brute_force'
		);
		CREATE INDEX IF NOT EXISTS idx_auth_bans_banned_until ON auth_bans(banned_until);
	`); err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}

	if err := backfillStableCustomers(db); err != nil {
		return fmt.Errorf("failed to backfill stable customers: %w", err)
	}

	return nil
}

func backfillStableCustomers(db *sql.DB) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO customers (public_id, customer_type, display_name, billing_email)
		SELECT 'CUS-' || UPPER(hex(randomblob(16))),
		       COALESCE(NULLIF(MAX(customer_type), ''), 'business'),
		       NULLIF(MAX(display_name), ''), normalized_email
		FROM (
			SELECT LOWER(TRIM(email)) AS normalized_email, 'business' AS customer_type, '' AS display_name FROM licences
			UNION ALL
			SELECT LOWER(TRIM(email)), COALESCE(customer_type, 'business'), COALESCE(billing_name, '') FROM orders
			UNION ALL
			SELECT LOWER(TRIM(customer_email)), 'business', COALESCE(customer_name, '') FROM invoices
			UNION ALL
			SELECT LOWER(TRIM(email)), 'business', '' FROM customer_accounts
		)
		WHERE normalized_email != ''
		GROUP BY normalized_email;

		INSERT OR IGNORE INTO customer_users (customer_id, email, role)
		SELECT id, billing_email, 'owner' FROM customers;

		INSERT OR IGNORE INTO customer_accounts (email, customer_id)
		SELECT billing_email, id FROM customers;
		UPDATE customer_accounts SET customer_id = (
			SELECT c.id FROM customers c WHERE LOWER(c.billing_email) = LOWER(customer_accounts.email)
		) WHERE customer_id IS NULL;

		UPDATE licences SET customer_id = (
			SELECT c.id FROM customers c WHERE LOWER(c.billing_email) = LOWER(licences.email)
		) WHERE customer_id IS NULL;
		UPDATE orders SET customer_id = (
			SELECT c.id FROM customers c WHERE LOWER(c.billing_email) = LOWER(orders.email)
		) WHERE customer_id IS NULL;
		UPDATE invoices SET customer_id = COALESCE(
			(SELECT o.customer_id FROM orders o WHERE o.order_id = invoices.order_id),
			(SELECT c.id FROM customers c WHERE LOWER(c.billing_email) = LOWER(invoices.customer_email))
		) WHERE customer_id IS NULL;
		UPDATE customer_login_tokens SET customer_id = (
			SELECT ca.customer_id FROM customer_accounts ca WHERE LOWER(ca.email) = LOWER(customer_login_tokens.email)
		) WHERE customer_id IS NULL;
		UPDATE customer_sessions SET customer_id = (
			SELECT ca.customer_id FROM customer_accounts ca WHERE LOWER(ca.email) = LOWER(customer_sessions.email)
		) WHERE customer_id IS NULL;
		UPDATE customers SET renewal_reminders_enabled = COALESCE((
			SELECT ca.renewal_reminders_enabled FROM customer_accounts ca
			WHERE ca.customer_id = customers.id ORDER BY ca.created_at ASC LIMIT 1
		), renewal_reminders_enabled);
	`)
	if err != nil {
		return err
	}
	if err := ensureColumn(db, "technician_2fa_challenges", "team_member_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "technician_2fa_challenges", "email_code_hash", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "technician_2fa_challenges", "email_code_expires_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumn(db, "technician_2fa_challenges", "email_code_sent_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumn(db, "customer_2fa_challenges", "email_code_hash", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "customer_2fa_challenges", "email_code_expires_at", "DATETIME"); err != nil {
		return err
	}
	if err := ensureColumn(db, "customer_2fa_challenges", "email_code_sent_at", "DATETIME"); err != nil {
		return err
	}
	return ensureColumn(db, "customer_2fa_challenges", "email_code_allowed", "INTEGER NOT NULL DEFAULT 1")
}

func ensureColumn(db *sql.DB, table string, column string, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

// ValidateLicense checks that a license exists, is active, has not expired and
// has free connection capacity.
func ValidateLicense(db *sql.DB, licenseID string, licenseKey string) (*License, error) {
	lic, err := ValidateLicenseCredentials(db, licenseID, licenseKey)
	if err != nil {
		return nil, err
	}
	if lic.CurrentConnections >= lic.MaxConnections {
		return nil, fmt.Errorf("maximum connections reached")
	}
	return lic, nil
}

// ValidateLicenseCredentials validates identity and license state without
// consuming or requiring a free concurrent application slot.
func ValidateLicenseCredentials(db *sql.DB, licenseID string, licenseKey string) (*License, error) {
	lic, err := getLicenseByWhere(db, "license_id = ? AND license_key = ?", licenseID, licenseKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid license ID or key")
		}
		return nil, err
	}

	if lic.Status != "active" {
		return nil, fmt.Errorf("license is not active")
	}
	if !lic.ExpiresAt.After(time.Now().UTC()) {
		return nil, fmt.Errorf("license has expired")
	}
	if lic.MaxConnections <= 0 {
		return nil, fmt.Errorf("license has invalid connection capacity")
	}
	return lic, nil
}

// GetLicenseByKey retrieves a license by its key.
func GetLicenseByKey(db *sql.DB, licenseKey string) (*License, error) {
	lic, err := getLicenseByWhere(db, "license_key = ?", licenseKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid license key")
		}
		return nil, err
	}
	return lic, nil
}

// GetLicense retrieves a license by its ID.
func GetLicense(db *sql.DB, licenseID string) (*License, error) {
	lic, err := getLicenseByWhere(db, "license_id = ?", licenseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("invalid license ID")
		}
		return nil, err
	}
	return lic, nil
}

// GetLicenseByID retrieves a license by its ID.
func GetLicenseByID(db *sql.DB, licenseID string) (*License, error) {
	return GetLicense(db, licenseID)
}

// IncrementConnections increments current_connections if capacity is available.
func IncrementConnections(db *sql.DB, licenseID string) error {
	res, err := db.Exec(`
		UPDATE licences
		SET current_connections = current_connections + 1,
		    last_connection_at = CURRENT_TIMESTAMP
		WHERE license_id = ?
		  AND status = 'active'
		  AND expires_at > CURRENT_TIMESTAMP
		  AND current_connections < max_connections
	`, licenseID)
	if err != nil {
		return fmt.Errorf("failed to increment connections: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect increment result: %w", err)
	}
	if affected == 0 {
		return errors.New("license has no available connection slot")
	}
	return nil
}

// DecrementConnections decrements current_connections while keeping it >= 0.
func DecrementConnections(db *sql.DB, licenseID string) error {
	_, err := db.Exec(`
		UPDATE licences
		SET current_connections = MAX(0, current_connections - 1)
		WHERE license_id = ?
	`, licenseID)
	if err != nil {
		return fmt.Errorf("failed to decrement connections: %w", err)
	}
	return nil
}

// LogConnection inserts a connection log entry and returns its row ID.
func LogConnection(db *sql.DB, licenseID string, clientIP string) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO connections_log (license_id, client_ip)
		VALUES (?, ?)
	`, licenseID, clientIP)
	if err != nil {
		return 0, fmt.Errorf("failed to log connection: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to read connection log ID: %w", err)
	}
	return id, nil
}

// LogDisconnection updates the disconnection time and duration for a log row.
func LogDisconnection(db *sql.DB, logID int64) error {
	res, err := db.Exec(`
		UPDATE connections_log
		SET disconnected_at = CURRENT_TIMESTAMP,
		    duration_seconds = CAST((julianday(CURRENT_TIMESTAMP) - julianday(connected_at)) * 86400 AS INTEGER)
		WHERE id = ?
	`, logID)
	if err != nil {
		return fmt.Errorf("failed to log disconnection: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect disconnection result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("connection log %d not found", logID)
	}
	return nil
}

// CreateLicense generates, stores and returns a new active license.
func CreateLicense(db *sql.DB, email string, days int, maxConn int, notes string) (*License, error) {
	if strings.TrimSpace(email) == "" {
		return nil, errors.New("email is required")
	}
	if days < 0 {
		return nil, errors.New("days must be >= 0")
	}
	if maxConn <= 0 {
		maxConn = 1
	}
	customer, err := EnsureCustomer(db, email, "business", "")
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		licenseID, err := generateLicenseID()
		if err != nil {
			return nil, err
		}
		licenseKey, err := generateLicenseKey()
		if err != nil {
			return nil, err
		}

		expiresAt := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		_, err = db.Exec(`
			INSERT INTO licences (customer_id, license_id, email, license_key, expires_at, max_connections, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, customer.ID, licenseID, email, licenseKey, expiresAt.Format("2006-01-02 15:04:05"), maxConn, notes)
		if err == nil {
			return GetLicenseByKey(db, licenseKey)
		}
		lastErr = err
	}
	return nil, fmt.Errorf("failed to create unique license: %w", lastErr)
}

// RevokeLicense marks a license as revoked and stores the reason.
func RevokeLicense(db *sql.DB, licenseID string, reason string) error {
	res, err := db.Exec(`
		UPDATE licences
		SET status = 'revoked',
		    revoked_at = CURRENT_TIMESTAMP,
		    revoke_reason = ?
		WHERE license_id = ?
	`, reason, licenseID)
	if err != nil {
		return fmt.Errorf("failed to revoke license: %w", err)
	}
	return ensureAffected(res, "license not found")
}

// ExtendLicense adds days to a license expiration date, starting from now if expired, and reactivates it.
func ExtendLicense(db *sql.DB, licenseID string, days int) error {
	if days <= 0 {
		return errors.New("days must be > 0")
	}
	res, err := db.Exec(`
		UPDATE licences
		SET expires_at = datetime(CASE WHEN expires_at > CURRENT_TIMESTAMP THEN expires_at ELSE CURRENT_TIMESTAMP END, '+' || ? || ' days'),
		    status = 'active',
		    revoked_at = NULL,
		    revoke_reason = NULL
		WHERE license_id = ?
	`, days, licenseID)
	if err != nil {
		return fmt.Errorf("failed to extend license: %w", err)
	}
	return ensureAffected(res, "license not found")
}

// ListLicenses returns licenses filtered by optional status and email values.
func ListLicenses(db *sql.DB, status string, email string) ([]License, error) {
	query := `
		SELECT id, license_id, email, license_key, status, created_at, expires_at,
		       max_connections, current_connections, last_connection_at, notes,
		       revoked_at, revoke_reason
		FROM licences
		WHERE 1 = 1
	`
	args := make([]any, 0, 2)

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if email != "" {
		query += " AND email = ?"
		args = append(args, email)
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list licenses: %w", err)
	}
	defer rows.Close()

	var licences []License
	for rows.Next() {
		lic, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		licences = append(licences, *lic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate licenses: %w", err)
	}
	return licences, nil
}

// GetStats returns global license statistics.
func GetStats(db *sql.DB) (*Stats, error) {
	stats := &Stats{}
	queries := []struct {
		dst   *int
		query string
	}{
		{&stats.Total, `SELECT COUNT(*) FROM licences`},
		{&stats.Active, `SELECT COUNT(*) FROM licences WHERE status='active' AND expires_at > CURRENT_TIMESTAMP`},
		{&stats.Expired, `SELECT COUNT(*) FROM licences WHERE expires_at <= CURRENT_TIMESTAMP AND status != 'revoked'`},
		{&stats.Revoked, `SELECT COUNT(*) FROM licences WHERE status='revoked'`},
		{&stats.CurrentConnections, `SELECT COALESCE(SUM(current_connections), 0) FROM licences`},
	}

	for _, q := range queries {
		if err := db.QueryRow(q.query).Scan(q.dst); err != nil {
			return nil, fmt.Errorf("failed to calculate stats: %w", err)
		}
	}
	return stats, nil
}

func getLicenseByWhere(db *sql.DB, where string, args ...any) (*License, error) {
	row := db.QueryRow(`
		SELECT id, license_id, email, license_key, status, created_at, expires_at,
		       max_connections, current_connections, last_connection_at, notes,
		       revoked_at, revoke_reason
		FROM licences
		WHERE `+where, args...)
	return scanLicense(row)
}

// ParseSQLiteTime parses date/time values from SQLite in various formats.
func ParseSQLiteTime(val any) (time.Time, error) {
	if val == nil {
		return time.Time{}, errors.New("nil datetime value")
	}
	switch v := val.(type) {
	case time.Time:
		return v.UTC(), nil
	case *time.Time:
		if v == nil {
			return time.Time{}, errors.New("nil *time.Time")
		}
		return v.UTC(), nil
	case string:
		return ParseSQLiteTimeString(v)
	case *string:
		if v == nil {
			return time.Time{}, errors.New("nil *string")
		}
		return ParseSQLiteTimeString(*v)
	case []byte:
		return ParseSQLiteTimeString(string(v))
	case *[]byte:
		if v == nil {
			return time.Time{}, errors.New("nil *[]byte")
		}
		return ParseSQLiteTimeString(string(*v))
	case int64:
		return time.Unix(v, 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported datetime type: %T (%v)", val, val)
	}
}

// ParseSQLiteTimeString parses strings in formats supported by SQLite and Go.
func ParseSQLiteTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty datetime string")
	}

	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UTC(), nil
		}
	}

	if strings.Contains(s, " ") {
		tStr := strings.Replace(s, " ", "T", 1)
		for _, format := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
			if t, err := time.Parse(format, tStr); err == nil {
				return t.UTC(), nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("cannot parse datetime: %q", s)
}

type licenseScanner interface {
	Scan(dest ...any) error
}

func scanLicense(scanner licenseScanner) (*License, error) {
	var lic License
	var (
		createdAtRaw        any
		expiresAtRaw        any
		lastConnectionAtRaw any
		notesRaw            sql.NullString
		revokedAtRaw        any
		revokeReasonRaw     sql.NullString
	)

	err := scanner.Scan(
		&lic.ID,
		&lic.LicenseID,
		&lic.Email,
		&lic.LicenseKey,
		&lic.Status,
		&createdAtRaw,
		&expiresAtRaw,
		&lic.MaxConnections,
		&lic.CurrentConnections,
		&lastConnectionAtRaw,
		&notesRaw,
		&revokedAtRaw,
		&revokeReasonRaw,
	)
	if err != nil {
		return nil, err
	}

	if createdAtRaw != nil {
		if t, err := ParseSQLiteTime(createdAtRaw); err == nil {
			lic.CreatedAt = t
		}
	}
	if expiresAtRaw != nil {
		t, err := ParseSQLiteTime(expiresAtRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse expires_at %v: %w", expiresAtRaw, err)
		}
		lic.ExpiresAt = t
	}
	if lastConnectionAtRaw != nil {
		if t, err := ParseSQLiteTime(lastConnectionAtRaw); err == nil {
			lic.LastConnectionAt = &t
		}
	}
	if notesRaw.Valid {
		lic.Notes = notesRaw.String
	}
	if revokedAtRaw != nil {
		if t, err := ParseSQLiteTime(revokedAtRaw); err == nil {
			lic.RevokedAt = &t
		}
	}
	if revokeReasonRaw.Valid {
		lic.RevokeReason = revokeReasonRaw.String
	}

	return &lic, nil
}

func validateUsable(lic *License) error {
	if lic.Status != "active" {
		return fmt.Errorf("license is not active")
	}
	if !lic.ExpiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("license has expired")
	}
	if lic.CurrentConnections >= lic.MaxConnections {
		return fmt.Errorf("maximum connections reached")
	}
	return nil
}

func ensureAffected(res sql.Result, msg string) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect update result: %w", err)
	}
	if affected == 0 {
		return errors.New(msg)
	}
	return nil
}

func generateLicenseID() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate license ID: %w", err)
	}
	hexValue := strings.ToUpper(hex.EncodeToString(bytes))
	return fmt.Sprintf("MP-%s-%s-%s", hexValue[0:4], hexValue[4:8], hexValue[8:12]), nil
}

func generateLicenseKey() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate license key: %w", err)
	}
	return "mpsk_" + hex.EncodeToString(bytes), nil
}
