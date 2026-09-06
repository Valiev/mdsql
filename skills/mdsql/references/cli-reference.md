# mdsql CLI reference

Read only the section needed for the current request. `mdsql --help`, `mdsql
schema`, and `mdsql schema --json` are the authoritative reference for the
installed binary.

## Input sources

Invoke a query as `mdsql '<SQL>' [files...]`; `mdsql query '<SQL>'` is
equivalent. A source may be a Markdown file, multiple files, a recursively
scanned directory, a quoted glob, or stdin when no file is supplied. Quote
paths and globs. Directories skip hidden entries. File and directory symlinks
are followed while output keeps the logical source path.

`--root DIR` defines the base for relative paths and repeated `--include` and
`--exclude` filters. Patterns are root-relative: `*` stays in one path component
and `**` spans directories. A source clause such as
`FROM tasks IN 'notes/**/*.md'` overrides positional files.

With stdin, `--stdin-name NAME` or `MDSQL_STDIN_NAME` controls the `file` value.
Zero-byte stdin warns and exits 2 under `--strict`. SELECT accepts no trailing
semicolon. INSERT and UPDATE accept one optional trailing semicolon and exactly
one existing file.

## Projection and tables

mdsql supports one SELECT from one virtual table with expressions, `WHERE`,
`DISTINCT`, `GROUP BY`, `HAVING`, `ORDER BY`, `LIMIT`, `LIMIT N PER file`, and
`OFFSET`. Use `mdsql schema --tables` for current columns and types.

Common tables:

| Table | Purpose and useful location columns |
| --- | --- |
| `files` | File metadata and document counts; `file`, `size`, `mtime` |
| `frontmatter`, `frontmatter_values` | Whole YAML values or one row per scalar/list item; `file`, `key` |
| `headings`, `toc` | Headings and hierarchy; `file`, `line`, `path` |
| `sections`, `section` | Section source/body; `file`, `start`, `end`, `path` |
| `tasks` | Checkbox tasks and metadata; `file`, `line`, `state`, `tags` |
| `lines` | Physical source lines; `file`, `n`, `kind`, `path` |
| `code`, `blocks` | Fenced code or parsed blocks; `file`, `start`, `end` |
| `tables`, `cells` | Whole Markdown tables or individual cells; `file`, `table`, `row`, `col` |
| `links`, `tags` | Link/image and tag occurrences with source positions |

`SELECT *` follows the declared schema order and omits the potentially large
`fm` object except on `frontmatter`; select `fm` or `fm.key` explicitly when
needed. Explicit projections preserve SELECT order in CSV, TSV, grouped, and
plain output. Include `file` and a line/range column when attribution matters.

`frontmatter_values` expands one list level. Use `COUNT(*)` for occurrences and
`SELECT DISTINCT file` for documents. `sections.body` excludes the heading;
`sections.own` also excludes child sections; `sections.raw` preserves the whole
source section. `cells.is_header` distinguishes header cells.

An uncorrelated single-column `IN (SELECT ...)` subquery runs once on the same
documents. It cannot refer to outer columns or provide separate files. JOIN,
correlated/scalar subqueries, multiple statements, and DELETE are unsupported.

## Functions and operators

Use `mdsql schema` for the complete registry. Frequently useful functions are:

- `under(title)`, `under_like(pattern)`, `under_contains(text)`,
  `after_heading(title[, level])`, and `at(line)` for source position;
- `has_tag(tag)` for scoped hierarchical tag membership;
- `contains(string, substring)` and `contains(list, scalar)` for exact,
  case-sensitive membership; `icontains` for case-insensitive text;
- `lower`, `upper`, `trim`, `len`, `lines`, `head`, `tail`, `coalesce`,
  `basename`, and `dirname`;
- `date`, `today`, `now`, `round`, `floor`, and `ceil`;
- `COUNT`, `SUM`, `AVG`, `MIN`, and `MAX`, with NULL ignored by expression
  aggregates.

Conditions include typed comparisons, `IS NULL`, `LIKE`, regular-expression
matching, `IN`/`NOT IN`, `BETWEEN`, boolean operators, list `HAS`, and numeric
`+ - * / %`. `LIKE` uses `%` for any sequence and `_` for one character.
Function/type errors become query errors under `--strict`; ordinary bad typed
arguments otherwise warn and yield NULL.

## Output formats

- `--csv` and `--tsv` preserve projection order, include a header unless
  `--no-header`, and encode nested values as JSON cells.
- `--json` emits an array and `--format jsonl` emits one object per line. Nested
  values and typed nulls are preserved.
- `--format plain` prints all selected values as tab-separated rows.
- `--format text` prints labelled fields and is useful for inspection.
- `--format md` emits selected `raw`, `body`, `own`, or `code` source fragments.
- `--format outline` expects `level` and `title`.

NULL is empty in CSV/TSV, JSON `null` in JSON formats, and `-` in text. Use
`--null STR` with text, grouped, CSV, or TSV when NULL must be distinct.

### Grouped output

Grouped output requires a selected string column named `file`. It prints other
columns once as a tab-separated header, then starts each file with `@"path"`.
Values escape literal tabs/newlines. Optional `root="prefix/"` and
`dir="prefix/"` markers compact common paths; recover a path by concatenating
root, current directory, and file suffix without normalization. Groups follow
the first SQL-result appearance, so files may be reordered. `--no-header` is
incompatible with grouped output.

## Limits, errors, and exit codes

Without SQL LIMIT, output is capped at 500 rows. Any explicit LIMIT removes that
default; explicit `--max-rows N` still wins, and `--max-rows 0` is unlimited.
Truncation is reported on stderr and exits 4 under `--strict`. `--max-lines`
shortens displayed multiline fields; it does not limit SQL rows or parsing.

Exit codes are 0 for success, 1 for argument/SQL/execution/output errors, 2 for
input or Markdown diagnostics under strict mode (and no loaded documents), 3
for an empty result with `--fail-empty`, and 4 for strict truncation. `-q`
suppresses warnings but not query errors, strict failures, or truncation notices.

## Cache and memory

The parse cache is off by default and contains source text. Enable it with
`--cache` or `MDSQL_CACHE=1`; `--no-cache` wins and is appropriate when source
freshness or confidentiality matters. Stdin is never cached. Inspect or clear
it with `--cache-info` and `--clear-cache`.

Ordinary queries stream parsed documents, retaining projected values and sort or
group keys. Subqueries use a batch path. Output limits do not bound parsing,
sorting, or aggregation memory; select only needed columns and prefer scoped
inputs.

## Writing Markdown

INSERT and UPDATE modify one existing file immediately. Preview with
`--dry-run`, inspect the unified diff and JSON report, then apply the same SQL
with `--expect-sha256 HASH` when binding the write to reviewed contents.
`--expect-rows N` guards UPDATE match counts. Reports distinguish `matched` and
`changed` and include `applied`, `base_sha256`, and `result_sha256`.

Writable targets are:

| Target | Operations |
| --- | --- |
| `lines` | INSERT nonempty text at a line, section, heading, or EOF |
| `tasks` | INSERT tasks; UPDATE state/status, text, due/scheduled, priority, tags |
| `sections` | INSERT a level 1..6 heading with optional body |
| `code` | INSERT a safe fenced block with optional language |
| `frontmatter_values` | INSERT a key/list item; UPDATE matching scalar/list items |

Body INSERT accepts at most one of `line`, `section`, `section_path`,
`heading_line`, or task-only `parent_line`. Section insertion uses the end of
the section's own text. A section task continues the last safe unquoted list;
`position='section_end'` starts a separate list. Ambiguous or protected
positions fail.

Task INSERT defaults to todo. State names are `todo`, `done`, `in_progress`, and
`cancelled`; `status` accepts the literal checkbox character instead. Task date,
priority, and tag fields use canonical Obsidian Tasks syntax. NULL clears those
metadata fields on UPDATE.

Frontmatter INSERT creates an absent top-level scalar key or appends a unique
typed scalar to an existing list. UPDATE changes matched scalar values or list
items and preserves string quote style. Nested/multiline list items and YAML
objects are rejected to avoid formatting loss.

Writes preserve bytes outside changed spans and use same-directory temporary
files plus atomic replacement with conflict checks. Symlinks, multiple files,
directories, stdin, and unsupported file types are rejected. Write failures
can still report `applied=true` after replacement; inspect the report before
retrying a non-idempotent INSERT.
