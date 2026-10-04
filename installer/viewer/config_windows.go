//go:build windows

package main

import "os"

// Injected by installer/build.ps1 after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var RUSTDESK_EXPECTED_SHA256 = "068898d94547f86a3322835f2580b9516e272ae3040f9b862310f49099665525"

// The installed service can differ from its portable wrapper.
var RUSTDESK_SERVICE_EXPECTED_SHA256 = "be393ac8d8c8000787c564f7ddf7cd27b12e3dce3f94e210f266c76894bf5f5e"

var (
	APPDATA = os.Getenv("APPDATA")
	TEMP    = os.Getenv("TEMP")

	RUSTDESK_CONFIG_DIR   = APPDATA + `\RustDesk\config`
	RUSTDESK_CONFIG_FILE  = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE = `RustDesk2.toml`

	DESKTOP_SHORTCUT_NAME = PRODUCT_NAME + ".lnk"

	RUSTDESK_INSTALLER_TEMP = TEMP + `\rustdesk_viewer_installer.exe`
)
