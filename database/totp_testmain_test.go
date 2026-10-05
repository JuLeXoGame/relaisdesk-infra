package database

import (
	"os"
	"testing"
)

// TOTP writes fail closed without a data key, so provision a fixed one for
// the whole package. Tests covering the keyless behavior unset it explicitly.
func TestMain(m *testing.M) {
	SetTOTPDataKey([]byte("0123456789abcdef0123456789abcdef"))
	os.Exit(m.Run())
}
