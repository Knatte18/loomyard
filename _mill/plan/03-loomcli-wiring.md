# Batch: loomcli-wiring

```yaml
task: 'loom: Plan-Write producer'
batch: 'loomcli-wiring'
number: 3
cards: 1
verify: go test ./internal/loomcli/...
depends-on: [2]
```

## Batch Scope

This batch fills the two new `shedrecipe.Env` seams for real, in the one place that can: `internal/loomcli`'s `wire()`, the only layer holding a `*lyxcwd.Location`.
It is one batch of one card because the production change and its assertions are a single, tightly-scoped edit to one function and its test file, and because nothing else in the tree can be written until the closures exist.
Until this batch lands, `PlanWrite` is constructible only from a test fixture — a real `lyx loom` run would fail construction on a nil `Env.PlanSpec`.
The external interface batch 4 consumes is nothing at all: batch 4 is stencil, prompt-assertion, and documentation work that depends only on this batch having established the as-built truth its docs describe.
Batch-local decision, differing from nothing in `## Shared Decisions`: `internal/loomcli` gains a direct `internal/planparser` import, which is legal — `loomcli` is the top wiring layer and already imports twelve internal packages, and `planparser.PlanDirRel()` is the only correct source of the commit pathspec under the Lyxdirs Single-Declarer Invariant.

## Cards

### Card 11: wire PlanSpec and CommitPlan in loomcli

- **Context:**
  - `internal/loomengine/plan.go`
  - `internal/planparser/parse.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/shedrecipe/entries_planwrite.go`
  - `internal/fabricengine/doc.go`
  - `internal/pattern/pattern.go`
  - `contracts/stencils/stencils.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/wiring.go`, add two fields to the `shedrecipe.Env` composite literal `wire` assigns to `c.env`, placed immediately after the existing `CommitDiscussion` field so the two write-producer pairs sit together.

  `PlanSpec` is a closure returning `loomengine.PlanSpec(location, websterGeom.StencilsDir, loomCfg, registry)`. Like `DiscussionSpec` beside it, it is evaluated per `Call` rather than resolved here, so the stencil is read at call time — what the Stencil Ownership Invariant requires. Note in its comment that `PlanSpec` takes no `autonomous` argument because the Plan producer is autonomous by design and hard-codes `Interactive: false`, unlike `DiscussionSpec`, which takes the flag.

  `CommitPlan` is a closure calling `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{planparser.PlanDirRel()}, fmt.Sprintf("loom: plan artifacts for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())`, discarding the returned sha and `committed` bool and returning only the error. Its comment must mirror `CommitDiscussion`'s three recorded reasons — it keeps the working tree clean for the rows that follow, makes the artifact durable across a crash or a resume, and sweeps the decorator's archive subdirectory into git rather than leaving it as untracked dirt — and must record that a re-commit over an already-clean tree is a no-op rather than an error, because `CommitAnchoredPaths` reports `committed == false` for an already-tracked clean path. It must also record that the commit fires before `Plan-Validate` has judged the plan, and that this is intentional and matches the discussion precedent: the commit keeps the artifact durable, it does not certify it.

  The pathspec is the whole plan directory via `planparser.PlanDirRel()` — never a hand-built `filepath.Join` naming the `_lyx` literal, which the Lyxdirs Single-Declarer Invariant forbids in production path-construction context. Add `"github.com/Knatte18/loomyard/internal/planparser"` to the import block in its sorted position. The trailing comment block in the same literal explains why `StencilsDir`, `RunRoot`, `Burler`, and `Now` are left zero and notes that `DiscussionSpec` captures `websterGeom.StencilsDir` directly rather than reading it back off `Env` — extend that sentence so it covers the `PlanSpec` closure doing the same thing. Change nothing else in `wire`.

  In `internal/loomcli/wiring_test.go`, first seed the plan stencil, then add the two tests.

  Seeding is not optional here. `loomengine.PlanSpec` reads `loom-template-plan` through `stencilstore.Read` at call time, and that read hard-errors on a missing file, but `hubLocation` today calls only `seedDiscussionStencil`, which writes `loom-template-discussion.md` alone — so evaluating `c.env.PlanSpec()` against an unseeded hub returns a non-nil error and the new shape test fails before it asserts anything. Add a `seedPlanStencil(t *testing.T, hubPath string)` helper beside `seedDiscussionStencil`, identical in shape but writing `stencils.LoomTemplatePlan` to `loom-template-plan.md` in the same `filepath.Join(fabricengine.StencilsDir(hubPath), "loom")` directory, and call it from `hubLocation` immediately after the `seedDiscussionStencil(t, hub)` call. Update `hubLocation`'s doc comment, which says its hub is seeded with the discussion stencil, to say both loom stencils. No pattern stencil is needed: `pattern.Directive` returns an empty string and a nil error when PATTERN is inactive at the anchor, which it is in a bare `t.TempDir()`.

  Then add two tests modelled on `TestWire_DiscussionSeamsFilled` and `TestWire_DiscussionSpecEvaluatesToExpectedShape`, both `t.Parallel()` and both driving `c.wire(loc, loc.AnchorPath())` against a `hubLocation(t, "warp", ".")`. `TestWire_PlanSeamsFilled` asserts `c.env.PlanSpec` and `c.env.CommitPlan` are each non-nil after `wire()`. `TestWire_PlanSpecEvaluatesToExpectedShape` evaluates `c.env.PlanSpec()` once and asserts `spec.Interactive` is false, `spec.Role` is `"plan"`, `spec.Timeout` equals `time.Duration(c.cfg.PlanTimeoutMin) * time.Minute`, `spec.Model` is non-empty, `spec.OutputFiles` deep-equals `[]string{planparser.PlanOverview(loc.AnchorPath())}` — the `AnchorPath()`-rooted overview path, never a `WorktreePath()`-rooted one — `spec.Prompt` is non-empty, and `spec.Prompt` contains no unrendered `{{` marker. Add `"github.com/Knatte18/loomyard/internal/planparser"` to this file's import block if it is not already present.
- **Commit:** `feat(loomcli): wire the PlanSpec and CommitPlan seams`

## Batch Tests

`verify: go test ./internal/loomcli/...` covers the one package this batch touches.
The two new tests are the whole gate: `TestWire_PlanSeamsFilled` proves `wire()` fills both seams non-nil, which is what `planWriteEntry`'s `requireSeam` checks would otherwise reject at construction time in a real run, and `TestWire_PlanSpecEvaluatesToExpectedShape` proves the closure evaluates to a Spec whose role, timeout, and anchored output path are the ones `internal/loomengine/plan.go` promises.
Both are tier 1 — `wire` resolves no cwd and spawns no process, and `hubLocation` builds its `*lyxcwd.Location` by hand, which is why `wiring.go` was extracted from the pre-run in the first place.
No other package is affected: `Env`'s two new fields were already added and fixture-filled in batch 2, so nothing outside `internal/loomcli` changes shape here.
