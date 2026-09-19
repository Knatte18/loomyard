# Batch: daemon wiring

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
batch: 'daemon wiring'
number: 3
cards: 6
verify: go test ./internal/reedcli/ ./internal/reedengine/ && go vet -tags integration ./internal/reedcli/
depends-on: [1, 2]
```

## Batch Scope

This batch wires batch 1's `ReapSession`/`ShellPath` and batch 2's pure seams into the running daemon: the reap pass inside `runWatchdogLoop`, the off-loop reap goroutines and their bookkeeping, the `--shell` flag from spawn site through command to loop, and the call-site updates the widened `runWatchdogLoop` signature forces on the existing tagged test file.
It is one batch because every card is a consequence of the same signature change — `runWatchdogLoop` gaining `shellPath` and `watchdogTiming` — and splitting them would leave the package uncompilable between batches.
It depends on both root batches: card 9 calls `reedengine.ReapSession`, card 10 calls `planReapCycle`/`hubIsLiveDir`/`worktreeRootGone`/`watchdogDefaultTiming`, and card 12 calls `Engine.ShellPath`.

Batch-local decision beyond `## Shared Decisions`: the in-flight set's synchronization is a plain `map[string]bool` owned exclusively by the loop goroutine, with removal driven by a buffered completion channel the loop drains at the top of every tick — never a mutex, and never a direct map write from a reap goroutine.

## Cards

### Card 9: the reap goroutine and its completion channel

- **Context:**
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the reap-dispatch helper `runWatchdogLoop` will call, in `internal/reedcli/watchdog.go`. It is a small function — name it `dispatchReap` — taking the `*sync.WaitGroup`, the completion channel, and the values one reap needs (`hub`, `tmuxPath`, `shellPath`, `sessionName`), calling `wg.Add(1)` and starting one goroutine.

  The goroutine's body: `defer wg.Done()`, call `reedengine.ReapSession(tmuxPath, shellPath, reedengine.ServerName(hub), sessionName)`, and on a non-nil error log it at `Warn` naming `hub`, `session` and `err` — a destructive unattended action's failure is what the operator needs in the hub's durable log. Then, as its **only** cross-goroutine act, perform a non-blocking send of `sessionName` on the completion channel: `select { case done <- sessionName: default: }`.

  A failed reap does nothing beyond the `Warn`: there is no retry loop, no backoff, and no escalation. Recovery is the ordinary loop — the gone-counter was deleted at dispatch, so a still-live orphan re-confirms across three more affirmative cycles and is reaped again. State this in the doc comment, along with why the send is non-blocking: a blocking or unbuffered send would deadlock a reap goroutine against the deferred `WaitGroup` wait at daemon exit, since nothing drains the channel once the loop is out of its `for`. A dropped send is inert — the loop is gone, and with it the set.

  The goroutine never touches the in-flight map, the gone-counter map, or the `known` map. All three stay single-threaded on the loop goroutine.
- **Commit:** `feat(reedcli): add the daemon's off-loop reap dispatch`

### Card 10: `runWatchdogLoop` gains the reap pass

- **Context:**
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `runWatchdogLoop`'s signature from `func runWatchdogLoop(ctx context.Context, hub, tmuxPath string) error` to `func runWatchdogLoop(ctx context.Context, hub, tmuxPath, shellPath string, timing watchdogTiming) error`. `shellPath` sits immediately after `tmuxPath` — the two are told together, travel together, and are both told-not-derived values.

  Log a `Warn` naming `hub` at loop start when `shellPath` is empty, stating that the reap will kill pane root pids without their descendants on Windows. This is the degradation the discussion's `the-daemon-is-told-its-shell` decision chose over refusing to start.

  Replace every use of the package constants inside the loop with the told values: the ticker is built from `timing.DiscoveryCycle`, the idle-exit comparison uses `timing.IdleCycles`, and the reap threshold is `timing.OrphanGoneCycles`.

  Add three loop-owned mutable structures beside the existing `known` map: `goneCounters := make(map[string]int)`, `inFlight := make(map[string]bool)`, and a buffered `reapDone := make(chan string, 64)`. Add `var wg sync.WaitGroup`. Extend the existing deferred cleanup so it cancels every `known` entry as it does today and **then** calls `wg.Wait()`, so the daemon never returns mid-reap and never force-kills its own reaper.

  Restructure one cycle's body, after the existing `sessionsAreIdle`/`idleCycles` handling and its `continue`, into this exact order:

  1. Drain `reapDone` non-blockingly in a loop — `for { select { case name := <-reapDone: delete(inFlight, name); default: } }`, breaking out on the default branch — clearing finished names from the in-flight set before anything reads it.
  2. `hubLive := hubIsLiveDir(hub)`.
  3. Build the per-name gone map only when `hubLive` is true: for each name in `live`, `gone[name] = worktreeRootGone(filepath.Join(hub, name))`. When `hubLive` is false, pass a nil map — the reap pass will not read it. The join is the exact mapping, not a scan: hub-mode `reedengine.SessionName(worktreeRoot)` is `filepath.Base(worktreeRoot)` verbatim, so with `hub` being the parent directory the join recovers the worktree root exactly.
  4. `reap, remaining := planReapCycle(live, hubLive, gone, goneCounters, inFlight, timing.OrphanGoneCycles)`.
  5. For each name in `reap`: look up its `known` entry, call its `cancel` when non-nil, `delete(known, name)`, set `inFlight[name] = true`, log the reap at `Warn` naming `hub` and `session`, and call `dispatchReap`. Cancelling the entry first is what stops a goroutine from polling a session the daemon is killing.
  6. Call `planSessionDiff(remaining, known)` in place of today's `planSessionDiff(live, known)`, and leave the appeared/departed handling below it exactly as it is.

  `remaining` already excludes both this cycle's selected names and every name still in-flight from an earlier cycle, which is load-bearing rather than belt-and-braces: a reap dispatched last cycle whose session tmux still lists would otherwise read as *appeared*, and the daemon would enter and start watching a session it is in the middle of killing.

  Do not change `planSessionDiff`, `sessionsAreIdle`, `enterSession` or `resolveWatchedSession`. The idle counter keeps its existing meaning — a cycle whose listing was affirmative resets `idleCycles` to zero even when every session in it was reaped, because the hub socket did answer with sessions, and a hub going genuinely quiet afterwards is observed by the following cycles through the existing path.

  Update `runWatchdogLoop`'s doc comment to describe the reap pass, the cycle ordering above, and why the reap runs off-loop: `reapPaneChildren` waits up to `reapExitTimeout` (15s) and then up to `forceKillExitGrace` (5s) per straggler — both values inlined here, no file read needed — so an inline reap would stall one tick for ~20s, which would break the confirmation rule's quoted cadence, delay entering a newly-appeared healthy session, and leave `ctx.Done()` unread for the whole stall.
- **Commit:** `feat(reedcli): reap orphaned sessions from the watchdog discovery loop`

### Card 11: `--shell` on the watchdog command

- **Context:**
  - `internal/reedengine/config.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `watchdogCmd`, declare a third flag variable `shellPath` beside the existing `hubPath`/`tmuxPath`, and register it with `cmd.Flags().StringVar(&shellPath, "shell", "", ...)`. Word its usage string so it reads as accepted-but-optional, unlike the two existing flags whose usage strings both end in `(required)` — this one is told to the daemon for a complete reap and is never refused when empty.

  Replace the two inline pre-flight checks in `RunE` with a single `validateWatchdogFlags(hubPath, tmuxPath)` call, reporting a non-nil error through the same `clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))` shape the inline checks use today and returning nil, so the command's observable output and exit behaviour are unchanged. The call runs before any side effect — before the durable-sink pointing, before the stdio discard, and before the lock acquisition. Do not pass `shellPath` to it: the function has no shell parameter, which is what makes the never-validated rule structural.

  Pass the flag through to the loop: `runWatchdogLoop(cmd.Context(), hubPath, tmuxPath, shellPath, watchdogDefaultTiming())`.

  Update the command's `Long` text and its `Example:` line, both of which today show only `--hub-path` and `--tmux` while advertising the hand-run foreground path as a real diagnosis path. Both gain `--shell` — it is what a hand-run daemon should be told for its reap to be complete, even though it is not enforced. Keep the existing `Use` and `Short` unchanged, so the `Command()`/`RunCLI` seam and the help-tree tests CONSTRAINTS.md's CLI / Cobra Invariant guards stay satisfied.
- **Commit:** `feat(reedcli): add --shell to lyx reed watchdog`

### Card 12: the spawn site tells the daemon its shell

- **Context:**
  - `internal/reedengine/lock.go`
- **Edits:**
  - `internal/reedcli/spawnwatchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the single `exec.Command` construction in `ensureWatchdogSpawned` to pass the new flag, appending `"--shell", c.eng.ShellPath()` after the existing `"--tmux", c.eng.TmuxPath()` arguments. There is exactly one such construction serving all three spawn sites, so this is one edit.

  Pass `c.eng.ShellPath()` unconditionally, empty or not. An empty value is never a reason to skip the spawn: `ensureWatchdogSpawned` is best-effort and its child's stderr is discarded, so refusing here would silently cost the hub its entire watchdog daemon over one empty config value, while an empty shell costs only the descendant half of a reap on Windows.

  Add `shell` to the `logger.Info` spawn line's key/value pairs beside the existing `exe`, `hub` and `tmux` keys, so the value the daemon was told is observable at the spawn.

  Update the file's header comment, which spells out the spawned command line as `lyx reed watchdog --hub-path <hub> --tmux <tmux>`, so it names `--shell` too.
- **Commit:** `feat(reedcli): tell the spawned watchdog its shell path`

### Card 13: update the existing tagged call sites

- **Context:**
  - `internal/reedcli/watchdog.go`
- **Edits:**
  - `internal/reedcli/watchdog_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/reedcli/watchdog_integration_test.go` calls `runWatchdogLoop(ctx, h.Path, tmuxPath)` at **four** places — once each inside the tests named `TestWatchdogIntegration_DiscoversAndDropsDepartedSessions`, `TestWatchdogIntegration_ResizeAppliesOnlyToThatWorktree`, `TestWatchdogIntegration_DownThenUpDoesNotKillDaemon` and `TestWatchdogIntegration_ReEntryReReadsFlippedConfig`. Update all four to the widened signature by passing the engine's own shell and the production timing: `runWatchdogLoop(ctx, h.Path, tmuxPath, eng1.ShellPath(), watchdogDefaultTiming())`, naming whichever engine variable is already in scope at each call site. Missing any one of the four leaves it on the old signature and fails this batch's own `go vet -tags integration` half.

  All four sit inside a `go func() { loopDone <- ... }()` wrapper, so each is a one-line argument-list edit.

  Passing `watchdogDefaultTiming()` rather than a compressed timing keeps every existing assertion's behaviour identical — these tests already wait on `watchdogHubDiscoveryCycle` multiples through their own `waitForCondition` helper, and changing their cadence here would be an unrelated change to tests this batch is only keeping compilable. Batch 4's new cases are the ones that drive compressed timings.

  Change nothing else in this file: no new assertions, no fixture changes, no helper changes. This is a mechanical signature update, and the three tests passing unchanged afterwards is the evidence the reap pass did not alter discovery, departure teardown, idle-exit, or config re-read behaviour.
- **Commit:** `test(reedcli): update watchdog integration call sites for the widened loop signature`

### Card 14: confirm no other call site broke

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/reedcli/watchdog_test.go`
  - `internal/reedcli/watchdog_integration_test.go`
  - `internal/reedcli/spawnwatchdog_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** A zero-diff verification gate. Grep the repository for `runWatchdogLoop` and confirm every call site outside the loop's own declaration and doc comments is one of the five this batch already updated — the one in the command's `RunE` (card 11) and the four in the integration test file (card 13). A grep returning any other count than five is the failure this gate exists to catch. Then run the batch's own `verify:` command and confirm both halves pass: the untagged test run, and the `go vet -tags integration` compile of the tagged file.

  This card exists because the `runWatchdogLoop` signature change is the one edit in this batch that can break a file no card lists, and because a partially-updated call-site set is the likeliest way to get it wrong. The untagged `go test` run alone cannot catch either — a build-tagged file is excluded from that compile entirely — so the `-tags integration` vet is the only gate that does.

  Confirm the gate rather than change anything: this card writes no file and produces no diff.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/reedcli/ ./internal/reedengine/ && go vet -tags integration ./internal/reedcli/` has two halves, both needed.

The `go test` half runs both untagged package suites this batch's wiring can regress — `internal/reedcli` for the loop and the command, `internal/reedengine` because card 9 introduces the package's first production caller of `ReapSession`.
Batch 2's `planReapCycle` and predicate tests and batch 1's `ReapSession` ordering tests both re-run here, which is the regression signal that the wiring uses them as they were specified.

The `go vet -tags integration` half compiles `internal/reedcli`'s `integration`-tagged files, which the untagged run excludes by build tag.
It is what catches card 13's three call-site updates being incomplete or wrong — a real risk, since the widened signature is a compile break in a file no untagged command touches.
It vets rather than runs, so the gate needs no live tmux server and stays fast;
actually executing the tagged suite is batch 4's verify.
