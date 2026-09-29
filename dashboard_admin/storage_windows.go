//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protectAdminSession(session AdminStoredSession) (adminSessionEnvelope, error) {
	raw, err := json.Marshal(session)
	if err != nil {
		return adminSessionEnvelope{}, err
	}
	defer func() { clear(raw) }()
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err = windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return adminSessionEnvelope{}, err
	}
	runtime.KeepAlive(raw)
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	encrypted := append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
	return adminSessionEnvelope{Version: 2, Protected: encrypted}, nil
}

func unprotectAdminSession(envelope adminSessionEnvelope) (AdminStoredSession, error) {
	if len(envelope.Protected) == 0 {
		return AdminStoredSession{}, errors.New("session chiffrée absente")
	}
	in := windows.DataBlob{Size: uint32(len(envelope.Protected)), Data: &envelope.Protected[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return AdminStoredSession{}, err
	}
	runtime.KeepAlive(envelope.Protected)
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	raw := unsafe.Slice(out.Data, int(out.Size))
	defer clear(raw)
	var session AdminStoredSession
	err := json.Unmarshal(raw, &session)
	return session, err
}
