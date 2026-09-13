# Batch: specs-seeding-wiring

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: specs-seeding-wiring
number: 4
cards: 5
verify: go test ./cmd/lyx/... ./internal/cliwire/... ./internal/stencilcli/... && go test -tags integration ./cmd/lyx/... ./internal/cliwire/... ./internal/stencilcli/...
depends-on: [1, 2, 3]
```

## Batch Scope

This batch makes the deployed specs actually exist on disk, in both modes, and makes the two `lyx stencil` verbs that cover specs cover them.
It adds a second `stencilstore.Reconcile` call to the hub root pre-run and to the standalone prologue, a second `CommitSeededStencils` call to the hub pre-run and to `lyx stencil sync`, and specs rows to `lyx stencil list`.

It is one batch because the same reconcile-then-commit pair is repeated across the two wiring sites and the one CLI verb, and because the two integration tests that prove the seeding are written against those sites together.

The external interface batch 5 consumes is `cliwire.Standalone.SpecsDir` — the resolved specs directory the standalone prologue now returns alongside `StencilsDir`.

Batch-local decisions.
The specs reconcile in standalone runs unconditionally, outside the `stencilsDir == ""` guard: that guard protects an operator's curated prompt set from being rewritten, and a told stencils override says nothing about specs.
Inheriting the skip would leave the specs directory resolvable but empty, which reproduces the dead reference this task exists to remove, in the mode hardest to notice it.
The specs reconcile passes `sourceDir = ""`, so no port-back drift comparison runs; that is what keeps `internal/stencilstore` unmodified.
Failure posture at each site matches its neighbour exactly: best-effort and logged in the hub pre-run, a hard error in the standalone prologue.

## Cards

### Card 12: Seed and commit the specs subtree in the hub pre-run

- **Context:**
  - `contracts/specs/specs.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `cmd/lyx/stencilseed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `seedStencilsAt` in `cmd/lyx/stencilseed.go` so the once-per-process pre-run pass reconciles and commits the deployed-specs subtree alongside the stencils subtree.

  Split the existing body's reconcile-then-commit sequence into a reusable local helper rather than duplicating it, e.g. `seedSubtree(hub, baseDir, subtreeRel string, registry stencilstore.Registry, mode stencilstore.Mode, sourceDir, label string)`, which performs the `stencilstore.Reconcile` call, returns early when nothing was written, performs the `fabricengine.CommitSeededStencils` call with the told subtree pair, and logs the same `logger.Warn` / `logger.Info` lines the current body logs, with `label` distinguishing the two passes in the log fields.
  `seedStencilsAt` then calls it twice: once for stencils, with `fabricengine.StencilsDir(hub)`, `fabricengine.StencilsSubtreeRel()`, `stencils.Registry()`, and the existing `sourceDir` derivation; once for specs, with `fabricengine.SpecsDir(hub)`, `fabricengine.SpecsSubtreeRel()`, `specs.Registry()`, and `sourceDir = ""`.

  The empty `sourceDir` on the specs pass is deliberate and gets a comment saying so: `sourceDir` exists only to drive the port-back drift warning, which serves an authoring workflow specs do not have — the loomyard-side file is the single source of truth and a deployed copy is never authored.
  Do not derive a per-name source mapping for the specs pass; the two travelling docs live in different directories and one's basename differs from its registered name, so no single `sourceDir` shape fits.

  Keep both passes best-effort exactly as the stencils pass is today: a reconcile or commit failure logs a `logger.Warn` and returns without failing the command, since the root pre-run runs before every single `lyx` invocation.
  A failure of the stencils pass must not skip the specs pass, and vice versa — call them independently.

  Keep `seedStencils`' `testing.Testing()` early return, its `skipStencilSeed` annotation gate, and `stencilSeedTarget` unchanged: a command that reads no stencils reads no specs either, so the existing opt-out covers both.
  Import `github.com/Knatte18/loomyard/contracts/specs` alongside the existing `contracts/stencils` import.
- **Commit:** `feat(lyx): seed and commit the deployed specs subtree in the root pre-run`

### Card 13: Resolve and seed the specs directory in the standalone prologue

- **Context:**
  - `contracts/specs/specs.go`
  - `internal/standalonegeom/specsdir.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `internal/cliwire/standalone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `SpecsDir string` field to the `Standalone` result struct in `internal/cliwire/standalone.go`, declared immediately after `StencilsDir`, with a comment stating that unlike `StencilsDir` it is never told by a flag — there is no `--specs-dir` — so it is always the derived default.

  In `ResolveStandalone`, after the existing stencils resolve-and-seed block and before the `result := Standalone{...}` literal, add the specs resolve-and-seed:

  ```go
  specsDir := standalonegeom.SpecsDir(stateDir)
  if _, err := stencilstore.Reconcile(specsDir, specs.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), ""); err != nil {
  	return Standalone{}, fmt.Errorf("%s: seed the standalone specs directory %s: %w", m.Name, specsDir, err)
  }
  ```

  Then set `SpecsDir: specsDir` in the `result` literal.

  This block sits OUTSIDE the `if stencilsDir == ""` guard, deliberately and with a comment saying why: that guard exists so an operator who named a curated stencil set with `--stencils-dir` never has it rewritten from under them, and that rationale is about operator-authored prompts.
  It does not extend to a normative spec — `--stencils-dir` is a stencils override and says nothing about specs, there is no `--specs-dir` flag and none is being added, and a spec has no customisation story a curated set would express.
  Inheriting the skip would leave the specs directory resolvable but empty, so `{{.specs_dir}}` would point at nothing and reproduce the dead reference this task exists to remove, in the one mode hardest to notice it.
  State in that comment that an operator who has edited a deployed spec is still protected, because the reconcile policy is unchanged and its edited row warns and never overwrites.

  The failure posture is a hard error, not a logged warning, matching the stencils seed immediately above it and for the same reason its comment already gives: nothing else will ever create this directory, so a reconcile failure here would otherwise surface much later as a far less informative prompt-render failure.

  Respect the ordering obligation the function's own doc comment records: this block logs nothing and sits below the durable-sink redirect, so no statement above the redirect gains a logging call.
  Do not touch `RefuseUnreadableStencilsDir`, `resolveStandaloneTarget`, the nested-geometry refusal, or the plan-dir resolution.
  Extend `ResolveStandalone`'s own doc comment where it enumerates the ordered prologue steps, so the specs seed is named as one of them and its unconditionality is recorded there too.
- **Commit:** `feat(cliwire): resolve and unconditionally seed the standalone specs directory`

### Card 14: Cover specs in `lyx stencil list` and `lyx stencil sync`

- **Context:**
  - `contracts/specs/specs.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/stencilcommit.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:**
  - `internal/stencilcli/cli.go`
  - `internal/stencilcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend two of the five `stencil` subcommands in `internal/stencilcli/cli.go` to cover deployed specs, and leave the other three alone.

  `list`: after the existing loop over `stencils.Registry()` against `fabricengine.StencilsDir(l.HubPath)`, run the identical loop over `specs.Registry()` against `fabricengine.SpecsDir(l.HubPath)`, appending into the same `list` slice.
  Add a `kind` field to the anonymous `stencilInfo` struct, JSON-tagged `kind`, carrying the literal `"stencil"` for rows from the first loop and `"spec"` for rows from the second, so an operator can tell the two classes apart in one envelope.
  Extract the per-registry loop into a local helper taking the baseDir, the registry, and the kind label, rather than writing the body twice.
  `list` is the only remaining verb that surfaces a deployed spec whose state is edited, which the unchanged reconcile policy depends on an operator being able to see — say that in a comment at the specs loop.

  `sync`: after the existing stencils `ForceRefresh` + `CommitSeededStencils` pair, add the same pair for specs — `stencilstore.ForceRefresh(fabricengine.SpecsDir(l.HubPath), specs.Registry(), "")` followed by `fabricengine.CommitSeededStencils(l.HubPath, fabricengine.SpecsSubtreeRel(), fabricengine.SpecsDir(l.HubPath), ...)` — accumulating into the same `rec` so one envelope reports both passes' mutations.
  The specs `ForceRefresh` passes an empty sourceDir for the same reason the seeding pass does.
  Force-refreshing is the documented remedy for a deployed spec an operator has edited, so both halves are needed, not only the commit.
  Report both commits in the envelope: keep the existing `committed` and `sha` keys for the stencils commit and add `specs_committed` and `specs_sha` for the second, rather than silently overwriting the first pair.

  A failure in the specs half — from either its force-refresh or its commit — returns through the same shapes the stencils half already uses, not through a new one: a force-refresh failure returns a bare `output.Err`, and a commit failure returns `errWithRecord` carrying the record snapshot accumulated so far, which by then already includes the stencils half's own mutations.
  Both return early, leaving the stencils commit landed and reported as partial.
  State this explicitly rather than leaving the second half's failure posture to be inferred from the first half's.

  Widening `list` breaks an existing integration assertion, which moves in this same card: `internal/stencilcli/cli_integration_test.go`'s list-and-validate test asserts the returned entry count equals the stencils registry's own name count, and that equality fails the moment the specs rows join the same slice.
  Change that assertion to the sum of both registries' name counts, and extend the test's per-name loop so the specs names are checked for presence too rather than silently ignored.
  Add one assertion the widening makes worth having: every returned row carries a `kind` of either the stencil label or the spec label, and the count of each matches its own registry's name count — so a future change that appends rows from one registry under the other's label fails here.
  Leave the rest of that test alone, including its validate half, which specs deliberately do not reach.

  Leave `validate`, `diff`, and `promote` untouched, and record why in a short comment beside the specs `list` loop.
  `validate` compares top-level marker sets via `stencil.TopLevelMarkers`; a spec is not a template, so both sides are empty and the pass would be a guaranteed no-op that falsely implies a check ran.
  `diff` and `promote` both need a worktree `sourceDir`, which specs deliberately do not have.
  No verb gains a flag or a subcommand, so the no-new-CLI-surface decision holds.
- **Commit:** `feat(stencilcli): cover deployed specs in list and sync`

### Card 15: Hub seeding integration test

- **Context:**
  - `cmd/lyx/stencilseed.go`
  - `cmd/lyx/stencilseed_integration_test.go`
  - `contracts/specs/specs.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/specsseed_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `cmd/lyx/specsseed_integration_test.go` in `package main`, carrying the `//go:build integration` constraint, building its hub through `internal/hubforge` exactly as `cmd/lyx/stencilseed_integration_test.go` does — no hand-assembled hub, per the hubforge Fabric-Fixture Invariant.
  It drives `seedStencilsAt` directly, which is what that function exists as a separately-callable value for.

  `TestSeedStencilsAt_SeedsBothSubtrees` asserts that after one call against a fresh hub, both subtrees exist and are populated: the stencils tree as it is today, and, under `fabricengine.SpecsDir(hub)`, the two files `loom/loom-plan-spec.md` and `loom/loom-plan-card-format.md` plus a `.gitattributes`.
  For each seeded spec, assert `stencilstore.ParseStamp` reports a parseable stamp and that `stencilstore.Classify` over the on-disk bytes reports the untouched state — proving the seeded copy is immediately self-consistent rather than reclassified as edited on the next pass.

  `TestSeedStencilsAt_SecondRunWritesNothing` calls `seedStencilsAt` twice and asserts the second call leaves both trees byte-identical to what the first produced, and that the board repository is clean afterwards.
  This is what pins that the pass stays free on an ordinary run.

  `TestSeedStencilsAt_CommitsTheSpecsSubtree` asserts the seeded specs files are tracked in the board repository after the first call, not merely present on disk.
  This is the assertion that would have caught the write-but-never-stage defect the generalisation in batch 3 exists to fix, so it must assert tracked-ness through git rather than through `os.Stat`.

  Use `fabricengine.SpecsDir`, `stencilstore.Path`, and `stencilstore.RelPath` to construct every expected path rather than joining directory literals, per the geometry-literal enforcement walk.
- **Commit:** `test(lyx): pin hub-mode specs seeding, idempotence, and weft commit`

### Card 16: Standalone seeding integration test, including a told stencils override

- **Context:**
  - `internal/cliwire/standalone.go`
  - `internal/cliwire/cliwire_test.go`
  - `internal/cliwire/module.go`
  - `internal/standalonegeom/specsdir.go`
  - `internal/stencilstore/stencilstore.go`
- **Edits:** none
- **Creates:**
  - `internal/cliwire/specsseed_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/cliwire/specsseed_integration_test.go` in the same package the existing `internal/cliwire/cliwire_test.go` uses, carrying the `//go:build integration` constraint, driving `Module.ResolveStandalone` against a `t.TempDir()`-rooted target.

  `TestResolveStandalone_SeedsTheSpecsDirectory` asserts that a plain standalone resolve populates `Standalone.SpecsDir`, that the directory equals `standalonegeom.SpecsDir(res.StateDir)`, and that both registered specs exist under it with a parseable stamp.

  `TestResolveStandalone_ToldStencilsDirStillPopulatesSpecs` is the key scenario and the one most likely to regress silently.
  It resolves with a non-empty `StencilsDirFlag` pointing at a readable curated directory the test creates, then asserts three things in order: the told stencils directory was NOT seeded (its contents are exactly what the test put there, proving the existing skip still protects a curated prompt set); `Standalone.SpecsDir` still resolves; and the seeded spec files exist on disk under it.
  Assert the files exist, not merely that the path resolves — a resolvable-but-empty specs directory reproduces the original dead reference while looking correct from a path assertion alone, which is precisely the failure this test exists to catch.

  `TestResolveStandalone_SpecsDirIsAbsolute` asserts `filepath.IsAbs(res.SpecsDir)`.
  This is not cosmetic: a deployed spec lives outside the agent's own worktree, so the rendered marker must be an absolute path for the agent to be able to open it at all.

  Follow whatever `Module` fixture construction the existing tests in this package already use rather than introducing a second way to build one.
- **Commit:** `test(cliwire): pin standalone specs seeding, including under a told stencils directory`

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/cliwire/... ./internal/stencilcli/... && go test -tags integration ./cmd/lyx/... ./internal/cliwire/... ./internal/stencilcli/...`

The untagged half compiles and runs the three packages the cards edit, catching a missed call-site signature or a broken envelope in `internal/stencilcli`.

The `-tags integration` half is required, not optional: cards 15 and 16 both create files carrying the `//go:build integration` constraint, so the untagged invocation never compiles them.
It is scoped to the two packages those files live in rather than the repository, since no other package's tagged tests change in this batch.

`internal/stencilcli` is in the tagged half too, and deliberately so: card 14 edits that package's own integration test, whose entry-count assertion the `list` widening breaks.
Without the package in the tagged scope, that regression would be invisible to every batch verify in this plan and would surface only at the final repository-wide done gate, after the whole plan had already landed.
Card 14's `sync` change needs no separate tagged coverage of its own — card 15's hub assertions exercise the same force-refresh and commit pair through the seeding path.
