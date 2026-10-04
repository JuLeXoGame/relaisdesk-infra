//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// selfUpdateSpawnDetached starts a process that survives this one (used to
// relaunch the application after a self-replace).
func selfUpdateSpawnDetached(path string, args []string) error {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd := exec.Command(path, args...)
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
