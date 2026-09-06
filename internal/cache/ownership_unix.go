//go:build !windows

package cache

import (
	"os"
	"syscall"
)

func owned(fi os.FileInfo) bool {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func openEntry(path string) (*os.File, error) {
	// Do not follow a replaced entry or block if it became a named pipe.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
