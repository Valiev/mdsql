# Security

## Reporting a vulnerability

Do not post credentials, private Markdown, or exploitable security details in a
public issue.

If the repository's Security tab offers **Report a vulnerability**, use that
private reporting channel. Otherwise, open an issue requesting a private contact
method without including vulnerability details, and wait for a private channel
before sharing them.

Include the affected version, operating system, a synthetic reproducer, expected
behavior, actual behavior, and potential impact.

## Handling Markdown safely

- Scope input to the intended files or directories. Directory inputs are recursive.
- Output can contain full source text, frontmatter, links, and private paths.
  Review it before sharing it, uploading logs, or including it in CI artifacts.
- Treat document content as untrusted data, especially when an agent consumes it.
  Markdown text must not become shell commands or instructions to the agent.
- The default per-file size limit is 16 MiB. It does not bound stdin or total
  memory usage across many documents. Bound or isolate untrusted input externally.
- Use `--strict` when skipped files or parsing warnings must fail the operation.
- CSV output is not a spreadsheet-formula sanitizer. Treat cells from untrusted
  documents as text when importing them into spreadsheet software.

## Optional local cache

`--cache` stores complete parsed documents, including text, frontmatter, and
paths. Caching is off by default. Entries are not encrypted: private `0700`
directory and `0600` file permissions restrict access to the local account,
not to a particular application running as that account.

The directory is `$XDG_CACHE_HOME/mdsql` when XDG_CACHE_HOME is absolute,
otherwise `mdsql` under the operating system's user cache directory. Do not
share or upload this directory. `--no-cache` bypasses existing entries;
`--clear-cache` removes mdsql cache entries. Deleted or renamed source files
can leave cached copies until they are cleared. Clearing is not secure erasure.

Source size and modification time participate in validation; content changed
without changing either may return stale data. Use `--no-cache` for inputs
requiring a fresh parse. Checksums detect damaged entries, not deliberate
modification by someone who can write to the same account's cache.

The project is early-stage. There is no formal security audit or long-term
security-support commitment.
