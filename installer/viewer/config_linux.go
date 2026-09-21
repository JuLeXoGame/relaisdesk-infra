//go:build linux

package main

import "os"

// Injected at build time. Empty values make installation and execution fail
// closed, so an upstream RustDesk binary cannot be shipped accidentally.
var (
	RUSTDESK_EXPECTED_SHA256         = "36421cadea72a2a61b50c1b1bda247541c20163f3d501013eb2521cc6c56abff"
	RUSTDESK_PACKAGE_EXPECTED_SHA256 = "b40be3e770028e83623a4ad97d879eb8080205c52ce00a7091cc76c4741bdd91"
	RUSTDESK_SO_EXPECTED_SHA256      = "2c331c9fe6e99a14294dabc1a628b697294ca32f40f36eb1d92247c5087b9087"
)

var (
	HOME                  = os.Getenv("HOME")
	RUSTDESK_CONFIG_DIR   = HOME + "/.config/rustdesk"
	RUSTDESK_CONFIG_FILE  = "RustDesk.toml"
	RUSTDESK_CONFIG2_FILE = "RustDesk2.toml"

	RUSTDESK_INSTALLER_TEMP = "/tmp/rustdesk_viewer_installer.deb"
)
