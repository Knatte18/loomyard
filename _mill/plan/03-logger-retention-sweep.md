# Batch: logger-retention-sweep

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: logger-retention-sweep
number: 3
cards: 4
verify: go test ./internal/logger/...
depends-on: []
```

## Batch Scope

Implements the retention sweep as a standalone, directory-scoped function in a new `internal/logger/retention.go` — per discussion.md's `no-reader-cli` forward-compatibility note ("The retention sweep is a plain exported-or-not function over a directory path, callable independently of the emit path"), this batch has **no dependency** on the sink (batch 4), on geometry (batch 1), or on trace identity (batch 2): it operates on any `dir string` a caller hands it and needs only `internal/proc.IsAlive` plus stdlib. Covers the `retention` decision's age+count sweep and its liveness rule; the per-file 8 MB size cap and truncation marker are **not** here — those guard the sink's write path (batch 4) under the shared mutex from `concurrency-contract`, not the directory sweep.

## Cards

### Card 5: Age + count sweep over the trace-file grammar

- **Context:**
  - `internal/reedengine/lifecycle.go`
- **Edits:** none
- **Creates:**
  - `internal/logger/retention.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/logger/retention.go` implementing discussion.md's `retention` decision, modeled on the prefix-scoped precedent at `internal/reedengine/lifecycle.go:312-333` (`pruneServerLogsLocked`, invoked three times for three filename prefixes before a fresh log is written) — read that function for the general shape (list dir, filter by prefix, sort, delete overflow) but this sweep's own bounds are age **and** count, not reed's newest-3 count-only policy.

  `func Sweep(dir string) error` (or similar exported name) performs:
  1. **Grammar scope.** Only filenames matching `trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>-<pid>.log` are candidates for either bound. A file that does not parse against this grammar is never deleted and never counted toward the count bound (any other file, e.g. a `tmux-server-1234.log` a different tool wrote to the same directory, is ignored entirely).
  2. **Age bound.** A candidate file whose filename timestamp segment is older than 14 days from the current time is deleted — read from the filename, never from the file's mtime (mtime advances on every append under `O_APPEND`, which would let a long-running process's file never age out).
  3. **Count bound.** After the age pass, if more than 50 candidates from the eligible set remain, delete the oldest beyond the newest 50, ranked by the same filename timestamp segment the age bound reads — never mtime.
  4. **Delete-failure tolerance.** A per-file delete failure (Windows holding a file open; a sibling process racing this sweep) is skipped, not propagated — the sweep continues over the remaining candidates and never fails the sweep call or the log call that triggered it.
  5. **Empty/absent directory** is a no-op, returning `nil`.

  Do not call `hubgeometry` or resolve geometry anywhere in this file — `Sweep` takes a directory path from its caller (batch 4's `sinkOnce` open).
- **Commit:** `feat(logger): add age+count retention sweep over the trace-file directory`

### Card 6: Liveness rule

- **Context:**
  - `internal/proc/proc_linux.go`
- **Edits:**
  - `internal/logger/retention.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend `Sweep` (Card 5) with discussion.md's liveness rule: before either bound (age or count) considers a candidate file for deletion, parse its `<pid>` segment and skip the file — unconditionally kept, never deleted by either pass — when `internal/proc.IsAlive(pid)` reports true, and always skip the current process's own file (compare against `os.Getpid()`). Import `github.com/Knatte18/loomyard/internal/proc` for `IsAlive` (its signature: `func IsAlive(pid int) bool`, `internal/proc/proc_linux.go:18-35` — stdlib-only package, safe to import from internal/logger).

  **Live-skipped files do not consume the count bound's 50-slot budget.** Compute the count bound's ranking (which files count toward "newest 50") over the set of files eligible for deletion (dead `<pid>`, and not this process) only — a live file is kept in addition to whatever that eligible-set pass keeps, never counted as one of the 50 slots. A worktree with many concurrently running lyx processes can therefore temporarily hold more than 50 trace files.
- **Commit:** `feat(logger): add liveness-skip to the retention sweep, exempt from the count budget`

### Card 7: Retention sweep tests

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/logger/retention_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Untagged unit tests over a `t.TempDir()` (pure filesystem logic, no git/exec spawns — Test Tier Purity compliant since this package has zero non-stdlib production imports besides internal/proc, which is itself allowlisted for spawning only inside its own tests, not here):
  - Files older than the 14-day age bound are deleted; files within it are kept.
  - Seeding more than 50 dead-pid, in-age-bound files causes the newest 50 **by filename timestamp** to be kept and the rest deleted — seed one file with an old filename timestamp but a fresh mtime (`os.Chtimes` or write-after-create) to pin that the ranking key is the filename, not mtime.
  - A file not matching the `trace-<UTC>-<16-hex>-<pid>.log` grammar (e.g. a seeded `tmux-server-1234.log`) is never deleted and never counted toward the 50.
  - A file that cannot be deleted (simulate via a read-only directory permission on non-Windows, or document the Windows-only case if the CI platform cannot exercise a real lock) is skipped without failing the sweep or returning an error from `Sweep`.
  - An empty or absent directory is a no-op — `Sweep` on a nonexistent path returns `nil` with no error and no panic.
- **Commit:** `test(logger): cover retention sweep age/count/grammar-scope/delete-failure-tolerance`

### Card 8: Retention liveness tests

- **Context:** none
- **Edits:**
  - `internal/logger/retention_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/retention_test.go` (created by Card 7, which runs first):
  - A file whose `<pid>` segment names this test process's own live PID (`os.Getpid()`) is never deleted by either bound, even when its filename timestamp is old enough to otherwise qualify for the age bound and it would otherwise be evicted by the count bound.
  - A file whose `<pid>` is a large, implausible-to-be-alive value (e.g. a PID far above what the test platform would ever actually assign) which is over-age or over-count is deleted. **Do not spawn a real process to obtain a guaranteed-dead PID** — `retention_test.go` is an untagged file, and `exec.Command`/`exec.CommandContext` as a raw substring in an untagged file is a hard failure under the Test Tier Purity Invariant's banned-substring guard (`TestTierPurity_UntaggedTestsSpawnNothing`); the large-PID approach is the only one this file may use.
  - **Live-skip does not consume the newest-50 budget**: seed one live-pid file plus 50 dead-pid, in-bound files; assert all 50 dead-pid files survive the count pass (none of them is evicted to make room for the live file, since the live file was never one of the 50 slots to begin with).
- **Commit:** `test(logger): cover retention liveness rule and live-skip budget exemption`

## Batch Tests

`verify: go test ./internal/logger/...` runs `retention_test.go` (Cards 7+8) alongside `trace_test.go` (batch 2) and `logger_test.go` — all independent, no shared mutable package state between them.
</content>
