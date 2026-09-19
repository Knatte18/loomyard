# Batch: selvagepane-extraction

```yaml
task: 'reed: extract Selvage-pane lifecycle'
batch: selvagepane-extraction
number: 1
cards: 9
verify: go test ./internal/reedengine/
depends-on: []
```

## Batch Scope

This batch delivers the whole code half of the task: a new `internal/reedengine/selvagepane.go` owning every piece of Selvage-specific code in the package, the four host files (`lifecycle.go`, `reconcile.go`, `spawn.go`, `apply.go`) plus `generation.go` rewired to call a narrow seam instead of inlining Selvage logic, an AST enforcement test that keeps the scatter from returning, and the tests whose subject functions moved relocated alongside them.

It is one batch because Go requires a moved function and its changed call sites in the same compilable unit, and because the enforcement test only goes green once all four hosts are cleared — see the overview's `one-batch-for-the-whole-extraction` Shared Decision.
Card 1 is committed deliberately red and the batch ends green.

The external interface batch 2 consumes is the finished file layout: `selvagepane.go` as the named owner, the seam helper names listed in the overview's `seam-naming-under-the-enforcement-check` Shared Decision, and the enforcement test as the teeth `doc.go`'s module-local Selvage rules can point at.

No batch-local decision differs from the overview's Shared Decisions.

## Cards

### Card 1: AST enforcement test, committed red against the pre-extraction tree

- **Context:**
  - `internal/cliwire/bannedecl_enforcement_test.go`
  - `internal/gitkit/callerset_enforcement_test.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/generation.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/selvagepane_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an untagged test file in package `reedengine` carrying two tests plus one shared scanning helper.

  The shared helper is `selvageIdentViolations(fset *token.FileSet, astFile *ast.File) []string`, returning one formatted `position: identifier` string per violating identifier.
  It walks the file with `ast.Inspect` and flags every `*ast.Ident` whose `strings.ToLower` form contains `selvage`, with exactly two exempt positions.
  Exemption (i) is the function position of a call expression — the `Fun` of an `*ast.CallExpr`, whether a bare `f(...)` identifier or the `Sel` of an `x.f(...)` selector.
  Exemption (ii) is a composite-literal field key — the `Key` of a `*ast.KeyValueExpr` appearing inside a `*ast.CompositeLit`.
  Everything else is flagged: field selectors, composite-literal types, func/method/type/var declarations, struct field declarations, parameters and locals.
  Comments and string literals are never read, so a package-doc paragraph naming Selvage and a `yaml:"selvage"` struct tag both pass untouched.

  The first test, `TestSelvageIdentifiersConfinedToSelvagePane`, resolves the repository root from `runtime.Caller(0)` by walking up from this file's directory, exactly as the two precedent tests do, then parses every non-`_test.go` `.go` file in `internal/reedengine` itself and reports any violation whose file is outside the allowlist.
  The allowlist is a plain file set: `selvagepane.go`, `state.go`, `config.go`.
  It scans that one directory only, never `internal/reedengine/render/` and never any sibling package.

  The second test, `TestSelvageIdentViolations_ExemptionsAndFlags`, drives `selvageIdentViolations` over synthetic source parsed from in-memory strings rather than from disk, so the exemptions stay verified rather than assumed and keep being verified after the extraction lands.
  It asserts as flagged: a `render.Selvage{...}` composite-literal type, an `e.cfg.Selvage` selector, an `st.SelvagePaneID` selector, and a parameter named `selvagePaneID`.
  It asserts as not flagged: a `render.Params{Selvage: x}` composite-literal key, and an `e.ensureSelvagePaneLocked(st)` call.

  The file's doc comment must state four things.
  What each allowlisted file is allowlisted for — `selvagepane.go` for the whole Selvage seam, `state.go` for the `SelvagePaneID` field declaration, `config.go` for `SelvageConfig` and `Config.Selvage` — since the file-set allowlist does not itself enforce that intent.
  Why the match is case-insensitive: a case-sensitive rule would silently permit exactly the `selvagePaneID` parameters and locals the extraction exists to eliminate.
  The honest residual, in the house style the two precedents already use: the call-position exemption covers same-package calls only, so a Selvage-named function exported from another package would be exempt at its call site and unscanned at its declaration — latent rather than open today, since `internal/reedengine/render/` exports the type `Selvage` (caught as a composite-literal type) and no Selvage-named function.
  And the grounding for the allow shape: no existing enforcement test in this repo uses a file-level allowlist, so this one introduces it; the nearest precedent is `internal/gitkit/callerset_enforcement_test.go`'s single allowed-directory const, the same allow shape at coarser granularity.
  Cite `internal/cliwire/bannedecl_enforcement_test.go` only for what it supplies — the AST walk, the `runtime.Caller(0)` repository-root resolution, and the house style of recording a residual in a doc comment.
  Its policy shape is the inverse of an allowlist, so the doc comment must not present it as an allowlist precedent.

  Per `CONSTRAINTS.md`'s Test Tier Purity Invariant this file is untagged and therefore performs no `exec.Command`, no `gitexec.Run`, no `gitkit.Copy*`, no `hubforge.NewHub`, and no sleep of one second or longer.
  An AST walk over source files satisfies that; a `go/packages` load or a shelled-out grep does not.

  Before committing, run this file's two tests against the pre-extraction tree and record the first test's violation list in the commit message.
  The expected flagged files are `apply.go`, `generation.go`, `lifecycle.go`, `reconcile.go` and `spawn.go`, and the record must name `apply.go`'s `render.Selvage` composite-literal type and its `e.cfg.Selvage` selector explicitly, since that line is the one a field-selector-only rule would have missed.
  The commit message must state that the first test is red at this commit by design and goes green when the extraction lands later in this batch.
- **Commit:** `test(reedengine): add the Selvage-confinement AST enforcement test, red before the extraction`

### Card 2: create selvagepane.go and move the create/heal path into it

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/generation.go`
- **Creates:**
  - `internal/reedengine/selvagepane.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the new file in package `reedengine` with a file-level doc comment in the package's existing per-concern-file style, stating that this file owns every piece of Selvage-specific code in the package and that the four host files call into it.

  Move four declarations out of `internal/reedengine/lifecycle.go` into it, each carrying its full existing doc comment and body unchanged: `ensureSelvagePaneLocked`, `bottommostPaneID`, `splitSelvagePaneAtBottomLocked`, `splitPaneBelowLocked`.
  `splitSelvagePaneAtBottomLocked`'s doc comment records the even-vertical retry's review finding and both routes that reach the wedged state — it moves verbatim per the overview's `comments-move-verbatim-with-their-code` Shared Decision.
  These four keep their `Locked` suffix and their op-lock assumption unchanged.
  They continue to call `aliveIDSet` and `liveIDSet` from `internal/reedengine/apply.go` and `validateSplitCreatedNewPane` from `internal/reedengine/spawn.go`, all same-package, so no signature changes.

  Add `clearSelvagePaneBinding(st *ReedState)`, a package-level function whose whole body is `st.SelvagePaneID = ""`, with a doc comment stating it is the single writer of that clear and naming its three callers.
  Replace all three existing `st.SelvagePaneID = ""` assignments with a call to it: two in `internal/reedengine/lifecycle.go`, inside `upLocked`'s and `Resume`'s `if booted` blocks, and one in `internal/reedengine/generation.go`, inside `adoptPaneGenerationLocked`.
  Leave the explanatory comments that sit above those assignments where they are — they explain why the reborn session's reused pane id must not be mistaken for a live Selvage, which is still the host's reason for calling.

  The two `e.ensureSelvagePaneLocked(st)` call sites in `upLocked` and `Resume` stay in `internal/reedengine/lifecycle.go` unchanged.
- **Commit:** `refactor(reedengine): move Selvage's create/heal path into selvagepane.go`

### Card 3: reconcile reap-policy seam

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/reedengine/selvagepane.go` an unexported value type `reapPolicy` with two unexported fields holding the Selvage pane id and whether it is alive, built by `newReapPolicy(st *ReedState, live []LivePane) reapPolicy`.
  Neither the type's name nor any of its three method names contains `selvage`, because a host holds the value in a parameter and a local — see the overview's `seam-naming-under-the-enforcement-check` Shared Decision.

  Give it exactly three methods, answering the three questions `planReconcile` asks today, each staying a distinct question:
  `exemptFromDeadKill(paneID string) bool`, true when the policy's pane id is non-empty and equals `paneID`;
  `exemptFromUntrackedReap(paneID string) bool`, with the same predicate but its own doc comment naming its own reason;
  `authorizesReap() bool`, true only when the Selvage pane is present and not dead.
  The non-empty guard is explicit in both predicates rather than relying on a live pane id never being the empty string.

  Change `planReconcile`'s third parameter from `selvagePaneID string` to `policy reapPolicy`, leaving the first two parameters and the `reconcilePlan` return unchanged.
  Rewrite its three Selvage touches to ask the policy: the dead-kill loop's `p.ID != selvagePaneID` condition becomes a negated `exemptFromDeadKill` call; the `selvageAlive` local and the block computing it are deleted, and the reap gate becomes `anyBoundPresent || policy.authorizesReap()`; the `exemptPaneIDs` map keeps holding bound strand pane ids alone and the untracked-reap loop gains a separate `exemptFromUntrackedReap` disjunct instead.
  Keeping the two exemptions as separate terms rather than folding Selvage back into `exemptPaneIDs` is what makes the three questions visibly three at the call site.

  The two long comment blocks explaining why a dead-but-present Selvage is spared the dead-pane kill, and why presence-exemption and aliveness-authorization must not be folded together, move into the new file alongside the predicates they now explain, verbatim.
  `planReconcile`'s own doc comment keeps its "spares Selvage" sentence and gains no new Selvage detail.

  `reconcileLocked` builds the value inline at its single call site: `planReconcile(st.Strands, live, newReapPolicy(st, live))`.
  It holds no local for the policy and reads `st.SelvagePaneID` nowhere.
- **Commit:** `refactor(reedengine): move reconcile's Selvage exemption policy into selvagepane.go`

### Card 4: strand-claim seed seam

- **Context:**
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `seedSelvageClaim(st *ReedState, claimed map[string]bool)` to `internal/reedengine/selvagepane.go`.
  It adds the Selvage pane id to `claimed` when that id is non-empty, and its doc comment states the rule it encodes: a strand may never own Selvage's pane.

  In `clearConflictingPaneBindings`, replace the two-line block that seeds `claimed` from the state field with a single call to the new helper, placed at the same point in the function so the first-writer-wins ordering is unchanged.
  The function itself stays in `internal/reedengine/reconcile.go` — its subject is strand-versus-strand binding conflicts, and Selvage is one seed among them.
  Its long doc comment, which records both observed corruption shapes, stays with it unchanged.
- **Commit:** `refactor(reedengine): route the Selvage strand-claim seed through selvagepane.go`

### Card 5: split-target seam, carrying the insert-above decision

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Move `planPaneTarget` out of `internal/reedengine/spawn.go` into `internal/reedengine/selvagepane.go`, carrying its whole existing doc comment — including the adoption history and the two review findings it records — unchanged.
  Its entire subject is picking a split target relative to Selvage, so unlike `planReconcile` and `toRenderInputs` it moves wholesale.

  Change its signature to `planPaneTarget(st *ReedState, live []LivePane) (splitTargetID string, insertAbove bool, err error)`.
  The three targeting tiers keep their existing logic and order, reading the Selvage pane id from `st` instead of from a parameter.
  `insertAbove` is true only when the third tier fires — the tier that falls back to `live[0]` because no non-Selvage pane exists at all — and false for the tallest-alive and present-corpse tiers.
  The error return for an empty `live` is unchanged.

  This is exactly equivalent to the condition it replaces.
  Today `launchStrandLocked` appends `-b` when the chosen target equals the Selvage pane id, and that can only hold when tier three fired: tiers one and two both exclude the Selvage pane by construction, and tier three is reachable only when every present pane is the Selvage pane.
  State that equivalence in the moved function's doc comment so the next reader does not have to re-derive it.

  Move the `-b` rationale paragraph out of `launchStrandLocked`'s inline comment block into the moved function's doc comment, next to the tier it explains, and leave the `-c` paragraph about `Geometry.PaneCwd` where it is.
  At the call site, `launchStrandLocked` calls `planPaneTarget(st, live)`, appends `-b` when `insertAbove` is true, and carries a one-line comment pointing at the moved function for why.
  It reads `st.SelvagePaneID` nowhere.
- **Commit:** `refactor(reedengine): move planPaneTarget into selvagepane.go and fold in the insert-above decision`

### Card 6: render-params seam

- **Context:**
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
- **Edits:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/apply.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func (e *Engine) selvageRenderParams(st *ReedState, presentIDs map[string]bool) render.Selvage` to `internal/reedengine/selvagepane.go`.
  It returns the whole `render.Selvage` value: the pane id taken from `st`, blanked to the empty string when `presentIDs` does not hold it, and `HeightRows` read from `e.cfg.Selvage.HeightRows`.
  Returning the whole value rather than only the blanked id is what moves all three pieces — the blanking rule, the value construction, and the config read — out of the host in one step.
  This is the package's only `render.Selvage` construction and, outside `config.go`, its only `e.cfg.Selvage` read, and both now live in this one file.

  In `toRenderInputs`, delete the local holding the pane id and the two lines that blank it, and assign the seam's return value straight into the composite literal's `Selvage` field.
  The rest of the literal — `CollapsedStripRows` and `MinFullRows` — is unchanged, as are the function's signature, its `presentIDs` local, and its `strands` and `paneOrder` fields.
  Update its doc comment so the sentence describing the blanking now points at the seam instead of describing the rule inline.
- **Commit:** `refactor(reedengine): move the Selvage render-params mapping into selvagepane.go`

### Card 7: relocate the tests whose subject functions moved

- **Context:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/lifecycle_test.go`
  - `internal/reedengine/spawn_test.go`
- **Creates:**
  - `internal/reedengine/selvagepane_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  A test relocates exactly when the function it tests relocated; behavioral and integration tests stay where they are.

  Relocate eight tests out of `internal/reedengine/lifecycle_test.go` into the new test file: `TestBottommostPaneID`, and the seven whose names begin `TestEnsureSelvagePaneLocked_` — `SplitsWithPaneCwdNotAnchorPath`, `RebuildRejectsSilentSplitFailure`, `RecoversWhenTheBottomPaneIsTooSmallToSplit`, `LaunchesTheCommandOnTheSplitNotViaSendKeys`, `RecordsThePaneIDAfterLaunch`, `RetriedSplitAlsoCarriesTheLaunchCommand`, `SplitsBelowTheBottommostPaneWithNoBFlag`.
  All eight relocate unchanged — their subjects moved without a signature change, so passing unchanged is the evidence the move preserved behavior.

  Relocate `TestPlanPaneTarget` out of `internal/reedengine/spawn_test.go` into the same new file.
  The order is relocate first, then adapt: move its table across unchanged, then apply the one edit card 5's signature change forces — each case builds a `ReedState` carrying the case's Selvage pane id and passes it in place of the bare string, and the call takes the third return value.
  Existing cases assert the chosen target only; the new `insertAbove` return is covered by card 9 rather than here.

  Leave every other test in both source files untouched, including the helper `guids` and the integration-tagged suites.
  Add a file doc comment to the new test file in the package's existing style.
- **Commit:** `test(reedengine): relocate the tests whose subjects moved into selvagepane_test.go`

### Card 8: adapt the in-place reconcile test call sites

- **Context:**
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `TestPlanReconcile`'s table stays in this file, because `planReconcile` did not move.
  Apply the one edit card 3's signature change forces: each case builds its policy with `newReapPolicy` from a `ReedState` carrying that case's Selvage pane id plus the case's live pane slice, and passes it as `planReconcile`'s third argument in place of the bare string.
  Every case's inputs and expectations are otherwise unchanged — an assertion that needs rewriting means behavior moved.
  `TestClearConflictingPaneBindings` and the two `TestReconcileLocked_` tests need no edit and get none.
- **Commit:** `test(reedengine): build the reap policy in TestPlanReconcile's table`

### Card 9: unit tests for the new seam helpers

- **Context:**
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/config.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedengine/selvagepane_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add table-driven tests for every helper card 3 through card 6 introduced.

  For `newReapPolicy`, assert the three questions independently rather than through a single combined predicate, so a future fold-together fails here: over the cases alive Selvage, dead-but-present Selvage corpse, Selvage pane id naming an absent pane, and empty Selvage pane id, assert `exemptFromDeadKill` and `exemptFromUntrackedReap` for both the Selvage pane id and an unrelated pane id, and assert `authorizesReap` separately.
  The corpse case is the one that pins the distinction: it is exempt from both kills and authorizes neither reap.

  For `planPaneTarget`, add cases covering the new `insertAbove` return across all three tiers — false for the tallest-alive tier, false for the present-corpse tier, true for the Selvage-only tier.

  For `selvageRenderParams`, assert the present, absent and empty-id cases, and that `HeightRows` comes through from the engine's config rather than being defaulted.

  For `seedSelvageClaim`, assert it adds the id when non-empty and leaves the map untouched when empty.
  For `clearSelvagePaneBinding`, assert it empties a set id and is a no-op on an already-empty one.
- **Commit:** `test(reedengine): cover the new Selvage seam helpers`

## Batch Tests

`verify: go test ./internal/reedengine/` runs the package's whole untagged tier, which is the correct scope here rather than a narrower `-run` pattern for two reasons.
The batch edits shared unexported functions every one of the package's untagged test files exercises transitively — `planReconcile`, `planPaneTarget` and `toRenderInputs` are reached from `lifecycle_test.go`, `spawn_test.go`, `reconcile_test.go`, `apply_test.go`, `reapply_test.go`, `strand_test.go`, `server_test.go` and `windowsize_test.go` alike — so a pattern-scoped run would leave exactly the regressions this refactor risks unobserved.
And the enforcement test added in card 1 is itself part of the gate: it is what proves the four host files ended up clear, and it lives in this same package.

The run covers the untagged tier only; `integration`- and `smoke`-tagged files are excluded by their build tags.
Those two tiers are verified in batch 2 card 14 and by the configured `pipeline.done_gate`, not here, because the `smoke` tier drives a live tmux server and is too slow to run after every implementer and fixer round.

The 276 case-sensitive `Selvage` matching lines across the package's test files must pass with no edits beyond the two the changed signatures force — `TestPlanReconcile`'s policy construction in card 8, and `TestPlanPaneTarget`'s in card 7.
Any other test edit means behavior moved and the pure-refactor claim is false.
