# Batch: ensure-server-supervised

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: ensure-server-supervised
number: 6
cards: 4
verify: go test -count=1 ./internal/codeintelengine/... ./internal/proc/... ./cmd/lyx/...
depends-on: [3, 4, 5]
```

## Batch Scope

Builds the `supervised` strategy end to end — a lyx-owned,
worktree-scoped singleton `gopls serve -listen=unix;<path>` daemon,
spawn-race-fenced, state-file-recorded, dialed by every reconnecting
caller, and restarted on staleness — and proves it against a **plain**
`gopls` via its own dedicated integration test, per the design's "V1
builds `supervised` and tests it against a plain `gopls`" requirement.
Per the overview's "`EnsureServer` has exactly one live dispatch arm in
V1" Shared Decision, `ensureSupervised` is **not** wired into
`ensureServer`'s dispatch — it is called directly, only by its own
tests, in V1.

This batch also lands the `detached-spawn-windows` fix
(`CREATE_BREAKAWAY_FROM_JOB`) `internal/proc` needs before
`ensureSupervised` can correctly detach the daemon on Windows, reusing
the `internal/proc` Leaf Invariant allowlist entry batch 4 already added
(no second `CONSTRAINTS.md` edit needed here).

Batch-local decision: the daemon's Unix-socket path is **deterministic**,
not randomly chosen at spawn time —
`filepath.Join(filepath.Dir(layout.CodeintelDaemonStateFile(lang)),
"daemon.sock")`, i.e. a fixed function of `(worktreeRoot, lang)`. This is
a simplification `_mill/discussion.md`'s `state-file-location-and-content`
decision leaves open ("`<path>` is chosen by lyx") without ruling out: a
deterministic path trivially satisfies the round-5 NOTE's "reusing the
same path keeps the state file's `address` field stable across
restarts" requirement (there is nothing to "choose" or persist
separately from recomputing it), and removes an entire class of bug
(losing track of a previously-chosen random path) for free.

## Cards

### Card 22: `proc.DetachBreakaway` — `CREATE_BREAKAWAY_FROM_JOB` on Windows, alias on Linux

- **Context:** none
- **Edits:**
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
  - `internal/proc/proc_windows_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `ensureSupervised` (card 24) calls
  `proc.DetachBreakaway(cmd)` unconditionally from a cross-platform file
  with no GOOS build split — every existing `proc` function
  (`HideWindow`, `Detach`, this plan's own `IsAlive` from card 12) is
  defined on **both** platform files for exactly this reason, so
  `DetachBreakaway` must be too, or `go build`/`go vet` fails on
  Linux/macOS with `undefined: proc.DetachBreakaway`. **Windows**
  (`proc_windows.go`): add `const createBreakawayFromJob uint32 =
  0x01000000` alongside the file's existing `createNoWindow`/
  `createNewProcessGroup` consts. Add
  `func DetachBreakaway(cmd *exec.Cmd)`: sets `cmd.SysProcAttr` with
  `HideWindow: true` and `CreationFlags: createNoWindow |
  createNewProcessGroup | createBreakawayFromJob` — a superset of
  `Detach`'s existing flags, so `supervised`'s daemon keeps
  `Detach`'s existing "new process group, no console window" guarantees
  and additionally survives a Windows Job Object with kill-on-close
  closing (`lyx` itself may run inside one; `CREATE_NEW_PROCESS_GROUP`
  alone does not save a child from that). Leave `Detach` itself
  completely untouched — this is a new function, not a modification of
  the existing one, so every current `Detach` caller's behavior is
  unaffected, matching `detached-spawn-windows`'s "leaving `Detach`'s
  current callers/behavior untouched" requirement. **Linux**
  (`proc_linux.go`): add `func DetachBreakaway(cmd *exec.Cmd) {
  Detach(cmd) }` — a trivial alias. Linux has no Job Object concept;
  `Setsid`-based `Detach` already gives the process the
  survive-parent-exit property `CREATE_BREAKAWAY_FROM_JOB` provides on
  Windows, so there is nothing extra to add here — this function exists
  purely so the cross-platform call site in `ensureserver.go` compiles
  on every OS this repo targets. Extend `proc_windows_test.go` with
  `TestDetachBreakaway` (mirroring `TestDetach`'s exact assertion shape)
  checking `HideWindow == true` and
  `CreationFlags == (createNoWindow|createNewProcessGroup|createBreakawayFromJob)`,
  and `TestDetachBreakawayDoesNotAffectDetach` proving a separate `Detach`
  call on a fresh `*exec.Cmd` still produces the old, narrower flag set
  (a regression guard for the "leave `Detach` untouched" requirement).
  No new Linux test is needed beyond the existing `TestDetachSetsSetsid`
  in `proc_linux_test.go` — `DetachBreakaway` on Linux is a pure
  delegation with no independent behavior to assert.
- **Commit:** `feat(proc): add DetachBreakaway with CREATE_BREAKAWAY_FROM_JOB for Windows Job Objects`

### Card 23: Add `ErrServerSpawnTimeout`

- **Context:** none
- **Edits:**
  - `internal/codeintelengine/errors.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `ErrServerSpawnTimeoutSentinel` and
  `ErrServerSpawnTimeout{Lang string}` following this file's exact
  existing five-error pattern (sentinel var, struct, `Error() string`
  naming `Lang`, `Is(target error) bool` comparing against the
  sentinel). Wording: "codeintel: gave up waiting for the supervised
  daemon for %q to become ready" — distinct phrasing from
  `ErrServerTimeout` (which names a specific LSP request phase) since
  this error means "a live process is holding the spawn lock and never
  produced a healthy daemon within the deadline," not "one RPC call
  didn't answer in time." This card lands **before** card 24
  (`ensureSupervised`), which constructs this type, deliberately — a
  card's own commit must compile in isolation, and `ensureSupervised`
  references `ErrServerSpawnTimeout` from its very first draft.
- **Commit:** `feat(codeintelengine): add ErrServerSpawnTimeout`

### Card 24: `ensureSupervised` — spawn-race lock, detached spawn, dial-or-restart, retry-exhaustion

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/lock/lock.go`
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
  - `internal/codeintelengine/lspclient.go`
  - `internal/codeintelengine/errors.go`
  - `internal/codeintelengine/daemonstate.go`
- **Edits:**
  - `internal/codeintelengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add
  `func ensureSupervised(ctx context.Context, command []string, lang,
  targetDir, worktreeRoot string, timeout time.Duration) (*lspClient,
  error)`. `command` is the raw daemon launch argv (e.g. `[]string{"gopls"}`
  for the integration test's plain-`gopls` proof) — unlike `ensureNative`,
  this function takes no `Entry` and does no toolchain resolution; it is
  the low-level strategy `_mill/discussion.md` describes as "built and
  proven against a plain `gopls`... not `-remote=auto`", independent of
  any registry wiring. Resolve
  `layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}`,
  `statePath := layout.CodeintelDaemonStateFile(lang)`,
  `lockPath := layout.CodeintelDaemonLock(lang)`, and the deterministic
  `socketPath := filepath.Join(filepath.Dir(statePath), "daemon.sock")`
  (see Batch Scope). Bound the whole function by a single
  `deadline := time.Now().Add(timeout)` the retry loop below checks
  against — this is the "bounded retry, not indefinite blocking" the
  `concurrency-locking` decision requires. **A non-nil `error` return from
  either `readDaemonState` or `lock.TryAcquireWriteLock` (a genuine
  OS-level failure — a permissions error, a corrupt state file, disk
  full — distinct from "lock held by someone else" or "no state file
  yet," both of which are `(false, nil)`/`(daemonState{}, false, nil)`,
  not errors) aborts the loop immediately: wrap and return the error,
  do not fold it into the "not acquired"/"absent" retry branches below.**
  Loop: (1) `readDaemonState`;
  if it exists and `!daemonStale(state)`, split `state.Address` on the
  first `;` into `network, address`, attempt
  `newLSPClientDial(ctx, network, address)` then
  `finalizeConnection(ctx, client, rootURI, timeout)` (build `rootURI` via
  card 19's `rootURIFor` helper); on success, return the client — this is
  the common-case reconnect-to-a-healthy-daemon path, and it never
  touches the lock at all. (2) If the dial/finalize above failed, or the
  state was absent/stale, attempt `lock.TryAcquireWriteLock(lockPath)`.
  If not acquired (someone else is spawning or restarting), sleep a short
  bounded interval (100ms) and loop back to step (1) to re-check whether
  the state has become healthy meanwhile — if `time.Now()` has passed
  `deadline`, return `&ErrServerSpawnTimeout{Lang: lang}` (card 23). (3)
  Once the lock is acquired, **double-check**: re-run `readDaemonState`;
  if it is now healthy (another process spawned a fresh daemon while this
  one was waiting for the lock), `lock.Release()`, **sleep the same short
  bounded interval as step (2) (100ms)**, and loop back to step (1)
  without spawning. This sleep matters even though the lock and state are
  both already healthy: the winner writes the state file (step 5) and
  releases the lock *before* its own dial-retry loop (step 6) confirms
  the daemon has actually finished binding its listen socket, so a loser
  reaching step (3) immediately after the winner's release can otherwise
  win the free lock, see healthy state, release, and land back on step
  (1) — dialing a socket that isn't bound yet — repeatedly with no
  backoff anywhere in that specific path, a tight spin for the duration
  of the daemon's bind window. Step (2)'s sleep only covers the
  "lock not acquired" branch; this one covers the "acquired, but nothing
  to do" branch, which is exactly as capable of spinning. (4) Otherwise
  this call is the winner:
  `os.Remove(socketPath)` (ignore a not-exist error — this is the
  stale-socket cleanup the round-5 NOTE requires, and it runs
  unconditionally before every spawn, first-ever or restart, since a
  nonexistent-path removal is a harmless no-op); build `argv :=
  append(append([]string{}, command...), "serve",
  fmt.Sprintf("-listen=unix;%s", socketPath))`; construct
  `cmd := exec.Command(argv[0], argv[1:]...)`, call
  `proc.DetachBreakaway(cmd)` (card 22), `cmd.Stderr = os.Stderr`
  (matching `newLSPClient`'s existing convention of surfacing the
  server's own diagnostics rather than discarding them), and
  `cmd.Start()`; on a start failure, `lock.Release()` and return the
  wrapped error (no retry — a spawn that fails to even start is not a
  race-losable condition worth looping on). (5) Write the state file
  **before** releasing the lock —
  `writeDaemonState(statePath, daemonState{PID: cmd.Process.Pid, Address:
  "unix;" + socketPath, ProtocolVersion: supervisedProtocolVersion,
  StartedAt: time.Now().UTC().Format(time.RFC3339)})` — so a losing
  caller that acquires the lock immediately after release always sees a
  state file, never a window where the lock is free but no state exists
  yet. `lock.Release()`. (6) Dial the daemon just spawned, with a short
  bounded retry of its own (the process needs a moment to bind the
  listen socket after `cmd.Start()` returns) — up to 10 attempts, 50ms
  apart, calling `newLSPClientDial(ctx, "unix", socketPath)`; if every
  attempt fails, return the last dial error wrapped. (7) On a successful
  dial, call `finalizeConnection(ctx, client, rootURI, timeout)` and
  return its result. Note in the function's doc comment that this daemon
  is **never** killed by the caller — its process is intentionally
  detached and outlives this call; the only thing that ever terminates it
  is its own idle timeout (gopls's own `-listen.timeout`, unconfigured
  here, defaulting per gopls itself) or a future restart's stale-socket
  cleanup finding it already dead. **Also record a known limitation in
  that same doc comment**: `daemonStale` only checks PID liveness and
  protocol version, not whether the daemon actually answers a dial — a
  process that is alive but hung or never finished binding its listen
  socket is never classified stale, so every caller's dial-then-finalize
  keeps failing against a state that keeps reading "healthy," and no
  caller ever restarts it; the bounded retry in step (2)/(3) still
  returns `ErrServerSpawnTimeout` per call rather than hanging, but a
  fresh call later hits the identical wedged daemon and times out again,
  indefinitely, until the process dies on its own or an operator
  intervenes. This is accepted as a known gap rather than fixed with a
  dial-failure-triggers-restart heuristic, because that heuristic risks
  misclassifying a daemon that is merely slow to bind on first spawn (the
  exact case step (6)'s own 10-attempt/50ms retry already exists to
  tolerate) as wedged, and getting that distinction right needs empirical
  grounding this task has no reason to invest in — `supervised` has no
  live V1 dispatch path, so this limitation affects no production
  caller yet.
- **Commit:** `feat(codeintelengine): implement ensureSupervised with spawn-race lock and staleness-triggered restart`

### Card 25: Tests — staleness/restart/retry-exhaustion (fake) and the plain-`gopls` proof (integration)

- **Context:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/errors.go`
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/refs_integration_test.go`
  - `internal/lock/lock.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:**
  - `internal/codeintelengine/supervised_test.go`
  - `internal/codeintelengine/supervised_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** **`supervised_test.go`** (untagged, offline —
  exercises the state-file/lock/retry-exhaustion decision logic without
  spawning `gopls`, by pointing `ensureSupervised` at a `command` that
  spawns a trivial always-fails-fast fake binary, or more directly, by
  unit-testing the sub-pieces `ensureSupervised` composes rather than the
  whole function): (1) `readDaemonState`+`daemonStale` combination
  already covered in `daemonstate_test.go` — do not duplicate; instead
  assert `ensureSupervised`'s **retry-exhaustion** path specifically: pre-write
  a state file recording a PID that is confirmed alive (spawn-and-hold a
  real short-lived test subprocess so `proc.IsAlive` reports true for the
  duration of the sub-test) and a `ProtocolVersion` that matches, so the
  state reads as healthy, but have the recorded `Address` point at a
  socket path nothing is listening on — this makes the dial-and-finalize
  attempt fail every time, and, critically, also pre-acquire the spawn
  lock in the test itself (hold it for the sub-test's duration) so
  `ensureSupervised` can never win it either; call `ensureSupervised`
  with a short `timeout` (e.g. 300ms) and assert it returns within that
  bound with `errors.Is(err, ErrServerSpawnTimeoutSentinel)` — proving
  the bounded-retry contract rather than hanging. (2) **stale-socket
  cleanup**: pre-create an empty regular file (not a real socket) at the
  deterministic `socketPath` a hand-built `Layout` resolves to, call
  `ensureSupervised` with a `command` that spawns a real, short-lived
  `gopls`-shaped fake — this sub-test may reasonably be skip-gated on
  `exec.LookPath("gopls")` if it needs a real listener to prove the bind
  actually succeeds post-cleanup (state this trade-off in the test's own
  comment: it is testing the cleanup step's *mechanics*, which do need a
  real bind attempt to prove `EADDRINUSE` was actually avoided). **
  `supervised_integration_test.go`** (`//go:build integration`, gated on
  `exec.LookPath("gopls")` via `t.Skip`, mirroring
  `refs_integration_test.go`): the central "prove `supervised` against a
  plain `gopls`" deliverable. Call `ensureSupervised(ctx,
  []string{"gopls"}, "go", repoRoot(t), t.TempDir(), 30*time.Second)`
  (a fresh temp `worktreeRoot` per test run keeps runs isolated); assert
  a state file now exists at the expected path with a live PID and the
  correct `ProtocolVersion`; issue a real `workspace/symbol` query on the
  returned client to prove it actually works end to end; call
  `ensureSupervised` a **second** time with the same `worktreeRoot`/`lang`
  and assert it reconnects (dials the *same* recorded address) rather
  than spawning a second daemon (assert the PID recorded in the state
  file is unchanged); then manually kill the daemon process (read the PID
  from the state file, `syscall.Kill`/equivalent) and call
  `ensureSupervised` a **third** time, asserting it detects the dead PID
  via `daemonStale`, respawns, and the state file's PID changes while its
  `Address` (the deterministic socket path) stays the same — this last
  assertion is what proves the stale-socket-cleanup-before-rebind logic
  (card 24, step 4) actually avoids `EADDRINUSE` on a real bind, not just
  in a mocked scenario. **`supervised_test.go`'s spawns trip the Test
  Tier Purity Invariant** (it is untagged and both its sub-tests spawn a
  real child process): add
  `"internal/codeintelengine/supervised_test.go": "spawns short-lived
  test subprocesses for the retry-exhaustion PID-liveness fixture and
  the stale-socket-cleanup bind proof",` to `allowedSpawners` in
  `cmd/lyx/tierpurity_test.go` — a file-level entry, mirroring card 16's
  identical fix for `daemonstate_test.go`, and kept file-scoped for the
  same reason (this package's other untagged tests stay genuinely
  spawn-free and should stay covered by the guard).
- **Commit:** `test(codeintelengine): cover supervised staleness, retry-exhaustion, and the plain-gopls proof; allowlist its spawn in tierpurity`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...
./internal/proc/... ./cmd/lyx/...` — the third package because card 25
edits `tierpurity_test.go`'s allowlist. Card 25's integration test is
excluded from this gate (no `-tags integration`) and runs separately,
alongside
`refs_integration_test.go` and batch 5's `ensureserver_integration_test.go`,
on a machine with `gopls` installed. This batch's `verify:` is the one
place in the whole plan where `-race` would be valuable (the spawn-race
lock is exactly the kind of shared-mutable-coordination code a race
detector catches bugs in that a single-threaded test run cannot) —
consider running `go test -race -tags integration -count=1
./internal/codeintelengine/...` manually before merge, even though it is
not this batch's mechanical `verify:` gate, mirroring how the
`native-clients-migration` plan on this same branch's history required
`-race` for its own shared-mutable-state batches.
</content>
