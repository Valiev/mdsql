package mdsql

import "testing"

func TestVersion(t *testing.T) {
	if got, want := Version(), "0.1.0"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
}
