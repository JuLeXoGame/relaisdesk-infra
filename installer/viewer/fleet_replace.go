package main

import "strings"

// isFleetReplaceableError reports whether an enrollment failure only means a
// previous permanent enrollment is still present, in which case the caller
// may remove it and retry with the new code.
func isFleetReplaceableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "existe déjà") ||
		strings.Contains(msg, "déjà présente") ||
		strings.Contains(msg, "déjà enregistré")
}
