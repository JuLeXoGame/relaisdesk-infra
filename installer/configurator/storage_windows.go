//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

// DPAPI is bound to the current Windows user, never the whole machine.
func protectCredentials(credentials SavedCredentials) (credentialEnvelope, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return credentialEnvelope{}, err
	}
	defer func() { clear(raw) }()
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return credentialEnvelope{}, err
	}
	runtime.KeepAlive(raw)
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	encrypted := append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
	return credentialEnvelope{Version: 2, Protected: encrypted}, nil
}

func unprotectCredentials(envelope credentialEnvelope) (SavedCredentials, error) {
	if len(envelope.Protected) == 0 {
		return SavedCredentials{}, errors.New("identifiants chiffrés absents")
	}
	in := windows.DataBlob{Size: uint32(len(envelope.Protected)), Data: &envelope.Protected[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return SavedCredentials{}, err
	}
	runtime.KeepAlive(envelope.Protected)
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	raw := unsafe.Slice(out.Data, int(out.Size))
	defer clear(raw)
	var credentials SavedCredentials
	err := json.Unmarshal(raw, &credentials)
	return credentials, err
}
