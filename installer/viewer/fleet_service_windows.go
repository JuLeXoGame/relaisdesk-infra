//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func fleetEngineExecutable() (string, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	return filepath.Join(root, "RelaisDeskEngine", "rustdesk.exe"), err
}

// Ownership is path-based, separate from payload integrity: an old version of
// our engine may be upgraded, but a different RustDesk installation is untouched.
func fleetServiceCommandMatches(command, target string) bool {
	args, err := windows.DecomposeCommandLine(command)
	return err == nil && len(args) == 2 && args[1] == "--service" &&
		filepath.IsAbs(args[0]) && strings.EqualFold(filepath.Clean(args[0]), filepath.Clean(target))
}

func checkFleetServiceCommand(command string) error {
	target, err := fleetEngineExecutable()
	if err != nil {
		return err
	}
	if !fleetServiceCommandMatches(command, target) {
		return errors.New("un autre service RustDesk est installé ; il ne sera ni remplacé ni supprimé automatiquement")
	}
	return nil
}

// terminateFleetAgentProcess stops an orphaned authorization agent (a running
// viewer-agent.exe without its service), which would otherwise lock the state
// files during cleanup. Only our own copy inside dir is ever terminated.
func terminateFleetAgentProcess(dir string) error {
	target := filepath.Join(dir, "viewer-agent.exe")
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "viewer-agent.exe") {
			continue
		}
		handle, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if e != nil {
			continue // No guessing when the process identity cannot be checked.
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		e = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		if e == nil && strings.EqualFold(windows.UTF16ToString(buffer[:size]), target) {
			e = windows.TerminateProcess(handle, 1)
			windows.CloseHandle(handle)
			if e != nil {
				return e
			}
		} else {
			windows.CloseHandle(handle)
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}

// Never terminate every process called rustdesk.exe during elevated operations.
func terminateFleetEngineProcesses() error {
	target, err := fleetEngineExecutable()
	if err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "rustdesk.exe") {
			continue
		}
		handle, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if e != nil {
			continue // No guessing when the process identity cannot be checked.
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		e = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		if e == nil && strings.EqualFold(windows.UTF16ToString(buffer[:size]), target) {
			e = windows.TerminateProcess(handle, 1)
			windows.CloseHandle(handle)
			if e != nil {
				return e
			}
		} else {
			windows.CloseHandle(handle)
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	return nil
}
