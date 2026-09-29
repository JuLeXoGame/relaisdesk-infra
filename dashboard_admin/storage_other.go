//go:build !windows

package main

// Without an integrated OS credential vault, remember only public identifiers.
// Never silently fall back to a plaintext token.
func protectAdminSession(session AdminStoredSession) (adminSessionEnvelope, error) {
	return adminSessionEnvelope{Version: 2, Email: session.Email, LicenseID: session.LicenseID}, nil
}

func unprotectAdminSession(envelope adminSessionEnvelope) (AdminStoredSession, error) {
	return AdminStoredSession{Email: envelope.Email, LicenseID: envelope.LicenseID}, nil
}
