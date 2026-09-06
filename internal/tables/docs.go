package tables

// Shared columns have the same meaning across views; exceptions are qualified.
// Keep each description on one line so the text schema remains a small reference.
func populateColumnDocs() {
	common := map[string]string{
		"file": "Source path", "start": "First line (1+)", "end": "Last line (incl.)",
		"line": "Line (1+)", "n": "Line (1+)", "kind": "Block kind",
		"text": "Content text", "raw": "Source text", "section": "Heading title",
		"path": "Heading path", "lang": "Code language", "info": "Fence info",
		"fence": "Fence marker", "lines": "Line count", "fm": "Frontmatter",
		"table": "Table no. (1+)", "row": "Row; header=0", "col": "Column (1+)",
		"header": "Column title", "value": "Field value", "code": "Fence body",
		"name": "Basename", "dir": "Directory", "size": "Bytes", "words": "Word count",
		"headings": "Heading count", "tasks": "Task count", "open_tasks": "Undone count",
		"code_blocks": "Code count", "mtime": "Modified time", "key": "YAML key",
		"level": "Heading level", "title": "Title", "slug": "Heading ID",
		"nested": "Nested flag", "section_end": "Section last line", "parent": "Parent title",
		"depth": "Tree depth", "type": "Type", "source": "Origin",
		"target": "Link target", "anchor": "Heading anchor", "block_ref": "Block ID",
		"external": "External URL?", "fold": "Callout +/-", "body": "Body text",
		"children": "Direct children", "own": "Own section body", "tag": "Tag (no #)",
		"block_kind": "Source kind", "status": "Checkbox mark", "state": "Task state",
		"done": "Completed?", "due": "Due date", "scheduled": "Scheduled date",
		"start_date": "Start date", "done_at": "Done date", "cancelled_at": "Cancel date",
		"created": "Created date", "recurrence": "Repeat rule", "id": "Task ID",
		"depends_on": "Dependency IDs", "priority": "Priority rank", "priority_name": "Priority name",
		"root": "Root task line", "tags": "Task tags", "container": "Quote kind",
		"is_header": "Header cell?", "rows": "Data row count", "columns": "Column count",
	}
	overrides := map[string]string{
		"tasks.parent": "Parent task line", "tasks.depth": "Indent depth",
		"headings.depth": "Tree depth (1+)", "sections.depth": "Tree depth (0+)",
		"sections.level": "Level; root=0", "sections.body": "Body + children",
		"quotes.type": "Callout type", "quotes.body": "Without marker",
		"links.type": "Link syntax", "links.source": "Body or YAML",
		"tags.source": "Body or YAML", "cells.value": "Cell text",
	}
	for i := range Registry {
		for j := range Registry[i].Columns {
			column := &Registry[i].Columns[j]
			column.Doc = common[column.Name]
			if doc, ok := overrides[Registry[i].Name+"."+column.Name]; ok {
				column.Doc = doc
			}
		}
	}
}
