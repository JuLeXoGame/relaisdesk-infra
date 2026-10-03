package database

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// customerSessionTestToken creates a licence-backed customer with a password
// and returns one of its valid session tokens.
func customerSessionTestToken(t *testing.T, dbPath string) (string, string) {
	t.Helper()
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "session@example.invalid", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = SetCustomerPassword(db, lic.Email, "SessionPassword2026!"); err != nil {
		t.Fatal(err)
	}
	token, _, _, err := ValidateCustomerPassword(db, lic.Email, "SessionPassword2026!")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("aucun jeton de session émis")
	}
	return dbPath, token
}

// TestCustomerSessionValidationSurvivesWriteLock is the regression test for
// "Session client invalide ou expirée" shown to connected users: the
// last_used_at refresh is bookkeeping, so lock contention must never turn a
// valid session into a 401.
func TestCustomerSessionValidationSurvivesWriteLock(t *testing.T) {
	path, token := customerSessionTestToken(t, filepath.Join(t.TempDir(), "write-lock.db"))

	db, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Single connection + short busy wait so lock contention fails fast
	// instead of riding out the 5 s production busy timeout.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA busy_timeout = 50`); err != nil {
		t.Fatal(err)
	}

	locker, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	// One pooled connection so BEGIN and ROLLBACK hit the same handle.
	locker.SetMaxOpenConns(1)
	if _, err = locker.Exec(`BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	identity, err := ValidateCustomerSessionIdentity(db, token)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("session valide rejetée pendant un verrou d'écriture : %v", err)
	}
	if identity == nil || identity.Email == "" {
		t.Fatal("identité de session manquante malgré le succès")
	}
	if elapsed > 4*time.Second {
		t.Fatalf("validation trop lente sous verrou : %v", elapsed)
	}
	if _, err = locker.Exec(`ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	if _, err = ValidateCustomerSessionIdentity(db, token); err != nil {
		t.Fatalf("session invalide après la levée du verrou : %v", err)
	}
}

// TestCustomerSessionConcurrentValidation mirrors the dashboard loading many
// panels in parallel against the same session token.
func TestCustomerSessionConcurrentValidation(t *testing.T) {
	path, token := customerSessionTestToken(t, filepath.Join(t.TempDir(), "concurrent.db"))

	db, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const workers = 32
	const iterations = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers*iterations)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if _, err := ValidateCustomerSessionIdentity(db, token); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("validation concurrente rejetée : %v", err)
	}
}
