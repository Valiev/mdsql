//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package edit

import (
	"os"
	"syscall"
)

func openSource(path string) (*os.File, error) {
	// O_NONBLOCK avoids hanging if another process replaces the file with a FIFO;
	// O_NOFOLLOW refuses a final-component symlink even during the open race.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

const atomicWritesSupported = true
