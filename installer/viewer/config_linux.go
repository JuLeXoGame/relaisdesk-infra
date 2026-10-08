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

// Space-separated lib/ payload of the pinned embedded deb, injected at build
// time (-X main.RUSTDESK_BUNDLED_LIBS) like the pins above. The literal is the
// fallback for builds without -X. The fork links media codecs statically
// since the 2026-10-08 nightly, so libvpx/libaom/libyuv/libjpeg are gone; a
// stale entry here breaks enrollment on healthy installs. Keep sorted, update
// together with the pins. Enforced by TestViewerBundledLibManifest.
var RUSTDESK_BUNDLED_LIBS = "libapp.so libdesktop_drop_plugin.so libdesktop_multi_window_plugin.so libfile_selector_linux_plugin.so libflutter_custom_cursor_plugin.so libflutter_linux_gtk.so librustdesk.so libscreen_retriever_plugin.so libtexture_rgba_renderer_plugin.so liburl_launcher_linux_plugin.so libwindow_manager_plugin.so libwindow_size_plugin.so"

var (
	HOME                  = os.Getenv("HOME")
	RUSTDESK_CONFIG_DIR   = HOME + "/.config/rustdesk"
	RUSTDESK_CONFIG_FILE  = "RustDesk.toml"
	RUSTDESK_CONFIG2_FILE = "RustDesk2.toml"

	RUSTDESK_INSTALLER_TEMP = "/tmp/rustdesk_viewer_installer.deb"
)
