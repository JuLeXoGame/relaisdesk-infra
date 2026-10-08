//go:build linux

package main

import "os"

// Injected at build time. Empty values make installation and execution fail
// closed, so an upstream RustDesk binary cannot be shipped accidentally.
var (
	RUSTDESK_EXPECTED_SHA256         = "58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52"
	RUSTDESK_PACKAGE_EXPECTED_SHA256 = "7ba39cc325f55ff0793b956ff23a6876a75d8682ab8126a9ce0451fe81bde2cf"
	RUSTDESK_SO_EXPECTED_SHA256      = "851a44b2ac449f45ce61acf89d96253c496823202883ff50049c269bdcd93ff9"
)

var (
	HOME                  = os.Getenv("HOME")
	RUSTDESK_CONFIG_DIR   = HOME + "/.config/rustdesk"
	RUSTDESK_CONFIG_FILE  = "RustDesk.toml"
	RUSTDESK_CONFIG2_FILE = "RustDesk2.toml"

	RUSTDESK_INSTALLER_TEMP = "/tmp/rustdesk_viewer_installer.deb"
)
