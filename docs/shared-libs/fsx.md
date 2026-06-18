# `internal/fsx`

Filesystem-safety primitives: atomic file writes and validation of untrusted relative paths.
Extracted from `internal/board`. Zero internal dependencies; does not touch `internal/paths`.

- **`AtomicWriteBytes(absPath, data)`** — the general primitive. Creates missing parent
  directories, writes `data` to a temp file in that directory, then atomically renames
  the temp file onto `absPath`. The rename is the atomic swap; concurrent readers are
  excluded by external synchronization (e.g. a lock held at the call site), ensuring
  they never see partial writes.
- **`PathGuard(relPath)`** — validates untrusted relative paths. Rejects empty paths,
  absolute paths (both Unix `/` and Windows `X:` forms), and any `..` component. Used
  to gate untrusted inputs before writing.
- **`AtomicWrite(dir, relPath, content)`** — guarded convenience. Calls `PathGuard(relPath)`
  then `AtomicWriteBytes(filepath.Join(dir, relPath), content)`. Combines the guard and
  the writer for call sites where the relative path is untrusted but the base directory
  is controlled internally.
- **`PathError`** — the error type returned by `PathGuard`. A simple string wrapper for path
  validation errors.
