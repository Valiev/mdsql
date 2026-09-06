// Package edit plans byte-preserving edits to one existing Markdown file.
package edit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

type Options struct {
	DryRun       bool
	ExpectRows   *int
	ExpectSHA256 string
	// SourceName matches the SELECT-visible file identity independently of the
	// physical write path. Empty uses the caller's original path.
	SourceName string
}

type Result struct {
	Mode         string `json:"mode"`
	File         string `json:"file"`
	Target       string `json:"target"`
	Applied      bool   `json:"applied"`
	Matched      int    `json:"matched"`
	Changed      int    `json:"changed"`
	BaseSHA256   string `json:"base_sha256"`
	ResultSHA256 string `json:"result_sha256"`
	Diff         string `json:"diff"`
}

func Execute(path string, stmt *query.WriteStatement, opts Options) (*Result, error) {
	if err := validateStatement(stmt); err != nil {
		return nil, err
	}
	if opts.ExpectRows != nil && *opts.ExpectRows < 0 {
		return nil, fmt.Errorf("expect-rows must be nonnegative")
	}
	if opts.ExpectSHA256 != "" {
		b, err := hex.DecodeString(opts.ExpectSHA256)
		if err != nil || len(b) != sha256.Size {
			return nil, fmt.Errorf("expect-sha256 must contain 64 hexadecimal digits")
		}
	}
	if path == "" || path == "-" || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("write requires one existing regular file, without stdin or pattern expansion")
	}
	identityPath := filepath.Clean(path)
	if opts.SourceName != "" {
		identityPath = opts.SourceName
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Resolve directory aliases so cooperating writers use the same lock. The
	// final component is deliberately not resolved: a symlink is not writable.
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	path = filepath.Join(dir, filepath.Base(abs))
	original, info, err := readOriginal(path)
	if err != nil {
		return nil, err
	}
	base := digest(original)
	if opts.ExpectSHA256 != "" && !strings.EqualFold(base, opts.ExpectSHA256) {
		return nil, fmt.Errorf("expect-sha256 mismatch: got %s", base)
	}
	doc, err := parser.ParseFile(identityPath, string(original), info.Size(), info.ModTime())
	if err != nil {
		return nil, err
	}
	var after []byte
	var matched, changed int
	if stmt.Table == "frontmatter_values" {
		after, matched, changed, err = planFrontmatter(original, stmt, doc)
	} else {
		after, matched, changed, err = planText(original, stmt, doc)
	}
	if err != nil {
		return nil, err
	}
	if opts.ExpectRows != nil && *opts.ExpectRows != matched {
		return nil, fmt.Errorf("expect-rows mismatch: expected %d, matched %d", *opts.ExpectRows, matched)
	}
	result := &Result{Mode: strings.ToLower(stmt.Operation), File: path, Target: stmt.Table, Matched: matched, Changed: changed, BaseSHA256: base, ResultSHA256: digest(after), Diff: unifiedDiff(path, original, after)}
	if opts.DryRun || bytes.Equal(original, after) {
		return result, nil
	}
	result.Applied, err = atomicWrite(path, original, after, info, writerHooks{})
	return result, err
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

// selectWriteRows deliberately uses the strict SELECT engine without row caps.
func selectWriteRows(stmt *query.WriteStatement, doc *model.Document) ([]query.Row, error) {
	if stmt.Where == nil {
		return nil, fmt.Errorf("UPDATE requires WHERE")
	}
	if query.HasAggregate(stmt.Where) {
		return nil, fmt.Errorf("aggregate functions are not allowed in WHERE")
	}
	now := time.Now()
	result, err := engine.Execute(&query.Query{Table: stmt.Table, Select: []query.SelectItem{{Alias: "*"}}, Where: stmt.Where}, []*model.Document{doc}, engine.Options{Strict: true, Now: now, Today: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

func validateStatement(stmt *query.WriteStatement) error {
	if stmt == nil {
		return fmt.Errorf("missing write statement")
	}
	allowed := map[string]bool{}
	add := func(names ...string) {
		for _, name := range names {
			allowed[name] = true
		}
	}
	if stmt.Operation == "INSERT" {
		if stmt.Where != nil {
			return fmt.Errorf("INSERT does not support WHERE")
		}
		switch stmt.Table {
		case "lines":
			add("text")
		case "tasks":
			add("text", "state", "status", "position", "due", "scheduled", "priority", "tags")
		case "sections":
			add("level", "title", "body")
		case "code":
			add("lang", "code")
		case "frontmatter_values":
			add("key", "value")
		default:
			return fmt.Errorf("unsupported INSERT target %q", stmt.Table)
		}
		if stmt.Table != "frontmatter_values" {
			add("line", "section", "section_path", "heading_line")
		}
		if stmt.Table == "tasks" {
			add("parent_line")
		}
	} else if stmt.Operation == "UPDATE" {
		if stmt.Where == nil {
			return fmt.Errorf("UPDATE requires WHERE")
		}
		switch stmt.Table {
		case "tasks":
			add("state", "status", "text", "due", "scheduled", "priority", "tags")
		case "frontmatter_values":
			add("value")
		default:
			return fmt.Errorf("unsupported UPDATE target %q", stmt.Table)
		}
	} else {
		return fmt.Errorf("unsupported write operation %q", stmt.Operation)
	}
	if len(stmt.Values) == 0 {
		return fmt.Errorf("write requires values")
	}
	positions := 0
	for key, value := range stmt.Values {
		if !allowed[key] {
			return fmt.Errorf("unsupported %s column %q for %s", stmt.Operation, key, stmt.Table)
		}
		nullableTaskMetadata := stmt.Table == "tasks" && (key == "due" || key == "scheduled" || key == "priority" || key == "tags")
		if value.Kind == query.ValueNull && !(stmt.Table == "frontmatter_values" && key == "value") && !nullableTaskMetadata {
			return fmt.Errorf("NULL is supported only for frontmatter value and nullable task metadata")
		}
		switch key {
		case "line", "section", "section_path", "heading_line", "parent_line":
			positions++
		}
	}
	if positions > 1 {
		return fmt.Errorf("at most one position selector is allowed")
	}
	if _, state := stmt.Values["state"]; state {
		if _, status := stmt.Values["status"]; status {
			return fmt.Errorf("use state or status, not both")
		}
	}
	if position, ok := stmt.Values["position"]; ok {
		if position.Kind != query.ValueString || position.String() != "section_end" {
			return fmt.Errorf("unsupported task position %q", position.String())
		}
		_, section := stmt.Values["section"]
		_, sectionPath := stmt.Values["section_path"]
		_, headingLine := stmt.Values["heading_line"]
		if !section && !sectionPath && !headingLine {
			return fmt.Errorf("task position requires a section selector")
		}
	}
	return nil
}
