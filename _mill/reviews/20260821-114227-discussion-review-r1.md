MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder

```yaml
duration_s: 142.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:consistency] Preflight fake defeats the type-equality assertion
**Demoted-from:** BLOCKING
**Section:** Testing § loom-equivalence
**Issue:** The section asserts `reflect.TypeOf(Producer)` equality per index while also saying "Both sides need a `Preflight` fake"; `shedrecipe.preflightEntry` returns `preflightshed.NewPreflight(name, env.Cwd)` (`internal/shedrecipe/entries_simple.go:25`), so a fake on the `loomshed.Deps.Preflight` side (the `coverageGuardFakePreflight{}` pattern) makes row 1's types differ by construction.
**Fix:** State which resolution the plan takes — hand the loomshed side a real `preflightshed.NewPreflight` (construction spawns nothing) rather than a fake, or explicitly exempt row 1 from the type assertion and say why.

### [NIT:scope] Equivalence Env needs five Webster seams never named
**Demoted-from:** BLOCKING
**Section:** Testing § loom-equivalence / Technical context
**Issue:** `websterEntry` requires non-nil `Env.WebsterRun` plus `WebsterDeps.Starter`, `.Reed`, `.Engine`, `.RefMatcher` (`entries_simple.go:150-172`), while `loomshed.New` validates none of them, so `coverage_guard_test.go` leaves them zero and is not the pattern to copy here; the discussion enumerates only a Preflight fake, a `mergeresolve.Shuttle` fake, and `landingshed.Deps`.
**Fix:** Add the four `websterengine.RunDeps` seam fakes plus the `shedadapters.WebsterRunner` fake to the fixture inventory, naming the interfaces they must satisfy.

### [BLOCKING:design] `Build` is not filesystem-free; twelve-engine test needs on-disk fixtures
**Section:** Decisions § Byte-slice core / Testing § `Build`
**Issue:** "Nothing else in the package touches the filesystem" holds only for shedbuild's own code — `bouncerEntry` and `burlerRoundEntry` do `os.MkdirAll(runDir)` (`entries_bouncer.go:71`, `entries_burler.go:71`) and `bouncerEntry`/`singleLLMEntry` eagerly `stencilstore.Read` the rubric/stencil, so the "every one of `Names()` is buildable" case needs a writable `RunRoot` and real stencil files under `StencilsDir`, which the table-driven-in-Go framing does not provide.
**Fix:** State that `Build` inherits constructor-side filesystem effects, and specify the fixture (temp `RunRoot`, hand-written stencil files — `stencilstore.Read` is a plain `os.ReadFile`, no stamp needed).

### [NIT:scope] `manifest/designs/shed.md` carries the superseded Segment statement
**Demoted-from:** BLOCKING
**Section:** Scope § In (docs list) / Decisions § `segment` is a recipe row field
**Issue:** `manifest/designs/shed.md:352-353` states `blind-gate` "replaces the departing `Segment` rule" and that dropping `Segment` belongs to "the recipe-loader items"; this task's reversal makes both false, but the doc list names only `shed-recipe.md`, `docs/overview.md`, `CONSTRAINTS.md`, and `roadmap.md`.
**Fix:** Add `manifest/designs/shed.md` to the same-commit doc list, or state why those two lines stay as written.

### [NIT:decision] `version` and `Load`'s path absoluteness have no stated disposition
**Section:** Decisions § Strict unknown-key rejection / § `shedbuild` defines no on-disk location
**Issue:** It is unstated whether `Recipe` surfaces the decoded `version` at all, and whether `Load` rejects a relative path (sibling `standalonestate` rejects; `shedrecipe.requireAbsRoot` rejects) or simply reads whatever it is handed.
**Fix:** Record both dispositions in one line each.

## Verdict

REQUEST_CHANGES
Equivalence-test fixture and Segment doc scope need resolving before plan writing.
_Note: 3 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
