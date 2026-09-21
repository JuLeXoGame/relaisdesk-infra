//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDesktopConfigRejectsDirectoryLinks(t *testing.T) {
	home, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".config")); err != nil {
		t.Fatal(err)
	}
	fd, err := openDesktopDirectory(home)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := writeDesktopFleetConfigAt(fd, uint32(os.Getuid()), uint32(os.Getgid()), []byte("config")); err == nil {
		t.Fatal("followed directory symlink")
	}
	if fd, err := openDesktopDirectory(filepath.Join(home, ".config")); err == nil {
		unix.Close(fd)
		t.Fatal("followed ancestor symlink")
	}
}

func TestDesktopConfigDoesNotTruncateLinkTarget(t *testing.T) {
	for _, hardLink := range []bool{false, true} {
		home, outside := t.TempDir(), filepath.Join(t.TempDir(), "protected")
		cfgDir := filepath.Join(home, ".config", "rustdesk")
		if err := os.MkdirAll(cfgDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(cfgDir, "RustDesk2.toml")
		link := os.Symlink
		if hardLink {
			link = os.Link
		}
		if err := link(outside, target); err != nil {
			t.Fatal(err)
		}
		fd, err := openDesktopDirectory(home)
		if err != nil {
			t.Fatal(err)
		}
		err = writeDesktopFleetConfigAt(fd, uint32(os.Getuid()), uint32(os.Getgid()), []byte("new config"))
		unix.Close(fd)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(outside)
		if err != nil || string(got) != "unchanged" {
			t.Fatalf("link target modified: %q %v", got, err)
		}
		got, err = os.ReadFile(target)
		if err != nil || string(got) != "new config" {
			t.Fatalf("configuration not updated: %q %v", got, err)
		}
	}
}
