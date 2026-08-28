# Plan: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
task: 'reed: resume/down leak lock directories at the stale pre-rename session-name path'
slug: 'reed-lock-stale-session-name'
approved: true
started: '20260828-144005'
parent: 'main'
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: refuse-a-vanished-worktree-root
    file: 01-refuse-a-vanished-worktree-root.md
    depends-on: []
    verify: go test ./internal/reedengine/... && go vet -tags integration ./internal/reedengine/...
  - number: 2
    name: watch-loop-dormant-mode
    file: 02-watch-loop-dormant-mode.md
    depends-on: [1]
    verify: go test ./internal/reedengine/... && go vet -tags integration ./internal/reedengine/...
```

## Shared Decisions

### Decision: the-predicate-is-worktreeroot-liveness-never-anchorpath

- **Decision:** The new refusal gates on `Geometry.WorktreeRoot` existing as a directory, never on `Geometry.AnchorPath`.
  The existing `os.MkdirAll(e.stateDir())` in both lock helpers stays exactly as it is and keeps creating `AnchorPath/.lyx`.
- **Rationale:** reed has two `Geometry` tellers that mean different things by `AnchorPath`.
  In hub mode `internal/hubgeom/hubgeom.go` sets `AnchorPath` to a path inside a worktree that must pre-exist.
  In standalone mode `internal/standalonegeom/reedgeom.go` sets `AnchorPath` to a derived state directory that `internal/standalonestate/standalonestate.go`'s `Derive` never creates on disk, and which `withOpLock`'s own `MkdirAll` is what materializes on the `--stencils-dir` first-run path.
  Gating on `AnchorPath` would break standalone first run and would report "this worktree was renamed" about a path that is not a worktree.
  `WorktreeRoot` is the one field both tellers agree on: `<hub>/<name>` in hub mode — precisely the directory a rename removes and precisely the one the stray conjured out of nothing — and the operator's own target directory in standalone mode.
- **Applies to:** all batches

### Decision: only-proven-gone-carries-the-sentinel

- **Decision:** Exactly two outcomes wrap the package sentinel `errWorktreeRootGone`: `os.Stat` failing with `fs.ErrNotExist`, and the path existing but not being a directory.
  Every other outcome — an empty `WorktreeRoot`, a relative `WorktreeRoot`, and any non-`fs.ErrNotExist` stat failure such as `EACCES` or `EIO` — refuses the operation with a plain error that does NOT match the sentinel under `errors.Is`.
- **Rationale:** the sentinel is the single signal that drops the watch loop into its dormant 60s cadence.
  A momentary stat blip on a healthy session must not silently slow that session's self-heal for the rest of the header pane's life, and a shape violation is a caller bug that must refuse loudly rather than back off quietly — an empty `WorktreeRoot` makes `os.Stat("")` return `fs.ErrNotExist`, which mapped onto the sentinel would strand the watcher dormant forever on a string that can never start existing.
- **Applies to:** all batches

### Decision: matching-is-errors-is-never-a-substring

- **Decision:** Every consumer of the proven-gone condition matches it with `errors.Is(err, errWorktreeRootGone)`.
  No consumer matches on message text.
- **Rationale:** the three refusal messages are deliberately distinct and operator-facing, so they are free to be reworded; a substring match would silently couple the watch loop's cadence decision to prose.
- **Applies to:** all batches

### Decision: refusal-messages-name-the-path-and-both-causes

- **Decision:** Three distinct messages.
  Vanished (`fs.ErrNotExist`) quotes the told worktree root, states reed never creates it, and names BOTH causes without asserting either — a hub worktree that was renamed or removed, and a standalone `--target-dir` naming a directory that does not exist.
  Not-a-directory quotes the path and says it is not a directory, with no rename remedy.
  Any other stat failure reports the underlying error verbatim and says reed could not determine whether the worktree root exists.
- **Rationale:** the engine is told its geometry and nothing else, so it cannot tell hub mode from standalone mode; a message asserting a rename misdiagnoses a standalone operator, and a vanished-path message about a path that exists as a regular file sends a hub operator looking for the wrong thing.
- **Applies to:** all batches

### Decision: the-standalone-refusal-is-an-intended-behaviour-change

- **Decision:** `lyx <cmd> --target-dir <path-that-does-not-exist>` now refuses at the first engine op instead of proceeding and deriving a state directory for it.
  This is documented in `internal/reedengine/doc.go` and gets its own regression test; it does not arrive as a surprise in review.
- **Rationale:** `resolveStandaloneTarget` in `internal/burlercli/wiring.go` and `internal/webstercli/wiring.go` only absolutises the flag with no stat, and `standalonestate.Derive` explicitly handles a target that does not exist yet, so such a geometry reaches the lock helpers today and succeeds.
  Reed running a session against a directory that does not exist is not a thing to support, and silently deriving a state directory for it is the same class of bug as the stray this task removes.
- **Applies to:** batch 1

### Decision: fixture-sweep-criterion-is-reaches-the-lock-helpers

- **Decision:** The test-fixture inventory is "every in-package test that reaches `withOpLock` or `withTryOpLock`", not "every caller of `newTestEngine`".
  Inline `Geometry` literal sites materialize their own `WorktreeRoot` in place rather than migrating onto the shared helper.
- **Rationale:** `internal/reedengine/lock_test.go`'s `TestWithOpLock_PathIsUnderDotLyx` builds an inline literal with an uncreated worktree root and calls `withOpLock`, so a helper-only change misses it.
  Each inline site was written to observe a specific field arrangement — that test exists precisely to make `AnchorPath` diverge from `WorktreeRoot` — which folding it into the shared fixture would erase.
- **Applies to:** batch 1

### Decision: integration-tagged-files-are-vetted-not-run-in-verify

- **Decision:** Both batches' `verify:` commands compile the `integration`-tagged files with `go vet -tags integration ./internal/reedengine/...` rather than running them with `go test -tags integration`.
- **Rationale:** `go test` with no tags never compiles the tag-gated files, so a sweep that stops at the default build silently skips the inline `Geometry` literals in `internal/reedengine/contract_integration_test.go` and `internal/reedengine/mouse_boot_integration_test.go`.
  `go vet -tags integration` type-checks those files without running them, which is what the sweep needs.
  Running them in `verify:` is not viable — see the next Decision.
- **Applies to:** all batches

### Decision: a-pre-existing-integration-failure-will-trip-the-done-gate

- **Decision:** Record, do not fix.
  `TestWatchdogSelfHeal_HookProbeMatchesLiveTmux` in `internal/reedengine/watchdog_integration_test.go` fails deterministically at this branch's tip, on unmodified code, before any change from this plan lands.
  No card in this plan touches it, and no `verify:` command in this plan runs it.
- **Rationale:** measured on the worktree tip with a clean tree, three consecutive runs, all failing identically: `hookInstalledLocked() installed = false, want true`.
  `go test ./...` (untagged, whole repo) passes; the failure is confined to the `integration` tag.
  It arrived with commit `8002cf976`, the watchdog-daemon commit, and it is exactly the contention the discussion's `window-resized-hook-has-no-install-site` Decision documents as a follow-up: `installResizePinsLocked` rebuilds the `window-resized` option with `resize-pane` pin bodies on every apply and attach, so the probe's exact match against `resizeHookCommand` can never succeed.
  Resolving it means resolving that contention, which the discussion explicitly rejects as part of this task — it is a separable watchdog feature-completion change with its own live-tmux verification burden.
- **Operator consequence, stated plainly:** this hub's `pipeline.done_gate` is `go test ./... && go test -tags integration ./...`, so mill-go's Handoff pre-done gate WILL fail on this pre-existing test, not on anything this plan changes.
  Nothing in this plan silently narrows the gate or mutates `mill-config.yaml` to hide it.
  The operator's two routes are: land the hook-contention follow-up first, or run the gate manually and confirm `TestWatchdogSelfHeal_HookProbeMatchesLiveTmux` is the only failure before finalizing.
- **Applies to:** all batches

### Decision: docs-land-in-the-commit-that-changes-behaviour

- **Decision:** `internal/reedengine/doc.go` and `tools/sandbox/SANDBOX-REED-SUITE.md` are edited by the card that makes the behaviour change they describe, never by a trailing documentation-only card.
- **Rationale:** the repo's task-completion rule requires observable-behaviour changes to update their docs in the same commit, and each card produces exactly one commit.
- **Applies to:** all batches

### Decision: no-cleanup-of-existing-strays

- **Decision:** Ship prevention only.
  No sweeper, no new verb, no delete-on-`down`, and no code anywhere in this plan removes a hub-level directory.
- **Rationale:** the Fabric Destruction Chokepoint Invariant exists to keep hub-level deletion out of ad-hoc code paths, and a stray is indistinguishable from a legitimately empty sibling worktree directory the operator created.
  One manual `rmdir` clears the historical case; the refusal makes new ones impossible.
- **Applies to:** all batches

## All Files Touched

- `internal/reedengine/doc.go`
- `internal/reedengine/lock.go`
- `internal/reedengine/lock_test.go`
- `internal/reedengine/server.go`
- `internal/reedengine/server_test.go`
- `internal/reedengine/watchdog.go`
- `internal/reedengine/watchloop.go`
- `internal/reedengine/watchloop_test.go`
- `tools/sandbox/SANDBOX-REED-SUITE.md`
