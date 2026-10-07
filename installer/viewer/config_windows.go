//go:build windows

package main

import "os"

// Injected by installer/build.ps1 after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var RUSTDESK_EXPECTED_SHA256 = "86d2b9069f1f6b7cedcbad4acd260b0091725a7c52643badfb77d12e0be69893"

// The installed service can differ from its portable wrapper.
var RUSTDESK_SERVICE_EXPECTED_SHA256 = "82260953b6542bd5e68701ce62c12a923e703c7e449e50a6b408a8e89c6ea13d"

var (
	APPDATA = os.Getenv("APPDATA")
	TEMP    = os.Getenv("TEMP")

	RUSTDESK_CONFIG_DIR   = APPDATA + `\RustDesk\config`
	RUSTDESK_CONFIG_FILE  = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE = `RustDesk2.toml`

	DESKTOP_SHORTCUT_NAME = PRODUCT_NAME + ".lnk"

	RUSTDESK_INSTALLER_TEMP = TEMP + `\rustdesk_viewer_installer.exe`
)
