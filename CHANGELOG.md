# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

### Added

- Single-file INSERT into lines, tasks, sections, code, and scalar frontmatter
  values; UPDATE task checkboxes and scalar frontmatter values with WHERE.
  Writes apply immediately; `--dry-run` previews a diff. Physical-line insertion
  shifts existing lines, while section and parent-task selectors choose semantic
  positions. `--expect-rows` and `--expect-sha256` support checked edits.
- Byte-preserving edits, atomic file replacement, optimistic source-conflict
  detection, and a separate writable-field schema alongside read projections.

- ORDER BY column positions and non-aggregate SELECT aliases in WHERE, with
  source columns taking precedence over colliding aliases.
- Scoped `has_tag()` support for inline heading/line/block tags and YAML tags on
  files/frontmatter, alongside existing task tags.
- `COUNT(expression)`, `SUM`, `AVG`, `MIN`, and `MAX`, with NULL handling,
  one-column grouping, aliases, and scalar functions around aggregate results.
- `frontmatter_values(file, key, value)` with one-level list expansion, and
  `cells(file, table, row, col, header, value, is_header)` for Markdown table cells.
- Tasks inside blockquotes and callouts, with `tasks.container` attribution.
- `tables.rows` and `tables.columns` for data-row counts and header width.
- Parenthesized, quoted source lists after `FROM ... IN`.
- `links.source` and `links.key` to distinguish body links from YAML wikilinks.
- `--tsv`, `--no-header` for CSV/TSV, and `--null STR` for text, grouped, CSV,
  and TSV output; JSON retains typed null.
- `--max-rows` output limits, with an explicit SQL LIMIT disabling the default
  500-row cap and an explicit `--max-rows` taking precedence. Truncation under
  `--strict` returns exit code 4 after writing the partial result.
- Private parse caching through `--cache` or `MDSQL_CACHE=1`, overridden by
  `--no-cache`; `--clear-cache` and `--cache-info` manage or inspect entries.
- A complete function registry in `schema`, errors for unknown functions,
  Unicode SQL identifiers, `today()`/`now()` aliases, and positioned parse errors.

### Changed

- CSV, TSV, grouped, and text retain nested values as JSON; text labels every
  value, including counts and multiline fields.
- `head`, `tail`, and `lines` trim blank edge lines by default; an optional boolean
  exact mode preserves the previous newline-splitting behavior.
- Finite YAML floating-point values remain numeric in query results.
- The bundled agent skill is limited to 7000 bytes, with focused README links and
  a reproducible synthetic recipe verifier at `scripts/check_skill_recipes.py`.
- Schema output has short, single-line descriptions for every column.
- `under_like()` now applies case-insensitive SQL LIKE patterns; `under_contains()`
  retains the earlier literal substring behavior.
- `grouped` prints one column header and tab-separated values with file markers
  and optional shared root/directory prefixes. Text output omits trailing tabs
  and extra blank record separators.
- `SELECT *` expands schema columns in schema order and excludes `fm` except
  from `frontmatter`; metadata and dynamic task fields remain explicitly selectable.
- Metadata-only `files` queries can use stat without parsing Markdown. Such
  queries do not validate document contents, including under `--strict`.
- Parse-cache entries are compressed and omit reconstructible line/path copies.
- CSV, TSV, and grouped NULL values default to empty cells; text keeps `-`.
- Include and exclude filters share root-relative `*`/`**` matching rules.

### Fixed

- Existing literal source paths containing glob characters take precedence over
  pattern expansion, allowing the same filename for SELECT and write operations.

- Unquoted blank lines separate quotes and callouts; quoted blank lines stay inside.
- Physical line counts no longer include a trailing phantom line, and empty or
  frontmatter-only documents have no invalid root section.
- Indented task list items expose the same nesting flag as ordinary items.
- Invalid roots and date flags fail as argument errors before query execution.
- Unicode-equivalent source paths resolve consistently and display in NFC.
- Indexed section attribution avoids repeated full-section scans on heading-heavy input.
- A leading UTF-8 BOM no longer prevents frontmatter or first-line headings from parsing.
- Repeated automatic projection names receive unique suffixes; duplicate explicit
  aliases fail instead of silently overwriting output values.
- File and directory symlinks retain logical source paths; directory cycles warn
  and stop that traversal branch, with strict input errors preserved.
- Function argument counts are validated before input is read, and incompatible
  types yield diagnostics instead of implicit conversions.
- Optional frontmatter paths warn only when absent from every loaded document.
- Typed dates and timestamps compare with compatible ISO strings, including in
  BETWEEN and IN; invalid members retain warning/strict diagnostics.
- Combined `code --lang` and `--section`/`--under` filters now form one valid
  query; repeated languages remain alternatives.
- Task tags are extracted once per source occurrence while retaining repeats.
- DISTINCT row keys preserve types and nested values without delimiter collisions.
- Integer literals outside int64 and LIMIT/OFFSET values outside platform bounds
  fail during parsing; both signed 64-bit endpoints remain valid literals.
- SQL arguments beginning with a `--` comment in the `query` command.
- TSV serialization of `level`/`title` projections and output writer errors.
- Invalid delimited YAML diagnostics and body isolation; strict parsing exits 2.
- Empty stdin now warns and exits 2 with `--strict`.
- Overlapping URL extraction no longer duplicates one occurrence; bare URLs
  discard trailing punctuation and unmatched closing parentheses.
- Longer backtick and tilde code fences isolate nested content from body tables.
- Section hierarchy, direct-child counts, depth, and `raw`/`body`/`own` ranges.
- YAML date-only values now use ISO dates and date comparisons; recognized
  datetimes preserve time/offset in RFC 3339 output.
- Agent recipes and reference documentation for the updated query, parsing,
  output, and cache contracts.

## [0.1.0] - 2026-09-02

### Added

- CLI support for SQL queries, aliases, and output formats, including CSV and the `tables` alias.
- Multiline `LIKE` patterns with escape handling.
- List membership checks with `contains`.
- Strict diagnostics and warning diagnostics.
- `GROUP BY` and `ORDER BY` support for `fm` queries.
- Output format validation.
- Version output through `--version` and `-V`.
- A README and a bundled agent skill.
- CI and publication preparation.
