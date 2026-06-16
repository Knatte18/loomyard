# Batch: fsx-package

```yaml
task: "Extract internal/fsx and build internal/state"
batch: "fsx-package"
number: 1
cards: 2
verify: go test ./internal/fsx/...
depends-on: []
```

## Batch Scope

Create the new `internal/fsx` package — the shared filesystem-safety primitives extracted from
`internal/board/git.go`. This batch only *adds* a package; it does not touch board, so board keeps
its own `AtomicWrite`/`PathGuard` until batch 2 removes them (duplicate symbols across two packages
compile fine). The external interface consumed by batches 2–4 is the three functions plus `PathError`
defined in `## Shared Decisions → fsx public API`. Zero internal dependencies (stdlib only).

## Cards

### Card 1: Create the fsx package

- **Context:**
  - `internal/board/git.go`
- **Edits:** none
- **Creates:**
  - `internal/fsx/fsx.go`
- **Deletes:** none
- **Requirements:** New file `internal/fsx/fsx.go` in `package fsx`. Define `type PathError string`
  with `func (e PathError) Error() string { return string(e) }` (the renamed `board.BoardPathError`).
  Define `func PathGuard(relPath string) error` by moving the body of `board.PathGuard`
  (`internal/board/git.go:33-59`) verbatim, returning `PathError` instead of `BoardPathError` for the
  empty / absolute / `..`-component rejections. Define `func AtomicWriteBytes(absPath string, data []byte) error`:
  `os.MkdirAll(filepath.Dir(absPath), 0o755)`, `os.CreateTemp(filepath.Dir(absPath), ".tmp-")` with
  `defer os.Remove(tmpPath)`, write `data`, close, then `os.Rename(tmpPath, absPath)` — preserving
  the error-wrapping (`fmt.Errorf("mkdir: %w", err)` etc.) and the rename-is-the-atomic-swap comment
  from `board.AtomicWrite` (`internal/board/git.go:61-96`). Define `func AtomicWrite(dir, relPath, content string) error`
  as the guarded convenience: `if err := PathGuard(relPath); err != nil { return err }` then
  `return AtomicWriteBytes(filepath.Join(dir, relPath), []byte(content))`. Imports: `fmt`, `os`,
  `path/filepath`, `strings`.
- **Commit:** `feat(fsx): add filesystem-safety primitives package`

### Card 2: fsx unit tests

- **Context:**
  - `internal/board/git_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fsx/fsx_test.go`
- **Deletes:** none
- **Requirements:** New file `internal/fsx/fsx_test.go` in `package fsx_test`. Port `TestPathGuard`
  from `internal/board/git_test.go:17-41` (retarget `board.PathGuard` → `fsx.PathGuard`; same cases:
  empty, unix-absolute, windows-absolute, `..` mid-path, `..` at start, valid relative, valid single
  file, valid nested). Port `TestAtomicWrite` from `internal/board/git_test.go:43-97` (retarget
  `board.AtomicWrite` → `fsx.AtomicWrite`; same sub-tests: correct content, creates parent dirs, no
  `.tmp-` file left). Add `TestAtomicWriteBytes` covering: writes raw bytes to an absolute path
  (`filepath.Join(t.TempDir(), "f.json")`), creates missing parent directories
  (`filepath.Join(t.TempDir(), "deep/nested/f.bin")`), overwrites an existing file, and leaves no
  `.tmp-` file in the target dir. Use only stdlib `testing`/`os`/`path/filepath`/`strings`.
- **Commit:** `test(fsx): cover PathGuard, AtomicWrite, AtomicWriteBytes`

## Batch Tests

`verify: go test ./internal/fsx/...` runs the new `internal/fsx/fsx_test.go` only. Covers the guard
rejection table, the relative guarded write path (`AtomicWrite`), and the absolute raw-bytes primitive
(`AtomicWriteBytes`) including parent-dir creation, overwrite, and temp-file cleanup. Package builds
standalone (stdlib only), so the focused scope is sufficient.
