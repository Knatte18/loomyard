# Batch: watchdog-daemon

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "watchdog-daemon"
number: 5
cards: 14
verify: go test ./internal/reedcli/ ./internal/reedengine/ ./internal/clihelp/ ./cmd/lyx/
depends-on: [4]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

This batch rehomes the watchdog. `lyx reed header` is replaced by `lyx reed statusline` (preview only, no `--blocking`), a new blocking `lyx reed watchdog --hub-path <abs> --tmux <path>` verb hosts a detached, single-instance, per-hub daemon, `up`/`resume`/`attach` each attempt its spawn after their own engine op returns without error, and `internal/reedengine` gains exactly one new engine-less function (`ListSessions`) so the daemon can enumerate a hub socket's sessions before it has any engine.
It is one batch because deleting `header --blocking` deletes the only caller of `eng.Watch(ctx)` in hub mode — shipping that deletion without the daemon in the same batch silently turns the watchdog off for every worktree, which is the precise trap the design doc records an earlier draft falling into.

The external interface batch 6 and 7 consume: `reedengine.ListSessions(tmuxPath, socketKey string) ([]string, error)`, the `lyx reed statusline` and `lyx reed watchdog` verbs, and a `reedCLI` carrying `hubPath`, `suppressWatchdogSpawn` and `ensureWatchdogSpawned`.

Batch-local decisions beyond `## Shared Decisions`: the daemon derives nothing — both the hub path and the tmux binary are told on its command line, and it opts out of reed's `PersistentPreRunE` entirely rather than resolving a `Location` it must not use.
The session→worktree mapping is a **direct join** (`filepath.Join(hub, sessionName)` then `lyxcwd.ResolveWorktree` on that one path), not a scan: hub-mode `reedengine.SessionName` is `filepath.Base(worktreeRoot)` verbatim and refuses an unusable name rather than sanitizing it, so the derivation is exactly invertible.
`reed down` never kills the daemon; the daemon exits itself after three consecutive non-affirmative enumerations and releases its lock on the way out.

## Cards

### Card 28: add the engine-less ListSessions seam

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/server.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/overlay.go`
- **Creates:**
  - `internal/reedengine/overlay_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add exactly one exported, engine-less function to `internal/reedengine/overlay.go`: `ListSessions(tmuxPath, socketKey string) ([]string, error)`, built on `NewTmuxCmd(tmuxPath, socketKey).output("list-sessions", "-F", "#{session_name}")` — the identical invocation five existing call sites in this package already use. It splits the captured output on newlines, trims each line, drops empty lines, and returns the resulting slice; a round-trip error is returned unwrapped beyond `output`'s own `wrapTmuxError`, so the caller can distinguish "exit 0 with names", "exit 0 with none", and "failed". Document why it exists: `TmuxCmd.run`/`output` are unexported and every exported `*Engine` method is bound to one session, so a daemon that has no engine before discovery has no way to enumerate; and telling it the binary rather than having it load a config keeps the Told-Geometry Invariant intact, since the sessions share one socket and one binary is the only coherent answer for enumeration anyway. Do not export `TmuxCmd.run` or `TmuxCmd.output`. Create `internal/reedengine/overlay_test.go` with a table-driven test for the parsing half, driven through `TmuxCmd`'s existing `execHook` seam rather than a real server, covering the three outcomes the daemon's idle rule distinguishes: a multi-line listing, an exit-0 empty listing, and an error. Include a case with trailing whitespace and a trailing newline to pin the trimming.
- **Commit:** `feat(reedengine): add the engine-less ListSessions seam`

### Card 29: replace the header verb with statusline

- **Context:**
  - `internal/reedengine/statusline.go`
  - `internal/clihelp/annotations.go`
  - `internal/output/output.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/statusline.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/reedcli/header.go` -> `internal/reedcli/statusline.go`
- **Requirements:** After `git mv`, edit `internal/reedcli/statusline.go` surgically. Delete `blockForever`, the `headerWatch` package var, the `headerPark` package var, and `headerBlockingPayload` outright, along with the `--blocking` flag, its whole `if blocking { … }` branch, and the now-unused `context`, `fmt`, `io`, `strings`, `time`, `logger` and `reedengine` imports. Rename `headerCmd` to `statuslineCmd`, change `Use` to `"statusline"`, and give it a non-empty `Short` reading that it renders the session's status-line text. Call `c.eng.StatusLineText()` in place of `c.eng.HeaderText()`. Keep `clihelp.ShouldAbort` as the first statement in `RunE`, keep the `clihelp.SkipStencilSeedAnnotation` annotation set to `clihelp.AnnotationEnabled`, and keep the `output.Ok(out, map[string]any{"text": text})` envelope. Rewrite the `Long` help: it previews the rendered status-line text over this hub's configured template (or the embedded default), the same `tokenvocab` pipeline `Engine.ValidateStatusLine` checks eagerly at boot; the text now lives in a tmux option that `pinGeometryOptionsLocked` rewrites on every boot and every attach, so the old "the running pane keeps its old text until the header is next rebuilt" caveat is gone and must not be carried over; name `status_line.template` in `reed.yaml` as the key an operator edits; and give one `lyx reed statusline` example. Rewrite the file's leading comment to match, keeping the paragraph explaining why the stencil-seed annotation is carried (neither the command nor the gate reads a stencil, and declining keeps the hub free of a preview command's git commits).
- **Commit:** `feat(reedcli): replace the header verb with statusline`

### Card 30: add the watchdog daemon's timing constants and idle rule

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/watchdog.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedcli/watchdog.go` and declare in it the daemon's two fixed timing constants, beside the discovery loop that will consume them: `watchdogHubDiscoveryCycle = 5 * time.Second` and `watchdogHubIdleCycles = 3`. Document why they live here rather than beside `internal/reedengine/watchdog.go`'s existing unexported `watchdog*` constants: those govern `Engine.Watch`'s own internals, which `reedengine` still owns, and exporting two constants from that package purely so another package could read them would put the timings somewhere their only consumer is not. Document the values: five seconds is well below any human `down` + `up` gap while being an order of magnitude slower than the 100ms signal tick, so discovery costs one `list-sessions` round trip per hub every five seconds rather than riding the per-worktree tick; three cycles covers a `down` immediately followed by an `up` without the daemon dying and respawning in between. In the same file add the pure idle-rule helper `sessionsAreIdle(names []string, err error) bool`, returning `false` only when `err == nil && len(names) > 0` and `true` for every other combination — an exit-0 empty listing, a "no server running" error, and any other failure alike. Document that this totality is forced rather than merely tolerated: the normal last-`down` case is an *error*, not an empty list, and `internal/reedengine/proctree_windows.go` records that psmux exits identically with and without a server, so a rule distinguishing a no-server error from a transient one would be unimplementable there. Do not change `internal/reedengine/watchdog.go`.
- **Commit:** `feat(reedcli): declare the watchdog daemon's timings and idle rule`

### Card 31: add the daemon's worktree discovery

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server.go`
  - `internal/reedengine/watchloop.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/watchdog.go`, add the discovery half of the daemon. Declare an unexported `watchedSession` struct holding the built `*reedengine.Engine` and a `cancel context.CancelFunc`, and a pure function `planSessionDiff(live []string, known map[string]watchedSession) (appeared, departed []string)` returning the names newly present in `live` and the known names absent from it — this is the TDD seam the discussion names, computable with no tmux and no filesystem. Add `resolveWatchedSession(hub, sessionName string) (*lyxcwd.Location, error)` performing the direct join: `filepath.Join(hub, sessionName)` then `lyxcwd.ResolveWorktree` on **that one path**. Document why the join is exact rather than a scan: hub-mode `reedengine.SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)` verbatim and `validateToldTmuxIdentity` refuses an unusable name instead of sanitizing it, so with the hub being `filepath.Dir(worktreeRoot)` the join recovers the worktree root exactly, with no candidate set; and `ResolveWorktree` is still required on top of it, both because `hubgeom.ReedGeometry` needs a resolved `*lyxcwd.Location` and because it is the gate that rejects a session name that is not a worktree at all. Add `enterSession(hub, tmuxPath, sessionName string) (watchedSession, error)` building `hubgeom.ReedGeometry(location)` plus `reedengine.LoadConfig(location.AnchorPath(), "reed")` and `reedengine.New`, then: when that worktree's `cfg.Watchdog` reads as **off**, return an entry with a **nil** cancel and start no goroutine at all — `watchLoop` answers a disabled watchdog by blocking on `<-ctx.Done()` rather than returning, so starting one would cost a goroutine per disabled worktree to do nothing, while the entry still keeps the session known and keeps departure bookkeeping uniform; otherwise derive a child context, start `go eng.Watch(ctx)`, and return the entry carrying its cancel. A name that does not resolve is skipped and logged at `logger.Debug`, not retried until it next re-appears in the live set. Log the `ResolveWorktree` git spawn at `logger.Debug`, per CONSTRAINTS.md's Live-Substrate Spawn Observability rule for a spawn inside a polling probe. Use `reedengine.LoadConfig` — not a strict load — so a worktree with no `_lyx` still resolves the embedded template, keeping `reedengine` on the degrading side of the Config Strictness Invariant.
- **Commit:** `feat(reedcli): add the watchdog daemon's direct-join worktree discovery`

### Card 32: add the daemon's discovery loop and lifetime

- **Context:**
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/server.go`
  - `internal/reedengine/watchloop.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/watchdog.go`, add `runWatchdogLoop(ctx context.Context, hub, tmuxPath string) error` — the daemon's outer loop. Each `watchdogHubDiscoveryCycle` it runs **one** `reedengine.ListSessions(tmuxPath, reedengine.ServerName(hub))` round trip. It feeds the result through `sessionsAreIdle`: an idle answer increments a consecutive counter and returns nil once that counter reaches `watchdogHubIdleCycles`; a non-idle answer resets the counter to zero. On a non-idle answer it calls `planSessionDiff` against its `map[string]watchedSession`, calls `enterSession` for each appeared name and stores the result, and — this half is not optional — for each departed name calls its stored `cancel` when non-nil and deletes the entry. Document why teardown is mandatory: `Engine.Watch` never returns while its context is live, so without it a worktree whose session goes away while siblings remain leaves a goroutine polling a dead session for the daemon's whole lifetime; and cancelling on departure is also what makes the config-re-read claim true, since `watchLoop` reads `cfg.Watchdog` exactly once at start, so "re-read on re-entry" only means anything if entries can leave. A nil cancel (a `watchdog: off` worktree) is simply skipped on departure. The loop also returns when `ctx` is done, cancelling every stored entry on its way out. Log the daemon's own start at `logger.Info` and each session enter/depart at `logger.Debug`. Do not rewrite anything inside `Engine.Watch`, `watchLoop`, `watchState`, the debounce, the per-event retry cap, the poll/signal/dormant mode machine or `reapplyLayout` — per the discussion's `watch-loop-internals-are-rehosted-not-rewritten` decision this batch rehosts that logic and owns only discovery, per-worktree scheduling, and process lifetime.
- **Commit:** `feat(reedcli): add the watchdog daemon's discovery loop and idle exit`

### Card 33: add the watchdog verb

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/lock/lock.go`
  - `internal/logger/logger.go`
  - `internal/logger/sink.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/reedengine/server.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/watchdog.go`, add `(c *reedCLI) watchdogCmd() *cobra.Command` for `Use: "watchdog"`, with a non-empty `Short` and two required string flags, `--hub-path` and `--tmux`. Its `RunE` checks `clihelp.ShouldAbort(cmd.Context())` first, then validates on the envelope before it blocks: an empty or non-absolute `--hub-path` and an empty `--tmux` each report through `output.Err` and `clihelp.SetExit`, never by blocking. It then, **in this order**: calls `logger.SetDurableSinkDir(fabricengine.HubLogsDir(hub))` as its first action after validation; calls `logger.SetOutput(io.Discard)`; and only then attempts `lock.TryAcquireWriteLock(filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock"))`. Document why the sink comes first: the durable sink's cwd-anchored fallback arms only inside a lyx-owned worktree and never from a hub cwd, which is exactly where `cmd.Dir` pins this process, so without the explicit call every diagnostic the design leans on would go nowhere — and this is what makes running `lyx reed watchdog --hub-path <abs> --tmux <path>` in the foreground a real diagnosis path for a daemon that was spawned detached. Answer the lock's two outcomes differently, because `lock.TryAcquireWriteLock` reports them differently: `(nil, false, nil)` is **contention** — another daemon holds it — which logs at `logger.Info` and exits 0, so a racing spawn costs one short-lived process and nothing else; a non-nil error is a **failure** (the lock path is unusable — absent parent directory, permissions, a read-only filesystem), which logs at `logger.Error` to the durable sink and exits non-zero, never silently. On acquiring the lock, `defer` its `Release` and call `runWatchdogLoop(cmd.Context(), hub, tmuxPath)`. This verb is blocking and envelope-exempt on its tail only — everything fallible runs pre-flight on the envelope before it starts blocking, and it still carries a non-empty `Short` and still checks `clihelp.ShouldAbort`.
- **Commit:** `feat(reedcli): add the blocking per-hub watchdog verb`

### Card 34: spawn the daemon from up, resume and attach

- **Context:**
  - `internal/boardengine/spawn.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/proc/proc_linux.go`
  - `internal/logger/logger.go`
  - `internal/reedcli/watchdog.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedcli/cli.go`
  - `internal/reedcli/up.go`
  - `internal/reedcli/resume.go`
  - `internal/reedcli/attach.go`
- **Creates:**
  - `internal/reedcli/spawnwatchdog.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedcli/cli.go`, add two fields to `reedCLI` beside `eng`: `hubPath string` and `suppressWatchdogSpawn bool`. Store `location.HubPath` into `c.hubPath` inside `PersistentPreRunE` — the hub path alone, deliberately not the whole `*lyxcwd.Location`, since the spawn needs nothing else from it and a stored `Location` would invite other code to re-read geometry the engine was already told. Initialise `suppressWatchdogSpawn` from `testing.Testing()` where `c` is constructed in `Command()`, with a doc comment stating that re-exec'ing `os.Executable()` from a test binary runs the whole suite recursively — the same hazard and the same shape as the `Engine.suppressHeaderLaunch` field batch 3 deleted, relocated to the layer that now owns the spawn — and that an in-package test flips it back off to drive the real spawn path. Extend the group-command early return so `watchdog` opts out of the whole cwd/location/config/geometry resolution: `if cmd.Name() == "reed" || cmd.Name() == "watchdog" { return nil }`, with a comment stating that the daemon is told its hub path and must never be reached through `c.eng`, and that letting it run the normal pre-run would make it refuse to start outside a worktree and hold a geometry it must not use. Register both new verbs on `parent.AddCommand`, replacing `c.headerCmd()` with `c.statuslineCmd()` and appending `c.watchdogCmd()`. Create `internal/reedcli/spawnwatchdog.go` holding `(c *reedCLI) ensureWatchdogSpawned()`: it returns immediately when `c.suppressWatchdogSpawn` or `c.hubPath` is empty; calls `os.MkdirAll(fabricengine.HubScratchDir(c.hubPath), 0o755)` first, because `lock.TryAcquireWriteLock` does not create the lock file's parent and a hub that has never booted a reed server may not have that directory — a `MkdirAll` failure is logged at `logger.Warn` naming the path and the spawn is skipped rather than failing the operator's `up`; resolves the binary with `os.Executable()`, matching `internal/boardengine/spawn.go` and `internal/fabricengine/spawn.go` exactly; builds `exec.Command(exe, "reed", "watchdog", "--hub-path", c.hubPath, "--tmux", c.eng.TmuxPath())`; sets `cmd.Dir = c.hubPath` explicitly and leaves stdin/stdout/stderr nil so no parent handles are inherited; calls `proc.Detach(cmd)`; logs the spawn at `logger.Info` per Live-Substrate Spawn Observability (a detached `Start` with no `Wait` logs the spawn alone, so there is no teardown to log); and `Start`s without `Wait`ing, logging a failure at `logger.Warn` and returning. Document why `cmd.Dir` is pinned to the hub: the process is per-hub and outlives the worktree that spawned it, and on Windows a held cwd handle on a worktree directory blocks that directory's deletion, which would break `fabric` teardown. Call `c.ensureWatchdogSpawned()` from `internal/reedcli/up.go`'s `upCmd` and `internal/reedcli/resume.go`'s `resumeCmd` immediately after their engine op returns without error and before the envelope write, and from `internal/reedcli/attach.go` after the `c.eng.Status()` pre-flight and **before** `attach.Run()` hands the operator's stdio over — a spawn placed after the handover would fire only once the operator detaches, which is the one moment it is useless. `downCmd` never spawns.
- **Commit:** `feat(reedcli): spawn the detached per-hub watchdog from up, resume and attach`

### Card 35: swap the CLI invariant's interactive-handoff exception

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/statusline.go`
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `CONSTRAINTS.md`'s CLI / Cobra Invariant, change the interactive-handoff exception bullet's reed entry from `reedengine` `attach`/`header --blocking` to `reedengine` `attach`/`watchdog`, leaving the other two entries (`lyx loom status --watch`, `lyx loom run`/`lyx run`) and every other bullet in that section untouched. This is a swap, not a deletion followed by an addition: the list is a closed enumeration of commands that legitimately never return an envelope, and replacing one entry for its replacement keeps it closed and keeps the invariant enforceable. Add no new invariant section — per the discussion's `no-new-cross-cutting-invariant` decision, Selvage's own rules bind exactly one package, are already enforced by that package's own tests, and live in `internal/reedengine/doc.go` and the module design doc instead.
- **Commit:** `docs(constraints): swap reed's interactive-handoff exception to the watchdog verb`

### Card 36: retarget the reedcli verb tests

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/reedcli/cli.go`
  - `internal/clihelp/annotations.go`
- **Edits:**
  - `internal/reedcli/statusline_test.go`
  - `internal/reedcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/reedcli/header_test.go` -> `internal/reedcli/statusline_test.go`
- **Requirements:** After `git mv`, edit `internal/reedcli/statusline_test.go` surgically: delete every test whose subject was `headerBlockingPayload`, `headerWatch`, `headerPark` or the `--blocking` flag, since all four are gone; retarget the remaining tests onto the `statusline` verb and `StatusLineText`; and assert the verb returns the rendered text on the envelope under the `text` key. In `internal/reedcli/cli_test.go`, extend the `wantSubs` slice — today `{"up", "down", "add", "remove", "status", "resume", "attach"}` — with `"statusline"` and `"watchdog"`, and add assertions that both carry a non-empty `Short`, per the CLI / Cobra Invariant. Add a test asserting `watchdog` takes the `PersistentPreRunE` early return: invoke `reedcli.RunCLIIn` from a directory that is **not** a git repository and assert the command does not fail with `lyxcwd.Resolve`'s not-a-git-repository error and that `c.eng` is never populated — the verb must run with no git repository present. Add a test asserting `watchdog` refuses an absent `--hub-path` and a relative `--hub-path` on the envelope, before blocking.
- **Commit:** `test(reedcli): retarget the verb tests onto statusline and watchdog`

### Card 37: test the daemon's pure discovery and spawn seams

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/reedengine/overlay.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/watchdog_test.go`
  - `internal/reedcli/spawnwatchdog_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedcli/watchdog_test.go` with untagged, table-driven tests over the daemon's pure seams, spawning nothing: `planSessionDiff` across an empty-to-populated transition, a populated-to-empty one, a partial overlap, and a no-change cycle, asserting both returned slices each time; and `sessionsAreIdle` across all four combinations the rule folds together — a non-empty listing with no error (not idle), an empty listing with no error (idle), a non-empty listing with an error (idle), and an empty listing with an error (idle) — which is the direct assertion of the `daemon-idle-rule-is-anything-but-an-affirmative-listing` Shared Decision. Create `internal/reedcli/spawnwatchdog_test.go` asserting that `ensureWatchdogSpawned` returns without spawning when `suppressWatchdogSpawn` is set and when `hubPath` is empty, and that the lock path it would target is `filepath.Join(fabricengine.HubScratchDir(hub), "reed-watchdog.lock")`. Do not write a test here that actually spawns the daemon or drives a real tmux: per CONSTRAINTS.md's Test Tier Purity Invariant an untagged file performs no `exec.Command`, and the daemon's live behaviour is the integration test's subject in card 41.
- **Commit:** `test(reedcli): pin the watchdog daemon's discovery and idle seams`

### Card 38: update the help tree for the two verb changes

- **Context:**
  - `internal/reedcli/cli.go`
  - `internal/reedcli/statusline.go`
  - `internal/reedcli/watchdog.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/helptree_test.go`, change the `reed` case's `wantSubs` slice from `{"up", "add", "remove", "status", "attach", "resume", "down", "header"}` to name `"statusline"` and `"watchdog"` in place of `"header"`. This site is not enumerated in the discussion's own disposition list and was found by re-running its mandated case-insensitive `header` scan — see the `the-header-scan-is-re-run-and-its-residue-is-in-scope` Shared Decision. `cmd/lyx/drift_test.go`'s `TestDriftGuard_AllCommandsHaveShort` needs no edit but must stay green, which is what card 29's and card 33's non-empty `Short` requirements discharge.
- **Commit:** `test(lyx): name statusline and watchdog in reed's help tree`

### Card 39: retarget the stencil-seed gate and its two prose references

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/clihelp/annotations.go`
  - `internal/reedcli/watchdog.go`
- **Edits:**
  - `cmd/lyx/stencilseedgate_test.go`
  - `cmd/lyx/stencilseed.go`
  - `internal/clihelp/annotations_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/stencilseedgate_test.go`, retarget every reference to the `header` subcommand onto `statusline`: the file's own leading comment, the test function name `TestReedHeaderCarriesTheStencilSeedSkipAnnotation`, its local variable, the `sub.Name() == "header"` match, the `t.Fatal` message, and the `t.Errorf` message. The gate itself is unchanged — only the command it names. In `cmd/lyx/stencilseed.go`, rewrite the comment that today reads "it -- and skipping also keeps a long-lived pane process (e.g. reed header's keepalive) from …" so it names the surviving long-lived process instead: `lyx reed watchdog`, the detached per-hub daemon. In `internal/clihelp/annotations_test.go`, change the file's leading comment's example from "reed header" to "reed statusline". These last two sites are not enumerated in the discussion's own disposition list; they were found by re-running its mandated scan and are in scope per the `the-header-scan-is-re-run-and-its-residue-is-in-scope` Shared Decision. Do not change `clihelp.SkipStencilSeedAnnotation`'s own value or the annotation mechanism.
- **Commit:** `refactor(lyx,clihelp): retarget the stencil-seed gate onto reed statusline`

### Card 40: correct the engine doc's two verb-naming entries

- **Context:**
  - `internal/reedcli/statusline.go`
  - `internal/reedcli/watchdog.go`
  - `internal/reedengine/watchloop.go`
- **Edits:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/watchloop.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/doc.go`, rewrite the two entries batch 3 deliberately left alone because the verb they name is renamed here. The SIGWINCH entry, which today reads "SIGWINCH is not a substitute (reedcli/header.go's blocking tail): with the header pinned to one row, growing the window delivers SIGWINCH …", must name the detached daemon's process instead of a pinned pane and must drop the `reedcli/header.go` file reference. The stdout/stderr entry, which today reads "The header pane's stdout/stderr is its screen (reedcli/header.go)", must describe the daemon's own discarded stderr instead and name `internal/reedcli/watchdog.go` — including that the daemon points its durable sink at `fabricengine.HubLogsDir(hub)` **before** discarding stderr, which is what keeps its diagnostics reachable. Also update the entry describing a long-lived process holding the logger's output for its whole life so it names the daemon rather than the header pane. In `internal/reedengine/watchloop.go`, rewrite the two comments naming the header pane: the one reading "header pane's RunE fall through and kill the keepalive — so an invalid value is off, never …" must state the same fail-safe rule without naming a pane that no longer exists, and the one reading "already-running signal-mode watcher keeps going until the next header-pane rebuild" must say that a flipped `watchdog:` value takes effect when the worktree's session re-enters the daemon's watched set (a `down` + `up`). Change no logic in `watchloop.go`.
- **Commit:** `docs(reedengine): retarget the package doc's verb references onto the daemon`

### Card 41: add the watchdog daemon integration test

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/reedengine/overlay.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubforge/hub.go`
  - `internal/reedengine/watchdog_integration_test.go`
  - `internal/reedcli/watchdog_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/watchdog_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedcli/watchdog_integration_test.go` carrying the daemon's live-behaviour assertions behind a `//go:build integration` tag. It is a separate file rather than tagged content inside card 37's `watchdog_test.go` because a build tag is per-file and tagging that file would hide its pure-seam tests from the untagged tier where they belong. Build the hub fixture through `internal/hubforge` per the hubforge Fabric-Fixture Invariant — never a hand-assembled hub — and read `internal/reedengine/watchdog_integration_test.go` for this repo's existing live-tmux fixture conventions. Assert, against a hub with two live worktree sessions: resizing one window re-applies only that worktree's layout; a second spawn attempt exits 0 without taking the lock, while an unusable lock path exits non-zero; a non-empty listing resets the idle counter while an empty listing and a killed server (an errored listing) each increment it; the daemon writes its diagnostics into `fabricengine.HubLogsDir(hub)` rather than nowhere, which is the only observable proof the durable sink was pointed before stderr was discarded; a `watchdog: off` worktree is entered as known but starts no watcher goroutine; the daemon exits after the last session goes away, within `watchdogHubIdleCycles * watchdogHubDiscoveryCycle` plus slack, and releases its lock so a later spawn takes it; a `down` immediately followed by an `up` does not kill it; a departing session's watcher is cancelled — bring two sessions up, `down` one, and assert the daemon drops that entry and stops touching that session while the sibling keeps being watched; re-entry re-reads config — `down` a worktree, flip its `watchdog:` key, `up` it again, and assert the new value takes effect without restarting the daemon; and that `attach` and `resume` each attempt the spawn when no daemon holds the lock.
- **Commit:** `test(reedcli): add the per-hub watchdog daemon integration suite`

## Batch Tests

`verify: go test ./internal/reedcli/ ./internal/reedengine/ ./internal/clihelp/ ./cmd/lyx/` names the four packages this batch edits: `reedcli` for the two verbs, the spawn helper and the daemon's pure seams; `reedengine` for the new `ListSessions` and the retargeted package doc; `clihelp` because its annotation test's prose changes here; and `cmd/lyx` for the help-tree and stencil-seed-gate assertions plus the repo-wide guards this batch must not break — `TestDriftGuard_AllCommandsHaveShort` (both new verbs carry a non-empty `Short`), `TestTierPurity_UntaggedTestsSpawnNothing` (cards 37's tests spawn nothing and card 41's live assertions are tagged), and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`.
The verify is deliberately **not** scoped narrower than these four: the daemon spawn is a new `os.Executable()` re-exec site, and `cmd/lyx`'s tier-purity and spawn-observability guards are exactly the mechanical checks that catch a suppression field wired wrong or a spawn left unlogged.
Card 41's integration-tagged assertions do not run under this untagged `verify` — they run under `pipeline.done_gate`'s `go test -tags integration ./...`, which is where every other live-tmux assertion in this repo already runs.
