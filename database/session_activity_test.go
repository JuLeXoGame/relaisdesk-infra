package database

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestCustomerActivityWritesAreCoalescedAndRevocationImmediate(t *testing.T) {
	path, token := customerSessionTestToken(t, filepath.Join(t.TempDir(), "activity.db"))
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE activity_writes(n INTEGER); INSERT INTO activity_writes VALUES(0);
        CREATE TRIGGER count_activity AFTER UPDATE OF last_used_at ON customer_sessions
        BEGIN UPDATE activity_writes SET n=n+1; END;`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if _, err := ValidateCustomerSessionIdentity(db, token); err != nil {
			t.Fatal(err)
		}
	}
	var writes int
	if err = db.QueryRow(`SELECT n FROM activity_writes`).Scan(&writes); err != nil || writes != 0 {
		t.Fatalf("recent activity produced %d writes: %v", writes, err)
	}
	if _, err = db.Exec(`UPDATE customer_sessions SET last_used_at=datetime('now','-1 minute'); UPDATE activity_writes SET n=0`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := ValidateCustomerSessionIdentity(db, token); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if err = db.QueryRow(`SELECT n FROM activity_writes`).Scan(&writes); err != nil || writes != 1 {
		t.Fatalf("parallel activity produced %d writes, want 1: %v", writes, err)
	}
	if err = DeleteCustomerSession(db, token); err != nil {
		t.Fatal(err)
	}
	if _, err = ValidateCustomerSessionIdentity(db, token); err == nil {
		t.Fatal("revoked session accepted")
	}
}
