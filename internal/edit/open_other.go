//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package edit

import "os"

func openSource(path string) (*os.File, error) { return os.Open(path) }

const atomicWritesSupported = false
