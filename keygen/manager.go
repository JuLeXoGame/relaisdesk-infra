package main

import (
	dbpkg "database"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"
)

type License = dbpkg.License

func CreateLicense(db *sql.DB, email string, days int, maxConnections int, notes string) (*License, error) {
	return dbpkg.CreateLicense(db, email, days, maxConnections, notes)
}

func ListLicenses(db *sql.DB, statusFilter, emailFilter string) error {
	licences, err := dbpkg.ListLicenses(db, statusFilter, emailFilter)
	if err != nil {
		return err
	}

	fmt.Printf("%-20s | %-25s | %-10s | %-12s | %s\n", "LICENSE ID", "EMAIL", "STATUT", "EXPIRE LE", "CONN")
	fmt.Println(strings.Repeat("-", 86))
	for _, lic := range licences {
		fmt.Printf("%-20s | %-25s | %-10s | %-12s | %d/%d\n",
			lic.LicenseID,
			lic.Email,
			displayStatus(lic),
			lic.ExpiresAt.Format("2006-01-02"),
			lic.CurrentConnections,
			lic.MaxConnections,
		)
	}
	return nil
}

func RevokeLicense(db *sql.DB, licenseID, reason string) error {
	return dbpkg.RevokeLicense(db, licenseID, reason)
}

func ExtendLicense(db *sql.DB, licenseID string, days int) error {
	return dbpkg.ExtendLicense(db, licenseID, days)
}

func CheckLicense(db *sql.DB, licenseKey string) error {
	lic, err := dbpkg.GetLicenseByKey(db, licenseKey)
	if err != nil {
		return err
	}

	fmt.Printf("License ID  : %s\n", lic.LicenseID)
	fmt.Printf("Email       : %s\n", lic.Email)
	fmt.Printf("Statut      : %s\n", displayStatus(*lic))
	fmt.Printf("Expire le   : %s\n", lic.ExpiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Connexions  : %d/%d\n", lic.CurrentConnections, lic.MaxConnections)

	if lic.Status == "revoked" {
		reason := lic.RevokeReason
		if reason == "" {
			reason = "Non specifiee"
		}
		fmt.Printf("\nREVOQUEE - Raison : %s\n", reason)
	}
	return nil
}

func PrintStats(db *sql.DB) error {
	stats, err := dbpkg.GetStats(db)
	if err != nil {
		return err
	}

	fmt.Println("Statistiques RelaisDesk")
	fmt.Printf("Total licences       : %d\n", stats.Total)
	fmt.Printf("Actives              : %d\n", stats.Active)
	fmt.Printf("Expirees             : %d\n", stats.Expired)
	fmt.Printf("Revoquees            : %d\n", stats.Revoked)
	fmt.Printf("Connexions en cours  : %d\n", stats.CurrentConnections)
	return nil
}

func ExportCSV(db *sql.DB, filename string) error {
	licences, err := dbpkg.ListLicenses(db, "", "")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	// OpenFile does not tighten an existing file's permissions on Unix.
	if err := os.Chmod(filename, 0600); err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"license_id", "email", "license_key", "status", "created_at", "expires_at", "max_connections"}); err != nil {
		return err
	}

	for _, lic := range licences {
		row := []string{
			lic.LicenseID,
			lic.Email,
			lic.LicenseKey,
			displayStatus(lic),
			lic.CreatedAt.Format(time.RFC3339),
			lic.ExpiresAt.Format(time.RFC3339),
			fmt.Sprintf("%d", lic.MaxConnections),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func displayStatus(lic dbpkg.License) string {
	if lic.Status == "active" && !lic.ExpiresAt.After(time.Now().UTC()) {
		return "expired"
	}
	return lic.Status
}
