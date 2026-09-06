# Contributing

Keep changes focused, document behavior changes, and use synthetic Markdown in
examples and regression tests. Do not include personal notes, employer documents,
internal URLs, credentials, or private file paths in issues or pull requests.

## Development

The module declares Go 1.22 as its minimum version. Prefer a currently supported
Go toolchain for everyday development.

From the repository root:

```bash
go build -trimpath -o mdsql ./cmd/mdsql
go test ./...
go test -race ./...
go vet ./...
```

Format changed Go files with `gofmt`. Keep `go.mod` and `go.sum` together when
changing dependencies, and explain why a new dependency is needed.

GitHub Actions is configured to check formatting, dependencies, vet, tests, and
builds on Linux, macOS, and Windows. A Linux job also runs the race detector.
These configured checks are not a claim that every platform has already passed.

## Where changes belong

- `cmd/mdsql`: executable entry point and exit codes.
- `internal/cli`: flags, aliases, and help.
- `internal/query`: SQL-like query parsing.
- `internal/parser`: Markdown parsing.
- `internal/tables` and `internal/schema`: table views and schema descriptions.
- `internal/engine`: filtering, projection, grouping, sorting, and limits.
- `internal/output`: output formats.
- `internal/source`: files, directories, and glob resolution.
- `skills/mdsql/SKILL.md`: agent usage instructions.

## Before submitting

1. Reproduce the change with a small, fabricated Markdown input.
2. Add a focused regression test when behavior changes.
3. Run the relevant tests, then the full test suite and vet.
4. Update CLI help, README examples, and the agent skill when applicable.
5. State which checks ran and any checks you could not run.

For bugs, include the version, operating system, exact command, synthetic input,
expected output, and actual output or exit code. Redact sensitive paths and data.
Report security-sensitive issues using [SECURITY.md](SECURITY.md).

By contributing, you agree that your contributions will be licensed under the
MIT License used by this project.
