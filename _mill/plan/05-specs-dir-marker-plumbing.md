# Batch: specs-dir-marker-plumbing

```yaml
task: Deploy cited spec/design docs to target repos like stencils
batch: specs-dir-marker-plumbing
number: 5
cards: 7
verify: go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomengine/... ./internal/websterengine/... ./internal/loomcli/... ./internal/burlercli/... ./internal/burlerengine/...
depends-on: [2, 4]
```

## Batch Scope

This batch makes `specs_dir` fillable everywhere the four affected prompts are composed, so that batch 6 can put `{{.specs_dir}}` into the stencil bodies and have it render.
It delivers the shared rubric-render helper, the told `specs_dir` parameter on each of the four render routes, and the wiring that supplies the value at each route's own call site.

It is one batch because the marker value has to arrive at all four routes at once: a route that gains the value while another does not is invisible today (an unused values-map entry is harmless) but becomes a hard render failure the moment batch 6 lands, and there is no intermediate state worth reviewing separately.

The order within the batch matters and is reflected in the card order: the helper first, its own tests second, then the three stencil-sourced rubric sites, then the two direct-template routes, then the wiring that feeds all of them.

Batch-local decisions.
The rubric-render helper lives in `internal/shedadapters` because both of its consumers already import that package and both already perform the read-and-strip it replaces.
`specs_dir` is a required marker, never optional: a blank render would silently reproduce the dead reference this whole task exists to remove, and `stencil.Fill` errors on an absent-or-empty required marker, which is exactly the loud-early failure wanted.
`specs_dir` follows whatever route each consumer already uses for its stencils directory — a geometry field for webster, a plain told parameter for loom and shed — rather than being normalised onto one shape, because that is what the Told-Geometry Invariant's existing per-consumer shapes already are.

Nothing in this batch changes a stencil body.
After it lands, every route supplies a `specs_dir` value that no template yet consumes, and every existing test stays green.

## Cards

### Card 17: The shared rubric-render helper

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/shedadapters/bouncer.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/rubric.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/rubric.go` in `package shedadapters`, declaring one exported function:

  ```go
  func ReadRubric(stencilsDir, name, specsDir string) (string, error)
  ```

  It performs, in this order: `stencilstore.Read(stencilsDir, name)`; `stencil.StripLeadingComment` over the read bytes; `stencil.Fill` over the stripped bytes with the single value map `{"specs_dir": specsDir}`; and returns the filled result as a string.
  Any error from either step is wrapped with a `shedadapters: read rubric %q:` prefix naming the rubric and returned.

  Three properties the doc comment must state, because each is load-bearing and each is what a later reader would otherwise undo.

  The strip is about **value** semantics, not about protecting `Fill`.
  `stencil.Fill` already strips a leading banner from the template it parses, but never from a marker value — and a rubric's destination is a marker value, interpolated into the Bouncer templates' `{{.rubric}}`.
  Unstripped bytes would inject the `<!-- lyx-stencil: sha256=... -->` line into the middle of the judge prompt.
  This is the same reasoning the two existing call sites' own comments already carry, and it must survive the move into the helper.

  The strip happens **before** the fill, not after: `Fill` strips its own template internally, so stripping afterwards would be too late to keep the banner out of the returned value.

  `specs_dir` is a **required** marker here — plain `Fill`, never `FillOptional`.
  A rubric that carries `{{.specs_dir}}` and is handed an empty value must error rather than render a blank path, because a blank path is precisely the dead reference this task removes.
  A rubric that carries no marker at all renders unchanged, so the helper is safe to route every stencil-sourced rubric through from the moment it exists.

  State the negative too: this helper is for **stencil-sourced** rubrics only.
  A literal rubric string — author-written prose reaching a producer through a config key rather than through the stencil store — never goes through here, because running author prose through `Fill` turns any bare `{{` in it into a parse-template error and imposes specs-dir semantics on text that never had them.

  Do not add a caching layer, a variant that accepts extra marker values, or an `Optional` sibling.
- **Commit:** `feat(shedadapters): add the shared stencil-sourced rubric render helper`

### Card 18: Rubric helper tests

- **Context:**
  - `internal/shedadapters/rubric.go`
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/stencilstore.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:** none
- **Creates:**
  - `internal/shedadapters/rubric_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedadapters/rubric_test.go` covering `ReadRubric` directly rather than only through a composed prompt.
  Build each fixture by writing a stamped file into a `t.TempDir()` at the path `stencilstore.Path(dir, name)` — use `stencilstore.ApplyStamp` with `stencilstore.BodyHash` to produce a realistically stamped file, not a bare body, since the stamp is what half these assertions are about.

  Five tests.

  `TestReadRubric_SubstitutesTheSpecsDir` — a rubric body containing `{{.specs_dir}}` renders with the told path substituted, and the returned string contains no `{{.` substring afterwards.

  `TestReadRubric_StripsTheStampBanner` — the returned string does not contain `stencilstore.StampPrefix`, nor the word-for-word stamp line the fixture was written with.
  This is the assertion that fails if a future edit drops the strip or reorders it after the fill, which would interpolate the banner into the middle of a judge prompt.

  `TestReadRubric_MarkerlessRubricRendersUnchanged` — a rubric body with no marker returns exactly its own stripped body, byte for byte.

  `TestReadRubric_EmptySpecsDirIsAnError` — a rubric carrying `{{.specs_dir}}` with `specsDir == ""` returns a non-nil error and an empty result, never a rendered-blank path.
  Assert the same for a whitespace-only value, since that is what `stencil.Fill`'s own empty check treats as unfilled.

  `TestReadRubric_UnreadableRubricIsAnError` — an unregistered or absent name returns a wrapped error naming the rubric.

  Do not assert on the exact error strings beyond the rubric name appearing in them.
- **Commit:** `test(shedadapters): pin the rubric helper's fill, strip, and required-marker behaviour`

### Card 19: Route the Bouncer's two rubric reads through the helper

- **Context:**
  - `internal/shedadapters/rubric.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `internal/shedadapters/bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `SpecsDir string` field to `BouncerConfig` in `internal/shedadapters/bouncer.go`, declared immediately after the existing `StencilsDir` field, documented as the absolute deployed-specs directory the rubric's `{{.specs_dir}}` marker is filled from.

  Replace the read-and-strip pair at both rubric sites with one `ReadRubric` call each.

  In `runSeedSpawn`, the current `stencilstore.Read(b.cfg.StencilsDir, b.cfg.RubricStencil)` followed by `stencil.StripLeadingComment` becomes `ReadRubric(b.cfg.StencilsDir, b.cfg.RubricStencil, b.cfg.SpecsDir)`.
  Keep this site's existing failure posture exactly: a `logger.Warn` with the same message key and the same fields, then `return nil` without further action — the seed spawn degrades rather than erroring, per that function's own doc comment.

  In `judgeCall`, the same pair becomes one `ReadRubric` call, keeping that site's existing failure posture: `b.degrade(ctx, "shedadapters: bouncer rubric unreadable", ...)` with its current fields.
  A fill failure now reaches these same branches, which is correct — a rubric whose `specs_dir` cannot be filled is exactly as unusable as one that cannot be read.

  Leave the eager rubric probe in `NewBouncer` alone.
  It calls `stencilstore.Read` purely as a construction-time readability check and discards the bytes, so it needs no fill.
  Add one sentence to its existing comment saying so explicitly, so a later reader does not "fix" it into a `ReadRubric` call: converting it would make construction fail on an empty `SpecsDir` for a rubric that carries no marker, which is a new failure mode rather than a tightening.

  Do not change `stencil.Fill`'s marker maps for the two Bouncer templates: the templates still receive `rubric` as a value, and `specs_dir` reaches them only inside that already-filled value.
  Removing the now-unused `stencil` import from this file is fine if nothing else in it uses the package; check before deleting.
- **Commit:** `refactor(shedadapters): route both Bouncer rubric reads through ReadRubric`

### Card 20: Thread the specs directory through the shed recipe registry

- **Context:**
  - `internal/shedadapters/rubric.go`
  - `internal/shedadapters/bouncer.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_bouncer.go`
  - `internal/shedrecipe/entries_burler.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `SpecsDir string` field to `shedrecipe.Env` in `internal/shedrecipe/recipe.go`, declared immediately after the existing `StencilsDir` field and documented in the same voice: the told deployed-specs directory, read by the Bouncer and BurlerRound entries.
  It is a run-wide root, which is exactly the class `Env` carries, so it belongs here rather than as a per-row config key — say so in its comment, since the type's own doc comment makes that distinction load-bearing.
  The Shed Recipe Registry Invariant requires it to be told, never derived: no entry may construct this path itself.

  In `internal/shedrecipe/entries_bouncer.go`, add a `requireAbsRoot("Bouncer", "SpecsDir", env.SpecsDir)` guard beside the existing `StencilsDir` guard, and set `SpecsDir: env.SpecsDir` in the `shedadapters.BouncerConfig` literal beside the existing `StencilsDir` line.
  The guard is what turns an unwired caller into a construction error naming the field, rather than into a rubric that fails to fill at first judge call.

  In `internal/shedrecipe/entries_burler.go`, change `burlerRoundProfile`'s signature from `(cfg Config, stencilsDir string)` to `(cfg Config, stencilsDir, specsDir string)`, and update its sole caller in the same file to pass `env.SpecsDir`.
  In the `rubricStencil != ""` branch, replace the `stencilstore.Read` plus `stencil.StripLeadingComment` pair with a single `shedadapters.ReadRubric(stencilsDir, rubricStencil, specsDir)` call, keeping the existing error wrapping shape that names the config key and the rubric name.
  Add a `requireAbsRoot("BurlerRound", "SpecsDir", specsDir)` guard inside that same branch, beside the existing `StencilsDir` guard — inside the branch, not at function entry, so a row using a literal `rubric:` still constructs with no specs directory wired, exactly as it does today.

  The literal-`rubric:` branch is untouched, and that is the point.
  The two config keys are already mutually exclusive, so a literal rubric never reaches the helper; running author-written prose through `Fill` would turn any bare `{{` in it into a parse-template error.
  Extend `burlerRoundProfile`'s doc comment where it describes `rubric_stencil` to say that the stencil route now fills `specs_dir` at read time while the literal route passes through unfilled, and that the asymmetry is deliberate.

  Do not add a `specs_dir` recipe config key, and do not add `specs_dir` to the SingleLLM entry's fill-token set — no SingleLLM-rendered stencil carries the marker.
- **Commit:** `feat(shedrecipe): thread the told specs directory into the Bouncer and BurlerRound entries`

### Card 21: Thread the specs directory into the Plan producer's prompt

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/stencilstore/reconcile.go`
- **Edits:**
  - `internal/loomengine/plan.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Thread the specs directory through the Plan producer's two composition functions in `internal/loomengine/plan.go` as a plain told parameter, matching how `stencilsDir` already arrives there.

  Change `composePlanPrompt`'s signature from `(stencilsDir, decisionRecordPath, planDir, overviewPath, patternDirective, frictionDirective string)` to `(stencilsDir, specsDir, decisionRecordPath, planDir, overviewPath, patternDirective, frictionDirective string)`, and add `"specs_dir": specsDir` to its `values` map.

  `specs_dir` must stay OUT of the optional-names slice passed to `stencil.FillOptional` — that slice keeps its current two entries, the pattern directive and the friction marker.
  A required `specs_dir` is the whole point: `FillOptional` errors on a required marker that is absent or empty, so a Plan prompt composed without a specs directory fails loudly at composition instead of rendering a blank path into the agent's instructions.
  Say that in the function's doc comment.

  Change `PlanSpec`'s signature from `(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry)` to `(layout *lyxcwd.Location, stencilsDir, specsDir string, cfg Config, reg modelspec.Registry)`, and pass `specsDir` through to `composePlanPrompt`.
  Document the new parameter as told, never derived — this package is bound by the Told-Geometry Invariant and derives no path of its own.

  Leave `DiscussionSpec` and the Discussion producer's own composer untouched: the discussion stencil cites no normative spec, so giving it a marker value it never consumes would be noise.
  Do not change `PlanSpec`'s returned `shuttleengine.Spec` fields.
- **Commit:** `feat(loomengine): thread the told specs directory into the Plan prompt`

### Card 22: Thread the specs directory into webster's implementer prompts

- **Context:**
  - `internal/stencil/stencil.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** The shared implementer-job body is composed into both the in-session fork prompt and the cold recovery prompt, so both renderers need the value.

  In `internal/websterengine/render.go`, add a `specsDir string` parameter to `RenderForkPrompt` and to `RenderRecoveryPrompt`, placed immediately after each one's existing `stencilsDir` parameter so the two told directories stay adjacent in both signatures.
  Add `"specs_dir": specsDir` to each function's `values` map.
  In both, `specs_dir` stays out of the optional-names slice passed to `stencil.FillOptional`, which keeps only the friction marker (and, in the recovery case, whatever it carries today) — a blank specs path must fail the render rather than reach the agent.

  Leave `RenderMasterPrompt` and `RenderIntegrationPrompt` alone: neither the Master template nor the integration template cites a normative spec.

  Update the two call sites to pass the told geometry field added in batch 2.
  In `internal/websterengine/beginbatch.go`, the `RenderForkPrompt` call gains `deps.Geom.SpecsDir` beside its existing `deps.Geom.StencilsDir` argument.
  In `internal/websterengine/recoverbatch.go`, the `RenderRecoveryPrompt` call gains the same.

  Update `internal/websterengine/render.go`'s file comment where it enumerates what each renderer fills, so the new marker is named there alongside the friction directive rather than discovered from the signature.
- **Commit:** `feat(websterengine): thread the told specs directory into both implementer prompts`

### Card 23: Supply the specs directory at every wiring site

- **Context:**
  - `internal/hubgeom/webstergeom.go`
  - `internal/cliwire/standalone.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomengine/plan.go`
  - `internal/websterengine/geometry.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Fill the told value at the one wiring site that owns loom's whole stack, in `internal/loomcli/wiring.go`.

  Set `SpecsDir: websterGeom.SpecsDir` in the `shedrecipe.Env` literal, immediately after the existing `StencilsDir: websterGeom.StencilsDir` line, and extend that line's existing comment — which already explains that the stencils directory is the same value the producer-spec closures use — to cover the specs directory the same way.

  Update the `PlanSpec` closure in the same literal to pass the new argument: `loomengine.PlanSpec(location, websterGeom.StencilsDir, websterGeom.SpecsDir, loomCfg, registry)`.

  Both values come off the already-built webster geometry rather than from a fresh `fabricengine.SpecsDir(location.HubPath)` call, for the reason the surrounding code already follows for the stencils directory: one resolution, one spelling, no chance of two parts of the stack disagreeing about where the directory is.

  No change is needed in webster's own CLI wiring: its geometry builder already carries `SpecsDir` in both modes, and unlike the stencils directory there is no flag that can override it after the builder returns.
  No change is needed in burler's CLI wiring either: its rubric arrives from a profile YAML key and is always literal, so it never reaches the render helper.
  Do not add a specs-directory field or flag to either.
- **Commit:** `feat(loomcli): supply the told specs directory to the shed registry and the Plan producer`

## Batch Tests

`verify: go test ./internal/shedadapters/... ./internal/shedrecipe/... ./internal/loomengine/... ./internal/websterengine/... ./internal/loomcli/... ./internal/burlercli/... ./internal/burlerengine/...`

The scope is every package a card edits, plus the two burler packages that must stay green for the negative half of this batch.
`internal/burlercli` and `internal/burlerengine` are in scope deliberately even though no card edits them: card 20's whole asymmetry claim is that a literal `rubric:` value still passes through unfilled, and those two packages own the literal route — the profile-YAML decode and the round prompt's own `rubric` value.
A regression there would mean author prose newly erroring as a malformed template, which is exactly the failure mode the decision rejects, and it would show up nowhere else.

New coverage in this batch is card 18's direct helper tests.
The rest of the batch is signature threading, where a missed call site is a compile failure rather than a test failure — which is why the verify scope is package-wide rather than `-run`-scoped.

No stencil body changes here, so the existing content-pinning tests in `contracts/stencils` are untouched and stay out of this batch's verify scope; batch 6 is where they move.
No integration-tagged file is edited, so no `-tags integration` invocation is needed.
