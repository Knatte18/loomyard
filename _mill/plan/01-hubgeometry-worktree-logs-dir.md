# Batch: hubgeometry-worktree-logs-dir

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: hubgeometry-worktree-logs-dir
number: 1
cards: 1
verify: go test ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

Adds the single new `hubgeometry` accessor every later batch needs: `Layout.WorktreeLogsDir()`, a WorktreeRoot-anchored `<WorktreeRoot>/.lyx/logs` path (discussion.md's `sink-location` decision). This is the one place in the whole task that constructs the durable-sink directory path, per the Hub Geometry Invariant — `internal/logger` (batches 3-7) calls this accessor and never joins `.lyx`/`logs` itself. No batch-local decisions beyond `## Shared Decisions` in the overview.

## Cards

### Card 1: Add `Layout.WorktreeLogsDir()` accessor

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/loomstatus_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/hubgeometry/worktreelogs_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new method on `Layout` (`internal/hubgeometry/hubgeometry.go`), placed near `HubLogsDir()` (currently at lines 569-581) and modeled on `LoomStatusFile()`'s doc-comment shape (lines 476-487):

  ```go
  // WorktreeLogsDir returns the path to the worktree-level directory where
  // internal/logger's durable trace sink writes one file per process. It is
  // WorktreeRoot-anchored, NOT Cwd-anchored: a caller invoked from a
  // subdirectory (Cwd != WorktreeRoot) must still resolve the one true logs
  // directory for the worktree, matching LoomStatusFile's anchoring
  // rationale above. It lives under the ephemeral, machine-bound ".lyx"
  // (dot) directory — the same lifecycle rationale DotLyxDir documents:
  // trace files are runtime forensic artifacts, never weft-synced.
  // WorktreeLogsDir returns the path only; it never creates the directory.
  //
  // Returns filepath.Join(WorktreeRoot, dotLyxDirName, "logs").
  func (l *Layout) WorktreeLogsDir() string {
  	return filepath.Join(l.WorktreeRoot, dotLyxDirName, "logs")
  }
  ```

  Use the unexported `dotLyxDirName` constant (the same one `DotLyxDir()`/`HubLogsDir()` already use for the `.lyx` token) — **not** `LyxDirName` (that is the weft-synced `_lyx` token and would violate the `sink-location` decision's explicit departure from a `_lyx`-scoped path). Do not add a directory-creation call here; `sinkOnce`'s open (batch 4) is what calls `MkdirAll`.

  Create `internal/hubgeometry/worktreelogs_test.go` mirroring `loomstatus_test.go`'s two-case shape exactly (`internal/hubgeometry/loomstatus_test.go:1-24` is the model — same header-comment style, same hand-built `&Layout{...}` construction with no `Resolve()` call):
  - `TestWorktreeLogsDir` — `Cwd` set to a subdirectory of `WorktreeRoot`; asserts `WorktreeLogsDir()` equals `filepath.Join(l.WorktreeRoot, ".lyx", "logs")` and is unaffected by `Cwd`.
  - `TestWorktreeLogsDir_CwdEqualsWorktreeRoot` — the paired case where `Cwd == WorktreeRoot`, same assertion, mirroring `loomstatus_test.go`'s companion `TestLoomStatusFile_CwdEqualsWorktreeRoot` (lines 40-50).
- **Commit:** `feat(hubgeometry): add WorktreeLogsDir accessor for the durable trace sink`

## Batch Tests

`verify: go test ./internal/hubgeometry/...` runs the new `worktreelogs_test.go` alongside the package's existing suite (including `TestEnforcement_GeometryLiterals`, which must stay green since the new method uses the existing `dotLyxDirName` token via the same `filepath.Join` shape as `HubLogsDir()`, not a new literal).
</content>
