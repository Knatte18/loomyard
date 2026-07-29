# Batch: daemon-state-and-locking

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: daemon-state-and-locking
number: 4
cards: 5
verify: go test -count=1 ./internal/codeintelengine/... ./internal/proc/...
depends-on: [1, 2]
```

## Batch Scope

Adds the two primitives both `EnsureServer` strategies (batch 5, 6) share
at the "is this connection healthy" layer: the daemon state-file
read/write/two-part-staleness-check, and the `probe` helper every
strategy runs as its final readiness gate regardless of what
spawn/reconnect machinery got it there. This is also where
`internal/proc` joins the Codeintelengine Leaf Invariant allowlist (see
the overview's "`internal/proc` is also added..." Shared Decision),
because the state file's PID-liveness half of its staleness check needs
a new cross-platform `proc.IsAlive` primitive.

**Scope boundary, stated explicitly because `_mill/discussion.md`'s
Testing section groups it differently:** the worktree-scoped
spawn-race **lock acquisition and retry-exhaustion loop** itself is
**not** in this batch — it is deeply intertwined with `ensureSupervised`'s
own control flow (read state → healthy? use it : try the lock → spawn or
lose the race → retry) and is implemented directly in batch 6, the same
way batch 2's toolchain-install lock-and-double-check lives inside
`resolveGoToolchain` itself rather than as a separately extracted
generic helper. This batch supplies only the two primitives that
retry loop calls into: state-file I/O and `probe`.

The external interface batches 5 and 6 consume:
`readDaemonState(path string) (daemonState, bool, error)`,
`writeDaemonState(path string, s daemonState) error`,
`daemonStale(s daemonState) bool`, and
`probe(ctx context.Context, client *lspClient, timeout time.Duration)
error`.

## Cards

### Card 12: `proc.IsAlive` — cross-platform PID liveness check

- **Context:** none
- **Edits:**
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func IsAlive(pid int) bool` to both files.
  **Linux** (`proc_linux.go`): `process, err := os.FindProcess(pid); if
  err != nil { return false }; return process.Signal(syscall.Signal(0))
  == nil` — on Unix, `os.FindProcess` always trivially succeeds
  regardless of whether the PID exists, so `Signal(0)` (which sends no
  actual signal, only checks deliverability/existence) is the real
  liveness probe; add `"os"` to this file's imports (it currently only
  imports `os/exec` and `syscall`). **Windows** (`proc_windows.go`):
  `process, err := os.FindProcess(pid); return err == nil` — on Windows,
  `os.FindProcess` itself calls `OpenProcess` and fails when the PID
  does not exist, so existence of a successful `FindProcess` call is
  itself the liveness signal there (no `Signal` call needed or
  reliable); add `"os"` to this file's imports too. Document on each
  `IsAlive` that a false positive is possible only in the narrow window
  of a PID being reused by an unrelated process after the original one
  exited — acceptable for a staleness check that is not the sole gate
  (the protocol-version half of `daemonStale`, and the `probe` step
  downstream, both catch what a stale-but-PID-reused daemon would miss).
- **Commit:** `feat(proc): add cross-platform IsAlive PID-liveness check`

### Card 13: CONSTRAINTS.md — allowlist `internal/proc` for codeintelengine

- **Context:**
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/codeintelengine/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `"github.com/Knatte18/loomyard/internal/proc": true,` to
  `allowedImports` in `leaf_enforcement_test.go` (alongside the
  `internal/lock` entry batch 2 added). In `CONSTRAINTS.md`'s
  "Codeintelengine Leaf Invariant" section, extend the opening
  statement's import list once more to add `internal/proc`, and add a
  bullet stating why: `internal/proc` supplies the cross-platform
  `IsAlive` PID-liveness primitive the daemon state file's staleness
  check needs (this batch) and the `Detach`/`DetachBreakaway` spawn
  primitive the `supervised` strategy needs (batch 6) — and, mirroring
  the existing **GitHub Auth Invariant** entry's own justification for
  the identical allowlist question, that `internal/proc`'s own
  production imports are `os/exec` and `syscall` only, so allowlisting
  it does not widen the leaf's real transitive dependency surface.
- **Commit:** `docs(constraints): allowlist internal/proc for codeintelengine`

### Card 14: Daemon state file — struct, atomic read/write, two-part staleness

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/proc/proc_linux.go`
- **Creates:**
  - `internal/codeintelengine/daemonstate.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Define
  `type daemonState struct { PID int `+"`json:\"pid\"`"+`; Address string
  `+"`json:\"address\"`"+`; ProtocolVersion string
  `+"`json:\"protocol_version\"`"+`; StartedAt string
  `+"`json:\"started_at\"`"+` }` — `Address` is the dial target in
  `network;addr` form (e.g. `"unix;/path/to/sock"`, matching `gopls
  serve -listen`'s own flag syntax so the recorded value can be split on
  the same `;` the daemon itself was told to listen on); `StartedAt` is
  RFC3339 (`time.Now().UTC().Format(time.RFC3339)`, produced by the
  caller, not this file — `daemonState` itself does no clock reads, to
  keep this file trivially unit-testable). Add
  `const supervisedProtocolVersion = "1"` — **this is lyx's own
  wire-compatibility version for the supervised daemon protocol, not
  gopls's version**: it exists to detect "a still-running daemon was
  spawned by an older lyx binary whose expectations of the daemon no
  longer match this binary's" and is bumped by a future lyx change, not
  by a `gopls` upgrade; document this distinction explicitly in a
  comment next to the const, since the name alone invites confusion with
  `Entry.PinnedVersion`. Add `func readDaemonState(path string)
  (daemonState, bool, error)`: `os.ReadFile`, translate
  `os.IsNotExist(err)` into `(daemonState{}, false, nil)` (absent state
  file means "no daemon recorded," not an error — the common case on a
  worktree's first `EnsureServer` call), any other read error wrapped
  and returned, successful read `json.Unmarshal`'d and returned with
  `true`. Add `func writeDaemonState(path string, s daemonState) error`:
  `os.MkdirAll(filepath.Dir(path), 0o755)`, marshal `s`, write to
  `path + ".tmp"` via `os.WriteFile`, then `os.Rename(path+".tmp", path)`
  — the temp-file-then-rename sequence is what makes a concurrent reader
  (a losing `EnsureServer` caller polling this same path, batch 6) never
  observe a partially-written file; `os.Rename` atomically replaces an
  existing destination on both Linux and Windows (Go's Windows
  implementation already uses `MOVEFILE_REPLACE_EXISTING`). Add
  `func daemonStale(s daemonState) bool { return !proc.IsAlive(s.PID) ||
  s.ProtocolVersion != supervisedProtocolVersion }` — either condition
  alone is sufficient to force a restart, per
  `state-file-location-and-content`'s "two-part staleness" decision.
- **Commit:** `feat(codeintelengine): add daemon state file read/write and two-part staleness check`

### Card 15: Shared `probe` helper

- **Context:**
  - `internal/codeintelengine/lspclient.go`
- **Creates:**
  - `internal/codeintelengine/probe.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func probe(ctx context.Context, client *lspClient, timeout
  time.Duration) error`: issue `client.workspaceSymbol(probeCtx, "")`
  (empty query) under its own `context.WithTimeout(ctx, timeout)`
  deadline (`defer cancel()`), and return whatever error that call
  produces verbatim (an `ErrServerTimeout` on deadline expiry, an
  `lspError`-derived wrap on a real protocol error, or `nil` on success
  — the empty-query result value itself is discarded, only the error
  matters). Document that this is the one readiness gate every
  `EnsureServer` strategy runs regardless of how it got its connection
  (fresh spawn for `native`, a reused dial or a fresh spawn for
  `supervised`) — per the design doc's caution that even a
  `-remote=auto`/dialed connection can silently hand back a reference to
  a hung shared instance, so a successful `initialize` handshake alone
  is not sufficient proof of health.
- **Commit:** `feat(codeintelengine): add shared probe readiness check`

### Card 16: Tests — state file round-trip, staleness, and probe

- **Context:**
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/probe.go`
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/daemonstate_test.go`
  - `internal/proc/isalive_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** **`internal/proc/isalive_test.go`** (untagged, no
  `//go:build` needed since `IsAlive` is defined on both platforms
  identically-named): `IsAlive(os.Getpid())` is `true` (the test process
  itself is always alive); `IsAlive` for a PID very unlikely to exist
  (e.g. spawn a short-lived `exec.Command` child, `Wait()` for it to
  exit, then call `IsAlive` on its now-dead PID) returns `false` — this
  file's own spawn is allowed under the Test Tier Purity Invariant's
  existing `internal/proc` allowlist entry ("process control is the
  package's subject — its tests must spawn"). **`daemonstate_test.go`**
  (untagged, offline, spawn-free — mirror this package's other
  untagged-test-file header-comment convention): (1) write-then-read
  round-trip via `t.TempDir()` preserves every field exactly; (2)
  `readDaemonState` on a non-existent path returns `(daemonState{},
  false, nil)`, not an error; (3) `daemonStale` returns `true` when
  `PID` names a process confirmed dead (spawn-and-wait, as in the
  `proc` test) even with a matching `ProtocolVersion`; (4) `daemonStale`
  returns `true` when `PID` is the test's own live PID but
  `ProtocolVersion` is `"stale-version"`; (5) `daemonStale` returns
  `false` when both are current (`os.Getpid()`,
  `supervisedProtocolVersion`); (6) a **concurrent-readers-never-see-a-partial-write**
  test: one goroutine calls `writeDaemonState` in a loop while N reader
  goroutines call `readDaemonState` concurrently on the same path,
  asserting every read either returns `false` (file not yet created) or
  a `json.Unmarshal`-clean `daemonState` — never a truncated-JSON parse
  error, which is exactly the failure mode the temp-file-then-rename
  write sequence exists to prevent; run this sub-test only when
  `testing.Short()` is false is **not** needed — keep it fast (a small,
  bounded iteration count) so it stays in the default `go test` run.
  Add a small `probe`-focused test in this same file using the existing
  `newPipeTransportPair`/`fakeServer` helpers from `lspclient_test.go`
  (same package, no import needed): a fake server that responds to
  `workspace/symbol` with an empty array makes `probe` return `nil`; a
  fake server that never responds makes `probe` return an
  `ErrServerTimeout`-satisfying error once its short test timeout
  expires (assert via `errors.Is(err, ErrServerTimeoutSentinel)`).
- **Commit:** `test(codeintelengine): cover daemon state file, staleness, and probe`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...
./internal/proc/...` — both packages this batch touches. No integration
tag needed anywhere in this batch: every card is pure file I/O, in-memory
fake-server protocol testing, or a short-lived local child-process
liveness check, none of which need a real language server.
</content>
