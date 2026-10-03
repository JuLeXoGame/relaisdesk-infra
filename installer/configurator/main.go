package main

import (
	"fmt"
	"os"
	"runtime"
)

// technicianCLI is set on Linux only: terminal fallback (--cli or headless).
var technicianCLI func() error

func main() {
	args := os.Args[1:]
	wantCLI := false
	if runtime.GOOS == "linux" && technicianCLI != nil {
		var rest []string
		wantCLI, rest = splitCLIArgs(args)
		args = rest
		if !wantCLI && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			wantCLI = true
		}
	}
	id, err := parseConnectURIArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		pendingFleetError = err.Error()
	} else {
		pendingFleetDevice = id
	}
	ensureFleetProtocol()
	runner := RunGUI
	if wantCLI {
		runner = technicianCLI
	}
	if err := runner(); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur fatale: %s\n", err)
		os.Exit(1)
	}
}
