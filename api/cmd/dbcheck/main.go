package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	dbpkg "database"
)

func main() {
	dbPath := flag.String("db", "", "copie SQLite à migrer et vérifier")
	flag.Parse()
	if strings.TrimSpace(*dbPath) == "" {
		log.Fatal("-db est obligatoire")
	}

	db, err := dbpkg.InitDatabase(*dbPath)
	if err != nil {
		log.Fatalf("migration impossible: %v", err)
	}
	defer db.Close()

	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		log.Fatalf("contrôle d'intégrité impossible: %v", err)
	}
	if integrity != "ok" {
		log.Fatalf("contrôle d'intégrité en échec: %s", integrity)
	}

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		log.Fatalf("contrôle des clés étrangères impossible: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		log.Fatal("contrôle des clés étrangères en échec")
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("contrôle des clés étrangères interrompu: %v", err)
	}

	fmt.Println("Migration et intégrité SQLite : OK")
}
