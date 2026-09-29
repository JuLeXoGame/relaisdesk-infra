package main

import (
	dbpkg "database"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	command, commandArgs, defaultDB := parseGlobalArgs(os.Args[1:])
	if command == "" {
		printUsage()
		os.Exit(1)
	}

	switch command {
	case "generate":
		generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
		email := generateCmd.String("email", "", "Email du client")
		days := generateCmd.Int("days", 30, "Duree en jours (defaut: 30 pour abonnement mensuel)")
		notes := generateCmd.String("notes", "", "Notes")
		maxConns := generateCmd.Int("max_connections", 1, "Connexions max (techniciens simultanes)")
		plan := generateCmd.String("plan", "", "Plan tarifaire : starter (10€), pro (20€), ultra (20€+)")
		techs := generateCmd.Int("technicians", 0, "Nombre de techniciens (pour les plans Pro ou Ultra)")
		dbPath := generateCmd.String("db", defaultDB, "chemin vers la base SQLite")
		generateCmd.Parse(commandArgs)

		if *email == "" {
			fail("email requis")
		}

		planDesc := ""
		monthlyPrice := 0.0
		if *plan != "" {
			planInfo, err := CalculatePlan(*plan, *techs)
			if err != nil {
				fail(err.Error())
			}
			*maxConns = planInfo.MaxConnections
			planDesc = planInfo.Name
			monthlyPrice = planInfo.MonthlyPrice
			if *notes == "" {
				*notes = fmt.Sprintf("Plan %s (%.2f€/mois)", planInfo.Name, planInfo.MonthlyPrice)
			}
		}

		db := initDB(*dbPath)
		defer db.Close()

		lic, err := CreateLicense(db, *email, *days, *maxConns, *notes)
		if err != nil {
			fail(err.Error())
		}

		fmt.Printf("\033[32mLicence creee avec succes !\033[0m\n\n")
		fmt.Printf("License ID   : %s\n", lic.LicenseID)
		fmt.Printf("License Key  : %s\n", lic.LicenseKey)
		fmt.Printf("Email        : %s\n", lic.Email)
		if planDesc != "" {
			fmt.Printf("Plan         : %s (%.2f € / mois)\n", planDesc, monthlyPrice)
		}
		fmt.Printf("Techniciens  : %d connexion(s) simultanee(s)\n", lic.MaxConnections)
		fmt.Printf("Viewers      : Illimites (codes temporaires 12h)\n")
		fmt.Printf("Expire le    : %s (%d jours)\n", lic.ExpiresAt.Format("2006-01-02"), *days)
		fmt.Printf("Statut       : %s\n", lic.Status)
		fmt.Println("\nEnvoyez ces informations au client.")

	case "list":
		listCmd := flag.NewFlagSet("list", flag.ExitOnError)
		status := listCmd.String("status", "", "Filtrer par statut")
		email := listCmd.String("email", "", "Filtrer par email")
		dbPath := listCmd.String("db", defaultDB, "chemin vers la base SQLite")
		listCmd.Parse(commandArgs)

		db := initDB(*dbPath)
		defer db.Close()

		if err := ListLicenses(db, *status, *email); err != nil {
			fail(err.Error())
		}

	case "revoke":
		revokeCmd := flag.NewFlagSet("revoke", flag.ExitOnError)
		id := revokeCmd.String("id", "", "License ID")
		reason := revokeCmd.String("reason", "", "Raison")
		dbPath := revokeCmd.String("db", defaultDB, "chemin vers la base SQLite")
		revokeCmd.Parse(commandArgs)

		if *id == "" {
			fail("id requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		if err := RevokeLicense(db, *id, *reason); err != nil {
			fail(err.Error())
		}
		fmt.Printf("\033[32mLicence %s revoquee.\033[0m\n", *id)

	case "extend":
		extendCmd := flag.NewFlagSet("extend", flag.ExitOnError)
		id := extendCmd.String("id", "", "License ID")
		days := extendCmd.Int("days", 30, "Jours a ajouter")
		dbPath := extendCmd.String("db", defaultDB, "chemin vers la base SQLite")
		extendCmd.Parse(commandArgs)

		if *id == "" {
			fail("id requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		if err := ExtendLicense(db, *id, *days); err != nil {
			fail(err.Error())
		}
		fmt.Printf("\033[32mLicence %s prolongee de %d jours.\033[0m\n", *id, *days)

	case "check":
		checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
		key := checkCmd.String("key", "", "License Key")
		dbPath := checkCmd.String("db", defaultDB, "chemin vers la base SQLite")
		checkCmd.Parse(commandArgs)

		if *key == "" {
			fail("key requise")
		}

		db := initDB(*dbPath)
		defer db.Close()

		if err := CheckLicense(db, *key); err != nil {
			fail(err.Error())
		}

	case "stats":
		statsCmd := flag.NewFlagSet("stats", flag.ExitOnError)
		dbPath := statsCmd.String("db", defaultDB, "chemin vers la base SQLite")
		statsCmd.Parse(commandArgs)

		db := initDB(*dbPath)
		defer db.Close()

		if err := PrintStats(db); err != nil {
			fail(err.Error())
		}

	case "export":
		exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
		file := exportCmd.String("file", "licences.csv", "Fichier d'export")
		dbPath := exportCmd.String("db", defaultDB, "chemin vers la base SQLite")
		exportCmd.Parse(commandArgs)

		db := initDB(*dbPath)
		defer db.Close()

		if err := ExportCSV(db, *file); err != nil {
			fail(err.Error())
		}
		fmt.Printf("\033[32mExporte vers %s avec succes.\033[0m\n", *file)

	case "viewer-code":
		cmd := flag.NewFlagSet("viewer-code", flag.ExitOnError)
		tech := cmd.String("technician", "", "ID de la licence technicien")
		email := cmd.String("email", "", "Email du client")
		dbPath := cmd.String("db", defaultDB, "chemin vers la base SQLite")
		cmd.Parse(commandArgs)

		if *tech == "" {
			fail("technician requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		vc, err := dbpkg.CreateViewerCode(db, *tech, *email)
		if err != nil {
			fail(err.Error())
		}

		fmt.Println("============================================")
		fmt.Println("Code viewer généré !")
		fmt.Printf("Code          : %s\n", vc.Code)
		fmt.Printf("Technicien    : %s\n", vc.TechnicianLicenseID)
		fmt.Printf("Client        : %s\n", vc.ClientEmail)
		fmt.Printf("Expire le     : %s\n", vc.ExpiresAt.Format("2006-01-02 15:04:05"))
		fmt.Println("Durée         : 12 heures")
		fmt.Println("Envoyez ce code à votre client.")
		fmt.Println("============================================")

	case "viewer-codes":
		cmd := flag.NewFlagSet("viewer-codes", flag.ExitOnError)
		tech := cmd.String("technician", "", "ID de la licence technicien")
		dbPath := cmd.String("db", defaultDB, "chemin vers la base SQLite")
		cmd.Parse(commandArgs)

		if *tech == "" {
			fail("technician requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		codes, err := dbpkg.ListViewerCodes(db, *tech)
		if err != nil {
			fail(err.Error())
		}

		fmt.Printf("%-20s | %-20s | %-19s | %-19s | %s\n", "CODE", "CLIENT", "CREATION", "EXPIRATION", "ACTIF")
		fmt.Println(strings.Repeat("-", 95))
		for _, c := range codes {
			active := "OUI"
			if !c.IsActive {
				active = "NON (revoqué)"
			}
			fmt.Printf("%-20s | %-20s | %-19s | %-19s | %s\n",
				c.Code, c.ClientEmail,
				c.CreatedAt.Format("2006-01-02 15:04:05"),
				c.ExpiresAt.Format("2006-01-02 15:04:05"),
				active)
		}

	case "viewer-revoke":
		cmd := flag.NewFlagSet("viewer-revoke", flag.ExitOnError)
		code := cmd.String("code", "", "Le code viewer à révoquer")
		dbPath := cmd.String("db", defaultDB, "chemin vers la base SQLite")
		cmd.Parse(commandArgs)

		if *code == "" {
			fail("code requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		if err := dbpkg.RevokeViewerCode(db, *code); err != nil {
			fail(err.Error())
		}
		fmt.Printf("\033[32mCode %s revoqué avec succès.\033[0m\n", *code)

	case "server-key":
		cmd := flag.NewFlagSet("server-key", flag.ExitOnError)
		add := cmd.String("add", "", "Clé publique serveur à activer")
		dbPath := cmd.String("db", defaultDB, "chemin vers la base SQLite")
		cmd.Parse(commandArgs)

		if *add == "" {
			fail("add requis")
		}

		db := initDB(*dbPath)
		defer db.Close()

		if err := AddServerKey(db, *add); err != nil {
			fail(err.Error())
		}
		fmt.Printf("\033[32mClé publique serveur activée.\033[0m\n")

	case "pricing":
		pricingCmd := flag.NewFlagSet("pricing", flag.ExitOnError)
		techs := pricingCmd.Int("technicians", 0, "Calculer le tarif Ultra pour un nombre specifique de techniciens")
		pricingCmd.Parse(commandArgs)

		if *techs > 0 {
			planInfo, err := CalculatePlan("ultra", *techs)
			if err != nil {
				fail(err.Error())
			}
			fmt.Printf("Plan Ultra (%d techniciens) : %.2f € / mois (viewers illimites)\n", planInfo.Technicians, planInfo.MonthlyPrice)
		} else {
			PrintPricingGrid()
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func parseGlobalArgs(args []string) (string, []string, string) {
	dbPath := "/data/relaisdesk/licences.db"

	for len(args) > 0 {
		switch {
		case args[0] == "--help" || args[0] == "-h":
			return "", nil, dbPath
		case args[0] == "--db" && len(args) > 1:
			dbPath = args[1]
			args = args[2:]
		case strings.HasPrefix(args[0], "--db="):
			dbPath = strings.TrimPrefix(args[0], "--db=")
			args = args[1:]
		default:
			return args[0], args[1:], dbPath
		}
	}
	return "", nil, dbPath
}

func initDB(dbPath string) *sql.DB {
	db, err := dbpkg.InitDatabase(dbPath)
	if err != nil {
		fail(fmt.Sprintf("connexion DB: %v", err))
	}
	return db
}

func fail(message string) {
	fmt.Fprintf(os.Stderr, "\033[31mErreur: %s\033[0m\n", message)
	os.Exit(1)
}

func printUsage() {
	fmt.Println("Usage: keygen [--db /data/relaisdesk/licences.db] <commande> [options]")
	fmt.Println("Commandes:")
	fmt.Println("  pricing       Afficher la grille tarifaire (Starter 24,90€, Pro 110€, Ultra 199€+)")
	fmt.Println("  generate      Generer une licence (--plan starter|pro|ultra --technicians N)")
	fmt.Println("  list          Lister les licences")
	fmt.Println("  revoke        Revoquer une licence")
	fmt.Println("  extend        Prolonger une licence")
	fmt.Println("  check         Verifier une licence")
	fmt.Println("  stats         Afficher les statistiques")
	fmt.Println("  export        Exporter en CSV")
	fmt.Println("  viewer-code   Generer un code viewer")
	fmt.Println("  viewer-codes  Lister les codes viewer")
	fmt.Println("  viewer-revoke Revoquer un code viewer")
	fmt.Println("  server-key    Activer une clé publique serveur (--add CLE)")
}
