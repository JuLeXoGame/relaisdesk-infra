package main

import (
	"fmt"
	"os"
)

func main() {
	id, err := parseConnectURIArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		pendingFleetError = err.Error()
	} else {
		pendingFleetDevice = id
	}
	ensureFleetProtocol()
	if err := RunGUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur fatale: %s\n", err)
		os.Exit(1)
	}
}
