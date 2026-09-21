//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

var (
	HOME                        = os.Getenv("HOME")
	RUSTDESK_CONFIG_DIR_SUFFIX  = filepath.Join("Library", "Preferences", "com.carriez.RustDesk")
	RUSTDESK_CONFIG_FILE        = "RustDesk.toml"
	RUSTDESK_CONFIG2_FILE       = "RustDesk2.toml"

	RUSTDESK_DEFAULT_APP_PATH   = "/Applications/RelaisDesk.app/Contents/MacOS/RelaisDesk"
	RUSTDESK_LEGACY_APP_PATH    = "/Applications/RustDesk.app/Contents/MacOS/RustDesk"
)

// Injected by build script after verifying the RelaisDesk client fork.
var RUSTDESK_EXPECTED_SHA256 string
var RUSTDESK_PACKAGE_EXPECTED_SHA256 string
