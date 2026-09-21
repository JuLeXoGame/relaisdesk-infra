// Package servicelegal fixes the separately accepted optional-service contract.
package servicelegal

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
)

const Version = "2026-09-19-prestations-v1"
const URL = "/prestations/conditions-2026-09-19.html"

//go:embed conditions-2026-09-19.txt
var Document string

func Hash() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(Document))) }
