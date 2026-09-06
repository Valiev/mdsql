package source

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// NormalizePath returns the NFC spelling used to compare and display paths.
// Keep the original physical path when opening a file or accessing its cache.
func NormalizePath(name string) string {
	return norm.NFC.String(name)
}

// ResolvePath finds the physical spelling of Unicode-equivalent path components.
// Exact spellings win when a filesystem contains both NFC and NFD names.
func ResolvePath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	volume := filepath.VolumeName(absolute) + string(filepath.Separator)
	relative := strings.TrimPrefix(absolute, volume)
	resolved, err := resolveUnicodePath(os.DirFS(volume), filepath.ToSlash(relative))
	if err != nil {
		return "", err
	}
	return filepath.Join(volume, filepath.FromSlash(resolved)), nil
}

func resolveUnicodePath(fsys fs.FS, name string) (string, error) {
	resolved := "."
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." {
			continue
		}
		if hasNonASCII(component) {
			entries, err := fs.ReadDir(fsys, resolved)
			if err != nil {
				return "", err
			}
			canonical := NormalizePath(component)
			match := ""
			for _, entry := range entries {
				if entry.Name() == component {
					match = component
					break
				}
				if match == "" && NormalizePath(entry.Name()) == canonical {
					match = entry.Name()
				}
			}
			if match != "" {
				component = match
			}
		}
		resolved = path.Join(resolved, component)
	}
	if _, err := fs.Stat(fsys, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return true
		}
	}
	return false
}
