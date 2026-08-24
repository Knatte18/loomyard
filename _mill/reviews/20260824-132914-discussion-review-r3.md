MILL_REVIEW_BEGIN
# Review: loom: Discussion-Write producer

```yaml
duration_s: 262.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-24
```

## Findings

### [NIT:decision] Five link retargets left to the plan writer
**Demoted-from:** BLOCKING
**Section:** Scope / Decisions → `delete-format-doc-per-its-own-lifecycle` / Technical context → "The five inbound links"
**Issue:** The discussion enumerates the five links but states no per-link disposition, deferring with "that call is this task's"; two of them are not retargetable to the stencil at all — `manifest/roadmap.md:14` and `manifest/designs/plan-card-format.md:3` both read "the discussion stencil's own scoped supersession claim now lives in [loom-format-discussion.md]", a sentence that becomes false once Fix 1 is folded in and the supersession no longer exists, so pointing it at the stencil ships a wrong statement rather than a working link.
**Fix:** State the disposition per link — delete-the-sentence vs retarget-to-stencil vs historical non-link — for all five, rather than deferring to the plan writer.

### [NIT:consistency] Stale-doc inventory misses the registry's own comment
**Section:** Scope (stale-doc list) / Testing → `internal/shedrecipe/registry_test.go`
**Issue:** `internal/shedrecipe/registry.go`'s literal doc comment says "The table is complete at twelve keys. Any thirteenth entry must arrive with a coverage-guard update in the same commit" and is not in the otherwise-exhaustive correction list (`loom.md`, `discussion.go` header, `stub.go`, `CONSTRAINTS.md`, `registry_test.go`); `manifest/designs/shed-recipe.md:12`'s premise that `SingleLLMProducer` differs across `Discussion-Write`/`Plan-Write` "only in ... `Config`" is likewise superseded by `spec-closure-in-env` and unmentioned.
**Fix:** Add `registry.go`'s count comment to the same-commit correction list and state whether `shed-recipe.md` is corrected or deliberately left as historical rationale.

### [NIT:design] Commit failure discards a completed agent run
**Section:** Decisions → `commit-produced-artifacts`
**Issue:** A commit error is returned as an error, which `internal/shedengine/run.go:175` persists as `StateFailed` and halts the run; the operator's resume re-calls `Discussion-Write`, and `SingleLLMProducer.Call` archives both freshly-written files and respawns a fresh agent — so a transient weft-git fault costs an entire completed discussion run (content survives only as archive siblings). The decision states the error-not-`Stuck` choice but not this consequence.
**Fix:** Record the resume consequence explicitly, or state why the wasted respawn is accepted.

## Verdict

APPROVE
Link-retarget dispositions undecided; two retargets would ship a false sentence.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
