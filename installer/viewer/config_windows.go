//go:build windows

package main

import "os"

// Injected by installer/build.ps1 after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var RUSTDESK_EXPECTED_SHA256 = "6e13fd769c0eb77ae899f6e96a4d112249aa42e3e285494ac6030d0d4b9900eb"

// The installed service can differ from its portable wrapper.
var RUSTDESK_SERVICE_EXPECTED_SHA256 = "ef73d755800f2a20ccb811c1ba226723012f2aa5fac0c43971c301b1632db170"

var (
	APPDATA = os.Getenv("APPDATA")
	TEMP    = os.Getenv("TEMP")

	RUSTDESK_CONFIG_DIR   = APPDATA + `\RustDesk\config`
	RUSTDESK_CONFIG_FILE  = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE = `RustDesk2.toml`

	DESKTOP_SHORTCUT_NAME = PRODUCT_NAME + ".lnk"

	RUSTDESK_INSTALLER_TEMP = TEMP + `\rustdesk_viewer_installer.exe`
)
