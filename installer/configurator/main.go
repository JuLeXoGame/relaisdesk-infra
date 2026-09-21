package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		if len(os.Args) != 3 || os.Args[1] != "--connect-uri" {
			fmt.Fprintln(os.Stderr, "Arguments non reconnus")
			os.Exit(1)
		}
		id, err := parseFleetURI(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		pendingFleetDevice = id
	}
	if err := RunGUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur fatale: %s\n", err)
		os.Exit(1)
	}
}
