package database

import (
	"database/sql"
	"errors"
)

// Never mix test and real subscriptions by changing only an environment key.
func BindBillingEnvironment(db *sql.DB, mode string) error {
	if mode != "test" && mode != "live" {
		return errors.New("mode Stripe inconnu")
	}
	if _, err := db.Exec(`INSERT INTO billing_environment(singleton,mode) VALUES(1,?) ON CONFLICT(singleton) DO NOTHING`, mode); err != nil {
		return err
	}
	var saved string
	if err := db.QueryRow(`SELECT mode FROM billing_environment WHERE singleton=1`).Scan(&saved); err != nil {
		return err
	}
	if saved != mode {
		return errors.New("cette base est liée à un autre mode Stripe : ne pas mélanger TEST et LIVE")
	}
	return nil
}
