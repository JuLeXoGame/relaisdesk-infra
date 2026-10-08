//go:build linux

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression: setFleetPermanentPassword spawns /usr/bin/rustdesk, which
// initializes GTK even for --password-file. The elevated child must hand it a
// display environment or enrollment fails with "cannot open display".

func listenUnixSocket(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen %s: %v", path, err)
	}
	t.Cleanup(func() { _ = l.Close() })
}

func staleUnixSocket(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen %s: %v", path, err)
	}
	// Closing without removing leaves the socket file behind, like a crashed
	// X server does. The dial probe must reject it.
	_ = l.Close()
}

func writeTempFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("cookie"), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func envHas(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func TestLinuxForwardDisplayEnvKeepsSessionOnly(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("XAUTHORITY", "/home/u/.Xauthority")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/run/user/1000/bus")
	t.Setenv("EMPTY_SHOULD_DROP", "")
	t.Setenv("SECRET_SHOULD_NOT_FORWARD", "x")
	env := linuxForwardDisplayEnv()
	for _, kv := range []string{"DISPLAY=:0", "XAUTHORITY=/home/u/.Xauthority", "WAYLAND_DISPLAY=wayland-0", "XDG_RUNTIME_DIR=/run/user/1000", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus"} {
		if !envHas(env, kv) {
			t.Fatalf("session var perdue %q dans %v", kv, env)
		}
	}
	for _, e := range env {
		if strings.HasPrefix(e, "SECRET_SHOULD_NOT_FORWARD=") || strings.HasPrefix(e, "EMPTY_SHOULD_DROP=") {
			t.Fatalf("variable non-session transmise : %q", e)
		}
	}
}

func TestLinuxGUIChildEnvPrefersForwardedSession(t *testing.T) {
	t.Setenv("DISPLAY", ":9")
	t.Setenv("PKEXEC_UID", "")
	t.Setenv("SUDO_UID", "")
	env := linuxGUIChildEnv()
	if !envHas(env, "DISPLAY=:9") {
		t.Fatalf("session transmise ignorée : %v", env)
	}
}

func TestLinuxGUIChildEnvEmptyWithoutSessionNorUID(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XAUTHORITY", "")
	t.Setenv("PKEXEC_UID", "")
	t.Setenv("SUDO_UID", "")
	if env := linuxGUIChildEnv(); len(env) != 0 {
		t.Fatalf("env inattendu sans session ni uid : %v", env)
	}
}

func TestLinuxInvokingUIDValidation(t *testing.T) {
	cases := []struct {
		name     string
		pkexec   string
		sudo     string
		expected string
	}{
		{"pkexec", "1234", "", "1234"},
		{"sudo", "", "1000", "1000"},
		{"pkexec gagne", "42", "1000", "42"},
		{"pkexec invalide repli sudo", "abc", "1000", "1000"},
		{"sudo negatif", "", "-1", ""},
		{"vide", "", "", ""},
		{"alphanumerique", "12a", "", ""},
		{"trop long", "12345678901", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("PKEXEC_UID", c.pkexec)
			t.Setenv("SUDO_UID", c.sudo)
			if got := linuxInvokingUID(); got != c.expected {
				t.Fatalf("uid=%q, attendu %q", got, c.expected)
			}
		})
	}
}

func TestLinuxResolveDisplayEnvForX11(t *testing.T) {
	x11 := t.TempDir()
	staleUnixSocket(t, filepath.Join(x11, "X0"))
	listenUnixSocket(t, filepath.Join(x11, "X9"))
	listenUnixSocket(t, filepath.Join(x11, "X10"))
	writeTempFile(t, filepath.Join(x11, "X2")) // fichier régulier, pas un socket
	writeTempFile(t, filepath.Join(x11, "Xfoo"))
	home := t.TempDir()
	writeTempFile(t, filepath.Join(home, ".Xauthority"))
	runBase := t.TempDir() // sans sous-dossier uid : pas de vars runtime

	env := linuxResolveDisplayEnvFor("1000", home, x11, runBase)
	if !envHas(env, "DISPLAY=:9") {
		t.Fatalf("DISPLAY=:9 attendu (min numérique vivant), obtenu %v", env)
	}
	if !envHas(env, "XAUTHORITY="+filepath.Join(home, ".Xauthority")) {
		t.Fatalf("XAUTHORITY attendu, obtenu %v", env)
	}
	if len(env) != 2 {
		t.Fatalf("env inattendu : %v", env)
	}
}

func TestLinuxResolveDisplayEnvForWayland(t *testing.T) {
	x11 := t.TempDir()
	runBase := t.TempDir()
	runDir := filepath.Join(runBase, "1000")
	if err := os.Mkdir(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	listenUnixSocket(t, filepath.Join(runDir, "wayland-0"))
	staleUnixSocket(t, filepath.Join(runDir, "wayland-1"))
	writeTempFile(t, filepath.Join(runDir, "wayland-x"))
	writeTempFile(t, filepath.Join(runDir, "bus"))

	env := linuxResolveDisplayEnvFor("1000", "", x11, runBase)
	if !envHas(env, "WAYLAND_DISPLAY=wayland-0") {
		t.Fatalf("WAYLAND_DISPLAY attendu, obtenu %v", env)
	}
	if !envHas(env, "XDG_RUNTIME_DIR="+runDir) {
		t.Fatalf("XDG_RUNTIME_DIR attendu, obtenu %v", env)
	}
	if !envHas(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(runDir, "bus")) {
		t.Fatalf("bus DBus attendu, obtenu %v", env)
	}
	for _, e := range env {
		if strings.HasPrefix(e, "DISPLAY=") {
			t.Fatalf("DISPLAY inattendu sans X11 : %v", env)
		}
	}
}

func TestLinuxResolveDisplayEnvForRequiresUID(t *testing.T) {
	x11 := t.TempDir()
	listenUnixSocket(t, filepath.Join(x11, "X0"))
	if env := linuxResolveDisplayEnvFor("", t.TempDir(), x11, t.TempDir()); env != nil {
		t.Fatalf("résolution sans uid : %v", env)
	}
}

func TestLinuxResolveDisplayEnvForIgnoresJunk(t *testing.T) {
	x11 := t.TempDir()
	staleUnixSocket(t, filepath.Join(x11, "X0"))
	for _, name := range []string{"X", "X-1", ".X11-lock"} {
		writeTempFile(t, filepath.Join(x11, name))
	}
	if err := os.Mkdir(filepath.Join(x11, "X5"), 0755); err != nil {
		t.Fatal(err)
	}
	runBase := t.TempDir()
	runDir := filepath.Join(runBase, "1000")
	if err := os.Mkdir(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeTempFile(t, filepath.Join(runDir, "wayland-x"))
	if env := linuxResolveDisplayEnvFor("1000", t.TempDir(), x11, runBase); env != nil {
		t.Fatalf("déchets acceptés : %v", env)
	}
}

func TestLinuxDisplayEnvHasDisplay(t *testing.T) {
	if !linuxDisplayEnvHasDisplay([]string{"DISPLAY=:0"}) {
		t.Fatal("DISPLAY non détecté")
	}
	if !linuxDisplayEnvHasDisplay([]string{"WAYLAND_DISPLAY=wayland-0"}) {
		t.Fatal("WAYLAND_DISPLAY non détecté")
	}
	if linuxDisplayEnvHasDisplay(nil) || linuxDisplayEnvHasDisplay([]string{"XAUTHORITY=/x", "XDG_RUNTIME_DIR=/r"}) {
		t.Fatal("faux positif sans display")
	}
}
