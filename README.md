# mdsql

SQL-like queries for Markdown files, directories, and stdin. Extract headings,
tasks, links, tables, frontmatter, and source fragments without loading documents
into a database. Output selected columns as text, JSON, CSV, or Markdown.

[![CI](https://github.com/Valiev/mdsql/actions/workflows/ci.yml/badge.svg)](https://github.com/Valiev/mdsql/actions/workflows/ci.yml)
[![skills.sh](https://skills.sh/b/Valiev/mdsql)](https://skills.sh/Valiev/mdsql)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

This is an early-stage CLI with a deliberately limited SQL dialect. The detailed
reference and known limitations below describe the current implementation.

## Install

Install Go 1.22 or newer, then install the latest version:

```bash
go install github.com/Valiev/mdsql/cmd/mdsql@latest
```

Ensure that `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset, is on
`PATH`. To build from a checkout instead:

```bash
go build -trimpath -o mdsql ./cmd/mdsql
./mdsql --help
```

On Windows, the generated executable is named `mdsql.exe`.
The reference below uses `./mdsql` for a checkout; use `mdsql` after installation.

## Quick start

Query a self-contained Markdown example from stdin:

```bash
printf '%s\n' '# Demo' '## Tasks' '- [ ] Ship #work' \
  | ./mdsql --csv 'SELECT line, raw FROM headings WHERE level = 2'
```

Output:

```csv
line,raw
2,## Tasks
```

Common workflows:

```bash
# Only selected columns; file and kind are not added implicitly.
./mdsql 'SELECT line, raw FROM headings WHERE level = 2' note.md

# Extract complete Markdown table blocks.
./mdsql tables note.md

# Todo tasks tagged work.
./mdsql tasks --open --tag work notes/

# Export CSV for further processing.
./mdsql --csv 'SELECT file, line, text, due FROM tasks' notes/ > tasks.csv
```

Use `mdsql schema --tables --json` to discover the available tables. Queries do
not implicitly include file or location columns, so select them explicitly.
Throughout this guide, `note.md`, `notes/`, `Project`, and similar names are
placeholders for your own inputs.

## Agent skill

The repository includes a portable agent skill at
[`skills/mdsql/SKILL.md`](skills/mdsql/SKILL.md). Install the CLI first, then
install the skill with the open [`skills`](https://github.com/vercel-labs/skills)
CLI:

```bash
# Inspect the skill discovered in this repository.
npx skills add Valiev/mdsql --list

# Install mdsql for the agents you choose interactively.
npx skills add Valiev/mdsql --skill mdsql

# Example: install globally for Codex without prompts.
npx skills add Valiev/mdsql --skill mdsql -g -a codex -y
```

The skill provides workflows and safety guidance; it does not install the
`mdsql` executable or grant permission to scan or edit files. Start a new agent
session if an installation is not discovered immediately.

Example requests:

```text
Use $mdsql to list level-2 headings in notes/release.md as CSV.
Use $mdsql to find todo and in-progress tasks tagged work in notes/project/.
Use $mdsql to extract Markdown tables from notes/report.md, including source lines.
Use $mdsql to list external links in docs/ as JSON.
```

After building the binary, maintainers can verify the bundled recipes and skill
links using only synthetic Markdown:

```bash
python3 scripts/check_skill_recipes.py --binary ./mdsql
```

## Invocation and help

```bash
./mdsql '<SQL>' [files...]
./mdsql query '<SQL>' [files...]
./mdsql <alias> [options] [files...]
```

Pass SQL as one quoted argument. Subsequent arguments are input files. SELECT does not support a trailing semicolon; INSERT and UPDATE accept one optional trailing semicolon. Write operations require exactly one existing file.

```bash
./mdsql
./mdsql -h
./mdsql --help
./mdsql help
./mdsql version
```

The first four commands print help and exit with code `0`. Running `./mdsql query` without SQL is an error.

Inspect the schema, functions, operators, and built-in examples:

```bash
./mdsql schema
./mdsql schema --tables
./mdsql schema --examples
./mdsql schema --json
./mdsql schema --tables --json
./mdsql schema --examples --json
```

Do not confuse `tables` with `schema --tables`: the former extracts Markdown tables from documents; the latter describes the tool's virtual tables.

Text schema output lists each column as `name:type description`, with a short description on one line. JSON exposes the same descriptions in `doc` fields.

## Input sources

One UTF-8 BOM at the beginning of an input is removed before parsing, so frontmatter and first-line headings remain recognizable. Line numbers are unchanged; `files.size` still includes the original BOM bytes. BOM characters elsewhere remain content.

### A file, multiple files, a directory, or a glob

```bash
./mdsql headings note.md
./mdsql headings first.md second.md
./mdsql headings 'notes/Design review.md'
./mdsql headings notes/
./mdsql headings 'notes/**/*.md'
```

Directories are traversed recursively. Accepted extensions are `.md` and `.markdown`, case-insensitively. Traversal skips hidden entries; explicitly named hidden sources can still be read. File and directory symlinks are followed for positional paths, SQL IN sources, and globs. Logical paths are preserved in `file`: different aliases to the same target remain separate sources, while repeated identical logical paths are removed. Directory-link cycles are skipped with a warning; `--strict` exits 2, and `-q` suppresses the warning without changing the exit policy.

Quote globs so that `mdsql`, rather than your shell, expands them.

An existing literal path takes precedence over glob expansion, so a filename
such as `[draft].md` can be read and written using the same argument.

### Root directory, include filters, and exclude filters

```bash
./mdsql --root notes headings .

./mdsql --root notes \
  --include '**/*.md' \
  --exclude 'archive/**' \
  --exclude 'private/**' \
  headings .
```

`--root` sets the base for relative input paths and `--include`/`--exclude` patterns. Both filters use the same root-relative glob rules: `*` stays within one path component, while `**` spans directories. Thus `*.md` selects root files, `**/*.md` also selects nested files, `Templates/*` matches direct children, and `Templates/**` matches the whole subtree. Filters can be repeated: one matching include is sufficient; any matching exclude removes the file. Templates are not excluded automatically.

For queries, `--root` must name an existing directory. A file or missing path produces an argument error (exit 1), including with `--quiet`, `--strict`, or `--explain`.

For example, exclude templates when querying a scoped Obsidian directory:

```bash
./mdsql --root notes --include '**/*.md' --exclude 'Templates/**' --csv 'SELECT file, line, text FROM tasks LIMIT 50' .
```

The output `file` field is relative for files inside the root and may remain absolute for files outside it.

Source paths are displayed in Unicode NFC. Positional paths, globs, and include/exclude patterns resolve Unicode-equivalent NFC/NFD names; reading and caching still use the physical filesystem path. Use the displayed NFC spelling in `WHERE file = ...` filters.

### stdin

If a query or alias has no input files, the document is read from stdin. Running just `./mdsql` prints help instead of reading stdin.

```bash
cat note.md | ./mdsql 'SELECT line, raw FROM headings'
cat note.md | ./mdsql tasks --open

# Explicit stdin.
cat note.md | ./mdsql headings -

# Combine stdin with a file.
cat first.md | ./mdsql 'SELECT file, line, raw FROM headings' - second.md

# Set the source name used by the file column.
cat note.md | ./mdsql --stdin-name imported.md 'SELECT file, line, raw FROM headings'

# Alternatively, use an environment variable.
cat note.md | MDSQL_STDIN_NAME=imported.md ./mdsql 'SELECT file, line FROM headings'
```

Source-name precedence: `--stdin-name`, then `MDSQL_STDIN_NAME`, then `<stdin>`. Zero-byte stdin produces a warning; with `--strict` it fails with exit code `2`. Whitespace-only input is not considered empty.

### Sources inside SQL: FROM ... IN

```bash
./mdsql "SELECT file, line, title FROM headings IN 'notes/**/*.md' WHERE level = 2"
./mdsql "SELECT file, line, title FROM headings IN 'first.md', 'second.md'"
./mdsql "SELECT file, line, title FROM headings IN ('first.md', 'second.md') WHERE level = 2"
```

`IN` after a table name specifies input sources and replaces positional file arguments if both are present. It is distinct from `IN` in a `WHERE` clause, which tests membership in a list of values.

Source lists accept either comma-separated specifications or a parenthesized list of quoted, nonempty specifications. Parenthesized lists reject empty lists, trailing commas, and unquoted paths. `--explain` normalizes parentheses away while preserving the selected sources.

## Aliases

Aliases expand into SQL. Use `--explain` to inspect the query without reading input files:

```bash
./mdsql tasks --open --tag work --explain
./mdsql tables --explain
```

Use SQL directly for custom columns, additional conditions, or sorting.

### headings: document headings

Selects `level, title, line` by default.

```bash
./mdsql headings note.md
./mdsql headings --level 2 note.md
./mdsql headings -l 2 note.md
./mdsql headings --min-level 2 --max-level 3 note.md
./mdsql headings --under 'Project' note.md

./mdsql 'SELECT line, raw FROM headings WHERE level = 2' note.md
./mdsql 'SELECT file, line, title, path FROM headings ORDER BY file, line' notes/
```

`line` is the heading's starting line. `raw` preserves its Markdown representation.

### toc: table of contents

Selects `level, title, line` and sorts by `line`.

```bash
./mdsql toc note.md
./mdsql toc --format outline note.md
```

`outline` formats headings as a list indented by heading level. With multiple input files, `toc` sorts by line number; use the SQL example under `headings` to sort by file and line instead.

### tasks: checkbox tasks

Selects `line, status, text, due, path` by default. Filters are combined with `AND`.

```bash
./mdsql tasks note.md
./mdsql tasks --open note.md
./mdsql tasks --done note.md
./mdsql tasks --in-progress note.md
./mdsql tasks --state cancelled note.md
./mdsql tasks --tag work note.md
./mdsql tasks --depth 0 note.md
./mdsql tasks --priority 4 note.md
./mdsql tasks --section 'Tasks' note.md
./mdsql tasks --path 'Project > Tasks' note.md

# Tasks due on a date or within an inclusive date range.
./mdsql tasks --due 2026-09-05 note.md
./mdsql tasks --due 2026-09-01..2026-09-07 note.md

# Fix today for reproducible date filtering.
./mdsql --today 2026-09-07 tasks --overdue note.md
./mdsql tasks --scheduled-before 2026-09-07 note.md
./mdsql tasks --scheduled-after 2026-09-01 note.md
./mdsql --today 2026-09-03 tasks --due 'today..today+7d' note.md

# Original Markdown lines instead of parsed fields.
./mdsql tasks --open --raw --format md note.md

# Filter by task ID.
./mdsql tasks --has-id note.md
./mdsql "SELECT line, text, id FROM tasks WHERE id != ''" note.md
```

States: `[ ]` maps to `todo`, `[x]`/`[X]` to `done`, `[/]` to `in_progress`, `[-]` to `cancelled`, and other markers to `other`.

Checkbox tasks are also extracted from blockquotes and Obsidian callouts, including nested quotes. `container` is `blockquote`, `callout`, or an empty string for an ordinary task. `raw` preserves the quote markers and `line` identifies the original source line; `text` omits those markers. Task depth follows list indentation after the quote markers. Fenced code inside a quote does not produce tasks, links, or tags.

In `lists` and `items`, `nested` follows indentation for both task and ordinary list items.

```bash
./mdsql --csv "SELECT line, container, text FROM tasks WHERE container != ''" note.md
```

`--open` means only `state = 'todo'`, not all unfinished states. `--overdue` means `due < today AND state != 'done'`, so it does not exclude cancelled tasks on its own. `--scheduled-before` and `--scheduled-after` use strict `<` and `>` comparisons.

`--due`, `--scheduled-before`, `--scheduled-after`, and `--today` accept an ISO date, `today`/`today()`, or one signed `d`, `w`, `m`, or `y` offset, such as `today+7d`. `--due` also accepts an inclusive `start..end` range with both bounds validated. Quote values containing spaces. Invalid values fail before input is read with exit 1 and the flag name, including under `--quiet`. A relative `--today` is anchored to the actual current date.

Examples of supported task metadata:

```markdown
- [ ] Release #work 📅 2026-09-05 ⏳ 2026-09-03 ⏫ 🆔 release
- [ ] Write docs #work/docs [due:: 2026-09-06] [priority:: 3]
- [x] Prepare checklist ✅ 2026-09-01
```

Tasks date markers: `📅 due`, `⏳ scheduled`, `🛫 start_date`, `✅ done_at`, `❌ cancelled_at`, `➕ created`. Priority markers map to `🔺 5`, `⏫ 4`, `🔼 3`, `🔽 2`, `⏬ 1`; an unspecified priority is `0`. The `recurrence`, `id`, and `depends_on` fields correspond to `🔁`, `🆔`, and `⛔`.

`--has-id` expands to `id != ''` and returns only tasks with a nonempty `🆔` value.

### code: fenced code blocks

Selects `lang, start, end, code` by default.

```bash
./mdsql code note.md
./mdsql code --lang go note.md
./mdsql code --lang go --lang python note.md
./mdsql code --section 'Examples' note.md
./mdsql code --under 'Examples' note.md
./mdsql code --lang go --format plain note.md

# raw includes the fences; code contains only the code.
./mdsql --format md "SELECT raw FROM code WHERE lang = 'go'" note.md

# Combine language and exact section filters.
./mdsql code --lang go --section 'Examples' note.md
./mdsql code --lang go --lang python --under 'Examples' note.md
./mdsql "SELECT lang, start, end, code FROM code WHERE lang = 'go' AND section = 'Examples'" note.md
```

A fence uses at least three backticks or tildes. Its closing fence must use the same character, be at least as long as the opener, and have no trailing info text. Shorter fences remain inside the code block. Fenced contents are excluded from headings, tasks, links, and tags; an unterminated fence extends to the end of the file. `lang` is the first info-string word in lowercase.

In this alias, `--under` is equivalent to an exact `--section` filter, not the `under()` function. Language and section filters combine with AND; repeated `--lang` values combine with OR. Use SQL when you need custom projections or other conditions.

### tables: Markdown tables

```bash
./mdsql tables note.md
./mdsql tables --csv note.md
./mdsql tables --format md note.md
./mdsql 'SELECT start, raw FROM tables' note.md
./mdsql --csv 'SELECT file, start, rows, columns FROM tables' note.md
```

Selects `file, start, end, raw`. Each result row represents a complete Markdown table block, not an individual table row or cell. In CSV, a multiline `raw` value is quoted.

`rows` counts data rows, excluding the header and delimiter; `columns` is the header width.

### cells: individual Markdown table cells

```bash
./mdsql --csv "SELECT file, table, row, value FROM cells WHERE header = 'Workflow' AND is_header = false" note.md
./mdsql --json 'SELECT * FROM cells' note.md
```

`cells(file, table, row, col, header, value, is_header)` exposes parsed cells. Table and column numbers start at `1`; `row = 0` contains column headings and data rows start at `1`. The delimiter row is omitted. `header` repeats the column's heading on each cell, including the heading cell itself; `is_header = false` or `row > 0` selects only data. Surrounding whitespace is trimmed and `\|` becomes a literal pipe. Missing cells become empty strings; cells beyond the header width are ignored. Tables without a leading `|` are not recognized by the current block parser. Use SQL; there is no `cells` alias.

### links: links and images

Selects `type, target, text, line` by default.

```bash
./mdsql links note.md
./mdsql links --external note.md
./mdsql links --internal note.md
./mdsql links --type wiki note.md
./mdsql links --type embed note.md
./mdsql links --type md note.md
./mdsql links --type image note.md
./mdsql links --type autolink note.md
./mdsql links --external --target '%example.com%' note.md

./mdsql 'SELECT line, target, anchor, block_ref FROM links' note.md
```

Types: `wiki` for `[[Note]]`, `embed` for `![[Note]]`, `md` for Markdown links, `image` for images, and `autolink` for automatically detected URLs. `--target` uses SQL `LIKE`, not a glob.

The `source` column is `body` or `frontmatter`. YAML string values contribute wikilinks and embeds, including strings in lists and nested objects. Their `key` is the dotted YAML field path; list indexes are omitted. Body links have an empty `key`. To retain body-only results, filter explicitly:

```bash
./mdsql --csv "SELECT file, line, target FROM links WHERE source = 'body'" note.md
./mdsql --csv "SELECT file, key, target FROM links WHERE source = 'frontmatter' AND target = 'Projects'" notes/
```

Overlapping Markdown-link, angle-autolink, and bare-URL matches are emitted once per source occurrence. Separate occurrences of the same URL remain separate rows; use `SELECT DISTINCT target` for unique targets. Bare URLs discard trailing `.`, `,`, `;`, and unmatched `)`; balanced URL parentheses are retained.

### tags: tag occurrences

```bash
./mdsql tags notes/
./mdsql tags --count notes/
./mdsql tags --source body notes/
./mdsql tags --source frontmatter notes/
./mdsql tags --source frontmatter --explode-frontmatter notes/
./mdsql tags --count --csv notes/
```

Without flags, selects unique tags sorted by name. `--count` counts tag occurrences, not distinct files. Body tags are returned without the leading `#`. The `tasks.tags` list also preserves source occurrences: one `#work` produces one element, while `#work #work` produces two.

Frontmatter tag arrays are already expanded into individual occurrences. `--explode-frontmatter` is accepted for compatibility but currently has no additional effect.

### frontmatter: YAML metadata

```bash
./mdsql frontmatter --json note.md
./mdsql 'SELECT file, fm.owner AS owner FROM frontmatter' note.md
./mdsql "SELECT file, fm.status AS status FROM frontmatter WHERE fm.status = 'active'" notes/
./mdsql 'SELECT file FROM frontmatter WHERE fm.owner IS NOT NULL' notes/
./mdsql --format md 'SELECT raw FROM frontmatter' note.md
```

Selects `file, fm`. Use JSON to preserve the complete nested object. A missing key inside `fm` returns `NULL`, unlike an unknown base table column, which is a query error. `frontmatter.raw` contains YAML without the `---` delimiters.

Invalid YAML inside a delimited frontmatter block produces a warning with the file and line. `--strict` stops with exit code `2`. The reserved block is excluded from body parsing even when invalid; `lines` still exposes the original lines with `kind = 'frontmatter'`. Filter `kind != 'frontmatter'` when you want body lines. Metadata-only `files` queries skip parsing and therefore do not diagnose YAML; see below.

Date-only YAML timestamps are normalized to `YYYY-MM-DD` and support date comparisons such as `fm.start BETWEEN 2026-09-01 AND today()`. Datetimes recognized by the YAML parser retain their time and offset and serialize as RFC 3339, including midnight timestamps. Valid quoted `YYYY-MM-DD` strings retain the existing inferred-date behavior; invalid date-like strings remain strings. These rules apply inside lists and objects as well as to top-level values.

```bash
./mdsql --csv --today 2026-09-07 'SELECT file, fm.start FROM frontmatter WHERE fm.start BETWEEN 2026-09-01 AND today()' notes/
```

### frontmatter_values: metadata values as rows

```bash
./mdsql --csv "SELECT DISTINCT file FROM frontmatter_values WHERE key = 'categories' AND value = '[[Projects]]'" notes/
./mdsql --csv "SELECT value, COUNT(*) AS n FROM frontmatter_values WHERE key = 'categories' GROUP BY value ORDER BY n DESC, value" notes/
```

This SQL table exposes `file, key, value`. Each top-level list element becomes a
row, preserving element order and duplicates. Scalar values, explicit `NULL`,
and objects produce one row each; empty lists and absent metadata produce none.
Keys are sorted lexicographically and remain literal: `meta.stage` is a key name,
not a path. Nested lists and objects remain intact in `value`; use JSON to export
them. Object fields can be selected with expressions such as `value.stage`.
Values use the same type normalization as `fm`.

The count example counts occurrences, including repeated values within one
file. Grouping requires scalar values; objects and nested lists cannot be group
keys. There is no `frontmatter_values` CLI alias; use a SQL query.

### files: metadata and document metrics

```bash
./mdsql files notes/
./mdsql files --csv notes/
./mdsql 'SELECT file, size, words, mtime FROM files ORDER BY size DESC LIMIT 10' notes/
./mdsql 'SELECT file, tasks, open_tasks FROM files WHERE open_tasks > 0' notes/
```

Selects `file, lines, headings, tasks, open_tasks` by default. `size` is measured in bytes. `open_tasks` counts every task with `done = false`, unlike `tasks --open`, which selects only `todo` tasks.

`files.words` counts whitespace-delimited words in the Markdown body. YAML frontmatter is excluded.

`files.lines` and the `lines` table count physical source lines: empty input has zero, `\n` has one, and both `foo` and `foo\n` have one. A terminating newline does not create an extra blank line.

Queries limited to stat-backed `file`, `name`, `dir`, `size`, and `mtime`, including `COUNT(*) FROM files`, can skip reading and parsing regular-file content. This also applies to filters and ordering using only those fields and literals. Document metrics, `fm`, `*`, and grouped queries require full parsing; stdin is always parsed. The metadata path still checks file size and stat errors, but does not validate Markdown or YAML contents, even with `--strict`.

### sections: section listing

```bash
./mdsql sections note.md
./mdsql 'SELECT title, start, end, path FROM sections WHERE level = 2' note.md
./mdsql 'SELECT title, tasks, code_blocks FROM sections' note.md
```

Selects `level, title, start, end, path`. The level-0, depth-0 root covers the document body after frontmatter and has an empty title. Each heading section extends through its descendants, ending before the next heading of equal or higher rank. `depth` counts edges in the actual tree, so a jump from h1 to h3 still increases depth by one. `parent` is the parent title; `children` counts direct child sections. Task and code-block metrics include descendants. Content rows are assigned to their innermost section.

An empty file or frontmatter-only file has no root section. A physical blank line after frontmatter is body content and produces a root with a valid range.

### section: content by title or slug

```bash
./mdsql section 'Examples' note.md
./mdsql section 'Examples' --format md note.md
./mdsql section examples --format plain note.md

./mdsql --format md "SELECT body FROM sections WHERE title = 'Examples'" note.md
./mdsql --format md "SELECT own FROM sections WHERE title = 'Examples'" note.md
```

Selects `raw` using `title = ... OR slug = ...`. `raw` includes the heading and descendant sections. `body` excludes the heading; `own` includes only text before the first child section. Both `body` and `own` trim surrounding whitespace, while `raw` preserves it. For the root, `own` is the preamble before the first heading; without headings, the root covers the entire body. Duplicate titles can return multiple sections. Automatic slugs currently work best with ASCII titles; use the exact `title` for non-ASCII headings.

### blocks: document blocks

```bash
./mdsql blocks note.md
./mdsql 'SELECT kind, start, end, raw FROM blocks' note.md
./mdsql "SELECT start, raw FROM blocks WHERE kind = 'paragraph'" note.md
```

Selects `kind, start, end, section` by default. `raw` preserves the source fragment; `text` is its processed representation.

A standalone thematic-break line such as `---`, `***`, or `___` in the body has `kind = 'hr'`, including the first body line when no closed frontmatter block is present.

### at: find the block containing a line

```bash
./mdsql at note.md:20
./mdsql at 'notes/Design review.md:20' --format md
./mdsql 'SELECT kind, start, end, raw FROM blocks WHERE at(20)' note.md
```

Selects blocks whose source range includes line `20`. A blank line that belongs to no block may produce an empty result.
The `file:line` alias accepts only positive, one-based line numbers; zero, negative, and non-numeric values exit with code `1`.

### lines: source lines

```bash
./mdsql lines note.md 20
./mdsql lines note.md 20:30
./mdsql lines note.md 20-30
./mdsql lines note.md 20:30 --csv
./mdsql lines note.md 20:30 --format plain
```

Ranges are inclusive, and line numbers start at `1`. Selects `n, text`. The line-number column in this table is `n`, not `line`.

### grep: search blocks with a regular expression

```bash
./mdsql grep 'TODO|FIXME' notes/
./mdsql grep '(?i)deadline' notes/
./mdsql grep '^Release' note.md

# Search source lines rather than blocks.
./mdsql "SELECT n, text FROM lines WHERE text ~* 'deadline'" note.md
```

Selects `kind, start, text` from `blocks`. Matching is case-sensitive by default. Use `(?i)` or the SQL `~*` operator for case-insensitive matching; there is no separate `grep -i` flag.

## Virtual tables: an example for each

Pass the query in the last column as one argument: `./mdsql '<SQL>' note.md`.

| Table | Contents | Example SQL |
| --- | --- | --- |
| `files` | Documents and metrics | `SELECT file, words, tasks FROM files` |
| `frontmatter` | YAML metadata | `SELECT file, fm FROM frontmatter` |
| `frontmatter_values` | Top-level metadata values, expanding lists once | `SELECT file, key, value FROM frontmatter_values` |
| `headings` | Headings | `SELECT level, line, raw FROM headings` |
| `sections` | Sections | `SELECT title, start, end, body FROM sections` |
| `blocks` | Markdown blocks | `SELECT kind, start, raw FROM blocks` |
| `paragraphs` | Plain-text blocks | `SELECT start, text FROM paragraphs` |
| `lists` | Blocks recognized as list items | `SELECT start, raw FROM lists` |
| `items` | A view of the same list blocks | `SELECT line, text FROM items` |
| `tasks` | Checkbox tasks | `SELECT line, text, due FROM tasks` |
| `code` | Fenced code blocks | `SELECT lang, start, code FROM code` |
| `tables` | Markdown table blocks | `SELECT start, end, raw FROM tables` |
| `cells` | Individual table cells | `SELECT table, row, col, header, value FROM cells` |
| `links` | Links and images | `SELECT type, target, line FROM links` |
| `tags` | Tag occurrences | `SELECT tag, source, line FROM tags` |
| `quotes` | Blockquotes and callouts | `SELECT type, title, body FROM quotes` |
| `lines` | Source lines | `SELECT n, text FROM lines` |

A callout example for the `quotes` table:

```markdown
> [!warning]- Release risk
> Check the rollback plan.
```

```bash
./mdsql "SELECT type, title, fold, body FROM quotes WHERE type = 'warning'" note.md
```

`fold` contains the `+` or `-` marker, if present. For ordinary blockquotes without a callout marker, `type`, `title`, and `fold` are empty.

An unquoted blank line ends a quote block; a quoted blank line (`>`) stays inside it. Separate quotes and callouts retain their own ranges and metadata. The blank line between them does not belong to either block.

## SQL: projection, filtering, sorting, and counting

Supports one `SELECT` from one virtual table:

```sql
SELECT [DISTINCT] <columns or expressions>
FROM <table> [IN <file specs>]
[WHERE <condition>]
[GROUP BY <column>]
[ORDER BY <expressions> [ASC|DESC]]
[LIMIT <integer>]
[OFFSET <integer>]
```

### Projection and column aliases

```bash
./mdsql 'SELECT line, raw FROM headings WHERE level = 2' note.md
./mdsql 'SELECT title AS heading, line AS location FROM headings' note.md
./mdsql --json 'SELECT * FROM headings' note.md
./mdsql 'SELECT DISTINCT lang FROM code ORDER BY lang' notes/
```

Ordinary projections return only the selected fields. `DISTINCT` removes duplicate projected rows using typed values, including complete nested lists and objects. Include `file` explicitly when you need the source path. Use lowercase table and column names. An unknown base column, such as `SELECT does_not_exist FROM headings`, causes a query error.

Repeated automatic output names receive suffixes such as `len`, `len_2`, and `len_3`, so one expression cannot overwrite another. Explicit aliases are reserved first; two identical explicit aliases are an error, including for empty results. Prefer explicit aliases when downstream code relies on column names.

WHERE can use SELECT aliases of non-aggregate columns and expressions. A real source column wins when its name collides with an alias. An aggregate alias in WHERE is an error; aliases do not chain through other SELECT aliases.

```bash
./mdsql --csv 'SELECT len(title), len(raw) FROM headings' note.md
./mdsql --csv 'SELECT len(title) AS title_bytes, len(raw) AS raw_bytes FROM headings' note.md
./mdsql --csv "SELECT lower(title) AS t FROM headings WHERE t = 'release'" note.md
```

### Wildcard columns

`SELECT *` expands the declared columns in the order below. It omits `fm` from every table except `frontmatter`; select `fm` or `fm.key` explicitly on tables that expose metadata when needed. Dynamic task values such as `fields.owner` also require an explicit projection. An explicit metadata selection remains valid even when the table has no rows.

CSV, TSV, grouped, JSON, and JSONL use this order for wildcard projections; JSON ordering applies to top-level keys. Explicit non-wildcard CSV, TSV, and grouped projections follow SELECT order; explicit JSON projections retain alphabetical order. Text prints single-line fields, including nested JSON values, first in SELECT/schema order, then multiline strings, so it does not preserve interleaving between those kinds of fields. Markdown and outline keep their content-specific behavior; plain prints every selected value in SELECT order.

The same declarations, including types and field descriptions, are available through `./mdsql schema --tables --json`.

| Table | Columns selected by `*`, in order |
| --- | --- |
| `blocks` | `file, start, end, kind, text, line, section, path, raw, lang, info, fence, lines` |
| `cells` | `file, table, row, col, header, value, is_header` |
| `code` | `file, start, end, line, lang, info, code, fence, lines, section, path, raw, text` |
| `files` | `file, name, dir, size, lines, words, headings, tasks, open_tasks, code_blocks, mtime` |
| `frontmatter` | `file, fm, raw, start, end` |
| `frontmatter_values` | `file, key, value` |
| `headings` | `file, start, end, line, level, title, slug, text, section, path, nested, section_end, parent, depth, raw` |
| `items` | `file, start, end, line, text, raw, section, path, nested` |
| `lines` | `file, n, text, kind, section, path` |
| `links` | `file, type, source, key, target, anchor, block_ref, text, raw, line, section, path, external` |
| `lists` | `file, start, end, line, text, raw, section, path, nested` |
| `paragraphs` | `file, start, end, line, text, raw, section, path, nested` |
| `quotes` | `file, start, end, type, title, fold, body, text, section, path` |
| `sections` | `file, level, title, slug, parent, depth, start, end, children, body, own, raw, path, tasks, open_tasks, code_blocks, lines` |
| `tables` | `file, start, end, line, text, raw, section, path, nested, rows, columns` |
| `tags` | `file, tag, source, line, section, path, block_kind` |
| `tasks` | `file, line, status, state, container, done, text, due, scheduled, start_date, done_at, cancelled_at, created, recurrence, id, depends_on, priority, priority_name, depth, parent, root, tags, section, path, start, end, raw` |

### Comparisons and logical conditions

```bash
./mdsql 'SELECT line, title FROM headings WHERE level = 2' note.md
./mdsql 'SELECT line, title FROM headings WHERE level == 2' note.md
./mdsql 'SELECT line, title FROM headings WHERE level != 1 AND level <= 3' note.md
./mdsql 'SELECT line, title FROM headings WHERE level <> 1 AND level >= 2' note.md
./mdsql 'SELECT line, title FROM headings WHERE level > 1 AND level < 4' note.md
./mdsql "SELECT text FROM tasks WHERE state = 'todo' OR state = 'in_progress'" note.md
./mdsql "SELECT text FROM tasks WHERE NOT (state = 'done' OR state = 'cancelled')" note.md
```

### LIKE, regular expressions, IN, and BETWEEN

```bash
./mdsql "SELECT title FROM headings WHERE title LIKE '%release%'" note.md
./mdsql "SELECT title FROM headings WHERE title NOT LIKE 'Draft%'" note.md
./mdsql "SELECT title FROM headings WHERE title LIKE 'Step _'" note.md
./mdsql "SELECT text FROM blocks WHERE text ~ 'TODO|FIXME'" note.md
./mdsql "SELECT text FROM blocks WHERE text ~* 'release'" note.md
./mdsql "SELECT text FROM blocks WHERE text !~ 'DEBUG'" note.md
./mdsql "SELECT text FROM blocks WHERE text !~* 'draft'" note.md
./mdsql "SELECT lang, code FROM code WHERE lang IN ('go', 'python')" note.md
./mdsql "SELECT text FROM tasks WHERE state NOT IN ('done', 'cancelled')" note.md
./mdsql "SELECT file, text FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE fm.status = 'active')" notes/
./mdsql "SELECT text FROM tasks WHERE section NOT IN (SELECT title FROM headings WHERE level = 1)" notes/
./mdsql 'SELECT line, title FROM headings WHERE level BETWEEN 2 AND 4' note.md
./mdsql 'SELECT line, title FROM headings WHERE level NOT BETWEEN 2 AND 4' note.md
```

In `LIKE`, `%` matches any sequence and `_` matches one character; matching is currently case-insensitive. `~` is case-sensitive and `~*` is case-insensitive. `!~` and `!~*` negate the match. `BETWEEN` includes both boundaries.

`IN` and `NOT IN` accept an uncorrelated, single-column `SELECT`. The subquery runs once over the same loaded documents before the outer rows are filtered. It may select any scalar column and use ordinary filters, grouping, ordering, and limits. References to outer-query columns, multiple selected columns, and separate source specifications inside the subquery are rejected. `--table-limit` applies only to the outer result and does not truncate the membership set. `--explain` lists each subquery as an execute-once plan step.

### NULL and tag lists

```bash
./mdsql 'SELECT text FROM tasks WHERE due IS NULL' note.md
./mdsql 'SELECT text, due FROM tasks WHERE due IS NOT NULL' note.md
./mdsql "SELECT text FROM tasks WHERE tags HAS 'work'" note.md
./mdsql "SELECT text FROM tasks WHERE tags NOT HAS 'work'" note.md
./mdsql "SELECT text FROM tasks WHERE has_tag('work')" note.md
```

Use `IS NULL`, not `= NULL`, to test for missing values. `HAS` and `has_tag()` match tags case-insensitively and treat `work/docs` as a match for `work`.

`has_tag()` uses task tags on `tasks`, inline tags in each source range on `headings`, `lines`, and `blocks`, and YAML frontmatter tags on `files` and `frontmatter`. Fenced code contributes no inline tags. This is not a whole-document inline-tag search on `files`.

```bash
./mdsql --csv "SELECT file FROM files WHERE has_tag('work')" notes/
./mdsql --csv "SELECT line, title FROM headings WHERE has_tag('work')" note.md
```

### Dates, time, and arithmetic

```bash
./mdsql 'SELECT text, due FROM tasks WHERE due = 2026-09-05' note.md
./mdsql "SELECT text, due FROM tasks WHERE due = date('2026-09-05')" note.md
./mdsql --today 2026-09-01 'SELECT text, due FROM tasks WHERE due <= today + 7d' note.md
./mdsql --today 2026-09-07 'SELECT text, due FROM tasks WHERE due BETWEEN today - 1w AND today' note.md
./mdsql 'SELECT today + 1m AS next_month, today + 1y AS next_year, now AS checked_at FROM files' note.md
./mdsql 'SELECT line, line + 1 AS next_line FROM headings' note.md
./mdsql --json 'SELECT size * 2 AS doubled, size / 1024 AS kib, size % 1024 AS remainder FROM files' note.md
./mdsql 'SELECT round(avg(size), 1) AS average_bytes FROM files' notes/
```

Unquoted `2026-09-05` is a date; `'2026-09-05'` is a SQL string. `today` and `today()` are equivalent current-date expressions; `now` and `now()` return the current timestamp. `--today` overrides only `today`. Duration units are `d` for days, `w` for weeks, `m` for months, and `y` for years. Integers support `+` and `-`.

Numeric expressions support `+`, `-`, `*`, `/`, and `%`, with parentheses and the usual multiplication-before-addition precedence. Integer multiplication and remainder retain integer results; mixed numeric operations return floats, and division always returns a float. Division or remainder by zero returns NULL and emits one deduplicated warning, or fails under `--strict`. Numeric overflow behaves the same way.

When one comparison operand is a typed date, an ISO date string on the other side is interpreted as a date. A typed timestamp accepts an RFC 3339 string with a zone, or a date-only string interpreted as midnight UTC. This applies to comparisons, BETWEEN, and IN; string-versus-string comparisons remain string comparisons. An invalid string against a date/timestamp warns, or fails under `--strict`. IN checks every member for incompatible values even when an earlier member matched.

Integer literals accept the signed 64-bit range, from `-9223372036854775808` to `9223372036854775807`. Out-of-range literals fail during SQL parsing with a byte offset, before input is read. LIMIT and OFFSET must also fit the platform's nonnegative `int` range. Arithmetic overflow is reported during evaluation.

### Sorting and limiting results

```bash
./mdsql 'SELECT text, due FROM tasks ORDER BY due ASC, text DESC LIMIT 10' notes/
./mdsql 'SELECT title FROM headings ORDER BY line LIMIT 10 OFFSET 20' note.md
./mdsql --csv 'SELECT title, line FROM headings ORDER BY 2 DESC' note.md
./mdsql --table-limit 20 'SELECT file, line, text FROM tasks' notes/
```

`ORDER BY` can reference a source column omitted from `SELECT` or a SELECT alias. `LIMIT 0` and `--table-limit 0` mean unlimited, not an empty result. Any explicit SQL LIMIT, including `LIMIT 0`, disables the default 500-row output cap; an explicit `--max-rows` still applies. `--table-limit` limits output rows after aggregation, so it does not limit the input to a global aggregate.

A standalone integer in ORDER BY selects a 1-based SELECT position after `*` expansion. Thus `ORDER BY 2 DESC` sorts by the second selected expression. Zero, negative, or out-of-range positions fail with exit 1. Integers inside expressions retain their ordinary expression meaning.

### Aggregates and GROUP BY

```bash
./mdsql "SELECT COUNT(*) FROM tasks WHERE state = 'todo'" notes/
./mdsql 'SELECT tag, COUNT(*) FROM tags GROUP BY tag ORDER BY count DESC' notes/
./mdsql --csv 'SELECT state, COUNT(*) FROM tasks GROUP BY state' notes/
./mdsql --csv 'SELECT COUNT(*) AS files, SUM(size) AS bytes, AVG(size) AS average_bytes FROM files' notes/
./mdsql --csv 'SELECT state, COUNT(due) AS dated, MIN(due) AS first_due, MAX(due) AS last_due FROM tasks GROUP BY state ORDER BY state' notes/
./mdsql --json 'SELECT coalesce(SUM(fm.estimate), 0) AS total FROM frontmatter' notes/
```

`COUNT(*)` counts rows; `COUNT(expression)` counts non-NULL values. `SUM` and `AVG` accept numbers. `MIN` and `MAX` accept numbers, or values of one consistent string, date, or timestamp type. Expression aggregates ignore NULL. With no non-NULL values, SUM/AVG/MIN/MAX return NULL and COUNT(expression) returns zero. Without GROUP BY, empty input produces one result row, with either COUNT form equal to zero; with GROUP BY, empty input produces no groups.

Integer-only SUM retains an integer result and reports overflow. Mixed numeric SUM and every AVG use float64; finite YAML floating-point values remain numbers, while YAML NaN and infinities remain strings. Incompatible aggregate values produce a warning and NULL, or exit 1 under `--strict`.

Grouping supports one column, including a SELECT alias. SELECT expressions must use that group key, aggregates, or constants; an ungrouped source column is rejected. Scalar functions can wrap aggregates, as in `coalesce(SUM(...), 0)`. ORDER BY can use aggregate aliases. Default output names are `count`, `sum`, `avg`, `min`, and `max`, with the usual suffixes for repeated names; use AS for stable names. Aggregates in WHERE, nested aggregates, and `COUNT(DISTINCT ...)` are not supported.

## Functions

The complete function registry is shown by `./mdsql schema` or `./mdsql schema --json`:

| Function | Arguments | Purpose |
| --- | --- | --- |
| `at(n)` | One integer | Test whether a source range contains a line |
| `after_heading(title[, level])` | String, optional integer 1..6 | On `lines`, test whether `n` is after the first exact heading in the same file |
| `under(title)` | One string | Match a section name or path component |
| `under_like(pattern)` | One string | Match a section or full path with case-insensitive SQL LIKE |
| `under_contains(text)` | One string | Match a case-insensitive substring of a section or path |
| `has_tag(t)` | One string | Test scoped hierarchical tag membership: task tags, inline row tags, or YAML tags |
| `lower(s)`, `upper(s)`, `trim(s)` | One string | Transform case or surrounding whitespace |
| `len(x)` | One string or list | Count string bytes or list elements |
| `lines(s[, exact])` | String, optional boolean | Count lines after trimming blank edge lines by default |
| `head(s, n[, exact])`, `tail(s, n[, exact])` | String, integer, optional boolean | Select first or last lines after trimming blank edge lines by default |
| `contains(string, substring)` | String, string | Case-sensitive substring membership |
| `contains(list, scalar)` | List, scalar | Exact list membership |
| `icontains(s, sub)` | String, string | Case-insensitive substring membership |
| `coalesce(a, b, ...)` | At least two values of any type | Select the first non-NULL value |
| `round(x[, digits])` | Number, optional integer | Round to an integer or the requested decimal places |
| `floor(x)`, `ceil(x)` | One number | Round down or up to an integer-valued float |
| `date(s)` | One string | Parse a date string |
| `basename(p)`, `dirname(p)` | One string | Extract file-path components |
| `today()`, `now()` | No arguments | Date and timestamp; also accepted without parentheses |
| `COUNT(*)` | Star | Count rows, optionally grouped by one scalar column |
| `count(x)` | One expression | Count non-NULL values |
| `sum(x)`, `avg(x)` | One numeric expression | Sum or average non-NULL values |
| `min(x)`, `max(x)` | One numeric expression or a consistent string/date/timestamp type | Minimum or maximum non-NULL value |

Function names are case-insensitive. Unknown functions fail with exit code `1` and list the available functions, including when a table has no rows. Wrong argument counts fail during SQL parsing with the function signature, before input is read. Incompatible argument types produce a deduplicated warning and NULL; `--strict` makes them execution errors (exit 1). NULL is accepted by typed arguments and generally propagates; `contains(list, NULL)` instead tests for a NULL element. SQL identifiers and aliases accept Unicode letters, for example `SELECT title AS Заголовок FROM headings`. Parse errors report a zero-based UTF-8 byte offset and the expected token.

These examples also accept a directory instead of `note.md`.

```bash
# Letter case and whitespace.
./mdsql 'SELECT lower(title) AS lower_title FROM headings' note.md
./mdsql 'SELECT upper(title) AS upper_title FROM headings' note.md
./mdsql 'SELECT trim(title) AS clean_title FROM headings' note.md

# String/list length and text line count.
./mdsql 'SELECT title, len(title) AS bytes FROM headings' note.md
./mdsql 'SELECT text, len(tags) AS tag_count FROM tasks' note.md
./mdsql 'SELECT lang, lines(code) AS line_count FROM code' note.md

# The beginning and end of a multiline value.
./mdsql 'SELECT head(code, 5) AS preview FROM code' note.md
./mdsql 'SELECT tail(code, 5) AS ending FROM code' note.md
./mdsql --csv "SELECT file, tail(raw, 2) AS recent FROM sections WHERE title = 'Log'" notes/
./mdsql 'SELECT tail(raw, 2, true) AS exact_ending FROM sections' note.md

# Case-sensitive and case-insensitive substring matching.
./mdsql "SELECT title FROM headings WHERE contains(title, 'Release')" note.md
./mdsql "SELECT title FROM headings WHERE icontains(title, 'release')" note.md

# The first non-NULL value.
./mdsql 'SELECT text, coalesce(due, scheduled, today) AS effective_date FROM tasks' note.md

# Convert a string to a date.
./mdsql "SELECT text FROM tasks WHERE due < date('2026-10-01')" note.md

# Source position and section membership.
./mdsql 'SELECT kind, raw FROM blocks WHERE at(20)' note.md
./mdsql "SELECT line, text FROM tasks WHERE under('Project')" note.md
./mdsql "SELECT line, text FROM tasks WHERE under_like('%proj%')" note.md
./mdsql "SELECT line, text FROM tasks WHERE under_contains('proj')" note.md

# Tags and file path components.
./mdsql "SELECT text FROM tasks WHERE has_tag('work')" note.md
./mdsql 'SELECT basename(file) AS name, dirname(file) AS dir FROM files' notes/
```

`len(string)` currently counts UTF-8 bytes, not characters. `coalesce` does not replace empty strings. `under()` checks a section name or its path components case-insensitively. `under_like()` matches the whole section name or whole path with case-insensitive SQL LIKE: `%` matches any sequence, `_` one character, and backslash escapes a wildcard. Use `under_contains()` for the earlier substring behavior; its `%` and `_` are literal characters.

`after_heading()` compares heading titles case-insensitively and starts after the
first exact match in each document. Pass a level from 1 through 6 to restrict the
match. It returns false outside the `lines` table.

`head`, `tail`, and `lines` discard blank or whitespace-only lines at both edges by default; internal blank lines and spaces on nonempty lines remain. Optional `exact = true` keeps the earlier split-at-every-newline behavior, including a trailing empty element. Thus `lines('')` is 0 and `lines('', true)` is 1. NULL arguments propagate NULL. These functions do not modify the underlying `raw` source fields.

## Output formats

### text: selected fields with names

```bash
./mdsql 'SELECT line, raw FROM headings WHERE level = 2' note.md
./mdsql --format text 'SELECT line, raw FROM headings WHERE level = 2' note.md
```

Single-line values are printed as tab-separated fields such as `line=2` and `raw=## Tasks`, without trailing tabs or an extra blank line between records. Every value is labelled: even a single count prints `count=2`, and each multiline string starts with its column name and a colon on a separate line, such as `raw:`. Lists and objects use compact JSON, for example `tags=["a,b","c"]`. Floating-point values use at most two decimal places with trailing zeroes removed. `NULL` defaults to `-`; use `--null STR` to choose another marker.

### grouped: shared columns and compact file paths

```bash
./mdsql --format grouped 'SELECT file, line, raw FROM headings ORDER BY file, line' notes/
```

Columns other than `file` are printed once, in selection order, as a tab-separated header. Each file starts with `@"path"`; following records contain only values. Tabs and newlines inside values use literal `\t` and `\n` escapes, with CSV quoting for the tab-separated fields. NULL cells are empty unless overridden by `--null STR`. `--max-lines` can shorten multiline values before escaping.

When this saves bytes, a shared `root="prefix/"` appears before the columns and `dir="prefix/"` appears before a run of files in one directory. An empty `dir=""` resets the directory prefix. File markers then contain only the remaining suffix. Paths are recovered by concatenating the unquoted root, current directory, and file suffix **exactly**, without path normalization. For example, the marker sequence `root="notes/"`, `dir="project/"`, `@"a.md"` represents `notes/project/a.md`. Root and directory prefixes are optional; absent prefixes are empty. Prefix and file markers use quoted-string escaping.

Groups follow first appearance in the SQL result, and rows retain their order within each group. This can change display order between files. Select a string column named `file`; omitting or renaming it is an error, including with an empty explicit projection. Valid empty results print nothing. Selecting only `file` prints only path markers. `--no-file` is accepted as a compatibility no-op because SELECT controls whether `file` is present; `--no-header` remains incompatible with grouped output.

### CSV: export with a header

```bash
./mdsql --csv 'SELECT line, raw FROM headings WHERE level = 2' note.md
./mdsql --format csv 'SELECT line, raw FROM headings WHERE level = 2' note.md
./mdsql tables --csv note.md > tables.csv
```

The header preserves explicit SELECT order; `SELECT *` uses the table's schema order. Commas, quotes, and newlines use standard CSV quoting. An empty result prints no header. NULL defaults to an empty cell, while a literal `-` remains `-`; `--null STR` overrides the NULL marker. An empty string and NULL are indistinguishable without a distinct marker or JSON. Lists and objects serialize as complete JSON inside each cell, preserving nested values; parse the CSV cell first, then decode its JSON. The same nested-value encoding is used in TSV, grouped, and text output.

Add `--no-header` to omit the header while preserving every data row. This flag
is supported only for CSV and TSV.

### JSON and JSON Lines

```bash
./mdsql --json 'SELECT line, raw FROM headings' note.md
./mdsql -f json 'SELECT line, raw FROM headings' note.md
./mdsql --format jsonl 'SELECT file, line, text, due FROM tasks' notes/ > tasks.jsonl
```

`json` prints an array of objects; `jsonl` prints one object per line. Nested objects and lists retain their structure, floating-point values retain native precision, and `NULL` becomes `null`. The CLI rejects `--null STR` for JSON/JSONL; that flag is available only for text, grouped, CSV, and TSV. With `*`, top-level object keys use the table schema order; explicit non-wildcard projections retain alphabetical JSON key order.

### TSV

```bash
./mdsql --format tsv 'SELECT line, raw FROM headings' note.md
./mdsql --tsv --no-header 'SELECT line, raw FROM headings' note.md
```

Columns are tab-separated in SELECT order; `SELECT *` uses schema order. Newlines and tabs inside values are represented as `\n` and `\t`, with CSV-style quoting. NULL defaults to an empty cell and can be overridden by `--null STR`. `--tsv` is equivalent to `--format tsv`; the last format selector wins. `--no-header` omits the column names without changing the data-column order.

### Markdown, plain, outline

```bash
# Original Markdown without key=value labels.
./mdsql --format md 'SELECT raw FROM headings WHERE level = 2' note.md
./mdsql section 'Examples' --format md note.md
./mdsql tables --format md note.md

# Text content only.
./mdsql --format plain 'SELECT raw FROM headings WHERE level = 2' note.md
./mdsql code --lang go --format plain note.md

# Heading hierarchy.
./mdsql toc --format outline note.md
```

`md` prints selected fields named `raw`, `body`, `own`, or `code`; select one of them for predictable output. `plain` prints every selected value in SELECT order, separated by tabs, followed by a newline for each result. String contents remain literal, nested lists and objects use JSON, and floats use the same compact two-decimal rendering as text. CSV, TSV, and grouped output keep full floating-point precision. `outline` expects a projection containing `level` and `title`.

## Limits, errors, and automation

```bash
# Limit displayed lines in a multiline value in text output.
./mdsql --max-lines 5 code note.md
./mdsql -n 5 code note.md

# Maximum regular input file size in bytes: 32 MiB here.
./mdsql --max-file-size 33554432 headings notes/

# Limit output rows, or explicitly request all result rows.
./mdsql --max-rows 100 --csv headings notes/
./mdsql --max-rows 0 --json headings notes/

# Strict input handling and warning suppression.
./mdsql --strict headings notes/
./mdsql --quiet headings notes/

# Return a nonzero exit code if no tasks match.
./mdsql --fail-empty tasks --open notes/
```

`--max-lines` controls how multiline values are displayed in text output, not which SQL rows are selected. Use `LIMIT` or `--table-limit` to limit result rows.

`--max-rows` defaults to 500 when SQL has no explicit LIMIT; `0` disables it.
An explicit SQL LIMIT, including `LIMIT 0`, disables the default output cap.
An explicit `--max-rows N` overrides that exemption and caps the result after
filtering, aggregation, DISTINCT, SQL OFFSET/LIMIT, and `--table-limit`.
When capped, stderr reports `truncated, N of M rows`, where M is the result size
after those SQL and table limits. The message remains visible with `--quiet`,
and stdout stays in the selected format. Truncation exits `0` normally or `4`
with `--strict`, after writing the partial result. This limit bounds output,
not input scanning or the rows needed for final ordering.

### Memory use

Ordinary queries parse and execute one document at a time. Once its table rows
have been filtered, mdsql retains copies of the selected values and any keys
needed for sorting or grouping, then releases the parsed document and source
text. Aggregate queries likewise retain extracted row values rather than whole
documents. Queries containing `IN (SELECT ...)` currently use the compatible
batch path because the outer query and subquery share the same parsed inputs.

Use the committed benchmark to compare a candidate binary with a baseline on a
representative corpus. It suppresses query contents and reports exit status,
row and byte counts, a SHA-256 digest, and peak RSS; it fails if stdout differs
or candidate RSS exceeds 100 MiB:

```bash
scripts/benchmark_memory.sh /path/to/baseline ./mdsql /path/to/notes
```

On the acceptance vault, the task query fell from 406,700,032 to 37,634,048 bytes peak RSS with identical 1,061-row stdout; the aggregate count path used 54,738,944 bytes. Corpus shape and platform affect these figures, so use the script for local comparisons.

Regular files are limited to 16 MiB by default. This limit currently does not apply to stdin. Without `--strict`, some read errors become warnings; successful partial processing is not guaranteed when resolving the input path list fails. `--quiet`/`-q` suppresses warnings, not query results.

| Exit code | Meaning |
| --- | --- |
| `0` | Success; an empty result is also successful without `--fail-empty` |
| `1` | Argument, SQL, execution, or output error |
| `2` | Input/Markdown diagnostic error under strict mode, empty stdin under strict mode, or no documents loaded |
| `3` | Empty result with `--fail-empty` |
| `4` | Output truncated by `--max-rows` with `--strict` |

A shell script example:

```bash
if ./mdsql --fail-empty --csv tasks --open notes/ > open-tasks.csv; then
  printf '%s\n' 'Tasks exported'
else
  result_code=$?
  case "$result_code" in
    3) printf '%s\n' 'No matching tasks' ;;
    *) exit "$result_code" ;;
  esac
fi
```

## Optional parse cache

```bash
./mdsql --cache --csv 'SELECT file, headings FROM files' notes/
./mdsql --no-cache --csv 'SELECT file, headings FROM files' notes/
./mdsql --cache-info
./mdsql --cache-info --json
./mdsql --cache-info --csv
./mdsql --cache-info --tsv --no-header
./mdsql --cache-info --format plain
./mdsql --cache-info --format jsonl
./mdsql --clear-cache
```

The parse cache is disabled by default. `MDSQL_CACHE=1` or `--cache` enables it;
`--no-cache` disables both reads and writes, overriding the environment. The
last explicit cache flag wins. Stdin is never cached. Metadata-only `files`
queries bypass the parse cache because they need only stat information.

`--cache-info` reports entry count and on-disk bytes without reading payloads.
Text uses `entries=N bytes=N`; JSON and JSONL emit one object; CSV and TSV emit
`entries, bytes` columns and support `--no-header`; plain separates both values
with a tab. Markdown, outline, and grouped formats are incompatible and exit `1`.
A missing cache reports zeros without creating a directory. `--clear-cache`
clears mdsql entries and exits silently. Both commands run separately from a
query or input files.

On Linux and macOS, an absolute `XDG_CACHE_HOME` selects `XDG_CACHE_HOME/mdsql`.
When the variable is unset, mdsql uses the operating system's user cache directory
plus `mdsql`. A relative value is rejected.
An invalid or unavailable location disables caching with a warning. Windows
currently runs queries without caching. The cache directory is private
(`0700`), with `0600` files. Cache entries contain complete document text and
metadata; see [SECURITY.md](SECURITY.md).

Cache validity depends on the absolute source path, modification time, size,
parser version, and cache format version. Missing or changed source files and
corrupt entries are not reused. Renaming a file gives it a new cache slot; old
slots remain until explicitly cleared. **Changes preserving both size and
modification time can produce stale results.** Use `--no-cache` when this matters.
Cache failures fall back to normal parsing, including with `--strict`.

The versioned cache compresses entries and restores redundant line/path views
from stored content instead of storing each copy independently. Compression is
not encryption: entries still contain source text and metadata. Disk use and
speed depend on the corpus; use `--cache-info` to inspect actual stored bytes.

## Current limitations

- No `JOIN`, correlated or scalar subqueries, multiple SQL statements, or `DELETE`. Uncorrelated single-column subqueries are supported only as the right side of `IN` and `NOT IN`. INSERT and UPDATE support the explicit targets in [Writing Markdown](#writing-markdown).
- Markdown parsing is simplified: ordinary lines may become separate paragraphs, `lists` and `items` expose the same list blocks, and table variants without a leading `|` are not recognized. Use `cells` for cells in recognized tables.
- Link extraction is not a complete CommonMark inline parser. It excludes fenced code and prevents overlapping duplicate matches, but repeated occurrences of the same URL remain separate rows. Use `SELECT DISTINCT target FROM links` for unique targets.
- Dynamic task `fields.*` values require explicit projections; wildcard output follows the declared schema. This is not a general SQL implementation.
- Unknown functions and invalid argument counts are errors; incompatible argument types are diagnosed. Some invalid regular expressions or string values can still return NULL/false without a diagnostic.
- Unknown flags are rejected; use `--explain` to inspect an alias query before executing it.
- `--with-file` and `--no-file` are accepted, but they no longer add or remove a column automatically. To include the source path, use `SELECT file, ...`; otherwise omit `file`.
- `--explode-frontmatter` currently has no effect. SQL `WITH` options are parsed but not applied; use the documented CLI flags instead.

After changing functionality, run `go test ./...` and rebuild `mdsql`: executing `./mdsql` runs the compiled binary, not the Go source files.

## Contributing and security

- [CONTRIBUTING.md](CONTRIBUTING.md): development commands and contribution guidance.
- [SECURITY.md](SECURITY.md): safe handling of documents and vulnerability reporting.
- [skills/mdsql/SKILL.md](skills/mdsql/SKILL.md): portable agent instructions.
- [LICENSE](LICENSE): MIT License.

CI is configured for formatting, vet, tests, and builds on Linux, macOS, and
Windows, with an additional race-detector run on Linux. Dependabot is configured
for Go modules and GitHub Actions.

## Text matching and frontmatter lists

These behaviors are covered by synthetic regression tests.

- LIKE matches across line breaks. Its percent wildcard matches zero or more
  characters, and its underscore wildcard matches one character, including a
  newline. Existing case-insensitive matching is preserved.
- Escape literal wildcard characters as `\%` and `\_`; `\\` matches a
  literal backslash. Other backslashes are preserved.
- `contains(string, string)` is a case-sensitive substring search.
- `contains(list, scalar)` tests exact, type-sensitive membership, not substring
  matching within each list element. An empty list returns false.
- Object/list needles and array indexing are not implemented.
- `frontmatter_values` exposes top-level metadata values as rows and expands
  lists by one level; it does not recursively flatten nested values.

```bash
# Multiline source matching.
./mdsql --csv "SELECT file FROM frontmatter WHERE raw LIKE '%Projects%'" /path/to/notes

# Exact membership in a frontmatter list.
./mdsql --csv "SELECT file FROM frontmatter WHERE contains(fm.categories, '[[Projects]]')" /path/to/notes
```

## Diagnostics, grouping, and CLI updates

The following contracts apply to missing fields, grouping aliases, and content-format validation.

- Missing `fm.*` values become NULL. An ordinary WHERE expression warns only
  when that metadata path is absent from every loaded document; a path observed
  even once, including explicit YAML null or a document with no queried-table
  rows, is treated as optional elsewhere. A path absent from all loaded documents
  still produces a deduplicated warning; `--strict` makes it an error.
- Missing optional fields remain NULL in SELECT, GROUP BY, and ORDER BY. Use
  `IS NULL`, `coalesce`, or a short-circuiting guard to handle intentional absence.
  An explicit YAML null is NULL, not the string "<nil>".
- Incompatible comparisons produce deduplicated warnings, or errors under
  `--strict`. `-q`/`--quiet` suppresses warnings without hiding query errors,
  strict failures, or truncation notices, and does not change their exit codes.
- Scalar-versus-list comparisons do not imply membership. Use
  `contains(list, scalar)`; incompatible comparisons otherwise evaluate false.
- Unknown SELECT columns remain errors. Unknown base columns in WHERE are
  diagnosed even with no rows. When a frontmatter key was observed, an unknown
  unqualified field such as `status` receives a `fm.status` suggestion.
- GROUP BY accepts one or more comma-separated scalar columns and supports
  SELECT aliases. HAVING filters aggregate groups and accepts aggregate aliases,
  including aliases of COUNT(*). ORDER BY supports SELECT aliases as before.
  WHERE supports non-aggregate aliases, with source columns taking precedence.
  Group values retain their types and NULL stays NULL.
- `LIMIT N PER column[, ...]` retains at most N rows for each distinct scalar
  key after ORDER BY. This makes per-file top-N queries deterministic. OFFSET is
  rejected with LIMIT PER. Explicit `--table-limit` and `--max-rows` caps apply
  after the per-group selection.
- Nested frontmatter objects are preserved and accessible through dotted paths.
- `outline` requires `title` as a string and `level` as an integer from 1 to 6.
  `md` requires string content in at least one of `raw`, `body`, `own`, or
  `code`. Invalid selections fail before output, with no silent fallback.
  Projection validation also applies to empty results; wildcard selections
  expand the declared schema before validation.
- `version`, `--version`, and `-V` are equivalent. The first help/version flag
  encountered takes precedence. Unknown flags are reported separately from
  unknown aliases.

```bash
./mdsql --version
./mdsql --csv "SELECT fm.stage AS s, COUNT(*) AS n FROM frontmatter GROUP BY s ORDER BY n DESC, s" /path/to/notes
./mdsql --csv "SELECT fm.stage AS s, fm.owner AS o, COUNT(*) AS n FROM frontmatter GROUP BY s, o HAVING n > 1 ORDER BY n DESC" /path/to/notes
./mdsql --format plain "SELECT text FROM lines WHERE after_heading('Log') AND text LIKE '- %' AND file IN (SELECT file FROM lines WHERE kind = 'frontmatter' AND contains(text, '[[Projects]]')) ORDER BY file, n DESC LIMIT 2 PER file" /path/to/notes
./mdsql --strict --csv "SELECT file FROM frontmatter WHERE coalesce(fm.status, '') = 'active'" /path/to/notes
```

The `query` subcommand accepts a SQL argument beginning with a `--` comment;
include a newline before SELECT in the same argument. Prefix dash-leading
filenames with a path, for example `./-note.md`.

## Versioning and changes

The current version is stored in [`VERSION`](VERSION) and embedded in the executable:

```bash
mdsql --version
mdsql -V
```

See [`CHANGELOG.md`](CHANGELOG.md) for release notes and known limitations. Update `VERSION` and the changelog together for a new release, then rebuild the executable.

## Writing Markdown

INSERT and UPDATE modify the one existing file supplied after SQL. They write
immediately; `--dry-run` shows a unified diff without modifying the file or
creating temporary/lock files. There is no required `--write` flag. SELECT remains
read-only. Write operations bypass the read cache.

```bash
# Preview an insertion before physical line 18.
./mdsql "INSERT INTO lines (line, text) VALUES (18, 'A new line')" note.md --dry-run

# Append to a section's own text, before any child headings.
./mdsql "INSERT INTO lines (section, text) VALUES ('Log', '- Tests passed')" note.md

# Continue the last task list in a section, start a separate list, or append a child.
./mdsql "INSERT INTO tasks (section, text, state) VALUES ('Next', 'Prepare release', 'todo')" note.md
./mdsql "INSERT INTO tasks (section, text, due, priority, tags) VALUES ('Next', 'Ship', '2026-09-07', 4, 'work, release')" note.md
./mdsql "INSERT INTO tasks (section, text, position) VALUES ('Next', 'Separate item', 'section_end')" note.md
./mdsql "INSERT INTO tasks (parent_line, text) VALUES (18, 'Write changelog')" note.md

# Insert a section or a fenced code block.
./mdsql "INSERT INTO sections (line, level, title, body) VALUES (25, 2, 'Results', 'Tests passed.')" note.md
./mdsql "INSERT INTO code (section, lang, code) VALUES ('Examples', 'bash', 'go test ./...')" note.md

# Change a checkbox without rewriting the task content.
./mdsql "UPDATE tasks SET state = 'done' WHERE line = 18" note.md --expect-rows 1
./mdsql "UPDATE tasks SET status = 'x' WHERE state = 'todo'" note.md --dry-run
./mdsql "UPDATE tasks SET text = 'Ship release', due = NULL, tags = 'done' WHERE line = 18" note.md

# Add a missing scalar YAML key, append to a list, or change a value conditionally.
./mdsql "INSERT INTO frontmatter_values (key, value) VALUES ('stage', 'implement')" note.md
./mdsql "INSERT INTO frontmatter_values (key, value) VALUES ('tags', 'mdsql')" note.md
./mdsql "UPDATE frontmatter_values SET value = 'verify' WHERE key = 'stage' AND value = 'implement'" note.md --expect-rows 1
```

`--json` returns a structured operation report. `--root DIR` resolves relative
file arguments. Directories, stdin, and multiple files are not write
sources; glob patterns are never expanded. An existing literal filename such as
`[draft].md` is valid. SELECT-only filtering, cache, limit, and output flags are not write
options. SQL accepts a single VALUES tuple for INSERT or literal assignments
for UPDATE; multiple statements and INSERT SELECT are unsupported. A single
trailing semicolon is accepted for writes. Escape an SQL apostrophe with `''`.

### Writable targets

| Target | INSERT inputs | UPDATE inputs |
| --- | --- | --- |
| `lines` | Nonempty `text`, possibly multiline, and optional position | Unsupported |
| `tasks` | Single-line `text`; optional `state`/`status`, `due`, `scheduled`, `priority`, `tags`, position selector, and `position='section_end'` | `state`, `status`, `text`, `due`, `scheduled`, `priority`, `tags` |
| `sections` | `level` (1-6), nonempty one-line `title`, optional `body`, and position | Unsupported |
| `code` | `code`, optional `lang`, and position | Unsupported |
| `frontmatter_values` | Absent top-level `key`, or existing list `key`, and scalar `value`; no position | Scalar or selected list-item `value` |

`schema` describes these inputs separately from read columns. Positions, paths,
counts, dates derived from task text, and other projected columns are not
implicitly writable. INSERT and UPDATE use the same task state names:
`todo`, `done`, `in_progress`, `cancelled`. `status` instead supplies the checkbox
character: space, `x`, `X`, `/`, or `-`. Do not specify both; an INSERT task defaults to todo.

Task writes use canonical Obsidian Tasks metadata: `due` and `scheduled` accept
ISO date strings and emit `📅 YYYY-MM-DD` and `⏳ YYYY-MM-DD`; priority 1..5 emits
`⏬`, `🔽`, `🔼`, `⏫`, or `🔺`; a comma- or whitespace-separated tag string emits
canonical `#tag` tokens. UPDATE preserves marker, checkbox, indentation, line
ending, and metadata fields it does not target. NULL clears these four metadata
fields; empty strings are errors. A targeted date with existing `[due:: ...]` or
`[scheduled:: ...]` metadata is rejected instead of mixing formats. Text-only
updates preserve such metadata suffixes byte-for-byte.

For frontmatter, INSERT creates an absent key or appends to an existing YAML
list. Re-appending the same typed value is a no-op. An existing scalar or object
is an error. UPDATE changes matching scalar keys or selected scalar list items
and never creates missing keys. Keys are literal top-level names; a dot does not
traverse YAML.
Strings, finite numbers, booleans, and NULL retain their YAML types. NULL means
YAML null, not key deletion. Block and one-line inline lists are supported;
nested or multiline list items are not writable. YAML objects cannot be replaced
through this interface. Unsupported complex YAML constructs are rejected rather
than rewritten with formatting loss. Table-row insertion, new-file creation,
DELETE, and UPSERT are deferred.

New plain-safe keys and string values are written without quotes. UPDATE keeps
the original single- or double-quote style of strings. Values that plain YAML
would parse differently, such as dates, numeric strings, wikilinks, empty
strings, and leading `#` or `[`, are double quoted.

Body insertions reject physical positions inside frontmatter, including line 1.
Every insertion is parsed before replacement, so a result with invalid YAML is
not written.

### Positions and matching

For body INSERT, provide at most one of `line`, `section`, `section_path`,
`heading_line`, or `parent_line` (tasks only). Omission appends at EOF.

- `line=N` inserts before the existing physical line N. Existing lines shift
  down by the number of inserted lines. N is 1-based and may be the line count
  plus one to append. An empty file has zero lines and accepts line 1. A final
  line ending does not add a phantom line. Invalid positions are errors.
- `lines` inserts logical lines, not a byte concatenation: appending to an
  unterminated last line adds the necessary separator. Original BOM and line
  endings are preserved; new text follows local line-ending style.
- `section` matches one exact heading title, `section_path` one exact heading
  path, and `heading_line` the first source line of the heading. Even identical
  full paths are ambiguous and rejected. Insertion is before trailing blank
  lines in the section's own text, before any child heading.
- A task inserted with a section selector continues the last unquoted task list
  in that section's own text, preserving its indentation, marker, spacing, and
  line endings. If there is no safe list to continue, it uses the normal section
  end. Add `position='section_end'` with the section selector to always create a
  separate list at the section end.
- `parent_line` appends the last child after the parent's existing subtree.
  Structural INSERT into unsupported containers or protected Markdown blocks
  fails instead of silently creating the wrong entity. `lines` is the direct
  line-editing escape hatch and can intentionally change Markdown structure.
- Inserting a heading can reparent following sections. That is the normal
  Markdown interpretation of the resulting text, not a promise of unchanged
  section hierarchy. Code fences are chosen so content cannot close them early.

UPDATE requires WHERE and changes all matches in the one input snapshot; a
zero-match update succeeds with a zero count. `--expect-rows N` requires exactly
N matches, including unchanged values, in both normal and dry-run modes. A
mismatch fails before writing. It is independent of SELECT output limits.
All positions and predicates refer to the original snapshot, so earlier edits
do not shift the targets of subsequent replacements in the same operation.
The `file` column in WHERE uses the same logical identity as SELECT with the
same positional input and `--root`; filesystem symlink aliases are not silently
collapsed into a different query identity.

### Write reports and guarantees

Reports distinguish `matched` from `changed` and include `applied`,
`base_sha256`, and `result_sha256`. To bind a reviewed preview to its source,
pass `--expect-sha256 HASH` when applying it. Without this option each command
plans against the current file. Repeating INSERT adds another entity; it is
not implicitly idempotent.

Edits preserve original bytes outside the changed spans. The writer uses a
temporary file in the same directory and atomic replacement, with source
identity, metadata, and content checks before replacement. These checks detect
observed conflicts; they are not a transaction with an external editor, which
can race the final check. Avoid simultaneous edits to the same file.

Write commands return 0 for a successful operation or preview, and 1 on error.
A failure after replacement may report `applied=true`; inspect this before
retrying an INSERT. Symlink write targets and unsupported file types are
rejected. Actual writes are supported on macOS/Linux and supported Unix targets;
other platforms, including Windows, allow preview only. Supported guarantees
concern ordinary local files, not multi-file transactions.
