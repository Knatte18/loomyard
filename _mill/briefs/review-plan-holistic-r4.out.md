MILL_REVIEW_BEGIN
# Review: loom: convert to a Shed recipe — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [BLOCKING:scope] Sweep misses in-package bare `New`/`Deps` doc references
**Location:** batch 5 card 20 / batch 6 card 26
**Issue:** Two production files in `internal/loomshed` carry doc comments falsified by card 20's deletion, in the unqualified in-package spelling card 26's sweep tokens (`loomshed.New`, `loomshed.Deps`) cannot match: `loompreflight.go:29-32` ("row 2 is still built internally by New (see loomshed.go), never injected -- unlike Deps.Preflight ...") and `seed.go:29-30` ("Seed takes bare told paths rather than a Deps ..."). Neither file is in any card's `Edits:` nor in `## All Files Touched`, and card 26 instructs the implementer to *stop and report* rather than edit a file outside its list — so even a lucky hit leaves them stale. A third site sits inside a file card 20 does edit but does not name: `loomshed.go:19-22`'s `Name*` const-block doc ends "because loom's own producer table is what New assembles from them", and card 20 says only to *keep and extend* that comment.
**Fix:** Add `internal/loomshed/loompreflight.go` and `internal/loomshed/seed.go` to card 20's (or a new card's) `Edits:` and to `## All Files Touched`, name the two sentences to restate; extend card 20's const-block requirement to cover the trailing "what New assembles from them" clause; and add bare-word `Deps`/`New(` in-package spellings to card 26's sweep-token list.

### [NIT:design] Coherence-check ordering and the divergent-value shape are unpinned
**Location:** batch 1 card 3 / batch 2 card 7
**Issue:** Card 3 does not say whether `New`'s `env.StatusPath`/`paths.StatusPath` (and `StatusLockPath`) coherence check runs before or after `shedbuild.Parse`/`Build`, and card 7 says only to "overwrite one side" of `testEnv(t)`'s coherent pair. If the check runs after `Build` and the test overwrites with an empty or relative path, `loomPreflightEntry`'s `requireAbsRoot("LoomPreflight", "StatusPath", …)` fires first and card 7's "message contains both divergent values" assertion fails against a correct implementation.
**Fix:** Pin the check as `New`'s first act, ahead of `Parse`, in card 3, and state in card 7 that the overwritten value stays absolute-but-different.

### [NIT:consistency] Card 5 claims to consume a symbol its own file cannot reach
**Location:** batch 2 card 5
**Issue:** Card 5 says of `fakeAlwaysDoneProducer` "This card only consumes it; card 6 owns the single declaration" — but the converted `buildSequenceFixture` returns `(anchorPath, shedrecipe.Env, ShedPaths)`, neither of which carries a producer field, so nothing in the moved `fixture_test.go` references it at all. The real consumers are `shape_test.go`, `sequence_test.go`, and `resume_test.go` doing row-1 substitution.
**Fix:** Restate card 5's sentence as "this card neither declares nor consumes it; card 6 relocates the single declaration into this file for the substituting tests to use".

## Verdict

REQUEST_CHANGES
One production doc-comment scope gap the plan's own sweep provably cannot catch.
MILL_REVIEW_END
