MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder

```yaml
duration_s: 155.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:scope] Env fixture inventory incomplete; misses newTestEnv
**Demoted-from:** BLOCKING
**Section:** Technical context → "What already exists"; Testing → `Build` twelve-engine case
**Issue:** The discussion names `internal/shedrecipe/coverage_guard_test.go`'s fakes as "the pattern for constructing a `shedrecipe.Env`", but that file fills only `loomshed.Deps`-shaped values; `internal/shedrecipe/fixture_test.go`'s `newTestEnv` is the actual filled-`Env` builder (all nine roots plus `Shuttle`, `Burler`, `WebsterRun`, and the four `WebsterDeps` seams), and it is never mentioned. The enumerated fixture for the twelve-engine case ("a temp `RunRoot`, a temp `StencilsDir`, five Webster seam fakes") omits `Env.Shuttle` (required by `bouncerEntry` and `singleLLMEntry`), `Env.Burler` (required by `burlerRoundEntry`), and `Env.WorktreeRoot` (`bouncerEntry`'s `artifact_paths` resolution) — the test fails at `Bouncer` for a fixture reason, the exact failure mode the "`Build` inherits constructor-side filesystem effects" decision exists to prevent.
**Fix:** Point the fixture inventory at `internal/shedrecipe/fixture_test.go`'s `newTestEnv` as the shape to copy, and complete the enumeration with `Shuttle`, `Burler`, and `WorktreeRoot`.

### [BLOCKING:design] Strict-key decode mechanism vs promised error text
**Section:** Decisions → "Strict unknown-key rejection"; Testing → `Parse`
**Issue:** The discussion requires unknown-key errors that name the key *and* the row's index and name, and a `shedbuild: `-prefixed error for a `config:` that is a scalar or list — but never says how the document is decoded. `yaml.v3`'s only strict mechanism is `Decoder.KnownFields(true)` (unavailable on `yaml.Unmarshal`), whose error is `line N: field x not found in type ...` with no row identity, and a scalar decoded into a `map[string]any` field yields a raw `yaml:` type error, not a row-scoped one. The two decided behaviours are not both reachable from the obvious implementation.
**Fix:** State which decode strategy `Parse` uses (`KnownFields` vs decoding rows into `map[string]any`/`yaml.Node` and walking keys manually) and, if `KnownFields`, which of the promised message contents is relaxed.

### [NIT:consistency] Q&A docs list omits `manifest/designs/shed.md`
**Section:** Scope → In (docs bullet) vs Q&A "Which docs land in the same commit?"
**Issue:** Scope requires correcting `shed.md`'s two `Segment`-departure sentences (verified live at `manifest/designs/shed.md:352-353`), but the Q&A answer lists only `shed-recipe.md`, `docs/overview.md`, `CONSTRAINTS.md`, `manifest/roadmap.md`.
**Fix:** Add `manifest/designs/shed.md` to that Q&A answer.

### [NIT:consistency] CONSTRAINTS edit is a list append plus a count
**Section:** Constraints → Told-Geometry Invariant
**Issue:** The Told-Geometry Invariant's machine-enforced bullet ends "**Enforced by** the ten tests named above"; adding `internal/shedbuild` makes eleven, and the discussion describes only appending to the package list.
**Fix:** Note that the trailing count sentence changes in the same edit.

### [NIT:decision] Row type name and yaml tagging unstated
**Section:** Scope → In; Decisions → byte-slice core
**Issue:** `Recipe` is pinned (`Version int`, `Entry string`, `Terminals []string`, `Producers`), but the producer-row element type is only ever called "row" — its exported name, field set, and whether `config` stays `map[string]any` on an exported field are left to the plan.
**Fix:** Name the row type and its exported field list, since `Recipe` is the package's public surface.

## Verdict

REQUEST_CHANGES
Two blocking gaps: incomplete Env fixture inventory and an unspecified strict-decode mechanism.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
