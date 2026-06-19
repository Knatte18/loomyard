MILL_REVIEW_BEGIN
# Review: weft engine: paths geometry, paired worktrees, lyx weft — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-19
```

## Findings

### [BLOCKING] Decision violation: WEFT_SKIP_PUSH must skip the detached spawn
**Location:** Batch 2, Cards 8 + 10
**Issue:** The `env-skip-guards` decision states `WEFT_SKIP_PUSH=1` "commits locally but skips push + the detached spawn," but Card 10's `sync` calls `spawnPush(weftWorktree)` unconditionally and Card 8's `spawnPush` has no skip guard, so `sync` still forks a child process under SKIP_PUSH (the child then no-ops in `Push`). Tests that set `WEFT_SKIP_PUSH=1` will still spawn real `lyx` processes.
**Fix:** Gate the `spawnPush` call (Card 10 sync) or `spawnPush` itself (Card 8) on `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` so the spawn is suppressed, matching the decision.

### [BLOCKING] Status junction check: os.Readlink on Windows junctions
**Location:** Batch 2, Card 9
**Issue:** `junction_ok` is computed by `os.Readlink(hostLink)` then comparing the cleaned target to `weftLyxDir`. On Windows an NTFS junction is not a symlink: `os.Readlink` typically errors or returns a `\??\C:\…` device path, so the equality check will report false drift (or fail) on the primary platform. The existing codebase deliberately avoids Readlink for junctions and uses only the mode bitmask (`links.go`, `removeLinks`).
**Fix:** Specify a Windows-correct target resolution (e.g. accept the `\??\` prefix / normalize, or resolve via `os.Stat` identity / `filepath.EvalSymlinks`) and require the status_test to cover the Windows junction case, not only `os.Symlink`.

### [NIT] "seven methods" vs eight throughout Batch 1
**Location:** Batch 1, Batch Scope + Card 1
**Issue:** Scope and Card 1 say "seven `Layout` methods" and the downstream-interface list omits `WeftLyxDirFor`, but the Card 1 requirements (and Cards 3/4) enumerate eight methods including `WeftLyxDirFor(slug)`.
**Fix:** Change "seven" to "eight" in the scope/Card-1 prose and add `WeftLyxDirFor` to the "consumed downstream" list.

### [NIT] Card 16 Context missing worktree.go/config.go for w.cfg.BranchPrefix
**Location:** Batch 3, Card 16
**Issue:** Requirements compute `branch := w.cfg.BranchPrefix + slug`; `Worktree.cfg` is defined in `worktree.go` and `Config.BranchPrefix` in `config.go`, neither listed in Context (reachable only via the edited `remove.go` receiver).
**Fix:** Add `internal/worktree/worktree.go` (and `config.go`) to Card 16 Context, as Card 15 implicitly relies on add.go.

## Verdict

REQUEST_CHANGES
Two substantive issues: SKIP_PUSH spawn-suppression and Windows junction status resolution.
MILL_REVIEW_END
