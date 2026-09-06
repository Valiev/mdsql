package mdsql

import (
	_ "embed"
	"strings"
)

// embeddedVersion is the release version stored in the repository root.
//
//go:embed VERSION
var embeddedVersion string

// Version returns the current mdsql release version.
func Version() string {
	return strings.TrimSpace(embeddedVersion)
}
