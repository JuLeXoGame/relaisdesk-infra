//go:build windows

package main

import (
	"os/exec"

	"golang.org/x/sys/windows"
)

// selfUpdateSpawnDetached starts a process that survives this one (used to
// relaunch the application or run the pending installer after exit).
func selfUpdateSpawnDetached(path string, args []string) error {
	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	return cmd.Start()
}
