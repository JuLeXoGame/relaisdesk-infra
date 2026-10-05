package handlers

import (
	"os"
	"testing"

	dbpkg "database"
)

// TOTP writes fail closed without a data key, so provision a fixed one for
// the whole package (covers the external handlers_test files too).
func TestMain(m *testing.M) {
	dbpkg.SetTOTPDataKey([]byte("0123456789abcdef0123456789abcdef"))
	os.Exit(m.Run())
}
