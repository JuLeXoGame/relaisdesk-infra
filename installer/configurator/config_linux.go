//go:build linux

package main

const (
	// RustDesk default installation paths on Linux.
	RUSTDESK_DEFAULT_PATH_X64 = `/usr/bin/rustdesk`
	RUSTDESK_DEFAULT_PATH_X86 = `/usr/bin/rustdesk`
	RUSTDESK_USER_PATH        = `/usr/bin/rustdesk` // Same for linux

	// Config path. On Linux, it's typically ~/.config/rustdesk/RustDesk2.toml
	// We'll resolve the home directory dynamically.
	RUSTDESK_CONFIG_DIR_SUFFIX = `.config/rustdesk`
	RUSTDESK_CONFIG_FILE       = `RustDesk.toml`
	RUSTDESK_CONFIG2_FILE      = `RustDesk2.toml`

	// Installer download path.
	RUSTDESK_INSTALLER_TEMP = `/tmp/rustdesk_installer.deb`
)

// Injected at build time. Empty values make installation and execution fail
// closed, so an upstream RustDesk binary cannot be shipped accidentally.
var (
	RUSTDESK_EXPECTED_SHA256         = "58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52"
	RUSTDESK_PACKAGE_EXPECTED_SHA256 = "190b938b370284e091242e915ecebefdacd23eeba142f227d1decb11bc629b30"
	// The Flutter runner can remain identical across engine updates. Pin the
	// native library too; release builders must supply its exact digest.
	RUSTDESK_SO_EXPECTED_SHA256 = ""
)
