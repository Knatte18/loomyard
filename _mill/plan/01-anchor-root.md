# Batch: anchor-root

```yaml
task: "Fix Bouncer anchor-path and run-dir clearing"
batch: "anchor-root"
number: 1
cards: 7
verify: go test ./internal/hubgeom/... ./internal/shedrecipe/... ./internal/burlercli/... ./internal/loomcli/... ./internal/loomrecipe/... ./internal/burlerengine/...
depends-on: []
```

## Batch Scope

This batch closes defect 1: a review segment's `_lyx` paths resolve against `Env.WorktreeRoot` (`location.WorktreePath()`) while the commit closures that commit those artifacts anchor at `location.AnchorPath()`.
Two production sites carry it — `shedrecipe.bouncerEntry`'s `artifact_paths` resolution and `hubgeom.BurlerGeometry`'s `WorktreeRoot` fill — and both are re-pointed at the anchor path here.
The remaining five cards are the stale-assertion sweep the `_mill/discussion.md` defect-1 inventory enumerates: every comment or operator-facing string that asserts the superseded root is reworded, rewritten, or deleted per that inventory's stated disposition.
Batch 2 consumes nothing from this batch except the shared `contracts/recipes/loom-recipe.yaml` file, which is why it depends on this one.

Batch-local decision, on top of `## Shared Decisions`: `internal/shedrecipe/entries_singlellm.go` and `internal/shedrecipe/entries_simple.go` survive the inventory's grep but keep their `env.WorktreeRoot` assertions unchanged — both genuinely still resolve there, and both are explicitly Out of scope.
`shedrecipe.Env.WorktreeRoot` itself is not re-pointed or removed; only its field doc changes.

## Cards

### Card 1: `hubgeom.BurlerGeometry` fills `WorktreeRoot` from the anchor path

- **Context:**
  - `internal/hubgeom/webstergeom.go`
  - `internal/hubgeom/webstergeom_test.go`
  - `internal/burlerengine/geometry.go`
  - `internal/hubgeom/doc.go`
  - `internal/standalonegeom/burlergeom.go`
- **Edits:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/hubgeom_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `BurlerGeometry` in `internal/hubgeom/hubgeom.go` to fill `burlerengine.Geometry.WorktreeRoot` from `l.AnchorPath()` instead of `l.WorktreePath()`, leaving its `AnchorPath` fill unchanged.
  Give `BurlerGeometry` a doc comment modelled on `WebsterGeometry`'s in `internal/hubgeom/webstergeom.go`: state plainly that `WorktreeRoot` is `l.AnchorPath()` and not `l.WorktreePath()`, name why that is correct here (a review segment's `_lyx` content is anchor-anchored, matching the commit seam that commits it), warn that converging or reverting the two would silently change behaviour in a subpath-anchored hub, and name `standalonegeom.BurlerGeometry` as the mode where `WorktreeRoot` and `AnchorPath` still legitimately diverge.
  Keep the existing sentences stating that the function performs no `os.Getwd`, no git discovery, and no path resolution of its own, and that `internal/lyxcwd` stays the sole owner of cwd resolution.
  Do not change `ReedGeometry`, whose `WorktreeRoot` stays `l.WorktreePath()`.
  In `internal/hubgeom/hubgeom_test.go`, update `TestBurlerGeometry` so it asserts `got.WorktreeRoot == anchorPath` and `got.AnchorPath == anchorPath`, and add an assertion that `got.WorktreeRoot != worktreeRoot` guarding the exact regression `TestReedGeometry`'s own `PaneCwd` guard guards — the existing table row already sets `anchorRel` to a two-segment subpath, so the two roots already diverge in the fixture.
  Write the updated test first, watch it fail against the unchanged `BurlerGeometry`, then make the production change.
- **Commit:** `fix(hubgeom): BurlerGeometry tells burler the anchor path as its profile root`

### Card 2: record hub mode's told root on `burlerengine.Geometry.WorktreeRoot`

- **Context:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/burlerengine/engine.go`
- **Edits:**
  - `internal/burlerengine/geometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the doc comment on the `WorktreeRoot` field of `Geometry` in `internal/burlerengine/geometry.go`.
  It currently says only that `WorktreeRoot` is the root `Engine.Run` resolves a `Profile`'s relative paths against, via `(*Profile).validate` — that sentence stays true and stays.
  Add that what fills it differs by mode: hub mode now tells it the anchor path (`hubgeom.BurlerGeometry`), while standalone tells it the reviewed target directory (`standalonegeom.BurlerGeometry`), which is why the field is not collapsible into `AnchorPath` and is not renamed.
  This is a doc-only card: change no field name, add no constructor, no validator, and no default, and change nothing in `internal/burlerengine/engine.go`.
  `internal/standalonegeom/burlergeom.go` is listed as `Context:` so the standalone half of the claim is verified against the code rather than copied from this card.
- **Commit:** `docs(burlerengine): record what each mode tells Geometry.WorktreeRoot`

### Card 3: `bouncerEntry` resolves `artifact_paths` under the anchor path

- **Context:**
  - `internal/shedrecipe/paths.go`
  - `internal/shedrecipe/env.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/fixture_test.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_bouncer_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `bouncerEntry`, change the `resolveUnderRoot("Bouncer", "artifact_paths", ...)` call to pass `env.AnchorPath` as its root instead of `env.WorktreeRoot`.
  Replace the `requireAbsRoot("Bouncer", "WorktreeRoot", env.WorktreeRoot)` guard with `requireAbsRoot("Bouncer", "AnchorPath", env.AnchorPath)`, keeping it in the same position relative to the `RunRoot`, `StencilsDir`, and `Shuttle` guards.
  Reword `bouncerEntry`'s own function doc comment sentence "resolves artifact_paths against env.WorktreeRoot" to name `env.AnchorPath`, leaving the rest of that comment unchanged.
  Change nothing else in the entry: `run_subdir` still resolves under `env.RunRoot`, the `os.MkdirAll` of the joined run directory stays, and the `commit_seam` switch is untouched.
  In `internal/shedrecipe/entries_bouncer_test.go`, replace the `BlankEnvWorktreeRoot` subtest with a `BlankEnvAnchorPath` subtest asserting the error names `AnchorPath`, and add a subtest proving a blank `env.WorktreeRoot` no longer prevents a Bouncer row from building.
  Add a subtest that resolves `artifact_paths` with an `Env` whose `AnchorPath` and `WorktreeRoot` are two different existing absolute directories and asserts the constructed row's artifact path is under `AnchorPath` — the fixture in `internal/shedrecipe/fixture_test.go` already creates them as separate subdirectories, and the assertion is worthless if they coincide.
  Because `shedadapters.BouncerConfig.ArtifactPaths` is unexported-adjacent state on the returned `shedengine.ShedProducer`, assert the resolved path by the means the package already has available; if no such means exists, add the narrowest package-internal accessor the assertion needs rather than exporting new surface.
  Keep the existing `artifact_paths` absolute-value and `..`-escape subtests, which now fire against the anchor root.
  Write the test changes first, watch them fail, then make the production change.
- **Commit:** `fix(shedrecipe): Bouncer resolves artifact_paths under Env.AnchorPath`

### Card 4: correct the two `shedrecipe.Env` field docs the fix falsifies

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_singlellm.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/loomshed/planvalidate.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update two field doc comments on the `Env` struct in `internal/shedrecipe/recipe.go`, per the defect-1 inventory row for `internal/shedrecipe/recipe.go:37-42`.
  `AnchorPath`'s doc currently lists its readers as `Batchifier`, `PlanValidate`, `Webster`, and `SingleLLM`'s `anchor_path` token — add `Bouncer` to that list.
  `WorktreeRoot`'s doc currently claims it is "the root every worktree-relative Config path resolves against" — that universal is now false and must go; replace it with its two remaining readers, `PlanValidate` and `SingleLLM`'s `output_files`.
  Change no field name and no field value; this is a doc-only card, and `Env.WorktreeRoot` itself stays filled from `location.WorktreePath()`.
  Verify the two remaining reader claims against `internal/shedrecipe/entries_singlellm.go`, `internal/shedrecipe/entries_simple.go`, and `internal/loomshed/planvalidate.go` before writing them, rather than copying the claim from this card.
- **Commit:** `docs(shedrecipe): correct Env.AnchorPath and Env.WorktreeRoot reader lists`

### Card 5: rewrite `loomcli.wire`'s geometry-choice comment

- **Context:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/hubgeom/webstergeom.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the comment immediately above the `burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir)` call in `internal/loomcli/wiring.go`.
  Its current text justifies choosing `BurlerGeometry` over `WebsterGeometry` on the grounds that the two fill `WorktreeRoot` from different accessors — a divergence card 1 removes, so the stated reason evaporates.
  The call itself does not change and still passes `hubgeom.BurlerGeometry(location)`.
  The replacement comment must say why that is still the right builder — it is burler's own geometry, carrying burler's `AnchorPath` semantics and the field set `burlerengine.Geometry` declares, rather than webster's — without citing a `WorktreeRoot` divergence that no longer exists.
  Change no other line in the file, and leave `websterGeom` and its `StencilsDir` use exactly as they are.
- **Commit:** `docs(loomcli): restate why burler gets BurlerGeometry, not WebsterGeometry`

### Card 6: reword `burlercli`'s three `--target-dir` strings to name the anchor path

- **Context:**
  - `internal/hubgeom/hubgeom.go`
  - `internal/burlercli/cli_test.go`
  - `internal/burlercli/wiring_test.go`
- **Edits:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `lyx burler`'s hub mode shares `hubgeom.BurlerGeometry`, so card 1 changed its observable behaviour, and the CLI/Cobra Invariant makes the affected help text a review obligation.
  Reword all three operator-facing strings that assert the superseded root, per the defect-1 inventory rows.
  In `internal/burlercli/wiring.go`, `wireHub`'s `--target-dir` refusal error currently reads "the worktree is already the target"; reword it to name the anchor path as what burler reviews in hub mode, keeping the trailing clause about stranding artifacts outside fabric's positive-only commit pathspec intact.
  In `internal/burlercli/cli.go`, the `Long` text's "refused in hub mode, where the worktree itself is structurally the target" and the `--target-dir` flag-usage string's "refused in hub mode, where the worktree is already the target" both get the same treatment.
  Leave `wireHub`'s existing "Both configs anchor at `loc.AnchorPath()` … never `WorktreeRoot` or any fabric sibling" comment in place — the inventory disposes of it as Leave, because it was already correct and is now also true of the geometry beside it; optionally extend it by one clause saying so.
  Change no flag name, no command name, no `Short`, and no control flow.
  Run the package's own help-tree and wiring tests afterwards and confirm none of them pinned the old wording; if one did, update that assertion to the new wording rather than reverting the string.
- **Commit:** `docs(burlercli): --target-dir refusal text names the anchor path`

### Card 7: apply the two defect-1 dispositions in the loom recipe

- **Context:**
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/loomrecipe/coverage_guard_test.go`
  - `internal/loomrecipe/shape_test.go`
- **Edits:**
  - `contracts/recipes/loom-recipe.yaml`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two comment-only edits, both from the defect-1 inventory, in `contracts/recipes/loom-recipe.yaml`.
  In the `Plan-Bouncer` row's `config:` block, delete the sentences recording that `artifact_paths` "resolves against `Env.WorktreeRoot`, which is knowingly not the `AnchorPath()` root `Env.CommitPlan` anchors at", that the two are identical while `AnchorRel` is `"."`, that the shipped Discussion pair carries the same shape, and that the fix is filed as its own roadmap item — that deferral is what this task closes and nothing in it survives.
  Keep the preceding sentences of the same comment, which explain why a single directory entry is the right `artifact_paths` value for a variable set of card files and that both consumers accept a directory.
  In the `Webster-Bouncer` row's `config:` block, reword "every entry resolves to an absolute path under `Env.WorktreeRoot`" to name `Env.AnchorPath`, leaving the surrounding rationale about why a directory entry is the least-bad `artifact_paths` value for a diff completely intact.
  Change no row name, no `engine`, no `segment`, no `max_bounces`, no `on_stuck`, no `on_done`, and no config value anywhere in the file — these are comment edits only.
  `Plan-Revalidate`'s own comment is deliberately not touched here; it belongs to defect 2 and is batch 2's card 12.
  Run `internal/loomrecipe`'s coverage guard and shape tests afterwards to confirm the comment edits disturbed neither the row names nor the parsed shape.
- **Commit:** `docs(loom-recipe): drop the closed anchor-root deferral, name AnchorPath`

## Batch Tests

`verify:` runs `go test` over the six packages this batch's edits can affect: `internal/hubgeom` (card 1's own test), `internal/shedrecipe` (card 3's own test, plus `entries_burler_test.go`, `entries_singlellm_test.go`, and the shared `fixture_test.go` `Env`), `internal/burlercli` (card 6's help-tree and wiring tests), `internal/loomcli` (card 5's file, and `wiring_test.go`'s assertion that `env.WorktreeRoot` is still `loc.WorktreePath()` — which must keep passing, since `Env.WorktreeRoot` is explicitly Out of scope), `internal/loomrecipe` (card 7's coverage guard and shape tests, and any fixture asserting a resolved artifact path), and `internal/burlerengine` (card 2's doc-only file, included so a stray edit there cannot pass unnoticed).

The scope is per-batch rather than repo-wide: no package outside these six is edited by any card here.
Cross-package regressions from the shared `hubgeom` and `shedrecipe` helpers are caught by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which mill-go runs from the git root before marking the task done.

The load-bearing assertions are the divergent-roots ones: `TestBurlerGeometry`'s table row already sets `anchorRel` to a two-segment subpath, and card 3's new subtest builds an `Env` whose `AnchorPath` and `WorktreeRoot` are distinct directories.
Either assertion made with coincident roots would pass identically before and after the fix and would prove nothing.
