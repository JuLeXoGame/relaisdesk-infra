//go:build windows

package main

const (
	// RustDesk default installation paths.
	RUSTDESK_DEFAULT_PATH_X64 = `C:\Program Files\RustDesk\rustdesk.exe`
	RUSTDESK_DEFAULT_PATH_X86 = `C:\Program Files (x86)\RustDesk\rustdesk.exe`

	// User installation path (if installed without admin rights).
	RUSTDESK_USER_PATH = `${LOCALAPPDATA}\RustDesk\rustdesk.exe`

	// Config path.
	RUSTDESK_CONFIG_DIR   = `${APPDATA}\RustDesk\config`
	RUSTDESK_CONFIG_FILE  = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE = `RustDesk2.toml`

	// Desktop shortcut name.
	DESKTOP_SHORTCUT_NAME = PRODUCT_NAME + ".lnk"

	// Installer download path.
	RUSTDESK_INSTALLER_TEMP = `${TEMP}\rustdesk_installer.exe`
)

// Injected by installer/build.ps1 after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var RUSTDESK_EXPECTED_SHA256 = "b8ff64630042a8f41492d7893163de65f9376b2a2d27d404ebf3812033124ae6"
