# Batch: gitexec-rename

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: gitexec-rename
number: 1
cards: 3
verify: go build ./... && go test ./internal/gitexec/ ./internal/board/ ./internal/weft/ ./internal/paths/
depends-on: []
```

## Batch Scope

Rename the leaf package `internal/git` → `internal/gitexec` and sweep every importer so the build stays green. This is the foundation: the new `warp` package and the surviving `weft`/`paths`/`board` packages all call the leaf. Logic is unchanged — only the package name, import path, and call-site qualifier (`git.RunGit` → `gitexec.RunGit`) change. The external interface the next batches consume is `gitexec.RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)`.

Batch-local decision: the worktree/gitclone files edited here (cards 3) are *also* updated for the rename even though batches 2–3 later delete them — they must compile in the interim so `go build ./...` passes at this batch boundary.

## Cards

### Card 1: Rename internal/git package to internal/gitexec

- **Context:**
  - `internal/proc/proc.go`
- **Edits:** none
- **Creates:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitexec/gitexec_test.go`
- **Deletes:**
  - `internal/git/git.go`
  - `internal/git/git_test.go`
- **Requirements:** Move `internal/git/git.go` to `internal/gitexec/gitexec.go` and `internal/git/git_test.go` to `internal/gitexec/gitexec_test.go` (prefer `git mv` to preserve history). Change the package clause from `package git` to `package gitexec` in both files. Keep `RunGit(args []string, cwd string) (stdout, stderr string, exitCode int, err error)` byte-for-byte identical in behaviour (it shells out via `proc.HideWindow`; non-zero git exit returned in `exitCode` with `err == nil`; spawn failure → `err` + `exitCode == -1`). Update the test's package clause and any unqualified references; no logic changes.
- **Commit:** `refactor(gitexec): rename internal/git to internal/gitexec`

### Card 2: Sweep surviving production importers

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/paths/paths.go`
  - `internal/paths/worktreelist.go`
  - `internal/board/git.go`
  - `internal/board/sync.go`
  - `internal/weft/sync.go`
  - `internal/weft/status.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In each file replace the import `github.com/Knatte18/loomyard/internal/git` with `github.com/Knatte18/loomyard/internal/gitexec` and rewrite every call-site qualifier `git.RunGit(` → `gitexec.RunGit(`. These four packages (`paths`, `board`, `weft`) survive this task and sit at or below `warp`; missing any leaves the build broken. No behaviour change.
- **Commit:** `refactor(gitexec): update paths/board/weft importers`

### Card 3: Sweep doomed-package and test importers

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/worktree/add.go`
  - `internal/worktree/remove.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/add_test.go`
  - `internal/worktree/weft_test.go`
  - `internal/gitclone/clone.go`
  - `cmd/lyx/main_test.go`
  - `internal/update/update_test.go`
  - `internal/initcli/initcli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Same import + qualifier sweep (`internal/git` → `internal/gitexec`, `git.RunGit` → `gitexec.RunGit`) for the remaining importers. `worktree` and `gitclone` are deleted in batches 2–3 but must compile now. The test files (`main_test.go`, `update_test.go`, `initcli_test.go`) reference the leaf only in test code. After this card `grep -r "internal/git\"" ` must return zero hits.
- **Commit:** `refactor(gitexec): update worktree/gitclone and test importers`

## Batch Tests

`verify: go build ./... && go test ./internal/gitexec/ ./internal/board/ ./internal/weft/ ./internal/paths/`. The module-wide `go build ./...` proves every importer was swept (a missed one fails to compile). The scoped `go test` runs the moved leaf test (`gitexec_test.go`) plus the three surviving production importers' suites to confirm behaviour is preserved. `worktree`/`gitclone` tests are not run here — they are deleted in the next two batches; their compilation is covered by `go build ./...`.
