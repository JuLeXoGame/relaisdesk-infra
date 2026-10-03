//go:build windows

package main

import "os"

// Injected by installer/build.ps1 after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var RUSTDESK_EXPECTED_SHA256 = "26ef612657f0edd0729275a341457cb96f2105eea4b4c5871244641fd0e35b7f"

// The installed service can differ from its portable wrapper.
var RUSTDESK_SERVICE_EXPECTED_SHA256 = "260cb7c32e7b929c88262c0460de94ae86dfaf0123ff03337707a17b7e4a0251"

var (
	APPDATA = os.Getenv("APPDATA")
	TEMP    = os.Getenv("TEMP")

	RUSTDESK_CONFIG_DIR   = APPDATA + `\RustDesk\config`
	RUSTDESK_CONFIG_FILE  = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE = `RustDesk2.toml`

	DESKTOP_SHORTCUT_NAME = PRODUCT_NAME + ".lnk"

	RUSTDESK_INSTALLER_TEMP = TEMP + `\rustdesk_viewer_installer.exe`
)
