# Batch: loomcli-wiring

```yaml
task: 'loom: Discussion-Review producer'
batch: 'loomcli-wiring'
number: 4
cards: 2
verify: go test ./internal/loomcli/...
depends-on: [2, 3]
```

## Batch Scope

This batch fills the four `shedrecipe.Env` fields `wire()` has deliberately left zero — `StencilsDir`, `RunRoot`, `Burler`, and `Now` — plus the four `Env.Review*` fields batch 2 added, and rewrites the comment that explained why they were empty.
It lands before the recipe rows (batch 5) on purpose: filling these fields while rows 5 is still a `Stub` is a no-op at runtime, whereas flipping the rows first would leave a window where `loomrecipe.New` fails at construction in production because the `Bouncer` and `BurlerRound` entries reject a zero `Env.RunRoot`, `Env.StencilsDir`, and `Env.Burler`.
The external interface batch 5 consumes is a fully-filled `Env`;
this batch adds no new function to the package.

## Cards

### Card 12: fill the four unfilled Env fields and the Review triple

- **Context:**
  - `internal/burlercli/wiring.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/burlerengine/config.go`
  - `internal/burlerengine/engine.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/review.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Load burler's config alongside the other per-module loads at the top of `wire()`.
  Note the actual signature is `burlerengine.LoadConfig(baseDir string)` — one argument, not the two-argument `(baseDir, module)` shape every other loader in `wire()` uses — so the call is `burlerengine.LoadConfig(anchorPath)`.
  It is an optional-file loader: an absent `burler.yaml` yields a zero `Config` and a nil error, so this adds no new seeding requirement to any caller or test.
  Build the burler engine after `runner` and `websterGeom` already exist: `burlerengine.New(runner, hubgeom.BurlerGeometry(location), burlerCfg, websterGeom.StencilsDir)`, mirroring `internal/burlercli/wiring.go`'s own hub-branch construction.
  Use `hubgeom.BurlerGeometry` as-is and do not converge it with `hubgeom.WebsterGeometry`: `BurlerGeometry` uses `l.WorktreePath()` for its `WorktreeRoot` while `WebsterGeometry` uses `l.AnchorPath()`, a deliberate divergence documented in both files.
  Resolve the review triple and timeout by calling `loomengine.ResolveReview(loomCfg, registry)` after the existing `registry` load, propagating its error the same way the other loads do.
  Fill six fields on the `shedrecipe.Env` literal: `StencilsDir: websterGeom.StencilsDir`, `RunRoot: loomengine.LoomReviewsDir(location)`, `Burler:` the engine built above, `Now: time.Now`, and `ReviewModel`/`ReviewEffort`/`ReviewVersion`/`ReviewTimeout` from the resolved settings.
  Rewrite the trailing comment that currently says `StencilsDir`, `RunRoot`, `Burler`, and `Now` are left zero because no row uses those engines yet.
  The replacement states that all four are now filled for the `Discussion-Bouncer`/`Discussion-Burler` segment, that `StencilsDir` is the same value the `DiscussionSpec` and `PlanSpec` closures capture directly (so the two are one value, not two that could drift), and that `Now` is filled explicitly rather than left nil even though nil defaults to `time.Now` inside the underlying constructors, because the Bouncer's archive-filename collision suffix is the one place a test wants to inject a clock.
  Keep the surviving half of the original comment — `Landing` is still deliberately unfilled here, for the eager-fabric-open reason `landingdeps.go` and `drive.go` already document — intact and unweakened.
  Add the `time` import and the `internal/burlerengine` import.
- **Commit:** `feat(loomcli): fill StencilsDir, RunRoot, Burler, Now, and the Review triple on Env`

### Card 13: assert the newly-filled Env fields

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomengine/review.go`
  - `internal/loomengine/config.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/hubgeom/hubgeom.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `TestWire_ReviewSegmentSeamsFilled` asserting, against the existing `hubLocation(t, ...)` fixture, that `c.env.StencilsDir` equals `fabricengine.StencilsDir(loc.HubPath)`, that `c.env.RunRoot` equals `loomengine.LoomReviewsDir(loc)`, that `c.env.Burler` is non-nil, and that `c.env.Now` is non-nil.
  Add `TestWire_ReviewTripleMatchesLoadedConfig` asserting `c.env.ReviewModel`, `c.env.ReviewEffort`, `c.env.ReviewVersion`, and `c.env.ReviewTimeout` equal what `loomengine.ResolveReview` returns for the same loaded config and registry — resolve it in the test rather than hardcoding the template's literal spec, so a later template edit does not silently break the assertion's meaning.
  Follow `TestWire_PathFieldsMatchLoomengineAccessors`' existing convention of asserting an `Env` path field against its own `loomengine` accessor rather than against a re-derived literal.
  Both tests are tier 1, driven against the hand-built `*lyxcwd.Location` the file's other tests already use, and need no new config seeding: `burlerengine.LoadConfig` tolerates an absent `burler.yaml`.
- **Commit:** `test(loomcli): assert the review-segment Env seams and Review triple`

## Batch Tests

`verify: go test ./internal/loomcli/...` runs the untagged tests of the whole `internal/loomcli` package.
That is the exact scope of this batch — both cards touch only files in that package.
The `//go:build smoke` suite in the same directory is deliberately excluded: it needs a real tmux server and a real built binary, and neither card changes anything it exercises.
The package's existing `wiring_test.go`, `landingdeps_test.go`, `cli_test.go`, `status_test.go`, and `validate_test.go` are the regression surface for card 12's added config load and Env widening.
