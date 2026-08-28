# Batch: refuse-a-vanished-worktree-root

```yaml
task: 'reed: resume/down leak lock directories at the stale pre-rename session-name path'
batch: 'refuse-a-vanished-worktree-root'
number: 1
cards: 3
verify: go test ./internal/reedengine/... && go vet -tags integration ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch stops the leak.
It adds one package-level sentinel error and one new told-geometry validator to `internal/reedengine/server.go`, updates the in-package test fixtures that would otherwise start failing, and wires the validator into both operation-lock helpers in `internal/reedengine/lock.go` — immediately after the existing `validateToldAnchorPath` call and BEFORE the `os.MkdirAll` that is what creates the stray directory today.
It is one batch because the sentinel, the validator, and the two wiring points are a single refusal with a single contract: the sentinel is meaningless without the validator, the validator is unreachable without the wiring, and the wiring cannot land without the fixture updates or the whole package's tests go red in the same commit.

The external interface batch 2 consumes is exactly one symbol: the unexported package-level sentinel `errWorktreeRootGone`, matchable with `errors.Is`.
Batch 2 adds no new error and changes no message here.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: the three cards are deliberately ordered so that every intermediate commit is green.
Card 1 adds an unreferenced validator (green).
Card 2 makes the fixtures satisfy a predicate nothing enforces yet (green).
Card 3 turns the predicate on (green, because card 2 already landed).

## Cards

### Card 1: The proven-gone sentinel and the WorktreeRoot liveness validator

- **Context:**
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lock_test.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonestate/standalonestate.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/server.go`
  - `internal/reedengine/server_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a package-level sentinel `errWorktreeRootGone` to `internal/reedengine/server.go`, declared with `errors.New`, carrying a short message such as `"told worktree root is gone"`.
  Give it a doc comment stating that it is the ONE signal the resize watch loop uses to decide the worktree root is provably gone rather than merely unreadable, that it is matched with `errors.Is` and never by substring, and that only two conditions wrap it.

  Add a new function `validateToldWorktreeRootLive(geom Geometry) error`, placed immediately after `validateToldAnchorPath` in the same file.
  Do not modify `validateToldAnchorPath`, and do not add filesystem I/O to it — it is a pure, table-tested shape validator over a different field and its table test must stay I/O-free.
  Give the new function a doc comment at the same depth as its two neighbours, explaining that this task promotes `WorktreeRoot` from a decorative field to a load-bearing control-flow predicate, that a shape violation is a caller bug rather than proof the world changed, and why only the terminal cases carry the sentinel.

  `validateToldWorktreeRootLive` checks shape first, then liveness, in this exact order:
  1. `geom.WorktreeRoot == ""` returns a plain error naming the empty field. It must NOT wrap the sentinel — `os.Stat("")` returns an `fs.ErrNotExist`-matching error, and mapping that onto the sentinel would strand the watch loop dormant forever on a string that can never start existing.
  2. `!filepath.IsAbs(geom.WorktreeRoot)` returns a plain error quoting the value and stating that a relative worktree root silently stats against whatever working directory the process happens to have. It must NOT wrap the sentinel.
  3. `os.Stat(geom.WorktreeRoot)`, then branch:
     - `errors.Is(statErr, fs.ErrNotExist)` returns an error wrapping `errWorktreeRootGone` with `%w`, quoting the path, stating that reed never creates its worktree root, and naming both causes without asserting either: in a hub worktree the directory was renamed or removed, and in standalone mode `--target-dir` names a directory that does not exist. Do not phrase this message as an assertion that a rename happened.
     - any other non-nil `statErr` returns a plain error reporting the underlying error verbatim and saying reed could not determine whether the worktree root exists. It must NOT wrap the sentinel.
     - a successful stat whose `FileInfo.IsDir()` is false returns an error wrapping `errWorktreeRootGone` with `%w`, quoting the path and stating it is a file rather than a directory. This message carries no rename remedy, because nothing was renamed.
     - a successful stat on a directory returns `nil`.

  Add the imports `errors`, `io/fs`, and `os` to `internal/reedengine/server.go` as needed; `path/filepath` is already imported there.
  Derive nothing: the function stats a told value and never computes a path, so the Told-Geometry Invariant and the Cwd Resolution Invariant both hold.

  Add a focused table test `TestValidateToldWorktreeRootLive` to `internal/reedengine/server_test.go`, modelled on the existing `TestValidateToldAnchorPath`'s shape but I/O-aware, covering: an existing directory (nil), an empty value (error, `errors.Is` false), a relative value (error, `errors.Is` false), a path that does not exist (error, `errors.Is` true), and an existing regular file (error, `errors.Is` true).
  Assert the relative-value row refuses regardless of the test process's working directory by pointing the relative value at a name that does exist relative to the package source directory — assert on the returned error alone, and do not change the process working directory.
  Assert the vanished-path message names both causes and asserts neither: it must contain the quoted path and a mention of `--target-dir`, and a message that claims a rename outright is a test failure.
  Assert the not-a-directory row's message does NOT carry the rename remedy the vanished row does.

  Add a separate test `TestValidateToldWorktreeRootLive_UnreadableParentIsNotTheSentinel` provoking a real non-`fs.ErrNotExist` stat failure: create a parent directory, create the worktree root inside it, `os.Chmod` the parent to `0o000`, register a `t.Cleanup` restoring the mode so `t.TempDir` cleanup can succeed, then assert the returned error is non-nil and `errors.Is(err, errWorktreeRootGone)` is false.
  Skip the test on `runtime.GOOS == "windows"` and when `os.Geteuid() == 0`, since both defeat the permission bit.
  Do not introduce an injectable stat seam anywhere in production code.
- **Commit:** `fix(reed): add a WorktreeRoot liveness validator and its proven-gone sentinel`

### Card 2: Materialize WorktreeRoot in every in-package fixture that reaches the lock helpers

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server.go`
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/mouse_boot_integration_test.go`
  - `internal/reedengine/watchdog_integration_test.go`
  - `internal/reedengine/attachgeometry_integration_test.go`
  - `internal/reedengine/header_test.go`
  - `internal/reedengine/server_test.go`
- **Edits:**
  - `internal/reedengine/lock_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Make `newTestEngine` in `internal/reedengine/lock_test.go` create its `worktreeRoot` directory with `os.MkdirAll(worktreeRoot, 0o755)`, failing the test via `t.Fatalf` on error.
  Create the WORKTREE ROOT only.
  Do not create `anchorPath`, and do not create the `PaneCwd` directory — leaving both uncreated preserves the fixture's stated intent that the fields are distinct values so a field mix-up surfaces, and it makes the shared fixture stand in for the standalone shape, where the anchor is a state directory the engine itself materializes.
  Extend the helper's doc comment to say so.

  Make `TestWithOpLock_PathIsUnderDotLyx` in the same file create its own inline `worktreeRoot` the same way, before building the engine.
  Do not migrate this test onto `newTestEngine` — it exists precisely to make `AnchorPath` diverge from `WorktreeRoot`, and the shared fixture would erase the distinction it was written to observe.

  Sweep the rest of the package for any other in-package test that reaches `withOpLock` or `withTryOpLock` through an inline `Geometry` literal, INCLUDING the `integration`-tagged files, and materialize its worktree root in place if it does not already.
  The sweep is a verification of a known inventory rather than an open-ended hunt.
  Grepping this package for the token `Geometry{` finds inline literals in exactly five files, and every one is listed in this card's `Context:` or `Edits:`: `internal/reedengine/lock_test.go`, `internal/reedengine/server_test.go`, `internal/reedengine/header_test.go`, `internal/reedengine/contract_integration_test.go`, and `internal/reedengine/mouse_boot_integration_test.go`.
  Re-run that grep to confirm the inventory has not drifted before concluding the sweep, and report any sixth file as a finding rather than editing it blind.
  Four facts to verify rather than assume, all four expected to need no change:
  - `newIntegrationEngine` in `internal/reedengine/mouse_boot_integration_test.go` already creates its worktree directory with `os.MkdirAll`, and its inline literal points `WorktreeRoot` at that created directory.
  - Both inline literals in `internal/reedengine/contract_integration_test.go` set `WorktreeRoot` to a `t.TempDir()` that already exists.
  - `internal/reedengine/watchdog_integration_test.go` contains no `Geometry` literal at all: it builds every engine through `setupAttachGeometryFixture`, which routes to `newIntegrationEngine`, so it inherits that helper's already-created worktree directory and needs no change.
  - `internal/reedengine/attachgeometry_integration_test.go` likewise contains no `Geometry` literal and builds through `newIntegrationEngine`, so it is not an inline site either.
  The literal in `internal/reedengine/header_test.go` sets only `RepoName` and `HubPath` and reaches neither lock helper, so it is out of the sweep's criterion and stays untouched.
  The three literals in `internal/reedengine/server_test.go` are table-test rows that call the told-geometry validators directly rather than either lock helper, so they are out of the sweep's criterion too — card 1 and card 3 own that file's changes, and this card must not edit it.
  If a test in this package was silently relying on its worktree root being absent, that is a real finding to report rather than something to paper over.
  Leave `TestWithTryOpLock_ToldGeometryValidationFailureLeavesTheLockFileUntouched` and `TestEngine_SocketAndSessionName` alone: the first refuses at the tmux-identity check before the new predicate is ever reached, and the second acquires no lock.
- **Commit:** `test(reed): materialize WorktreeRoot in the reedengine lock fixtures`

### Card 3: Refuse at both op-lock helpers, before any directory is created

- **Context:**
  - `internal/reedengine/server.go`
  - `internal/reedengine/geometry.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/reapply.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/server_test.go`
  - `internal/reedengine/doc.go`
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/lock.go`, call `validateToldWorktreeRootLive(e.geom)` in BOTH `withOpLock` and `withTryOpLock`, returning its error immediately, positioned after the existing `validateToldAnchorPath` call and before the `dotLyx := e.stateDir()` line that precedes `os.MkdirAll`.
  `withOpLock` returns the bare error; `withTryOpLock` returns `(false, err)`, matching how it returns its two existing validation failures.
  Wiring only `withOpLock` would leave the watchdog — the actual leak source, which reaches the lock through `withTryOpLock` — untouched, so both helpers get the call.
  Do not change `e.stateDir()`, and do not gate the `os.MkdirAll(dotLyx, 0o755)` call on anything: it must keep creating the anchor's `.lyx` directory exactly as it does today.

  Extend `withOpLock`'s doc comment to record the new ordering — told tmux identity, then told anchor shape, then told worktree-root shape and liveness, then `MkdirAll`, then the lock — and to state why the new check runs last among the validators: it is the only one that touches the filesystem, so the two cheap contract refusals should still fire first on a geometry that is wrong in several ways at once.
  Extend `withTryOpLock`'s doc comment to say it keeps all THREE told-geometry pre-flight validations, updating its existing "both told-geometry pre-flight validations" wording.

  Add these tests to `internal/reedengine/server_test.go`, each mirroring `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`'s shape — the operation body must never run, the error must name the vanished path, and the lock file must not exist afterwards:
  - `withOpLock` refuses when `WorktreeRoot` names a directory that does not exist, and afterwards none of the worktree directory, the anchor path, the anchor's `.lyx` directory, or the lock file has been created.
  - The same for `withTryOpLock`, written as its own separate test rather than a shared subtest, so a regression that fixes only one helper fails here.
  - `WorktreeRoot` exists as a regular file rather than a directory: refused, with the not-a-directory message rather than the vanished-path one.
  - The standalone non-existent-target shape: `WorktreeRoot` does not exist AND `AnchorPath` is a different, also non-existent path. Refused with the vanished-path message, and neither path is created. This is the regression guard on the intended standalone behaviour change.
  - The standalone first-run shape: `WorktreeRoot` exists as a directory and `AnchorPath` is a DIFFERENT path that does not exist yet, which is `newTestEngine`'s fixture as of card 2. The operation must SUCCEED and the anchor's `.lyx` directory must have been created. This is the test that fails if a later change re-gates the predicate on `AnchorPath`.
  - The hub first-run shape: `WorktreeRoot` exists, `AnchorPath` equals it, and no `.lyx` exists yet. The operation must SUCCEED and create `.lyx`.
  - The refusal error from `withOpLock` is matchable with `errors.Is(err, errWorktreeRootGone)`, asserted with `errors.Is` and never by substring.
  Reuse the existing `fileExists` helper in this file rather than adding a second one.

  In `internal/reedengine/doc.go`, add a geometry-lifetime bullet to the package doc's existing bullet list introduced by the line "Load-bearing behavioral assumptions, each with the rationale that makes it", recording two things.
  First the rule this fix establishes: a told `Geometry` is resolved once per process and pinned for that process's whole life, so a long-lived process such as the header pane's keepalive holds a frozen `WorktreeRoot` that a `mv` of the worktree makes stale; every operation therefore re-checks that told worktree root's liveness at the op-lock chokepoint, and refuses rather than creating substrate under a path that is no longer a worktree.
  Second the user-visible standalone consequence: a `--target-dir` naming a directory that does not exist is now refused at the first engine op instead of proceeding and deriving a state directory for it.

  In `tools/sandbox/SANDBOX-REED-SUITE.md`, extend the M24 and M25 "Watch:" text.
  M24 gains an assertion that after the `kill-session` and `resume` in the renamed worktree, the hub root contains no directory named after the pre-rename worktree.
  M25 gains the same assertion, plus the instruction to re-check the hub root after waiting well past the two-second watchdog poll cycle — because `down` deliberately leaves the abandoned session running, checking too early would mask a watcher that resumed leaking.
  Add the no-stray-directory assertions only.
  Do not add any wording about the abandoned session's header pane quieting down: that is batch 2's dormancy behaviour, this card's commit does not implement it, and card 4 adds that line to the same two milestones in the commit that does.
  Follow the file's existing prose conventions and the repo's semantic-line-break markdown rule.
  Do not add a new milestone, do not renumber existing milestones, and do not change the verdict-summary block at the end of the file.
- **Commit:** `fix(reed): refuse a vanished told worktree root at the op-lock chokepoint`

## Batch Tests

`verify:` runs `go test ./internal/reedengine/...`, which covers every untagged test in the package and in `internal/reedengine/render`, and `go vet -tags integration ./internal/reedengine/...`, which type-checks the `integration`-tagged files without running them.
The vet half is the tagged-file sweep this batch's fixture card needs: `go test` with no tags never compiles those files, so a fixture sweep that stopped at the default build would silently skip their inline `Geometry` literals.

The new coverage lands in `internal/reedengine/server_test.go` (the validator table, the unreadable-parent non-sentinel case, and the six lock-helper refusal and success shapes) and in `internal/reedengine/lock_test.go` (the fixture changes, which every existing test in the package inherits).

Running the `integration`-tagged tests for real is deliberately NOT part of `verify:`.
`TestWatchdogSelfHeal_HookProbeMatchesLiveTmux` in `internal/reedengine/watchdog_integration_test.go` fails deterministically at this branch's tip on unmodified code, for the reason recorded in the overview's `a-pre-existing-integration-failure-will-trip-the-done-gate` Decision, and no card in this batch touches it.
The implementer should still run `go test -tags integration ./internal/reedengine/...` once by hand after card 3 and confirm that this one test is the ONLY failure — a second failure would be a real regression from this batch.
