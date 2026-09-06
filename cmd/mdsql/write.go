package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Valiev/mdsql/internal/cli"
	"github.com/Valiev/mdsql/internal/edit"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/source"
)

type writeConfig struct {
	sql    string
	file   string
	root   string
	format string
	opts   edit.Options
}

// Only inspect the command/SQL slot. A SELECT literal, alias argument, option
// value, or later file named INSERT must never dispatch a write.
func writeSQLIndex(args []string) (int, string) {
	index := -1
	waitingQuery := false
	commandSeen := false
	priority := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !commandSeen && isWriteSQL(a) {
			index, commandSeen = i, true
			continue
		}
		if !commandSeen && (strings.HasPrefix(strings.TrimSpace(a), "/*") || strings.HasPrefix(a, "--") && (strings.ContainsAny(a, "\r\n") || strings.HasPrefix(a, "-- ") || strings.HasPrefix(a, "--\t"))) {
			return -1, ""
		}
		if a == "--help" || a == "-h" || a == "--version" || a == "-V" {
			if priority == "" {
				priority = a
			}
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			if writeOptionTakesValue(a) && i+1 < len(args) {
				i++
			}
			continue
		}
		if !commandSeen {
			if !waitingQuery && strings.EqualFold(a, "query") {
				waitingQuery = true
				continue
			}
			// Leave all other commands entirely to the existing CLI.
			return -1, ""
		}
	}
	return index, priority
}

func writeOptionTakesValue(flag string) bool {
	switch flag {
	case "--root", "--format", "--expect-rows", "--expect-sha256",
		"--null", "-f", "-n", "--include", "--exclude", "--today", "--stdin-name",
		"--max-lines", "--max-rows", "--table-limit", "--max-file-size",
		"--under", "-l", "--level", "--min-level", "--max-level", "--state", "--tag",
		"--depth", "--due", "--scheduled-before", "--scheduled-after", "--priority",
		"--path", "--section", "--lang", "--type", "--target", "--source":
		return true
	default:
		return false
	}
}

func isWriteSQL(sql string) bool {
	for {
		sql = strings.TrimSpace(sql)
		if strings.HasPrefix(sql, "--") {
			end := strings.IndexByte(sql, '\n')
			if end < 0 {
				return false
			}
			sql = sql[end+1:]
			continue
		}
		if strings.HasPrefix(sql, "/*") {
			end := strings.Index(sql[2:], "*/")
			if end < 0 {
				return false
			}
			sql = sql[end+4:]
			continue
		}
		break
	}
	for _, operation := range []string{"INSERT", "UPDATE"} {
		if len(sql) < len(operation) || !strings.EqualFold(sql[:len(operation)], operation) {
			continue
		}
		rest := sql[len(operation):]
		if rest == "" || strings.ContainsAny(rest[:1], " \t\r\n(") || strings.HasPrefix(rest, "/*") || strings.HasPrefix(rest, "--") {
			return true
		}
	}
	return false
}

func parseWriteArgs(args []string, sqlIndex int) (writeConfig, error) {
	cfg := writeConfig{sql: args[sqlIndex], format: "text"}
	var files []string
	for i := 0; i < len(args); i++ {
		if i == sqlIndex {
			continue
		}
		a := args[i]
		if i < sqlIndex && strings.EqualFold(a, "query") {
			continue
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			files = append(files, a)
			continue
		}
		name, value, inline := strings.Cut(a, "=")
		switch name {
		case "--dry-run", "--json":
			if inline {
				return cfg, fmt.Errorf("%s does not take a value", name)
			}
			if name == "--dry-run" {
				cfg.opts.DryRun = true
			} else {
				cfg.format = "json"
			}
		case "--root", "--format", "--expect-rows", "--expect-sha256":
			if !inline {
				if i+1 >= len(args) || i+1 == sqlIndex {
					return cfg, fmt.Errorf("%s requires value", name)
				}
				i++
				value = args[i]
			}
			switch name {
			case "--root":
				cfg.root = value
			case "--format":
				cfg.format = value
			case "--expect-rows":
				n, err := strconv.Atoi(value)
				if err != nil || n < 0 {
					return cfg, fmt.Errorf("--expect-rows requires a non-negative integer")
				}
				cfg.opts.ExpectRows = &n
			case "--expect-sha256":
				decoded, err := hex.DecodeString(value)
				if err != nil || len(decoded) != 32 {
					return cfg, fmt.Errorf("--expect-sha256 requires 64 hexadecimal characters")
				}
				cfg.opts.ExpectSHA256 = strings.ToLower(value)
			}
		default:
			return cfg, fmt.Errorf("flag %q is not supported for INSERT/UPDATE", a)
		}
	}
	if cfg.format != "json" && cfg.format != "text" {
		return cfg, fmt.Errorf("INSERT/UPDATE requires --format=json or --format=text")
	}
	if len(files) != 1 {
		return cfg, fmt.Errorf("INSERT/UPDATE requires exactly one existing file")
	}
	if files[0] == "-" {
		return cfg, fmt.Errorf("INSERT/UPDATE does not accept stdin")
	}
	cfg.file = files[0]
	return cfg, nil
}

func resolveWriteFile(cfg writeConfig) (string, error) {
	root := cfg.root
	if root != "" {
		var err error
		root, err = source.ResolvePath(root)
		if err != nil {
			return "", fmt.Errorf("invalid value for --root %q: %w", cfg.root, err)
		}
		info, err := os.Stat(root)
		if err != nil {
			return "", fmt.Errorf("invalid value for --root %q: %w", cfg.root, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("invalid value for --root %q: root is not a directory", cfg.root)
		}
	}
	path := cfg.file
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Resolve Unicode-equivalent spellings, retaining the logical final path
	// so Lstat still rejects a symlink. Never expand wildcard characters.
	path, err = source.ResolvePath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("INSERT/UPDATE requires a regular file (no directories or symlinks)")
	}
	return path, nil
}

func runWrite(args []string, execute func(string, *query.WriteStatement, edit.Options) (*edit.Result, error)) (int, bool) {
	sqlIndex, priority := writeSQLIndex(args)
	if sqlIndex < 0 {
		return 0, false
	}
	if priority != "" {
		if priority == "--help" || priority == "-h" {
			fmt.Print(cli.Help())
		} else {
			fmt.Printf("mdsql %s\n", appVersion)
		}
		return 0, true
	}
	fail := func(err error) (int, bool) {
		fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
		return 1, true
	}
	cfg, err := parseWriteArgs(args, sqlIndex)
	if err != nil {
		return fail(err)
	}
	stmt, err := query.ParseWrite(cfg.sql)
	if err != nil {
		return fail(fmt.Errorf("query parse error: %w", err))
	}
	path, err := resolveWriteFile(cfg)
	if err != nil {
		return fail(err)
	}
	cfg.opts.SourceName, err = writeSourceName(path, cfg.root)
	if err != nil {
		return fail(err)
	}
	result, writeErr := execute(path, stmt, cfg.opts)
	if result != nil && (writeErr == nil || result.Applied) {
		if cfg.format == "json" {
			err = json.NewEncoder(os.Stdout).Encode(result)
		} else if cfg.opts.DryRun {
			_, err = fmt.Fprint(os.Stdout, result.Diff)
		} else {
			label := "unchanged"
			if result.Applied {
				label = "applied"
			}
			_, err = fmt.Fprintf(os.Stdout, "%s: matched=%d changed=%d file=%s\n", label, result.Matched, result.Changed, result.File)
		}
		if err != nil {
			if writeErr != nil {
				fmt.Fprintf(os.Stderr, "mdsql: %v\n", writeErr)
			}
			return fail(err)
		}
	}
	if writeErr != nil {
		return fail(writeErr)
	}
	return 0, true
}

// Match run's SELECT-visible identity while retaining the absolute physical path
// for editing. ResolvePath preserves aliases; NormalizePath supplies NFC names.
func writeSourceName(path, root string) (string, error) {
	if root != "" {
		var err error
		root, err = source.ResolvePath(root)
		if err != nil {
			return "", err
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	identity := source.NormalizePath(path)
	if relative, err := filepath.Rel(source.NormalizePath(rootAbs), identity); err == nil && !strings.HasPrefix(relative, "..") {
		return filepath.ToSlash(relative), nil
	}
	return filepath.ToSlash(identity), nil
}
