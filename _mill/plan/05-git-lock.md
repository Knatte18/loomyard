# Batch: Git and Lock

```yaml
task: Port the wiki module to Go
batch: Git and Lock
number: 5
cards: 4
verify: PYTHONPATH= go test ./internal/wiki/
depends-on: [1]
```

## Batch Scope

Implements the git operations layer (`path_guard`, `atomicWrite`, `pull`, `commitPush`) and the write-lock wrapper (`AcquireWriteLock`). After this batch all primitives needed by the wiki orchestration layer (batch 6) are available. `atomicWrite` is also used by `Store.Save` (batch 2).

## Cards

### Card 11: internal/wiki/git.go

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/wiki/git.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Define sentinel error types `WikiPushError` and `WikiPathError` as named string types implementing the `error` interface (or simple struct types with an `Error() string` method). Implement `pathGuard(relPath string) error`: return `WikiPathError` if relPath is empty or is absolute (`filepath.IsAbs`). To detect `..` components portably on all platforms: call `filepath.Clean(relPath)`, split the cleaned path into components using `strings.Split(cleaned, string(filepath.Separator))`, and return `WikiPathError` if any component equals `".."`. This is correct on both Windows (backslash) and Unix (forward slash). Implement `atomicWrite(wikiPath, relPath, content string) error`: resolve `fullPath = filepath.Join(wikiPath, relPath)`; create parent dirs with `os.MkdirAll`; create a temp file in the same directory with `os.CreateTemp(dir, ".tmp-")`; write content as UTF-8; call `os.Rename(tmpPath, fullPath)`; on any error, attempt to remove the temp file. Implement `runGit(args []string, cwd string) (stdout, stderr string, err error)`: run `git` with the provided args, capture stdout and stderr, return them. Check exit code — on non-zero, set err to a `WikiPushError` wrapping the stderr. Implement `pull(wikiPath string) (updated bool, err error)`: run `git -C <wikiPath> pull --ff-only`; return `updated = true` if stdout does NOT contain `"Already up to date."`; return `WikiPushError` on non-zero exit. Implement `commitPush(wikiPath string, relPaths []string, message string) error`: (1) `git -C <wikiPath> add -- <relPaths...>`; (2) `git -C <wikiPath> diff --cached --quiet` — if exit 0 (nothing staged), return nil (idempotent); if exit code ≠ 1, return error; (3) `git -C <wikiPath> commit -m <message>`; (4) if `os.Getenv("WIKI_SKIP_PUSH") == "1"`, return nil; (5) for attempt 0 and 1: `git -C <wikiPath> push` — if success return nil; if stderr contains `"non-fast-forward"` or `"rejected"`: `git -C <wikiPath> pull --rebase` — if rebase succeeds, continue retry; if rebase fails: `git -C <wikiPath> rebase --abort`, return `WikiPushError`; if push fails for other reason, return `WikiPushError`; (6) after two attempts still failing, return `WikiPushError("push still failing after rebase retry")`.
- **Commit:** `feat(wiki): git operations layer`

### Card 12: internal/wiki/git_test.go

- **Context:**
  - `internal/wiki/task.go`
  - `internal/wiki/git.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/git_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Test `pathGuard`: (a) empty string → error; (b) absolute path → error; (c) path with `..` component → error; (d) valid relative path → nil. Test `atomicWrite`: (e) creates file with correct content in a `t.TempDir()`; (f) parent directories created if missing; (g) temp file not left on disk after success. Test `pull`: (h) create a bare repo + clone in `t.TempDir()`; call `pull` on the clone — returns `updated = false` when nothing to pull. Test `commitPush` with `WIKI_SKIP_PUSH=1`: (i) create a git repo in `t.TempDir()`, write a file, call `commitPush` — verify `git log` shows the commit; (j) idempotent — calling `commitPush` again with no changes to the file returns nil and creates no second commit. Test `commitPush` rebase-retry path: (k) set up two clones of a bare repo; push a commit from clone-B; then call `commitPush` on clone-A (which is behind) — verify it succeeds after rebasing (without `WIKI_SKIP_PUSH=1`). This test requires `git` on PATH; skip with `t.Skip("git not found")` if `exec.LookPath("git")` fails.
- **Commit:** `test(wiki): git operations tests`

### Card 13: internal/wiki/lock.go

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/wiki/lock.go`
- **Deletes:** none
- **Requirements:** Package `wiki`. Import `github.com/gofrs/flock`. Define `WriteLock` struct wrapping a `*flock.Flock`. Implement `AcquireWriteLock(lockPath string) (*WriteLock, error)`: create a `flock.New(lockPath)`, call `fl.Lock()` (blocking exclusive lock); return `&WriteLock{fl}, nil` on success or a wrapped error on failure. Implement `(l *WriteLock) Release() error`: call `l.fl.Unlock()`. The lock file path is `tasks.json.lock` in the wiki directory — but the caller (wiki.go in batch 6) constructs this path; `lock.go` only provides the primitives.
- **Commit:** `feat(wiki): write lock via gofrs/flock`

### Card 14: internal/wiki/lock_test.go

- **Context:**
  - `internal/wiki/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/wiki/lock_test.go`
- **Deletes:** none
- **Requirements:** Package `wiki_test`. Test `AcquireWriteLock`: (a) acquire lock on a file in `t.TempDir()` → lock file created, no error; (b) `Release()` succeeds after acquire; (c) acquire after release succeeds (not double-locked). The "auto-releases on process death" property is guaranteed by `gofrs/flock`'s OS-level implementation; no subprocess test required — note this in a comment.
- **Commit:** `test(wiki): write lock tests`

## Batch Tests

`go test ./internal/wiki/` compiles the full package and runs git_test.go + lock_test.go (plus prior batches' tests). Git tests use `t.TempDir()` for isolation. The rebase-retry test (card 12k) requires `git` on PATH.
