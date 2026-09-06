package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

// Only stat-backed columns may bypass Markdown parsing. Unknown expressions
// conservatively keep the regular loader and its diagnostics.
func metadataOnly(q *query.Query) bool {
	if q.Table != "files" || q.GroupBy != "" {
		return false
	}
	for _, item := range q.Select {
		if !metadataExpr(item.Expr) {
			return false
		}
	}
	if !metadataExpr(q.Where) {
		return false
	}
	for _, item := range q.OrderBy {
		if !metadataExpr(item.Expr) {
			return false
		}
	}
	return true
}

func metadataExpr(expr query.Expr) bool {
	switch value := expr.(type) {
	case nil, *query.LiteralExpr, *query.ContextValueExpr:
		return true
	case *query.ColumnExpr:
		switch value.Name {
		case "file", "name", "dir", "size", "mtime":
			return true
		}
	case *query.BinaryExpr:
		return metadataExpr(value.Left) && metadataExpr(value.Right)
	case *query.UnaryExpr:
		return metadataExpr(value.Expr)
	case *query.IsNullExpr:
		return metadataExpr(value.Expr)
	case *query.BetweenExpr:
		return metadataExpr(value.Left) && metadataExpr(value.Low) && metadataExpr(value.High)
	case *query.AggregateExpr:
		return value.Star || metadataExpr(value.Arg)
	case *query.SubqueryExpr:
		return metadataOnly(value.Query)
	case *query.FuncCallExpr:
		if strings.EqualFold(value.Name, "has_tag") || strings.EqualFold(value.Name, "after_heading") {
			return false
		}
		for _, argument := range value.Args {
			if !metadataExpr(argument) {
				return false
			}
		}
		return true
	}
	return false
}

func loadMetadata(path string, maxSize int64) (*parser.Document, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %s (%d bytes)", path, fi.Size())
	}
	return &parser.Document{Path: path, Size: fi.Size(), ModTime: fi.ModTime()}, nil
}
