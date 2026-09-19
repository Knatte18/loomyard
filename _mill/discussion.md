# Discussion: reed: extract Selvage-pane lifecycle

```yaml
task: 'reed: extract Selvage-pane lifecycle'
slug: reed-selvage-pane-extraction
status: discussing
parent: main
```

## Problem

The shipped `reed: replace the header pane with a native tmux status-line, a permanent "Selvage" terminal pane, and a detached per-hub watchdog process` item had three separation goals.
Two landed: the watchdog daemon talks to the engine through exactly one method (`watchloop.go`'s `e.reapplyLayout`), and the status-line content is a self-contained `statusline.go`.
The third did not: the Selvage keepalive pane's own lifecycle code still sits scattered across the same four files the original design doc named as the smell — `lifecycle.go`, `reconcile.go`, `spawn.go`, `apply.go` — just renamed from Header to Selvage.
There is no `selvagepane.go`.

**Why now:** a post-merge grep audit of `main` recorded the scatter (`lifecycle.go` 58 "Selvage" hits, `reconcile.go` 32, `spawn.go` 20, `apply.go` 10, `state.go` 6, `config.go` 4), and the item was promoted from Next Up to Planned on the strength of that audit.
Nothing is broken at runtime — this is a shotgun-surgery cost: every change to how Selvage is created, spared, targeted, or blanked currently means editing four files that are each about something else.

## Scope

**In:**

- A new `internal/reedengine/selvagepane.go` that owns every piece of Selvage-specific *code* in the package.
- Moving Selvage's creation/heal path out of `lifecycle.go`: `ensureSelvagePaneLocked`, `splitSelvagePaneAtBottomLocked`, `bottommostPaneID`, `splitPaneBelowLocked`.
- Extracting the Selvage *policy* currently inlined in `reconcile.go` (`planReconcile`'s dead-kill exemption, `selvageAlive`, reap exemption), `spawn.go` (`planPaneTarget`'s three-tier non-Selvage targeting, plus the `-b` insert-above decision), and `apply.go` (`toRenderInputs`'s present-pane blanking) into named helpers in the new file, so the host functions call a narrow seam instead of knowing about Selvage.
- Routing the three `st.SelvagePaneID = ""` binding-clears (`lifecycle.go` x2, `generation.go` x1) through one helper in the new file.
- A new AST enforcement test that fails the build if Selvage code returns to any non-allowlisted file, following the repo's existing `internal/cliwire/bannedecl_enforcement_test.go` / `internal/gitkit/callerset_enforcement_test.go` pattern.
- Moving the tests whose subject functions move, into `selvagepane_test.go`.
- Doc updates in the same commit: `internal/reedengine/doc.go`'s Selvage section, and a Status section plus audit correction in `manifest/designs/reed-selvage-pane-extraction.md`.

**Out:**

- Any observable behavior change. This is a pure refactor: same tmux calls, same order, same error strings, same log messages.
- `windowsize.go`'s `pinGeometryOptionsLocked`. The design doc names it as "the one complication" that "recombines all three concerns," but that claim is stale — see the Decision below. No split, no helper extraction, no edit beyond a comment if one is inaccurate.
- The watchdog and status-line separations. Both already landed cleanly and are not touched.
- `render`'s Selvage awareness (`internal/reedengine/render`: `render.Selvage`, `FixedHeightPins` pin ordering). The render leaf legitimately owns the Selvage *band* as display vocabulary; only the engine-side pane lifecycle is in scope.
- `validateSplitCreatedNewPane` (stays in `spawn.go` — it is shared by both split sites and is not Selvage-specific).
- A new `CONSTRAINTS.md` invariant. This stays module-local, consistent with `doc.go`'s existing explicit decision.
- Any new package or exported API. `internal/reedengine`'s public surface is unchanged.
- `manifest/roadmap.md` movement beyond marking this Planned item complete, and `docs/overview.md` (no module-table or execution-stack change).

## Decisions

### one-file-same-package

- Decision: the extraction target is a single new file `internal/reedengine/selvagepane.go` in the existing package. No sub-package, no exported interface, no `selvagePane` struct standing in for the Engine.
- Rationale: every moved function needs `e.tmux`, `e.cfg`, `e.geom` and `*ReedState`. A sub-package would need all of that passed back in through an interface, or would invert the dependency — cost with no benefit, and it would force `LivePane`/`ReedState` to become an exported cross-package vocabulary. The package's existing style is `Engine` methods plus pure package-level helpers per concern-file (`statusline.go`, `watchdog.go`, `generation.go`); a per-concern *file* is exactly the unit this repo already uses.
- Rejected: `internal/reedengine/selvage` sub-package (dependency inversion for no gain); a `selvagePane` type wrapping the engine (a second receiver for the same lock-holding methods, which would make the `Locked` naming convention lie).

### policy-moves-not-just-lifecycle

- Decision: the Selvage-specific *decision logic* moves too, not only the create/heal path. After the refactor, `lifecycle.go`, `reconcile.go`, `spawn.go` and `apply.go` contain zero Selvage-specific code — only prose comments referring to it.
- Rationale: the task's own framing is "leaving those files calling a narrow interface instead of inlining Selvage-specific logic." Moving only `ensureSelvagePaneLocked` would relocate 4 of the 6 files' hits and leave `reconcile.go`'s ~30 lines of exemption reasoning and `spawn.go`'s three-tier targeting exactly where they are — the same audit would still fail.
- Rejected: lifecycle-only extraction (leaves the scatter); moving `planReconcile`/`planPaneTarget`/`toRenderInputs` wholesale into the new file (those functions' subject is the reconcile schedule, the split target, and the render mapping respectively — Selvage is one input to each, so moving the host would just invert the scatter).

### narrow-seam-shapes

- Decision: the seam the four host files call is these helpers, all unexported, all in `selvagepane.go`. Exact names are mill-plan's call; the contracts are fixed here.
  - **Reconcile policy** — a small unexported value type built once from `(live, selvagePaneID)`, answering three questions `planReconcile` asks: is this pane exempt from the dead-pane kill; is this pane exempt from the untracked reap; does Selvage's aliveness authorize the reap at all. The three answers stay three distinct questions, never collapsed into one predicate — `reconcile.go`'s existing comments explain at length why presence-exemption and aliveness-authorization must not be folded together, and that distinction must survive the move verbatim.
  - **Split target** — `planPaneTarget` moves into `selvagepane.go` and grows its return to carry the insert-above decision: `(splitTargetID string, insertAbove bool, err error)`. `spawn.go`'s `launchStrandLocked` then appends `-b` on the flag instead of re-deriving `splitTargetID == st.SelvagePaneID`, so the positional knowledge (why Selvage must stay bottom-most) lives in one file.
  - **Render blanking** — a helper taking `(st, presentIDs)` and returning the Selvage pane id blanked when its pane is not present, called from `toRenderInputs`.
  - **Binding clear** — one helper clearing `st.SelvagePaneID`, called from `upLocked`, `Resume`, and `adoptPaneGenerationLocked`.
- Rationale: each seam is the one Selvage question its host actually asks, so the host reads as orchestration and the new file reads as Selvage policy.
- Rejected: a single god-helper returning a bag of Selvage facts (couples the four callers to each other's needs); leaving `planPaneTarget` in `spawn.go` and exporting just the `insertAbove` predicate (splits one decision across two files).

### pin-geometry-claim-is-stale

- Decision: `pinGeometryOptionsLocked` is left alone, and `manifest/designs/reed-selvage-pane-extraction.md`'s "The one complication" section is corrected in this task's commit.
- Rationale: the design doc says the function "issues the status-line `set-option` calls, pins Selvage as 'always pin index 0,' and installs the watchdog's resize-signal hook." Read against HEAD, that is no longer true. `pinGeometryOptionsLocked` does the status-line options, `window-size latest`, and the *unset* half of the watchdog hook. The whole install half — pins plus the watchdog signal entry — moved to `installResizePinsLocked`, and the "Selvage pin is always index 0" property is a consequence of `render.FixedHeightPins`' ordering, surfaced by `resizePinHookArgvs`. `windowsize.go` has exactly one "Selvage" hit in the entire file, and it is a comment. There is no three-way merge left to split, so splitting it would be churn against a live-verified atomic-writer design.
- Rejected: splitting `pinGeometryOptionsLocked` into three helpers anyway (no Selvage code to extract, and the single-writer property is the thing the file's comments say must not be weakened); leaving the design doc's stale claim in place (the next audit would re-raise it).

### enforcement-test-is-the-teeth

- Decision: add `internal/reedengine/selvagepane_enforcement_test.go`, an untagged AST test over the package's own non-test `.go` files that fails on either of two conditions outside an allowlist: (a) a selector expression reading or writing the `SelvagePaneID` field, (b) a top-level func or method declaration whose name contains `Selvage`. Allowlist: `selvagepane.go` for both; `state.go` for the field declaration itself; `config.go` for `SelvageConfig`. Comments and string literals are not policed.
- Rationale: the design doc asks for "re-run the same kind of grep-based audit after any extraction to confirm the scatter is actually gone, not just relocated again." A grep re-run by hand is a one-time check that rots; an enforcement test is the same check that keeps holding. The repo already has this exact pattern twice, with the AST-over-`go/parser` shape and the honest "residual: a fresh name passes by construction" note.
- Rejected: a hand-run grep recorded in the design doc (rots immediately); a raw text/regex scan (would flag every prose comment, which are explicitly meant to stay); policing locals and parameters named `selvagePaneID` (a host function receiving the id as a narrow value is the *desired* seam, not a violation).

### pure-refactor-no-behavior-change

- Decision: no observable behavior change, and no opportunistic fixes folded in. Moved functions keep their names, signatures (except `planPaneTarget`'s added return), doc comments, error strings and log messages.
- Rationale: it keeps the diff reviewable as a move, and it keeps the existing test suite as the regression net — `lifecycle_test.go`, `reconcile_test.go`, `spawn_test.go`, `apply_test.go`, `contract_integration_test.go` and `attachgeometry_integration_test.go` collectively hold ~350 Selvage assertions that must pass untouched apart from the ones that move.
- Rejected: folding in any behavior fix found along the way (would make the move unreviewable; file a separate roadmap item instead).

### tests-follow-their-subject

- Decision: a test moves to `selvagepane_test.go` exactly when the function it tests moves. Behavioral and integration tests stay where they are.
- Rationale: test file mirrors source file, which is the package's existing convention. Moving every Selvage-mentioning test would drag integration suites out of the files whose behavior they cover.
- Rejected: moving all Selvage-named tests (breaks the mirror); leaving all tests in place (leaves unit tests for moved pure functions orphaned from their subject).

### docs-in-the-same-commit

- Decision: this commit updates `internal/reedengine/doc.go` (its Selvage section names the owning file and the seam, and its "three module-local Selvage rules" block is re-pointed at `selvagepane.go`) and `manifest/designs/reed-selvage-pane-extraction.md` (a Status: Implemented section with the post-extraction audit counts, plus the `pinGeometryOptionsLocked` correction). `manifest/roadmap.md` gets this Planned item marked complete. `docs/overview.md` and `CONSTRAINTS.md` are not edited.
- Rationale: CLAUDE.md's Task-completion rule — module doc if touched, overview only if the module table or execution stack changes (it does not), CONSTRAINTS.md only for a new cross-cutting invariant (this one is module-local by `doc.go`'s own standing decision, and the enforcement test carries it).
- Rejected: adding a `Selvage Ownership Invariant` to `CONSTRAINTS.md` (contradicts `doc.go`'s explicit "kept here rather than in CONSTRAINTS.md because no other module can violate it"); deferring the design-doc correction to a follow-up.

## Technical context

Everything is inside `internal/reedengine` (a `Told-Geometry Invariant` bound package — it imports no `internal/lyxcwd` and must not start).

Current homes of the code that moves:

- `lifecycle.go:464-545` `ensureSelvagePaneLocked` — heal-or-create. Aliveness (not presence) is the check; a dead-but-present corpse is killed, then the replacement is split, then the corpse is best-effort re-killed if it was the sole pane.
- `lifecycle.go:544-556` `bottommostPaneID` — largest `pane_top`.
- `lifecycle.go:557-614` `splitSelvagePaneAtBottomLocked` — split below the bottom-most pane, with a one-shot retry behind an even-vertical re-tile (review finding R4-F4: a one-row Selvage cannot be split, so a stale `SelvagePaneID` would otherwise wedge the worktree permanently). The retry must keep carrying `launchCmd`.
- `lifecycle.go:615-652` `splitPaneBelowLocked` — sole caller is the above; guards with `validateSplitCreatedNewPane`.
- Call sites that stay: `upLocked` (`lifecycle.go:653`) and `Resume` (`lifecycle.go:752`) each clear the binding after a server rebirth and then call `ensureSelvagePaneLocked`.
- `reconcile.go:32-150` `planReconcile(strands, live, selvagePaneID)` — three Selvage touches: the `p.ID != selvagePaneID` dead-kill exemption, the `selvageAlive` local, the `exemptPaneIDs` insertion, plus the `anyBoundPresent || selvageAlive` reap gate.
- `reconcile.go:188-211` `clearConflictingPaneBindings` — claims `st.SelvagePaneID` so no strand can share it.
- `spawn.go:39-73` `planPaneTarget(live, selvagePaneID)` — tallest alive non-Selvage, else any present non-Selvage corpse, else `live[0]` (Selvage itself).
- `spawn.go:149` and `spawn.go:176-178` — the call, and the `-b` decision keyed on `splitTargetID == st.SelvagePaneID`.
- `apply.go:82-98` `toRenderInputs` — blanks `selvagePaneID` when the pane is absent, feeds `render.Selvage{PaneID, HeightRows}`.
- `generation.go:151` — clears `st.SelvagePaneID` in `adoptPaneGenerationLocked`.

Files whose Selvage hits are comments only today, and must stay that way: `windowsize.go` (1), `attach.go` (1), `overlay.go` (1), `template.go` (1).
`state.go` (the field + a comment) and `config.go` (`SelvageConfig`) keep their declarations.

Enforcement-test pattern to copy: `internal/cliwire/bannedecl_enforcement_test.go` — `go/parser` + `go/ast` walk over repository-relative dirs resolved via `runtime.Caller`, a package-level allowlist var with a doc comment explaining the residual, and `filepath`/`io/fs` traversal. `internal/gitkit/callerset_enforcement_test.go` is the second instance of the same shape.

Build prerequisite: this repo links quarry's tree-sitter grammars through cgo, so `CGO_ENABLED=1` plus a C compiler is needed for `go build`/`go test` (see CLAUDE.md).

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `internal/reedengine` is a bound package: it is handed absolute paths and must not import `internal/lyxcwd`. The new file derives no paths of its own.
- **Test Tier Purity Invariant** — the enforcement test is untagged, so it must do no `exec.Command`, no `gitexec.Run`, no `gitkit.Copy*`, no `hubforge.NewHub`, and no `time.Sleep` ≥ 1s. An AST walk over source files satisfies this; a `go/packages` load or a shelled-out `grep` would not.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle` for which docs are kept.
- **CLI / Cobra Invariant** — untouched: no CLI surface changes, so no `Command()`/`RunCLI` seam or help-tree work.

Discovered during exploration:

- The op-lock convention: every `…Locked` method assumes the op lock is already held. Moved methods keep the suffix and the "Assumes the op lock is already held" comment.
- `reconcile.go`'s comments document three live-verified hazards (a stale id emitted as a layout cell is accepted by tmux 3.6 and scrambles heights; presence-exemption vs aliveness-authorization must stay separate because `AddStrand`/`UpdateStrand` never heal a corpse). These must move with the code, not be paraphrased away.
- `manifest/designs/reed-selvage-pane-extraction.md`'s own audit numbers are the before-state baseline the new Status section is measured against.

## Testing

- **`selvagepane.go`'s pure helpers** — TDD candidates, in `selvagepane_test.go`: the reconcile-policy value type (table-driven over the alive/corpse/absent/empty-id cases, asserting the three questions independently so a future fold-together fails), `planPaneTarget`'s three tiers plus the new `insertAbove` flag (tier 3 true, tiers 1 and 2 false), `bottommostPaneID` (ties, single pane, empty), and the render-blanking helper (present vs absent vs empty id).
- **The enforcement test** — TDD candidate, and it must be written so it demonstrably fails before the extraction: run it against the pre-move tree and confirm it reports `lifecycle.go`, `reconcile.go`, `spawn.go`, `apply.go` and `generation.go`. A check that passes on both sides of the refactor proves nothing.
- **Moved lifecycle tests** — the existing `lifecycle_test.go` cases covering `ensureSelvagePaneLocked`'s heal-or-create matrix and the re-tile retry move to `selvagepane_test.go` unchanged. Passing unchanged is the evidence the move was behavior-preserving.
- **Existing suites, untouched** — `reconcile_test.go` (80 Selvage assertions), `spawn_test.go` (51), `apply_test.go` (18), `contract_integration_test.go` (73), `attachgeometry_integration_test.go` (45), `watchdog_integration_test.go` (17) must all pass with no edits beyond what a changed `planPaneTarget` arity forces.
- **Post-extraction audit** — re-run the per-file `Selvage` count the design doc recorded, and record the new numbers in its Status section. The four host files should show comment-only hits.
- **Full run** — `go build ./...` and `go test ./...` (cgo enabled), plus the integration-tagged reed suites, since Selvage's split path is only exercised for real against a live tmux server.

## Q&A log

- **Q:** Extract into a new file in `internal/reedengine`, a new sub-package, or a new type? **A:** [auto-pick] Single new file `selvagepane.go` in the existing package, Engine methods plus pure helpers. **Why:** every moved function needs `e.tmux`/`e.cfg`/`e.geom`/`*ReedState`; a sub-package would invert the dependency and force `LivePane`/`ReedState` into an exported cross-package vocabulary for no gain, and per-concern *files* are the package's existing unit.
- **Q:** Move only the create/reap/heal lifecycle, or the Selvage decision logic inlined in `reconcile.go`/`spawn.go`/`apply.go` as well? **A:** [auto-pick] Both — the four host files end up with zero Selvage-specific code, comments only. **Why:** lifecycle-only extraction would leave `reconcile.go`'s exemption reasoning and `spawn.go`'s three-tier targeting exactly where they are, so the same audit that opened this task would still fail.
- **Q:** Split `pinGeometryOptionsLocked` into three testable helpers, preserve it as-is, or make it a thin coordinator? **A:** [auto-pick] Leave it untouched and correct the design doc. **Why:** re-reading HEAD, the doc's "recombines all three concerns" claim is stale — the pin/signal *install* half already lives in `installResizePinsLocked`, the "Selvage pin at index 0" property comes from `render.FixedHeightPins`' ordering, and `windowsize.go`'s only Selvage hit in the whole file is a comment. There is no three-way merge left to split.
- **Q:** How is "the scatter is actually gone, not just relocated" verified — a hand-run grep, or an enforcement test? **A:** [auto-pick] An AST enforcement test following `internal/cliwire/bannedecl_enforcement_test.go`. **Why:** a hand-run grep is a one-time check that rots; the repo already has this exact pattern twice, and it is the only form of the check that keeps holding on the next edit.
- **Q:** What exactly does the enforcement test police, given the four host files must keep their explanatory Selvage comments? **A:** [auto-pick] Only `.SelvagePaneID` selector expressions and top-level declarations whose name contains `Selvage`, outside an allowlist of `selvagepane.go` / `state.go` / `config.go`. Comments, string literals, locals and parameters are not policed. **Why:** the prose comments are explicitly meant to stay, and a host function *receiving* the pane id as a narrow parameter is the desired seam rather than a violation.
- **Q:** Is any behavior change or opportunistic fix allowed in this refactor? **A:** [auto-pick] No — pure refactor, same tmux calls in the same order, same error strings and log messages. **Why:** it keeps the diff reviewable as a move and keeps the ~350 existing Selvage assertions usable as the regression net.
- **Q:** Do the existing Selvage tests move to `selvagepane_test.go`? **A:** [auto-pick] Only the ones whose subject function moved; behavioral and integration tests stay put. **Why:** test file mirrors source file, the package's existing convention; moving every Selvage-mentioning test would drag integration suites away from the behavior they cover.
- **Q:** Does this warrant a new `CONSTRAINTS.md` invariant? **A:** [auto-pick] No — it stays module-local in `doc.go`, with the enforcement test as its teeth. **Why:** `doc.go` already states explicitly that the Selvage rules are kept there "rather than in CONSTRAINTS.md" because no other module can violate them; a cross-cutting entry would contradict a standing decision.
- **Q:** Should `validateSplitCreatedNewPane` move into `selvagepane.go` along with the split helpers? **A:** [auto-pick] No — it stays in `spawn.go`. **Why:** both split sites (Selvage's and every strand's) share it and it encodes a psmux hazard, not Selvage policy; moving it would make `spawn.go` depend on the Selvage file for a generic guard.
- **Q:** Which docs move in this commit? **A:** [auto-pick] `internal/reedengine/doc.go`, `manifest/designs/reed-selvage-pane-extraction.md` (Status section + the `pinGeometryOptionsLocked` correction), and the roadmap item marked complete — not `docs/overview.md`, not `CONSTRAINTS.md`. **Why:** CLAUDE.md's task-completion rule: the module table and execution stack do not change, and there is no new cross-cutting invariant.
