//go:build linux

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runSelfUpdateAtStartupLinux performs a quick blocking check before the code
// prompt (the Linux viewer has a linear console/zenity flow, no event loop to
// hook a background check into). Offline machines only wait for the timeout.
func runSelfUpdateAtStartupLinux() {
	if exe, err := os.Executable(); err == nil {
		SelfUpdateCleanupPending(exe)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := SelfUpdateCheck(ctx, APIURL, ReleaseSigningPublicKey, APP_VERSION)
	if err != nil || info == nil || !info.Available {
		return
	}
	if !askLinuxUpdate(info.LatestVersion) {
		return
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel2()
	fmt.Println(T("su_working"))
	path, err := SelfUpdateDownload(ctx2, info)
	if err != nil {
		showLinuxDialog(T("su_title"), TF("su_failed", err), true)
		return
	}
	if err := SelfUpdateApply(info, path); err != nil {
		if errors.Is(err, ErrSelfUpdateAssisted) {
			showLinuxDialog(T("su_title"), err.Error(), false)
			return
		}
		_ = os.Remove(path)
		showLinuxDialog(T("su_title"), TF("su_failed", err), true)
		return
	}
	// Successful applies restart the process and never return.
	_ = os.Remove(path)
}

func askLinuxUpdate(version string) bool {
	msg := TF("su_available", version)
	if isDisplayAvailable() {
		if zenityPath, err := exec.LookPath("zenity"); err == nil {
			err := exec.Command(zenityPath, "--question", "--title="+T("su_title"), "--text="+msg, "--width=400").Run()
			return err == nil
		}
		if kdialogPath, err := exec.LookPath("kdialog"); err == nil {
			err := exec.Command(kdialogPath, "--yesno", msg, "--title", T("su_title")).Run()
			return err == nil
		}
	}
	fmt.Printf("%s [%s/%s] ", msg, T("su_install_restart"), T("su_later"))
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "o" || answer == "oui" || answer == "y" || answer == "yes"
}
