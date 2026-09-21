package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionTokensAreHashedAtRest(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "session@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	token, err := CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := db.QueryRow(`SELECT token FROM technician_sessions WHERE license_id = ?`, lic.LicenseID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != hashSessionToken(token) {
		t.Fatalf("session token was not hashed at rest")
	}
	if got, err := ValidateTechnicianSession(db, token); err != nil || got != lic.LicenseID {
		t.Fatalf("ValidateTechnicianSession() = %q, %v", got, err)
	}
	if err := DeleteTechnicianSession(db, token); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTechnicianSession(db, token); err == nil {
		t.Fatal("deleted session remained valid")
	}
}

func TestPlaintextLegacyTechnicianSessionIsRejected(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "legacy-sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "legacy-session@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	legacyToken := "legacy-session-token-stored-in-plaintext"
	if _, err := db.Exec(
		`INSERT INTO technician_sessions (token, license_id, expires_at, last_used_at) VALUES (?, ?, ?, ?)`,
		legacyToken, lic.LicenseID, time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05"), time.Now().UTC().Format("2006-01-02 15:04:05"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTechnicianSession(db, legacyToken); err == nil {
		t.Fatal("a legacy plaintext session token was accepted")
	}
}

func TestAdminSessionLifecycle(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "admin-sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	token, err := CreateAdminSession(db)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(`SELECT token_hash FROM admin_sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != hashSessionToken(token) {
		t.Fatal("admin session token was not hashed at rest")
	}
	if err := ValidateAdminSession(db, token); err != nil {
		t.Fatal(err)
	}
	if err := DeleteAdminSession(db, token); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAdminSession(db, token); err == nil {
		t.Fatal("deleted admin session remained valid")
	}
}

func TestAdminSessionExpiresAfterIdleTimeout(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "admin-idle.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	token, err := CreateAdminSession(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE admin_sessions SET last_used_at = ? WHERE token_hash = ?`,
		time.Now().UTC().Add(-adminSessionIdleTimeout-time.Minute).Format("2006-01-02 15:04:05"), hashSessionToken(token),
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAdminSession(db, token); err == nil {
		t.Fatal("idle admin session remained valid")
	}
}

func TestFulfillPendingOrderIsIdempotent(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := CreateOrder(db, "paid@example.com", "starter", 1, "bank_transfer", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FulfillPendingOrder(db, order.OrderID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := FulfillPendingOrder(db, order.OrderID, "duplicate"); err == nil {
		t.Fatal("duplicate fulfillment unexpectedly succeeded")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM licences WHERE email = ?`, order.Email).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate fulfillment created %d licenses", count)
	}
}

func TestViewerRustDeskIDCannotBeRebound(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "viewer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('test-key', 1)`); err != nil {
		t.Fatal(err)
	}
	lic, err := CreateLicense(db, "viewer@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	code, err := CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	deviceKey := base64.RawURLEncoding.EncodeToString(publicKey)
	if err := BindViewerNetworkDeviceKey(db, code.ID, deviceKey); err != nil {
		t.Fatal(err)
	}
	if err := SetViewerRustDeskID(db, code.Code, "123456789", deviceKey); err != nil {
		t.Fatal(err)
	}
	if err := SetViewerRustDeskID(db, code.Code, "987654321", deviceKey); err == nil {
		t.Fatal("viewer code allowed rebinding to another RustDesk ID")
	}
}

func TestViewerNetworkDeviceKeyCannotBeRebound(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "viewer-device.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('test-key', 1)`); err != nil {
		t.Fatal(err)
	}
	lic, err := CreateLicense(db, "viewer-device@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	code, err := CreateViewerCode(db, lic.LicenseID, "client@example.com")
	if err != nil {
		t.Fatal(err)
	}
	firstPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	firstKey := base64.RawURLEncoding.EncodeToString(firstPublic)
	if err := BindViewerNetworkDeviceKey(db, code.ID, firstKey); err != nil {
		t.Fatal(err)
	}
	if err := BindViewerNetworkDeviceKey(db, code.ID, firstKey); err != nil {
		t.Fatalf("same device key should be renewable: %v", err)
	}
	secondPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := BindViewerNetworkDeviceKey(db, code.ID, base64.RawURLEncoding.EncodeToString(secondPublic)); err == nil {
		t.Fatal("viewer code allowed rebinding to another network device key")
	}
}

func TestViewerNetworkDeviceKeyMigrationFromLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-viewer.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE viewer_codes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		code TEXT NOT NULL UNIQUE,
		technician_license_id TEXT NOT NULL,
		client_email TEXT,
		client_rustdesk_id TEXT DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME NOT NULL,
		used_at DATETIME,
		is_active INTEGER NOT NULL DEFAULT 1,
		max_connections INTEGER NOT NULL DEFAULT 1,
		current_connections INTEGER NOT NULL DEFAULT 0,
		first_client_ip TEXT,
		last_connection_at DATETIME,
		revoked_reason TEXT
	)`)
	if closeErr := legacy.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(viewer_codes)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "network_device_public_key" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("network_device_public_key was not added to the legacy viewer_codes table")
	}
}
