package main

import (
	"log"
)

func main() {
	if err := RunAdminGUI(); err != nil {
		log.Fatalf("Erreur d'exécution de l'application Admin : %v", err)
	}
}
