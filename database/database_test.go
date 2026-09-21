package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestInitDatabaseMigratesLegacyColumnsBeforeCreatingIndexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`
		CREATE TABLE licences (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			license_id TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL,
			license_key TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			max_connections INTEGER NOT NULL DEFAULT 1,
			current_connections INTEGER NOT NULL DEFAULT 0,
			last_connection_at DATETIME,
			notes TEXT,
			revoked_at DATETIME,
			revoke_reason TEXT
		);
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL,
			plan TEXT NOT NULL,
			technicians INTEGER NOT NULL DEFAULT 1,
			price REAL NOT NULL,
			payment_method TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			stripe_session_id TEXT,
			license_id TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			paid_at DATETIME,
			expires_at DATETIME,
			notes TEXT
		);
	`)
	if closeErr := legacy.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("legacy database migration failed: %v", err)
	}
	defer db.Close()

	var indexName string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_licences_customer'`).Scan(&indexName); err != nil {
		t.Fatalf("index on migrated column was not created: %v", err)
	}
	if indexName != "idx_licences_customer" {
		t.Fatalf("unexpected index created: %s", indexName)
	}
}

func TestCreateValidateRevokeAndStats(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "client@example.com", 365, 1, "test")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	if _, err := ValidateLicense(db, lic.LicenseID, lic.LicenseKey); err != nil {
		t.Fatalf("ValidateLicense failed: %v", err)
	}

	if err := IncrementConnections(db, lic.LicenseID); err != nil {
		t.Fatalf("IncrementConnections failed: %v", err)
	}

	if _, err := ValidateLicense(db, lic.LicenseID, lic.LicenseKey); err == nil {
		t.Fatal("ValidateLicense should fail when max connections is reached")
	}

	logID, err := LogConnection(db, lic.LicenseID, "127.0.0.1")
	if err != nil {
		t.Fatalf("LogConnection failed: %v", err)
	}
	if err := LogDisconnection(db, logID); err != nil {
		t.Fatalf("LogDisconnection failed: %v", err)
	}

	if err := DecrementConnections(db, lic.LicenseID); err != nil {
		t.Fatalf("DecrementConnections failed: %v", err)
	}

	if err := ExtendLicense(db, lic.LicenseID, 30); err != nil {
		t.Fatalf("ExtendLicense failed: %v", err)
	}

	if err := RevokeLicense(db, lic.LicenseID, "test revoke"); err != nil {
		t.Fatalf("RevokeLicense failed: %v", err)
	}

	if _, err := ValidateLicense(db, lic.LicenseID, lic.LicenseKey); err == nil {
		t.Fatal("ValidateLicense should fail for a revoked license")
	}

	stats, err := GetStats(db)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if stats.Total != 1 || stats.Revoked != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestExpiredLicenseIsRejected(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "expired@example.com", 0, 1, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := ValidateLicense(db, lic.LicenseID, lic.LicenseKey); err == nil {
		t.Fatal("ValidateLicense should reject an expired license")
	}
}
