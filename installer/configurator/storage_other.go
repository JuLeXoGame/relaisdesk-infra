//go:build !windows

package main

// Without an integrated OS credential vault, remember only public identifiers.
// Never silently fall back to a plaintext password or licence key.
func protectCredentials(credentials SavedCredentials) (credentialEnvelope, error) {
	return credentialEnvelope{Version: 2, Email: credentials.Email, LicenseID: credentials.LicenseID}, nil
}

func unprotectCredentials(envelope credentialEnvelope) (SavedCredentials, error) {
	return SavedCredentials{Email: envelope.Email, LicenseID: envelope.LicenseID}, nil
}
