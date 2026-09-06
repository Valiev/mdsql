package cache

import "os"

func owned(os.FileInfo) bool { return false }

func openEntry(path string) (*os.File, error) { return os.Open(path) }
