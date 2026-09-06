package source

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolveSpecs resolves specs into sorted absolute logical paths and detects stdin.
// Errors describe skipped sources; successfully resolved files are still returned.
func ResolveSpecs(specs []string, root string, include, exclude []string) ([]string, bool, error) {
	base := root
	if base == "" {
		base = "."
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		baseAbs = base
	}

	if len(specs) == 0 {
		return []string{}, true, nil
	}

	paths := make([]string, 0)
	useStdin := false
	var warnings []error

	for _, raw := range specs {
		if raw == "-" {
			useStdin = true
			continue
		}

		target := raw
		if !filepath.IsAbs(target) {
			target = filepath.Join(baseAbs, target)
		}
		target = filepath.Clean(target)

		var matches []string
		physical, literalErr := ResolvePath(target)
		if literalErr == nil {
			// Existing literal names win, including glob metacharacters and
			// Unicode-equivalent spellings. Patterns are only a fallback.
			matches = []string{physical}
		} else if isGlob(raw) {
			globBase, pattern := doublestar.SplitPattern(filepath.ToSlash(target))
			physicalBase, err := ResolvePath(filepath.FromSlash(globBase))
			if err == nil {
				matches, err = globFiles(physicalBase, pattern)
			}
			if err != nil {
				warnings = append(warnings, err)
			}
		} else {
			warnings = append(warnings, literalErr)
		}
		for _, target := range matches {
			info, err := os.Stat(target)
			if err != nil {
				warnings = append(warnings, err)
				continue
			}
			if info.IsDir() {
				walked, err := walkDirAt(target, ".")
				paths = append(paths, walked...)
				if err != nil {
					warnings = append(warnings, err)
				}
			} else if info.Mode().IsRegular() {
				paths = append(paths, target)
			}
		}
	}

	filtered := make([]string, 0, len(paths))
	for _, p := range paths {
		isMd := isMarkdown(p)
		if !isMd {
			continue
		}
		rel := toRelative(NormalizePath(p), NormalizePath(baseAbs))
		if !matchAny(rel, include, true) {
			continue
		}
		if matchAny(rel, exclude, false) {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		filtered = append(filtered, abs)
	}

	sort.Strings(filtered)
	uniqueWarnings := make([]error, 0, len(warnings))
	seenWarnings := make(map[string]bool)
	for _, warning := range warnings {
		if !seenWarnings[warning.Error()] {
			seenWarnings[warning.Error()] = true
			uniqueWarnings = append(uniqueWarnings, warning)
		}
	}
	return uniqueStrings(filtered), useStdin, errors.Join(uniqueWarnings...)
}

func ReadFileOrStdin(path string) ([]byte, bool, error) {
	if path == "-" {
		b, err := io.ReadAll(os.Stdin)
		return b, true, err
	}
	b, err := os.ReadFile(path)
	return b, false, err
}

// sourceFS follows links without changing their logical names. It only rejects
// directory targets already present in that path's ancestors; separate aliases
// to the same target remain distinct sources.
type sourceFS struct {
	fs.FS
	root     string
	warnings []error
}

func (s *sourceFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var ancestors []fs.FileInfo
	for parent := name; ; parent = path.Dir(parent) {
		info, err := fs.Stat(s.FS, parent)
		if err != nil {
			s.warnings = append(s.warnings, fmt.Errorf("%s: %w", filepath.Join(s.root, filepath.FromSlash(parent)), err))
			return nil, nil
		}
		for _, ancestor := range ancestors {
			if os.SameFile(info, ancestor) {
				s.warnCycle(name)
				return nil, nil
			}
		}
		ancestors = append(ancestors, info)
		if parent == "." {
			break
		}
	}
	entries, err := fs.ReadDir(s.FS, name)
	if err != nil {
		s.warnings = append(s.warnings, fmt.Errorf("%s: %w", filepath.Join(s.root, filepath.FromSlash(name)), err))
		return nil, nil
	}
	filtered := entries[:0]
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			child := path.Join(name, entry.Name())
			info, err := fs.Stat(s.FS, child)
			if err != nil {
				s.warnings = append(s.warnings, fmt.Errorf("%s: %w", filepath.Join(s.root, filepath.FromSlash(child)), err))
				continue
			}
			cycle := false
			if info.IsDir() {
				for _, ancestor := range ancestors {
					if os.SameFile(info, ancestor) {
						s.warnCycle(child)
						cycle = true
						break
					}
				}
			}
			if cycle {
				continue
			}
			entry = fs.FileInfoToDirEntry(info)
		}
		filtered = append(filtered, entry)
	}
	return filtered, nil
}

func (s *sourceFS) warnCycle(name string) {
	s.warnings = append(s.warnings, fmt.Errorf("symlink cycle skipped: %s", filepath.Join(s.root, filepath.FromSlash(name))))
}

func walkDirAt(root, start string) ([]string, error) {
	res := make([]string, 0)
	fsys := &sourceFS{FS: os.DirFS(root), root: root}
	err := fs.WalkDir(fsys, start, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && isMarkdown(name) {
			res = append(res, filepath.Join(root, filepath.FromSlash(name)))
		}
		return nil
	})
	return res, errors.Join(append(fsys.warnings, err)...)
}

// globFiles matches normalized names while retaining physical paths. Matching a
// directory includes its descendants, as for a positional directory source.
func globFiles(root, pattern string) ([]string, error) {
	pattern = NormalizePath(pattern)
	if !doublestar.ValidatePattern(pattern) {
		return nil, doublestar.ErrBadPattern
	}
	files := make([]string, 0)
	covered := make(map[string]bool)
	maxDepth := strings.Count(pattern, "/") + 1
	recursive := strings.Contains(pattern, "**")
	fsys := &sourceFS{FS: os.DirFS(root), root: root}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		matched, _ := matchMarkdownGlob(pattern, NormalizePath(name))
		matched = matched || (name != "." && covered[path.Dir(name)])
		if d.IsDir() {
			covered[name] = matched
			if name != "." && !matched && !recursive && strings.Count(name, "/")+1 >= maxDepth {
				return fs.SkipDir
			}
		} else if matched && d.Type().IsRegular() && isMarkdown(name) {
			files = append(files, filepath.Join(root, filepath.FromSlash(name)))
		}
		return nil
	})
	return files, errors.Join(append(fsys.warnings, err)...)
}

func toRelative(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func isGlob(s string) bool {
	return strings.Contains(s, "*") || strings.Contains(s, "?") || strings.Contains(s, "[")
}

func isMarkdown(path string) bool {
	low := strings.ToLower(path)
	return strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".markdown")
}

func matchAny(path string, patterns []string, defaultValue bool) bool {
	if len(patterns) == 0 {
		return defaultValue
	}
	for _, p := range patterns {
		ok, err := matchMarkdownGlob(NormalizePath(p), NormalizePath(path))
		if err == nil && ok {
			return true
		}
	}
	return false
}

// Markdown extensions are case-insensitive even when a glob spells the
// extension in one particular case. The rest of the path keeps the platform's
// ordinary case-sensitive matching semantics.
func matchMarkdownGlob(pattern, name string) (bool, error) {
	patternExt := path.Ext(pattern)
	nameExt := path.Ext(name)
	if (strings.EqualFold(patternExt, ".md") || strings.EqualFold(patternExt, ".markdown")) &&
		strings.EqualFold(patternExt, nameExt) {
		pattern = strings.TrimSuffix(pattern, patternExt) + strings.ToLower(patternExt)
		name = strings.TrimSuffix(name, nameExt) + strings.ToLower(nameExt)
	}
	return doublestar.Match(pattern, name)
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	res := make([]string, 0, len(in))
	for _, p := range in {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		res = append(res, p)
	}
	return res
}
