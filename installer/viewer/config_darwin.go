//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

// Injected by build script after verifying the RelaisDesk client fork.
// An empty value deliberately prevents the launcher from executing any binary.
var (
	RUSTDESK_EXPECTED_SHA256         string
	RUSTDESK_PACKAGE_EXPECTED_SHA256 string
)

var (
	HOME                  = os.Getenv("HOME")
	RUSTDESK_CONFIG_DIR   = filepath.Join(HOME, "Library", "Preferences", "com.carriez.RustDesk")
	RUSTDESK_CONFIG_FILE  = "RustDesk.toml"
	RUSTDESK_CONFIG2_FILE = "RustDesk2.toml"

	RUSTDESK_DEFAULT_APP_PATH = "/Applications/RelaisDesk.app/Contents/MacOS/RelaisDesk"
	RUSTDESK_LEGACY_APP_PATH  = "/Applications/RustDesk.app/Contents/MacOS/RustDesk"
)
