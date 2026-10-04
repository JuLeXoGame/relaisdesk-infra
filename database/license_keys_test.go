package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHashLicenseKeyFormat(t *testing.T) {
	h1 := HashLicenseKey("mpsk_testkey00000000000000000000001")
	h2 := HashLicenseKey("mpsk_testkey00000000000000000000001")
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash instable ou mal formé: %q", h1)
	}
	if !IsHashedLicenseKey(h1) {
		t.Fatal("hash non reconnu")
	}
	if IsHashedLicenseKey("mpsk_testkey00000000000000000000001") {
		t.Fatal("clé en clair reconnue comme hash")
	}
	if HashLicenseKey("mpsk_aaa") == HashLicenseKey("mpsk_aab") {
		t.Fatal("collision sur clés distinctes")
	}
}

func TestLicenseKeyHint(t *testing.T) {
	hint := LicenseKeyHint("mpsk_ab12cd34ef56gh78ij90kl12mn34op56")
	if !strings.HasPrefix(hint, "mpsk_ab12") || !strings.HasSuffix(hint, "op56") {
		t.Fatalf("indice inattendu: %q", hint)
	}
	if strings.Contains(hint, "cd34ef56") {
		t.Fatalf("indice trop révélateur: %q", hint)
	}
	if LicenseKeyHint("court") != "****" {
		t.Fatal("clé courte non masquée")
	}
}

func TestCreateLicenseStoresHashReturnsPlaintextOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "keyhash.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	created, err := CreateLicense(db, "hash@example.com", 30, 2, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.LicenseKey, "mpsk_") {
		t.Fatalf("la création doit retourner la clé en clair, obtenu %q", created.LicenseKey)
	}
	plaintext := created.LicenseKey
	if created.KeyHint == "" || created.KeyHint == plaintext {
		t.Fatalf("indice manquant ou égal à la clé: %q", created.KeyHint)
	}

	// The stored value must be the hash, never the plaintext.
	var stored, hint string
	if err := db.QueryRow(`SELECT license_key, key_hint FROM licences WHERE license_id = ?`,
		created.LicenseID).Scan(&stored, &hint); err != nil {
		t.Fatal(err)
	}
	if stored == plaintext {
		t.Fatal("clé stockée en clair")
	}
	if stored != HashLicenseKey(plaintext) {
		t.Fatal("valeur stockée différente du hash attendu")
	}
	if hint != created.KeyHint {
		t.Fatalf("indice stocké %q, attendu %q", hint, created.KeyHint)
	}

	// Plaintext still validates through every credential path.
	if _, err := ValidateLicenseCredentials(db, created.LicenseID, plaintext); err != nil {
		t.Fatalf("validation impossible avec la clé en clair: %v", err)
	}
	if _, err := GetLicenseByKey(db, plaintext); err != nil {
		t.Fatalf("recherche par clé impossible: %v", err)
	}
	if _, err := ValidateLicenseCredentials(db, created.LicenseID, "mpsk_fausse00000000000000000000000"); err == nil {
		t.Fatal("fausse clé acceptée")
	}
}

func TestMigrateLicenseKeysToHash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "keymig.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	legacy := "mpsk_legacy0000000000000000000000001"
	if _, err := db.Exec(`INSERT INTO licences (license_id, email, license_key, status, expires_at, max_connections)
		VALUES ('LIC-LEGACY', 'legacy@example.com', ?, 'active', '2030-01-01 00:00:00', 1)`, legacy); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLicenseKeysToHash(db); err != nil {
		t.Fatal(err)
	}
	var stored, hint string
	if err := db.QueryRow(`SELECT license_key, key_hint FROM licences WHERE license_id = 'LIC-LEGACY'`).Scan(&stored, &hint); err != nil {
		t.Fatal(err)
	}
	if stored != HashLicenseKey(legacy) {
		t.Fatal("ligne historique non migrée vers le hash")
	}
	if hint != LicenseKeyHint(legacy) {
		t.Fatalf("indice non rempli: %q", hint)
	}
	// Idempotent: a second run changes nothing and validates.
	if err := MigrateLicenseKeysToHash(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateLicenseCredentials(db, "LIC-LEGACY", legacy); err != nil {
		t.Fatalf("clé migrée invalide: %v", err)
	}
}

func TestScrubJobPayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "scrub.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if _, err := EnqueueJob(db, "test", `{"order_id":"RD-1","license_key":"mpsk_secret"}`, "k1", 3, now); err != nil {
		t.Fatal(err)
	}
	job, err := ClaimNextJob(db, now)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %+v", err, job)
	}
	if err := ScrubJobPayload(db, job.ID, `{"order_id":"RD-1"}`); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload FROM jobs WHERE id = ?`, job.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "mpsk_secret") {
		t.Fatalf("secret toujours présent: %q", payload)
	}
}
