//go:build !windows

package main

// Without an integrated OS credential vault, remember only public identifiers
// plus the device token. Passwords and licence keys are still never stored.
// The device token is a bearer designed for client-side storage (it only
// skips 2FA for 30 days); the file stays 0600 inside a 0700 directory, the
// same protection SSH keys rely on.
func protectCredentials(credentials SavedCredentials) (credentialEnvelope, error) {
	return credentialEnvelope{Version: 2, Email: credentials.Email, LicenseID: credentials.LicenseID, DeviceToken: credentials.DeviceToken}, nil
}

func unprotectCredentials(envelope credentialEnvelope) (SavedCredentials, error) {
	return SavedCredentials{Email: envelope.Email, LicenseID: envelope.LicenseID, DeviceToken: envelope.DeviceToken}, nil
}
