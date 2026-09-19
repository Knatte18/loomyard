# Batch: engine-seam

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'engine-seam'
number: 1
cards: 4
verify: go test ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch builds the whole engine-side seam and wires the first of its two call sites.
It factors the "session up and holding at least one pane" predicate out of `ensureServerAndSessionLocked` into one shared helper, extracts `Up()`'s closure body into `upLocked`, adds `ensureSessionLocked` plus its exported `EnsureSession` wrapper, and switches `AddStrand`'s pre-flight from `requireSessionLocked` to `ensureSessionLocked`.
It is one batch because every card reads the same two files and the seam is meaningless split across batch boundaries.

The external interface the next batch consumes is `func (e *Engine) EnsureSession() (booted bool, err error)` — the exported `withOpLock` wrapper `internal/reedcli/attach.go` calls.
This is the task's only exported-surface addition; `UpResult`, `Up()`, `Status()`, `AttachArgv()` and `AddStrand()` keep their current signatures.

Batch-local decision: `sessionSubstrateLocked` returns two booleans rather than one.
`ensureServerAndSessionLocked` needs to distinguish "not up at all" from "up but holding zero panes" so it can kill the husk before re-booting, while `ensureSessionLocked` reads only the combined `usable` answer.
A single-bool predicate would force the husk branch to re-probe, which is both a wasted round trip and a race.

## Cards

### Card 1: factor the liveness predicate and extract `upLocked`

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/probe.go`
  - `internal/reedengine/header.go`
  - `internal/reedengine/serverlog.go`
  - `internal/reedengine/mouse.go`
  - `internal/reedengine/watchdog.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an unexported method `func (e *Engine) sessionSubstrateLocked() (up bool, usable bool, err error)` to `internal/reedengine/lifecycle.go`.
  It calls `hasSession` once against this engine's own session name; on error it returns `(false, false, ...)` wrapping that error under the same "check session" prefix `ensureServerAndSessionLocked` produces today.
  When the session is absent it returns `(false, false, nil)` without listing panes.
  When the session is present it calls `listPanes`; on error it returns `(true, false, ...)` under today's "list panes" prefix, and otherwise returns `(true, <the live pane list is non-empty>, nil)`.
  Its doc comment must state that this is the single "usable substrate" predicate both `ensureServerAndSessionLocked`'s already-up early return and `ensureSessionLocked` read, that neither may carry its own copy of the non-empty-pane-list condition, and that a session holding zero panes is broken substrate a strand can never be split into.

  Rewrite `ensureServerAndSessionLocked`'s already-up block to call `sessionSubstrateLocked` instead of making the two round trips inline.
  The block keeps its current behaviour exactly: a usable session returns `(false, nil, nil)` early, and a session that is up but not usable falls through to the existing `kill-session` call built on `exactSessionTarget` and then to the fresh boot.
  Preserve the existing comment explaining why a zero-pane session is killed rather than early-returned past.
  Everything ahead of that block — the `debugLogArgs`, `mouseOption`, `watchdogOption`, `ValidateHeader` and `probeCapabilityLocked` validation, and the `refuseRecordedForeignSessionBeforeBootLocked` refusal — is untouched and stays in its current order.

  Extract `Up()`'s `withOpLock` closure body verbatim into a new unexported method `func (e *Engine) upLocked() (UpResult, bool, error)`, whose second return is `ensureServerAndSessionLocked`'s own `booted` flag passed straight out.
  Move the body without rewriting it: the booted-bookkeeping block that calls `clearAllPaneBindings` and clears `HeaderPaneID`, its comment warning that the `HeaderPaneID` clear deliberately lives at that site rather than inside `clearAllPaneBindings`, the `ensureHeaderPaneLocked` call, the `reconcileApplyPersistLocked` call, and the comment explaining why the result's strand count excludes the header all travel unchanged.
  `Up()` becomes a thin `withOpLock` wrapper that calls `upLocked`, discards the bool, and assigns the `UpResult` — its own doc comment and exported signature are unchanged.
  `upLocked`'s doc comment must state that no caller reaches it directly, that it is what `ensureSessionLocked` delegates to on the cold path, and that widening its return breaks no contract because it is unexported.

  Do not touch `Resume`.
  It carries a structurally similar booted-bookkeeping block but diverges after `ensureHeaderPaneLocked`, and folding the two into one shared helper is out of scope for this task.
- **Commit:** `refactor(reedengine): extract upLocked and share the session-liveness predicate`

### Card 2: add `ensureSessionLocked` and `EnsureSession`

- **Context:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func (e *Engine) ensureSessionLocked() (bool, error)` to `internal/reedengine/lifecycle.go`.
  It calls `sessionSubstrateLocked` and returns `(false, nil)` immediately when the session is usable, having done nothing else — no config validation, no reconcile, no state read, no write.
  Otherwise it calls `upLocked` and returns that call's own booted flag verbatim, never a hardcoded true.
  Its doc comment must state both reasons the early return exists rather than routing a warm call through `upLocked`: `upLocked`'s tail reaches `planReconcile`, which adds every live non-exempt pane to its kill list whenever the header is alive, and `ensureServerAndSessionLocked` runs its whole pre-tmux config-validation block ahead of its already-up early return.
  It must also state why the delegate path returns the flag verbatim: the session can come up between the two probes, so a hardcoded true would make the caller's attribution log claim a spawn that never happened.

  Add `func (e *Engine) EnsureSession() (booted bool, err error)` — the exported `withOpLock` wrapper around `ensureSessionLocked`, following `Up`'s own wrapper shape of declaring the result before the closure, assigning inside it, and returning after.
  Its doc comment must state that it boots this worktree's session only when there is nothing usable to attach to, that it reports whether a session was actually created, and that it reads no persisted state on the warm path — so a caller needing reed's state-level refusals must still make its own `Status` call.

  `requireSessionLocked` and `noSessionMessage` are not deleted, weakened, or moved: eight other call sites keep using them, and their behaviour must not change.
- **Commit:** `feat(reedengine): add EnsureSession, the boot-only self-heal seam`

### Card 3: `AddStrand` self-heals

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/spawn.go`
  - `internal/logger/`
- **Edits:**
  - `internal/reedengine/strand.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `AddStrand`'s `withOpLock` closure, replace the `requireSessionLocked` call with `ensureSessionLocked`, capturing its booted return.
  When that flag is true, log once via the already-imported logger package at `Info`, naming the verb that caused the boot alongside this engine's socket and session name, so a session appearing out of a bare `lyx reed add` is attributable in the log afterwards.
  Log nothing on the warm path.

  `validateIfAbsent` stays the first statement in the closure, ahead of the new call: a config-shaped rejection must never deposit a spawned tmux server as residue.
  Extend the existing comment above it to say that, rather than replacing its current argument about the friendly no-session error.
  Everything after the pre-flight — `loadOrInitStateLocked`, the `--if-absent` classification, the ordinary add path, and the reconcile-apply-persist tail — is unchanged.

  Rewrite `AddStrand`'s doc-comment sentence that currently describes the pre-flight as mirroring `Status` so that running add before up fails with the same friendly no-session error.
  The replacement must say that `AddStrand` now self-heals a cold worktree by booting the session through `ensureSessionLocked` with `up` semantics — a bare substrate, with no persisted strand relaunched — and must state that the foreign-session refusal survives because `ensureServerAndSessionLocked` consults `refuseRecordedForeignSessionBeforeBootLocked` ahead of anything that creates a session.

  Add a note to the same doc comment recording the one place up-semantics and `--if-absent` interact: after a self-heal boot every pane binding has just been cleared, so `classifyIfAbsent` reaches `ifAbsentRelaunch` rather than `ifAbsentNoOpAlive` for a name that exists in the persisted table.
  That is the intended answer, not a defect to fix.
- **Commit:** `feat(reedengine): AddStrand boots a cold worktree's session instead of refusing`

### Card 4: hermetic ordering and regression tests

- **Context:**
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two hermetic tests to `internal/reedengine/strand_test.go`, both built on the existing `newTestEngine` helper, whose configured multiplexer and shell binaries do not exist on disk.
  Neither test may make a real tmux round trip and neither may carry a build tag.

  The first asserts that an `AddStrand` call whose spec sets the `--if-absent` flag with no name override fails with `validateIfAbsent`'s own error rather than the nonexistent binary's, pinning that the config rejection still precedes any tmux contact at all.

  The second asserts that `AddStrand` against a cold engine no longer returns `noSessionMessage`'s text: the returned error must be non-nil, and a negative assertion must confirm it carries neither the no-session phrase nor the `lyx reed up` remedy that message names.
  Its comment must explain that the concrete error is the nonexistent binary's, reached through `sessionSubstrateLocked`'s own session probe, and that asserting that binary error's exact text would pin an OS-specific string.

  Add a comment block to the same file recording what is deliberately NOT tested hermetically: because `ensureSessionLocked`'s first act is a session probe, a validation-ordering test shaped like the existing `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact` cannot be written for `AddStrand`, and the liveness probe running first is the point of the seam rather than an ordering defect to fix.
  Cold-path validation ordering is covered by the smoke tier instead.

  Do not modify the existing `planUpLaunches`/`planResumeLaunches` tests or `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact` — they must keep passing unmodified.
- **Commit:** `test(reedengine): pin AddStrand's cold-path self-heal and config ordering`

## Batch Tests

`verify: go test ./internal/reedengine/` runs the package's untagged tier, which is where card 4's two new tests live alongside the existing hermetic suite in `strand_test.go`, `lifecycle_test.go`, `lock_test.go` and their siblings.
It is scoped to the one package every card in this batch edits.
The existing `Up`/`Resume` validation-ordering tests are the regression signal that matters most here: they must keep passing unmodified, since card 1 moves `Up`'s body but must not reorder anything inside `ensureServerAndSessionLocked`.
The tagged tiers for this package are exercised by batch 5.
