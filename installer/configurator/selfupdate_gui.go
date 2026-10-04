package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// startSelfUpdateAtStartup cleans stale files then checks for updates in the
// background, prompting the user when a newer verified version exists.
func startSelfUpdateAtStartup(win fyne.Window) {
	if exe, err := os.Executable(); err == nil {
		SelfUpdateCleanupPending(exe)
	}
	go func() {
		time.Sleep(3 * time.Second)
		if !SelfUpdateShouldCheck() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := SelfUpdateCheck(ctx, APIURL, RELEASE_PUBLIC_KEY, RELEASE_KEY_ID, APP_VERSION)
		if err != nil || info == nil || !info.Available {
			return // offline, unsupported install or up to date: stay silent
		}
		if selfUpdateRecentlyFailed(info.LatestVersion) {
			return
		}
		fyne.Do(func() { promptSelfUpdate(win, info) })
	}()
}

func promptSelfUpdate(win fyne.Window, info *SelfUpdateInfo) {
	msg := widget.NewLabel(TF("su_available", info.LatestVersion))
	msg.Wrapping = fyne.TextWrapWord
	dialog.NewCustomConfirm(T("su_title"), T("su_install_restart"), T("su_later"), msg, func(yes bool) {
		if !yes {
			SelfUpdateDismissForSession()
			return
		}
		runSelfUpdate(win, info)
	}, win).Show()
}

func runSelfUpdate(win fyne.Window, info *SelfUpdateInfo) {
	progress := dialog.NewProgressInfinite(T("su_title"), T("su_working"), win)
	progress.Show()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		path, err := SelfUpdateDownload(ctx, info)
		if err != nil {
			selfUpdateNoteFailure(info.LatestVersion)
			fyne.Do(func() {
				progress.Hide()
				dialog.ShowError(errors.New(TF("su_failed", err)), win)
			})
			return
		}
		if err := SelfUpdateApply(info, path); err != nil {
			if !errors.Is(err, ErrSelfUpdateAssisted) {
				_ = os.Remove(path)
				selfUpdateNoteFailure(info.LatestVersion)
			}
			fyne.Do(func() {
				progress.Hide()
				if errors.Is(err, ErrSelfUpdateAssisted) {
					dialog.ShowInformation(T("su_title"), err.Error(), win)
				} else {
					dialog.ShowError(errors.New(TF("su_failed", err)), win)
				}
			})
			return
		}
		// Successful applies restart the process and never return.
		_ = os.Remove(path)
	}()
}

// printSelfUpdateNoticeCLI reports an available update in headless mode.
// It never downloads or installs: the operator handles the machine update.
func printSelfUpdateNoticeCLI() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	info, err := SelfUpdateCheck(ctx, APIURL, RELEASE_PUBLIC_KEY, RELEASE_KEY_ID, APP_VERSION)
	if err != nil || info == nil || !info.Available {
		return
	}
	fmt.Printf("%s\n", TF("su_available", info.LatestVersion))
}
