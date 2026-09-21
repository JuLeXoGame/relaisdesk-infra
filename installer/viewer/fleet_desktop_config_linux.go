//go:build linux

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Resolve every directory without following links, keeping a descriptor rather
// than reopening a user-controlled pathname after checking it.
func openDesktopDirectory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, errors.New("chemin de bureau invalide")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}

func writeDesktopFleetConfig(homePath string, data []byte) error {
	fd, err := openDesktopDirectory(homePath)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var owner unix.Stat_t
	if err := unix.Fstat(fd, &owner); err != nil {
		return err
	}
	if owner.Uid == 0 {
		return errors.New("dossier personnel administrateur ignoré")
	}
	return writeDesktopFleetConfigAt(fd, owner.Uid, owner.Gid, data)
}

func writeDesktopFleetConfigAt(homeFD int, uid, gid uint32, data []byte) error {
	fd, err := unix.Dup(homeFD)
	if err != nil {
		return err
	}
	for _, part := range []string{".config", "rustdesk"} {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			return err
		}
		fd = next
		var info unix.Stat_t
		if err := unix.Fstat(fd, &info); err != nil {
			unix.Close(fd)
			return err
		}
		if info.Uid != uid {
			unix.Close(fd)
			return errors.New("dossier de configuration appartenant à un autre utilisateur")
		}
	}
	defer unix.Close(fd)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := ".relaisdesk-config-" + hex.EncodeToString(nonce[:])
	tmpFD, err := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(fd, name, 0)
	file := os.NewFile(uintptr(tmpFD), name)
	defer file.Close()
	if err := file.Chown(int(uid), int(gid)); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	// Replacing the directory entry never follows a destination symlink or
	// truncates a hard-linked file outside the user's configuration directory.
	return unix.Renameat(fd, name, fd, "RustDesk2.toml")
}
