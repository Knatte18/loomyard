# Batch: watchdog-seam-and-focus-pin

```yaml
task: "reed: born-as-strand for loom start's operator attach"
batch: "watchdog-seam-and-focus-pin"
number: 1
cards: 7
verify: go test ./internal/reedengine/... ./internal/reedcli/... ./internal/burlercli/... && go test -tags integration ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch prepares everything `internal/loomcli` needs from the reed side, and pins the two reed behaviours batch 2's design leans on but does not own.
It moves the per-hub watchdog daemon's spawn out of `internal/reedcli` into `internal/reedengine` as an exported package-level function, leaving `reedcli`'s own method a one-line caller so `up`/`attach`/`resume` behave identically;
re-pins the standalone-never-touches-the-hub-watchdog property that the move puts at risk, since `internal/reedengine` is also standalone's engine;
and adds two tests over reed behaviour batch 2 depends on — `focusTarget`'s bottom-most default across the three accepted focus outcomes, and `AddStrand` accepting an empty `Cmd`.

**External interface batch 2 consumes:** `reedengine.SpawnWatchdog(hubPath, tmuxPath string, suppress bool)`.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: the existing `internal/reedcli/spawnwatchdog_test.go` keeps its two early-return tests rather than deleting them.
The method has no return value and `internal/reedcli` deliberately gains no injected seam (`_mill/discussion.md`'s `watchdog-call-is-an-injected-seam-on-loomcli` decision scopes injection to `loomcli` alone), so forwarding is not directly observable from a test;
what those two tests still prove — that calling the method under a test binary re-execs nothing — remains exactly as valuable after the move as before it.

## Cards

### Card 1: Extract the watchdog spawn into `internal/reedengine`

- **Context:**
  - `internal/reedcli/spawnwatchdog.go`
  - `internal/reedengine/lock.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/spawnwatchdog.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/reedengine/spawnwatchdog.go` in `package reedengine`, holding one exported package-level function `SpawnWatchdog(hubPath, tmuxPath string, suppress bool)` with no return value. Its body is `ensureWatchdogSpawned`'s current body with the three receiver reads replaced by the parameters: `c.suppressWatchdogSpawn` becomes `suppress`, `c.hubPath` becomes `hubPath`, and `c.eng.TmuxPath()` becomes `tmuxPath`. Everything else is carried over unchanged — the `suppress || hubPath == ""` early return, the `fabricengine.HubScratchDir` + `os.MkdirAll` step with its `logger.Warn` on failure, the `os.Executable` resolution with its own `logger.Warn`, the `exec.Command(exe, "reed", "watchdog", "--hub-path", hubPath, "--tmux", tmuxPath)` construction, `cmd.Dir = hubPath`, the `proc.Detach(cmd)` call, the `logger.Info` spawn line, and the `cmd.Start()` call whose failure logs `logger.Warn` and returns. Do not add a `Wait()`: the Live-Substrate Spawn Observability invariant's detached-spawn clause requires the spawn be logged alone, which the carried-over `logger.Info` already does. Keep every log message string byte-identical to the originals, including their `reed: ` prefix, so operator-facing log output does not change. Write a file doc comment in this package's style stating that this file owns the per-hub watchdog daemon's detached spawn for every caller, that the daemon is per-hub and deliberately outlives any one worktree's `Engine` (which is why `cmd.Dir` is pinned to the hub and why this is a package function rather than an `Engine` method), and that `suppress` is passed in rather than computed here so the "never re-exec `os.Executable()` under `go test`" guard stays visible at each call site. Note in the doc comment that `internal/reedengine` is also the standalone engine and that this function is inert unless called with a non-empty hub path, which standalone never does. The three new imports this file needs — `os/exec`, `github.com/Knatte18/loomyard/internal/proc`, and `github.com/Knatte18/loomyard/internal/fabricengine` — create no import cycle: `fabricengine` imports no `reedengine` file today.
- **Commit:** `reed: extract the per-hub watchdog spawn into reedengine`

### Card 2: Reduce `reedcli`'s method to a one-line caller

- **Context:**
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/reedcli/cli.go`
- **Edits:**
  - `internal/reedcli/spawnwatchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `ensureWatchdogSpawned`'s body in `internal/reedcli/spawnwatchdog.go` with a single delegating call to `reedengine.SpawnWatchdog(c.hubPath, c.eng.TmuxPath(), c.suppressWatchdogSpawn)`. Keep the method on `*reedCLI` with its existing name and signature so `up`, `attach` and `resume` are untouched. Drop the now-unused `os`, `os/exec`, `fabricengine`, `logger` and `proc` imports, adding the `reedengine` import. Rewrite the file doc comment and the method doc comment so neither describes the mechanism any more — both now point at `reedengine.SpawnWatchdog` as the owner and state only what this layer contributes: the three values it supplies, and that `suppressWatchdogSpawn` is this CLI's own `testing.Testing()`-derived guard. Do not reorder or otherwise change the three call sites in `internal/reedcli`.
- **Commit:** `reed: delegate reedcli's watchdog spawn to the reedengine seam`

### Card 3: Tier 1 coverage for the extracted seam

- **Context:**
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/reedcli/spawnwatchdog_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/spawnwatchdog_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an untagged test file in `package reedengine` pinning `SpawnWatchdog`'s two no-spawn early returns, driven through the exported function directly. One test calls it with `suppress` true and a `t.TempDir()` hub path; another calls it with `suppress` false and an empty hub path. In both, reaching the end of the test is the assertion — either would re-exec the test binary and recurse the whole suite if the early return were lost, which is precisely what the Live-Substrate Spawn Observability invariant's "never re-exec `os.Executable()` under `go test`" clause bars. Say that in the file doc comment, and state that no test here may ever call `SpawnWatchdog` with `suppress` false and a non-empty hub path. This file performs no spawn and drives no tmux, so it stays untagged per the Test Tier Purity Invariant.
- **Commit:** `reed: pin SpawnWatchdog's no-spawn early returns`

### Card 4: Re-point `reedcli`'s surviving watchdog tests at the seam

- **Context:**
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/reedcli/spawnwatchdog.go`
- **Edits:**
  - `internal/reedcli/spawnwatchdog_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Keep all three existing tests in `internal/reedcli/spawnwatchdog_test.go` running and green, and rewrite the file doc comment plus the two `TestEnsureWatchdogSpawned_*` doc comments to say what they now pin: that the method reaches the `reedengine` seam and that a call under a test binary re-execs nothing, not the mechanism itself, which is now covered in `internal/reedengine`. Do not re-test `SpawnWatchdog`'s body here. Leave `TestWatchdogLockPath_IsHubScratchDirLockFile` unchanged — it pins a lock path this batch does not move.
- **Commit:** `reed: re-point reedcli's watchdog tests at the extracted seam`

### Card 5: Re-pin the standalone-never-touches-the-hub-watchdog boundary

- **Context:**
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/burlercli/cli.go`
- **Edits:**
  - `internal/burlercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `TestProductionFiles_NeverReferenceHubWatchdogMechanism` in `internal/burlercli/wiring_test.go` so the property survives the move. Two changes. First, widen the scanned file set: in addition to this package's own `*.go` production files, scan `internal/standalonegeom`'s production files, reached by a relative glob from this package's directory. Second, add `reedengine.SpawnWatchdog` to the banned-token set alongside the existing `fabricengine.HubScratchDir` and `reed watchdog` tokens, applied to every scanned file. Keep the existing `_test.go` skip and the existing per-token error messages, adding one for the new token that says standalone must never call the hub watchdog seam. Update the test's doc comment to record why the scan widened: the watchdog spawn now lives in `internal/reedengine`, which standalone also uses as its engine, so the property is no longer structurally obvious from `internal/burlercli`'s own files alone — this is the guard `_mill/discussion.md`'s `watchdog-seam-lives-in-reedengine` decision asks for in place of leaving the boundary unpinned. This test spawns nothing and stays untagged.
- **Commit:** `reed: widen the standalone hub-watchdog boundary guard to cover the new seam`

### Card 6: Pin `focusTarget`'s three accepted outcomes

- **Context:**
  - `internal/reedengine/render/focus.go`
  - `internal/reedengine/render/policy.go`
  - `internal/reedengine/render/types.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/render/focus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an untagged test file in `package render` pinning `orderStack` composed with `focusTarget` across the three focus outcomes batch 2's design accepts, one test each. (1) Cold bootstrap: two parentless strands in insertion order — a status strand then an operator strand, neither carrying `Display.Focus` — resolve to the operator strand's pane, because `orderStack` sorts by chain depth with `sort.SliceStable` and equal-depth strands keep insertion order, leaving the operator strand last, and `focusTarget` falls through to bottom-most. (2) Re-entrant bootstrap: the same two plus a depth-1 strand parented onto one of them resolve to the depth-1 strand's pane, because a parentless strand is no longer bottom-most once anything deeper exists. (3) A persisted focus flag wins regardless of depth: the same three plus a parentless strand carrying `Display.Focus: true` resolve to that strand's pane, because `focusTarget` scans for the bottom-most `Display.Focus` strand before falling back to bottom-most-overall. Give every strand a distinct non-empty `PaneID` and `GUID` so a failure message names which one won. State in the file doc comment that case 3 is the one an implementer is most likely to read as a bug and "fix", and that it is the expected outcome: in a worktree opened through the VS Code chain the operator's own working pane already exists and is the agent session the chain focused. This pins an assumption `internal/loomcli` relies on rather than declares, which is why the test lives here beside the code that owns it.
- **Commit:** `reed: pin focusTarget's three accepted focus outcomes`

### Card 7: Prove `AddStrand` accepts an empty `Cmd` against a real session

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/contract_integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/emptycmd_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `//go:build integration`-tagged test file in `package reedengine` proving that `AddStrand` with `Cmd: ""` succeeds against a real tmux session and leaves a live pane. Bring a session up and add one strand with an empty `Cmd`, a non-empty `NameOverride`, and `Display.Anchor: render.AnchorBelowParent`; assert `AddStrand` returns no error and that `Status()` reports that strand `Live`. Follow `internal/reedengine/contract_integration_test.go`'s own conventions for fixture setup, tmux-binary resolution, the skip when the multiplexer is absent, and teardown — do not invent a second rig. This is the one assumption in `_mill/discussion.md`'s `empty-cmd-leaves-the-panes-own-shell` decision that no existing code already pins: `launchStrandLocked` issues `send-keys -t <pane> -l ""` followed by `Enter` for an empty command, and `sendKeysLiteralArg("")` returns the empty string, so the test confirms tmux accepts that argument rather than assuming it. The file carries the `integration` tag rather than `smoke` per the overview's `real-tmux-tests-follow-each-packages-own-tag` Shared Decision.
- **Commit:** `reed: prove AddStrand accepts an empty Cmd against a real session`

## Batch Tests

`verify:` runs two chained commands.
The untagged half, `go test ./internal/reedengine/... ./internal/reedcli/... ./internal/burlercli/...`, covers every Tier 1 file this batch creates or edits: `internal/reedengine/spawnwatchdog_test.go` (card 3), `internal/reedcli/spawnwatchdog_test.go` (card 4), `internal/burlercli/wiring_test.go` (card 5), and `internal/reedengine/render/focus_test.go` (card 6, reached by the `./internal/reedengine/...` wildcard).
It also compiles cards 1 and 2's production changes and re-runs `internal/reedcli`'s existing suite, which is what proves the delegation did not change `up`/`attach`/`resume`.

The tagged half, `go test -tags integration ./internal/reedengine/...`, is chained with `&&` rather than folded into the untagged invocation because card 7's file is build-tagged and would otherwise never compile.
It is a separate invocation rather than a comma-joined `-tags` value so it makes no assumption about whether this repo gives its tagged suites mutually exclusive semantics.
This half needs a real tmux binary;
the suite skips rather than fails when one is absent, matching `internal/reedengine`'s existing integration tier.

No unbounded repo-wide run is used: the hub's `pipeline.done_gate` covers cross-package regressions once, at Handoff.
