//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed embedded/fleet/*
var fleetPayload embed.FS

func copyFileAtomically(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "deploy-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func verifiedNativePayload() (map[string][]byte, error) {
	entries, err := fleetPayload.ReadDir("embedded/fleet")
	if err != nil {
		return nil, err
	}
	payload := make(map[string][]byte)
	for _, entry := range entries {
		name := entry.Name()
		if name == "README.txt" {
			continue
		}
		if entry.IsDir() || (name != "rustdesk.exe" && !strings.HasSuffix(name, ".dll")) {
			return nil, errors.New("composant natif inattendu")
		}
		data, e := fleetPayload.ReadFile("embedded/fleet/" + name)
		if e != nil {
			return nil, e
		}
		payload[name] = data
	}
	if len(payload["rustdesk.exe"]) == 0 || len(payload["sciter.dll"]) == 0 || len(payload["dylib_virtual_display.dll"]) == 0 {
		return nil, errors.New("composants natifs de l'accès permanent absents : reconstruisez le Viewer avec le paquet natif vérifié")
	}
	sum := sha256.Sum256(payload["rustdesk.exe"])
	if len(RUSTDESK_SERVICE_EXPECTED_SHA256) != 64 || !strings.EqualFold(fmt.Sprintf("%x", sum), RUSTDESK_SERVICE_EXPECTED_SHA256) {
		return nil, errors.New("empreinte du service RustDesk intégrée absente ou incorrecte")
	}
	return payload, nil
}

func nativeDirectoryMatches(dir string, payload map[string][]byte) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		// Do not load an unreviewed DLL left by a different installation.
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".exe") {
			if _, ok := payload[name]; !ok {
				return false
			}
		}
	}
	for name, want := range payload {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(want)) {
			return false
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			return false
		}
	}
	return true
}

func installNativePayload(dir string, payload map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".exe") {
			if _, ok := payload[name]; !ok {
				return errors.New("composant natif inconnu présent dans le dossier du service ; installation refusée")
			}
		}
	}
	for name, data := range payload {
		path := filepath.Join(dir, name)
		tmp, err := os.CreateTemp(dir, "verified-*")
		if err != nil {
			return err
		}
		if _, err = tmp.Write(data); err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp.Name(), path)
		}
		if err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
	}
	if !nativeDirectoryMatches(dir, payload) {
		return errors.New("vérification des composants natifs installés impossible")
	}
	return nil
}
