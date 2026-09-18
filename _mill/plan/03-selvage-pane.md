# Batch: selvage-pane

```yaml
task: "Replace reed's header pane with a status-line and Selvage"
batch: "selvage-pane"
number: 3
cards: 8
verify: go test ./internal/reedengine/... && go test -tags integration ./internal/reedengine/...
depends-on: [2]
```

## Batch Scope

This batch turns the header pane into Selvage: `ReedState.HeaderPaneID` becomes `SelvagePaneID`, `ensureHeaderPaneLocked` becomes `ensureSelvagePaneLocked` splitting **below** the physically bottom-most pane and launching `e.cfg.Shell` rather than a re-exec of lyx, `internal/reedengine/headerpane.go` and `Engine.suppressHeaderLaunch` are deleted with the re-exec they existed for, and reconcile's exemption/authorization, spawn's split-target choice, apply's blanking and generation's clear all retarget onto the new field.
It is one batch because the state field's rename reaches six production files and roughly a dozen test files at once, and because deleting the pane's launch command and flipping the split direction are the same change to the same function — a pane that splits at the bottom but still re-execs lyx, or one that runs a shell but still splits at the top, is a state the repository must never be left in.

The external interface batches 4 and 5 consume: `ReedState.SelvagePaneID` (`json:"selvagePaneId,omitempty"`), `Engine.ensureSelvagePaneLocked`, and an `internal/reedengine` that no longer spawns any lyx process of its own.

Batch-local decisions beyond `## Shared Decisions`: the even-vertical re-tile retry survives the flip verbatim, targeting the bottom — tmux cannot split a one-row pane at all, and with the band at the bottom and `height_rows: 1` the bottom-most pane is a one-row Selvage whenever state is stale, which is exactly the wedge that retry was written for.
No signal-handling code is written for Selvage: it is a plain interactive shell, which ignores SIGINT at its idle prompt and still dies cleanly on `reed down`'s pty-close/SIGHUP path.
Selvage is never written to, never cleared, and never sent keys — the header pane's ED2/ED3 screen-clear payload exists only because it had to display text, which it no longer does.

## Cards

### Card 16: rename the persisted state field to SelvagePaneID

- **Context:**
  - `_mill/discussion.md`
  - `internal/reedengine/render/types.go`
- **Edits:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/state.go`, rename `ReedState.HeaderPaneID` to `SelvagePaneID` and change its JSON tag from `json:"headerPaneId,omitempty"` to `json:"selvagePaneId,omitempty"`. Rewrite its doc comment to describe the always-present Selvage pane — deliberately outside `Strands`, never itself a strand, excluded from strand accounting, from being the preferred split target, and from both halves of reconcile's kill schedule — and keep the "empty means it has not yet been created and must be (re)created at the next up/resume boot" sentence. Update `PaneGeneration`'s own doc comment, which today reads "every PaneID above — the strands' and HeaderPaneID alike" — it must name `SelvagePaneID`. Write **no** compatibility shim and **no** dual-read: an old state file's `headerPaneId` is simply not read, per the `no-migration-for-the-renamed-state-field` Shared Decision. Do not add a migration step to `loadOrInitStateLocked`. Also update the phrase "an alive header now authorizes reaping every other pane" inside `unreadableStateError`'s doc comment to name Selvage. In `internal/reedengine/state_test.go`, retarget every `HeaderPaneID` reference and every `headerPaneId` JSON literal to the new spellings.
- **Commit:** `refactor(reedengine): rename ReedState.HeaderPaneID to SelvagePaneID`

### Card 17: split Selvage below the bottom-most pane

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/render/rules.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/lifecycle.go`, rename `ensureHeaderPaneLocked` to `ensureSelvagePaneLocked`, `topmostPaneID` to `bottommostPaneID`, `splitHeaderPaneAtTopLocked` to `splitSelvagePaneAtBottomLocked`, and `splitPaneAboveLocked` to `splitPaneBelowLocked`, retargeting every `st.HeaderPaneID` read and write onto `st.SelvagePaneID`. `bottommostPaneID` returns the id of the pane with the **largest** `pane_top` rather than the smallest, keeping the same "live must be non-empty" precondition. `splitPaneBelowLocked` drops tmux's `-b` flag from its argv, leaving `{"split-window", "-t", target, "-c", e.geom.PaneCwd, "-P", "-F", "#{pane_id}"}` plus the trailing launch-command argument, and keeps `validateSplitCreatedNewPane` exactly as it is — psmux's silent too-small-to-split failure prints an existing pane's id with exit 0, and recording that id as Selvage would bind Selvage to a strand's pane. In `ensureSelvagePaneLocked`, replace the `os.Executable()` lookup and the `headerLaunchLine(shell.ForGOOS(), exe, e.suppressHeaderLaunch)` call with `e.cfg.Shell` as the trailing command argument, the same way `new-session` already launches the session's first pane, and delete the `launchCmd == ""` suppression branch and its `logger.Info` line along with them. Remove the now-unused `os` and `shell` imports if nothing else in the file uses them. Rewrite `splitSelvagePaneAtBottomLocked`'s doc comment so the physical-position argument reads for the bottom: `render.Rules` emits the band cell **last** and `paneIDsByTop` resequences by `pane_top`, so a Selvage pane that is not physically bottom-most would invert cell heights on the very first `select-layout`; and the even-vertical re-tile retry survives verbatim because tmux cannot split a one-row pane at all, which is just as true at the bottom as at the top. Rewrite `splitPaneBelowLocked`'s doc comment to explain why no `-b` is needed now and that Selvage is the only split in the engine that must land at a specific physical edge. Keep the corpse-kill-before-replacement logic, the sole-pane corpse special case, the `SaveState` tail, and the "no panes to split from" errors, retargeting their message text onto Selvage.
- **Commit:** `feat(reedengine): split Selvage in below the bottom-most pane`

### Card 18: delete the header pane's launch composer and its test suppression

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/lock.go`
  - `internal/reedengine/lifecycle_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/reedengine/headerpane.go`
  - `internal/reedengine/headerpane_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/reedengine/headerpane.go` (`headerLaunchCmd`, `headerLaunchLine`) and its test file `internal/reedengine/headerpane_test.go` outright — with Selvage launching `e.cfg.Shell` and nothing else, neither has a subject. In `internal/reedengine/lock.go`, delete the `suppressHeaderLaunch bool` field from `Engine` and its whole doc comment, and delete the `suppressHeaderLaunch: testing.Testing()` line from `New`'s returned literal; remove the `testing` import if nothing else in the file uses it. In `internal/reedengine/lifecycle_test.go`, delete the `enableHeaderLaunch` helper and both of its call sites, adapting each affected test to drive the split path directly without the flip (the launch command is now unconditionally `e.cfg.Shell`, so there is nothing to suppress and nothing to re-enable). State in the deleted field's place — as a short comment on `New`, or nowhere if it reads as noise — nothing: the mechanism is not gone from the codebase, it is relocated onto `reedCLI.suppressWatchdogSpawn` in batch 5, and that is where its rationale belongs. Do not weaken `gitkit.refuseCLIReexec`: it stays what it is, a backstop that aborts a test binary re-exec'd as a CLI, never the gate.
- **Commit:** `refactor(reedengine): delete the header pane launch composer with its re-exec`

### Card 19: retarget reconcile's exemption and authorization onto Selvage

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/lifecycle.go`
- **Edits:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/reconcile.go`, rename `planReconcile`'s third parameter from `headerPaneID` to `selvagePaneID` and the local `headerAlive` to `selvageAlive`, retargeting `reconcileLocked`'s call to pass `st.SelvagePaneID`. The three behaviours are unchanged and must stay so: a `pane_dead=1` Selvage is exempt from the dead-pane kill (a kept corpse stays enumerable, keeps the cell/pane count consistent, and is healed by `ensureSelvagePaneLocked` on the next up/resume); `selvageAlive` stays a third separate local never folded into `boundPaneIDs`/`anyBoundPresent`/`exemptPaneIDs`, so that mere presence exempts Selvage from being killed while only an **alive** Selvage authorizes reaping anything else; and `exemptPaneIDs` still gates only the untracked reap. Rewrite the two long comment blocks so they name Selvage and `ensureSelvagePaneLocked`, keeping their live-observed rationale verbatim. In `clearConflictingPaneBindings`, retarget `st.HeaderPaneID` to `st.SelvagePaneID` and rewrite the doc comment's first damage bullet — it today describes a strand sharing the header's pane id being placed twice by `planLayout`, "once as bandHeader's fixed top cell" — so it names `bandSelvage`'s fixed bottom cell, and drop the trailing clause naming `lyx reed header --blocking` as what the header pane was running, since that command no longer exists. In `internal/reedengine/reconcile_test.go`, retarget every `headerPaneID` argument and `HeaderPaneID` field, rename each affected test so it names Selvage, and keep the three cases the discussion's Testing section names: corpse never killed, alive Selvage authorizes reaping untracked panes, dead-but-present Selvage does not.
- **Commit:** `refactor(reedengine): retarget reconcile's exemption and authorization onto Selvage`

### Card 20: retarget the split-target choice onto Selvage

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/reconcile.go`
- **Edits:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/spawn.go`, rename `planPaneTarget`'s second parameter from `headerPaneID` to `selvagePaneID` and retarget its caller to pass `st.SelvagePaneID`. The three-tier preference is unchanged: the tallest alive non-Selvage pane, then any present non-Selvage pane (a corpse), then Selvage itself when nothing else exists. Rewrite the doc comment and the three inline comments so they name Selvage, including the paragraph explaining that once the untracked reap is authorized by an alive Selvage the initial pane is disposed of like any other untracked pane before this function ever runs. `validateSplitCreatedNewPane` is unchanged. In `internal/reedengine/spawn_test.go`, retarget every argument and field name and rename each affected test to name Selvage, keeping the three cases: prefers the tallest alive non-Selvage pane, falls back to a non-Selvage corpse, falls back to Selvage itself.
- **Commit:** `refactor(reedengine): retarget planPaneTarget's split choice onto Selvage`

### Card 21: retarget apply and generation onto Selvage

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/render/types.go`
  - `internal/reedengine/render/rules.go`
- **Edits:**
  - `internal/reedengine/apply.go`
  - `internal/reedengine/apply_test.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/generation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/apply.go`'s `toRenderInputs`, rename the local `headerPaneID` to `selvagePaneID` and read `st.SelvagePaneID`, keeping the blanking rule verbatim — when the pane is not present in `presentIDs` the id is blanked before it reaches `render.Params`, which is what stops a stale id being emitted as a layout cell. Update `renderInputs`' and `toRenderInputs`' doc comments, `planLayout`'s and `fixedHeightPins`' doc comments, and the `anyPlacedStrand` comment block that today names "the header" so each names Selvage. Neither `anyPlacedStrand` nor its two guard call sites may be weakened: a layout string enumerating zero panes is accepted by tmux with exit 0 and answered by destroying the session's whole pane set. In `internal/reedengine/generation.go`, retarget the `st.HeaderPaneID = ""` clear onto `st.SelvagePaneID` and update the file's leading comment, which today reads "a persisted reed.json's PaneIDs and HeaderPaneID were bound against". In `internal/reedengine/apply_test.go` and `internal/reedengine/generation_test.go`, retarget every field reference and rename each affected test to name Selvage; keep and retarget the `toRenderInputs` case asserting the id is blanked when the pane is absent.
- **Commit:** `refactor(reedengine): retarget apply and generation onto SelvagePaneID`

### Card 22: retarget the remaining lifecycle call sites and strand accounting

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/lifecycle.go`, retarget the two `st.HeaderPaneID = ""` clears on the server-respawn and heal paths onto `st.SelvagePaneID`, and rewrite the two comment blocks above them — each today explains that the field is cleared alongside strand bindings because a reborn server's pane ids would otherwise collide with the still-live header pane, and each must name Selvage and `ensureSelvagePaneLocked`. Retarget both `ensureHeaderPaneLocked` call sites onto `ensureSelvagePaneLocked`. Update the three comment blocks stating that `len(st.Strands)` deliberately excludes the band — the one in `Up`, the one in `Status`, and the one in `noSessionMessage`'s vicinity — so each names Selvage and `ReedState.SelvagePaneID` while keeping the rule itself verbatim: Selvage is not in `st.Strands`, and a future edit must never "fix" a missing row by appending one. Update the `Status` loop comment that today reads "This loop iterates st.Strands only — the header pane is (ReedState.HeaderPaneID)". In `internal/reedengine/lifecycle_test.go` and `internal/reedengine/strand_test.go`, retarget every `HeaderPaneID` reference and rename each affected test to name Selvage. Add a `lifecycle_test.go` case pinning the new split direction: given a scripted pane list whose largest `pane_top` is a known id, `bottommostPaneID` returns that id, and the argv `splitPaneBelowLocked` issues carries no `-b`.
- **Commit:** `refactor(reedengine): retarget lifecycle's heal paths and strand accounting onto Selvage`

### Card 23: rewrite the engine package doc and the integration fixtures

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/render/height.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/contract_integration_test.go`
  - `internal/reedengine/attach_test.go`
  - `internal/reedengine/attachgeometry_integration_test.go`
  - `internal/reedengine/watchdog_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/reedengine/doc.go`, rewrite every passage describing the header pane so it describes Selvage, keeping each passage's own reasoning: the "additional, permanent pane beyond its strands" block must name `ReedState.SelvagePaneID` and `ensureSelvagePaneLocked`; the paragraph describing the pane's creation by a split-window carrying `lyx reed header --blocking` as its trailing argument must instead describe a split-window carrying `e.cfg.Shell`, and the surrounding `headerLaunchLine`/`Engine.suppressHeaderLaunch` sentences must go with the mechanism they describe; the untracked-reap passage must read `anyBoundPresent || selvageAlive`; the "Header band divider row" measurement entry must name the Selvage band, `clampBandHeight` and `Selvage.HeightRows`; and the passage about `testing.Testing()` gating the header launch line must be deleted rather than retargeted, since there is no launch line left to gate. Add, in `doc.go`'s own voice, the module-local Selvage rules the discussion's `no-new-cross-cutting-invariant` decision deliberately keeps out of `CONSTRAINTS.md`: Selvage is physically bottom-most, is never a strand, and is never written to, cleared, or sent keys by reed. Leave the `reedcli/header.go`-referencing SIGWINCH and stdout/stderr entries alone in this card — they are batch 5's, since the verb they name is renamed there. In `internal/reedengine/contract_integration_test.go`, `internal/reedengine/attach_test.go`, `internal/reedengine/attachgeometry_integration_test.go`, and `internal/reedengine/watchdog_integration_test.go`, retarget every `HeaderPaneID` field and `e.cfg.Header` reference and rename each affected test to name Selvage; `TestHeaderNeverGetsZeroHeightLayoutCell` in particular keeps its subject and its assertion and is renamed for the Selvage band. `watchdog_integration_test.go` is added here (not listed anywhere else in the plan as an Edits target) because this batch's own `verify:` builds `internal/reedengine`'s integration tier, and that file constructs `ReedState`/`Config` values carrying the renamed fields.
- **Commit:** `docs(reedengine): rewrite the package doc and integration fixtures for Selvage`

## Batch Tests

`verify: go test ./internal/reedengine/... && go test -tags integration ./internal/reedengine/...` runs both tiers of the one module this batch changes, which is the right scope: the untagged half covers the pure planning functions the rename reaches (`planReconcile`, `planPaneTarget`, `toRenderInputs`, `bottommostPaneID`, the generation clears, and `render`'s own suite via the `...` wildcard), while the integration half covers `contract_integration_test.go`, `attachgeometry_integration_test.go` and `watchdog_integration_test.go`, each of which constructs `ReedState` values carrying the renamed field and would otherwise fail to compile unnoticed until the done gate.
Running the integration tier here rather than deferring it is deliberate — this batch is the one that renames the persisted field, so it is the batch that must prove the tagged fixtures still build and pass against it.
The smoke tier is **not** in scope for this batch's verify: its reed smoke files are adapted wholesale in batch 7, and `pipeline.done_gate` does not run the smoke tag either.
