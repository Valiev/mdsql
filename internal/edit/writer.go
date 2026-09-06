package edit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

func regularInfo(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("write requires an existing regular file (symlinks are not supported)")
	}
	// Nlink is available on Unix and some other platforms, but not in the
	// portable FileInfo interface. Do not guess on platforms without it.
	value := reflect.ValueOf(info.Sys())
	if value.IsValid() && value.Kind() == reflect.Ptr && !value.IsNil() {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		links := value.FieldByName("Nlink")
		if links.IsValid() {
			switch links.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if links.Uint() > 1 {
					return fmt.Errorf("hard-linked files are not supported")
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if links.Int() > 1 {
					return fmt.Errorf("hard-linked files are not supported")
				}
			}
		}
	}
	return nil
}
func sameMetadata(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}
func readOriginal(path string) ([]byte, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if err = regularInfo(before); err != nil {
		return nil, nil, err
	}
	file, err := openSource(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if err = regularInfo(opened); err != nil {
		return nil, nil, err
	}
	if !sameMetadata(before, opened) {
		return nil, nil, fmt.Errorf("file changed while opening")
	}
	// Bound reads to the captured size plus one byte, including conflict checks.
	if before.Size() < 0 || before.Size() == int64(^uint64(0)>>1) {
		return nil, nil, fmt.Errorf("invalid source size")
	}
	data, err := io.ReadAll(io.LimitReader(file, before.Size()+1))
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	named, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if err = regularInfo(named); err != nil {
		return nil, nil, err
	}
	if int64(len(data)) != before.Size() || !sameMetadata(before, after) || !sameMetadata(before, named) {
		return nil, nil, fmt.Errorf("file changed while reading")
	}
	return data, before, nil
}
func checkOriginal(path string, original []byte, info os.FileInfo) error {
	data, current, err := readOriginal(path)
	if err != nil {
		return fmt.Errorf("write conflict: %w", err)
	}
	if !sameMetadata(info, current) || !bytes.Equal(original, data) {
		return fmt.Errorf("write conflict: source metadata or content changed")
	}
	return nil
}

type writerHooks struct {
	beforeCheck   func()
	syncDirectory func(*os.File) error
}

// atomicWrite serializes cooperating mdsql writers. Its final comparison and
// rename are not a compare-and-swap against an unrelated external editor.
// Once renamed, applied remains true even if directory synchronization fails.
func atomicWrite(path string, original, after []byte, info os.FileInfo, hooks writerHooks) (applied bool, err error) {
	if !atomicWritesSupported {
		return false, fmt.Errorf("atomic writes are unsupported on this platform; use dry-run")
	}
	lockPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".mdsql.lock")
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, fmt.Errorf("acquire mdsql write lock: %w", err)
	}
	defer os.Remove(lockPath)
	if err = lock.Close(); err != nil {
		return false, err
	}
	if err = checkOriginal(path, original, info); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".mdsql-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if err = temp.Chmod(info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)); err != nil {
		return false, err
	}
	n, err := temp.Write(after)
	if err != nil {
		return false, err
	}
	if n != len(after) {
		return false, io.ErrShortWrite
	}
	if err = temp.Sync(); err != nil {
		return false, err
	}
	if err = temp.Close(); err != nil {
		return false, err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer directory.Close()
	if hooks.beforeCheck != nil {
		hooks.beforeCheck()
	}
	if err = checkOriginal(path, original, info); err != nil {
		return false, err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return false, err
	}
	if hooks.syncDirectory != nil {
		err = hooks.syncDirectory(directory)
	} else {
		err = directory.Sync()
	}
	if err != nil {
		return true, fmt.Errorf("write applied but directory sync failed: %w", err)
	}
	return true, nil
}
