# Batch: header-pane-runs-its-own-command

```yaml
task: "reed: header pane's boot sometimes leaves shell/log noise in its scrollback"
batch: "header-pane-runs-its-own-command"
number: 1
cards: 5
verify: go test ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch removes noise classes 1 and 2 — the echoed launch line and the operator's shell-RC errors — at their shared source: the header pane stops hosting an interactive shell.
`ensureHeaderPaneLocked` currently splits a bare shell and *types* the launch line into it with `send-keys -l` + `Enter`;
after this batch it passes the composed launch line to `split-window` as its own trailing shell-command argument, so no interactive shell exists in that pane and there is nothing to echo the command or read `~/.bashrc`.

Two changes are needed to make that observable in the hermetic tier, and they are deliberately separate cards.
`headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())` at the boot site has no injection point, so under `go test` the launch line is always `""` and the boot site issues neither a command argument nor any `send-keys` — an untagged test can therefore neither see the new behaviour nor fail on the old.
Card 1 moves the suppression decision onto the `Engine` as an unexported field initialised from `testing.Testing()`, which changes no default behaviour and exports nothing;
card 2 adds the pin (red at that point);
card 3 applies the launch change (green).
That card order is what produces P1's required pre-fix failure observation — see the overview's "pre-fix red observations" Shared Decision.

The batch also accepts and documents a latent-bug fix that falls out of it: with the keepalive as the pane's own process, `remain-on-exit on` corpses the pane when the keepalive dies, so `ensureHeaderPaneLocked`'s documented corpse-and-heal path starts working where the surviving `bash` had been silently defeating it.

Strand panes are untouched.
`launchStrandLocked` keeps its split-then-`send-keys` flow and `sendKeysLiteralArg` stays — an agent pane genuinely needs an interactive shell.

Batch-local decision beyond the overview's Shared Decisions: the retry path gets its own new test rather than an edit to the existing `TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit`, whose whole value is that it drives the *default* engine through the one-row-top-pane recovery it was written for.

## Cards

### Card 1: carry the header-launch suppression decision on the Engine

- **Context:**
  - `internal/reedengine/headerpane.go`
- **Edits:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/lock.go`, add an unexported `suppressHeaderLaunch bool` field to the `Engine` struct and have `New` initialise it from `testing.Testing()`, adding the `testing` import to that file.
  Document on the field that it is initialised from `testing.Testing()` because re-exec'ing `os.Executable()` from a test binary would run the whole suite recursively, and that it is a field rather than a hard-wired call so an in-package test can drive the real launch path against a fake tmux.
  In `internal/reedengine/lifecycle.go`, change `ensureHeaderPaneLocked`'s `headerLaunchLine(shell.ForGOOS(), exe, testing.Testing())` call to pass `e.suppressHeaderLaunch` instead, and drop the now-unused `testing` import from that file.
  Nothing exported changes and no default behaviour changes: every existing untagged test still sees the bare-shell header pane.
- **Commit:** `refactor(reedengine): carry header-launch suppression on the Engine`

### Card 2: pin that the header pane launches its own command

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/headerpane.go`
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an in-package test helper `enableHeaderLaunch(t *testing.T, e *Engine)` that sets `e.suppressHeaderLaunch = false`, documenting that it is the seam P1 needs and that nothing outside the package can reach it.
  Add `TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys`, built on the same `newTestEngine` + `e.tmux.execHook` pattern as the existing `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath`: one alive pane in `list-panes`, a genuinely new pane id returned from `split-window`, launch enabled via the helper.
  It records the `split-window` argv and counts every `send-keys` call, then asserts (a) the recorded `split-window` argv carries a trailing non-flag argument after the `-F` value, and that argument names the header keepalive — it contains both `reed` and `--blocking`, a substring check that holds for the posix and pwsh quoting `headerLaunchCmd` produces alike — and (b) exactly zero `send-keys` calls were issued.
  Both halves are the P1 pin;
  no `#{pane_current_command}` assertion is added anywhere, because that value is shell-dependent.
  Run this test now, before card 3, and keep the failure output — it is the pre-fix evidence card 3's commit body carries.
- **Commit:** `test(reedengine): pin the header pane launching its own command`

### Card 3: boot the header pane with its own command instead of send-keys

- **Context:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/headerpane.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give `splitPaneAboveLocked` a third parameter `launchCmd string` and append it to the `split-window` argv as a single trailing argument when it is non-empty, leaving the argv exactly as it is today when it is empty.
  Keep the `validateSplitCreatedNewPane` guard and its call site unchanged — the guard behaviour must not change and must not be duplicated.
  Give `splitHeaderPaneAtTopLocked` the same third parameter and thread it into **both** `splitPaneAboveLocked` calls, the first attempt and the post-re-tile retry, so a retried header never boots commandless.
  In `ensureHeaderPaneLocked`, move the `launchCmd := headerLaunchLine(shell.ForGOOS(), exe, e.suppressHeaderLaunch)` assignment above the `splitHeaderPaneAtTopLocked` call, pass `launchCmd` to it, and delete the `send-keys -t <pane> -l` / `send-keys -t <pane> Enter` pair together with the error wrapping around them.
  Keep the suppressed-path `logger.Info` line that records the pane was left as a bare shell under `go test`, moving it so it still fires exactly when `launchCmd` is empty.
  Do not change `launchStrandLocked` or `sendKeysLiteralArg` in `internal/reedengine/spawn.go`;
  strands keep the send-keys launch and `sendKeysLiteralArg` still has callers.
  Re-run card 2's test to confirm it is now green, and paste a condensed excerpt of the pre-fix failure output recorded in card 2 into this commit's message body, labelled as P1's seam-landed-launch-not-yet-applied red observation.
- **Commit:** `fix(reedengine): boot the header pane with its own command, not send-keys`

### Card 4: pin the suppressed default and the retried split

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/overlay.go`
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell`, which drives `ensureHeaderPaneLocked` on a default `newTestEngine` engine — launch suppression left on — and asserts the recorded `split-window` argv ends at the `-F` value with no trailing command argument, that zero `send-keys` calls were issued, and that `st.HeaderPaneID` is still recorded.
  This is the pin for the `go test` behaviour the discussion deliberately preserves.
  Add `TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand`, which reuses the wedged/retiled scripted substrate of the existing `TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit` — a one-row pane at `pane_top` 0 that the first `split-window` refuses, an `even-vertical` re-tile, then a successful retry — but with launch enabled, and asserts the **retried** `split-window` argv carries the launch command by the same `reed` + `--blocking` substring check card 2 uses.
  Leave `TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit` and `TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure` unedited;
  both are pins for behaviour this batch does not change.
- **Commit:** `test(reedengine): pin the suppressed default and the retried header split`

### Card 5: describe the header pane's direct-command boot

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/probe.go`
  - `internal/reedengine/io.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the header-pane paragraph of `internal/reedengine/doc.go`'s package comment — the block describing `ReedState.HeaderPaneID`, its boot, and its corpse-and-heal contract — to state that the header pane is created by a `split-window` carrying the keepalive command as its own trailing shell-command argument, so the pane hosts no interactive shell and no `send-keys` is issued for it.
  Record that this makes the already-documented corpse-and-heal path start working as written: with the keepalive as the pane's own process, `remain-on-exit on` corpses the pane when that process dies, where the surviving `bash` previously kept the pane alive and a dead header was silently mistaken for a working one.
  Record that the `go test` default is still a commandless bare-shell pane and that the decision now rides on an unexported `Engine` field rather than a hard-wired `testing.Testing()` call at the boot site.
  Do not change the enumerated tmux subcommand set in the "Subcommand set" paragraph — this change issues no new verb, and `send-keys` is still issued by strand launches and by `internal/reedengine/io.go`.
  Do not mention the stencil-seed opt-out or the git-commit exposure here;
  those are batch 2's concern and are a separate mechanism from this batch's noise classes.
  Semantic line breaks per the repo's markdown convention do not apply to Go comments — match the surrounding comment's own wrapping.
- **Commit:** `docs(reedengine): describe the header pane's direct-command boot`

## Batch Tests

`verify: go test ./internal/reedengine/` runs the whole untagged `reedengine` package, which is the right scope here rather than a single file: cards 1 and 3 change `Engine`'s construction and `splitPaneAboveLocked`'s signature, both reached by header geometry, corpse-heal, idempotence, and layout tests spread across `lifecycle_test.go`, `apply_test.go`, `reconcile_test.go`, and `spawn_test.go`.
The package is hermetic — fake tmux via `execHook`, no live substrate, no process spawn — so it is fast.

New coverage this batch adds, all untagged and all in `internal/reedengine/lifecycle_test.go`:

- `TestEnsureHeaderPaneLocked_LaunchesTheCommandOnTheSplitNotViaSendKeys` — the P1 pin (argv carries the command, zero `send-keys`).
- `TestEnsureHeaderPaneLocked_DefaultUnderGoTestSplitsACommandlessShell` — the preserved `go test` default.
- `TestEnsureHeaderPaneLocked_RetriedSplitAlsoCarriesTheLaunchCommand` — the re-tile retry path.

Existing coverage that must pass unchanged: `TestEnsureHeaderPaneLocked_SplitsWithPaneCwdNotAnchorPath`, `TestEnsureHeaderPaneLocked_RebuildRejectsSilentSplitFailure`, `TestEnsureHeaderPaneLocked_RecoversWhenTheTopPaneIsTooSmallToSplit`, and `internal/reedengine/headerpane_test.go`'s `TestHeaderLaunchCmd` and `TestHeaderLaunchLine` — `headerLaunchCmd`/`headerLaunchLine` are not edited by this batch and their `underTest=true` branch must still yield `""`.

No process is spawned by any new test, so the Test Tier Purity Invariant is untouched.
The composite end-to-end scrollback outcome is not asserted here and is not this batch's pin — see batch 3's B.
