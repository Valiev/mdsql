package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	mdsql "github.com/Valiev/mdsql"
	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/cli"
	"github.com/Valiev/mdsql/internal/edit"
	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/output"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/schema"
	"github.com/Valiev/mdsql/internal/source"
)

var appVersion = mdsql.Version()

func main() {
	os.Exit(run())
}

func run() int {
	if code, handled := runWrite(os.Args[1:], edit.Execute); handled {
		return code
	}
	cfg, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
		return 1
	}

	switch cfg.Mode {
	case cli.ModeCacheInfo:
		stats, err := cache.StatsDefault()
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsql: cache: %v\n", err)
			return 1
		}
		format := output.Format(cfg.Format)
		if format == output.FormatText {
			fmt.Printf("entries=%d bytes=%d\n", stats.Entries, stats.Bytes)
			return 0
		}
		if format != output.FormatJSON && format != output.FormatJSONL && format != output.FormatCSV && format != output.FormatTSV && format != output.Format("plain") {
			fmt.Fprintf(os.Stderr, "mdsql: format %q is incompatible with --cache-info\n", format)
			return 1
		}
		rows := []query.Row{{"entries": query.NewInt(int64(stats.Entries)), "bytes": query.NewInt(stats.Bytes)}}
		renderFormat := format
		if format == output.FormatJSON {
			renderFormat = output.FormatJSONL
		}
		if err := output.Render(os.Stdout, rows, renderFormat, 0, false, false, "cache-info", []string{"entries", "bytes"}, output.Options{NoHeader: cfg.NoHeader, SchemaOrder: true}); err != nil {
			fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
			return 1
		}
		return 0
	case cli.ModeClearCache:
		if err := cache.ClearDefault(); err != nil {
			fmt.Fprintf(os.Stderr, "mdsql: cache: %v\n", err)
			return 1
		}
		return 0
	case cli.ModeHelp:
		fmt.Print(cli.Help())
		return 0
	case cli.ModeSchema:
		format := output.Format(cfg.Format)
		if format == "" {
			format = output.FormatText
		}
		if format == output.FormatJSON {
			if err := schema.JSON(os.Stdout, cfg.SchemaTablesOnly, cfg.SchemaExamplesOnly); err != nil {
				fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
				return 1
			}
			return 0
		}
		schema.Text(os.Stdout, cfg.SchemaTablesOnly, cfg.SchemaExamplesOnly)
		return 0
	case cli.ModeVersion:
		fmt.Printf("mdsql %s\n", appVersion)
		return 0
	}
	if cfg.Root != "" {
		root, err := source.ResolvePath(cfg.Root)
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(root)
			if err == nil && !info.IsDir() {
				err = fmt.Errorf("root is not a directory")
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsql: invalid value for --root %q: %v\n", cfg.Root, err)
			return 1
		}
		cfg.Root = root
	}

	q, err := query.Parse(cfg.Query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsql: query parse error: %v\n", err)
		return 1
	}

	if cfg.Explain {
		fmt.Fprintln(os.Stdout, q.String())
		for i, subquery := range q.Subqueries() {
			fmt.Fprintf(os.Stdout, "subquery %d: execute once: %s\n", i+1, subquery.String())
		}
		return 0
	}
	if err := output.ValidateProjection(output.Format(cfg.Format), q.SelectKeys()); err != nil {
		fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
		return 1
	}

	sources := cfg.Files
	if len(q.In) > 0 {
		sources = q.In
	}
	if len(sources) == 0 {
		sources = []string{"-"}
	}

	files, useStdin, err := source.ResolveSpecs(sources, cfg.Root, cfg.Include, cfg.Exclude)
	if err != nil {
		if cfg.Strict {
			fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
			return 2
		}
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "mdsql: warning: %v\n", err)
		}
	}

	engineOpts := engine.Options{Today: cfg.Today, Now: time.Now(), TableLimit: cfg.TableLimit, Strict: cfg.Strict}
	var executor *engine.Executor
	docs := make([]*parser.Document, 0)
	if len(q.Subqueries()) == 0 {
		executor, err = engine.NewExecutor(q, engineOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
			return 1
		}
	} else {
		docs = make([]*parser.Document, 0, len(files)+1)
	}
	rootAbs, _ := filepath.Abs(cfg.Root)
	rootAbs = source.NormalizePath(rootAbs)
	loaded := 0
	consumeDocument := func(doc *parser.Document) int {
		for _, warning := range doc.Warnings {
			if cfg.Strict {
				fmt.Fprintf(os.Stderr, "mdsql: %s\n", warning)
				return 2
			}
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "mdsql: warning: %s\n", warning)
			}
		}
		docPath := source.NormalizePath(doc.Path)
		rel, relErr := filepath.Rel(rootAbs, docPath)
		if relErr == nil && !strings.HasPrefix(rel, "..") {
			doc.RelPath = rel
		} else {
			doc.RelPath = docPath
		}
		setDocumentPath(doc, filepath.ToSlash(doc.RelPath))
		if executor == nil {
			docs = append(docs, doc)
		} else if executeErr := executor.ConsumeDocument(doc); executeErr != nil {
			fmt.Fprintf(os.Stderr, "mdsql: %v\n", executeErr)
			return 1
		}
		loaded++
		return 0
	}

	if useStdin {
		name := cfg.ResolveStdinName()
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			if cfg.Strict {
				fmt.Fprintf(os.Stderr, "mdsql: cannot read stdin: %v\n", readErr)
				return 2
			}
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "mdsql: warning: cannot read stdin: %v\n", readErr)
			}
		} else {
			if len(data) == 0 {
				if cfg.Strict {
					fmt.Fprintf(os.Stderr, "mdsql: empty stdin (%s); provide input files or pipe Markdown\n", name)
					return 2
				}
				if !cfg.Quiet {
					fmt.Fprintf(os.Stderr, "mdsql: warning: empty stdin (%s); provide input files or pipe Markdown\n", name)
				}
			}
			doc, parseErr := parser.ParseFile(name, string(data), int64(len(data)), time.Time{})
			if parseErr != nil {
				if cfg.Strict {
					fmt.Fprintf(os.Stderr, "mdsql: cannot parse stdin: %v\n", parseErr)
					return 2
				}
				if !cfg.Quiet {
					fmt.Fprintf(os.Stderr, "mdsql: warning: cannot parse stdin: %v\n", parseErr)
				}
			} else {
				if code := consumeDocument(doc); code != 0 {
					return code
				}
			}
		}
	}

	cacheWarning := func(err error) {
		if !cfg.Quiet {
			fmt.Fprintf(os.Stderr, "mdsql: warning: cache: %v\n", err)
		}
	}
	var parsedCache *cache.Cache
	statOnly := metadataOnly(q)
	if cfg.Cache && len(files) > 0 && !statOnly {
		parsedCache, err = cache.OpenDefault()
		if err != nil {
			cacheWarning(err)
		}
	}
	for _, p := range files {
		if p == "-" {
			continue
		}
		var doc *parser.Document
		var err error
		if statOnly {
			doc, err = loadMetadata(p, cfg.MaxFileSize)
		} else {
			doc, err = loadDocument(p, cfg.MaxFileSize, parsedCache, cacheWarning)
		}
		if err != nil {
			if cfg.Strict {
				fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
				return 2
			}
			if !cfg.Quiet {
				fmt.Fprintf(os.Stderr, "mdsql: warning: %v\n", err)
			}
			continue
		}
		if code := consumeDocument(doc); code != 0 {
			return code
		}
	}

	if loaded == 0 {
		if !cfg.Quiet {
			fmt.Fprintln(os.Stderr, "mdsql: no files were loaded")
		}
		return 2
	}

	var res *engine.ExecuteResult
	if executor != nil {
		res, err = executor.Finish()
	} else {
		res, err = engine.Execute(q, docs, engineOpts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsql: %v\n", err)
		return 1
	}

	if !cfg.Quiet {
		for _, warning := range res.Warnings {
			fmt.Fprintf(os.Stderr, "mdsql: warning: %s\n", warning)
		}
	}
	format := output.Format(cfg.Format)
	if cfg.Json {
		format = output.FormatJSON
	}
	kind := strings.ToLower(q.Table)
	totalRows := len(res.Rows)
	maxRows := cfg.MaxRows
	if q.LimitSet && !cfg.MaxRowsSet {
		maxRows = 0
	}
	if maxRows > 0 && totalRows > maxRows {
		res.Rows = res.Rows[:maxRows]
	}
	schemaOrder := false
	for _, item := range q.Select {
		if column, ok := item.Expr.(*query.ColumnExpr); ok && column.Name == "*" {
			schemaOrder = true
		}
	}
	if err := output.Render(os.Stdout, res.Rows, format, cfg.MaxLines, cfg.WithFile, cfg.NoFile, kind, res.Columns, output.Options{NoHeader: cfg.NoHeader, Null: cfg.Null, SchemaOrder: schemaOrder}); err != nil {
		fmt.Fprintf(os.Stderr, "mdsql: render error: %v\n", err)
		return 1
	}
	if len(res.Rows) < totalRows {
		fmt.Fprintf(os.Stderr, "mdsql: truncated, %d of %d rows\n", len(res.Rows), totalRows)
		if cfg.Strict {
			return 4
		}
	}

	if cfg.FailEmpty && totalRows == 0 {
		return 3
	}
	return 0
}

func setDocumentPath(doc *parser.Document, path string) {
	doc.Path = path
	for i := range doc.Headings {
		doc.Headings[i].File = path
	}
	for i := range doc.Sections {
		doc.Sections[i].File = path
	}
	for i := range doc.Blocks {
		doc.Blocks[i].File = path
	}
	for i := range doc.Tasks {
		doc.Tasks[i].File = path
	}
	for i := range doc.Links {
		doc.Links[i].File = path
	}
	for i := range doc.Tags {
		doc.Tags[i].File = path
	}
	for i := range doc.Quotes {
		doc.Quotes[i].File = path
	}
	for i := range doc.LinesRows {
		doc.LinesRows[i].File = path
	}
}
