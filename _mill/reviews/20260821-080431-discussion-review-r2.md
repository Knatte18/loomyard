MILL_REVIEW_BEGIN
# Review: Shed-setup validity checker

```yaml
duration_s: 163.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Finding field mapping per kind is unspecified
**Section:** "`Finding`'s shape" + "Deterministic output order" + Testing §1
**Issue:** Tests are to assert the full slice as a literal over `Kind`/`Producer`/`Target` only, but two kinds have no defined mapping into those three fields: `done-cycle` "names its members" while `Finding` has no members field (a 3-row cycle cannot be expressed in one `Producer`+`Target` pair), and `bad-endpoints` never says which field carries the offending entry/terminal name — so the "members starting at the lowest list index" ordering rule is unpinnable by the very test claimed to machine-enforce determinism, and it would live only in the deliberately-unpinned `Message`.
**Fix:** State per kind what `Producer` and `Target` hold (including the empty cases), and either add a pinned members field for `done-cycle` or state explicitly that cycle membership lives in unpinned `Message` and drop the claim that its member ordering is machine-enforced; while there, name `Kind`'s Go type and its six exported constants.

### [NIT:consistency] "five tasks each hand-wire a perch" contradicts the roadmap
**Section:** Problem ("Why now"), and Testing §2
**Issue:** The discussion says "the five `loom: real LLM producers` tasks are each about to hand-wire a two-row `Bouncer`+`Burler` segment", then two lines later says "three of those"; `manifest/roadmap.md:32-52` shows only Discussion-Review, Plan-Review and Webster-Review wire a perch — Discussion-Write and Plan-Write are plain `SingleLLMProducer` rows.
**Fix:** Say "three of the five" in both places, so the plan writer does not scope the invariant test's comment against five perch-wiring tasks.

### [NIT:scope] Enforcement point may not survive the item sequenced before it
**Section:** Scope In (the `internal/loomshed` invariant test) and "Not wired into `shedengine.Run`"
**Issue:** The sole enforcement point is a test over `loomshed.New()`'s Go literal with `NamePreflight`/`NameFinalize`, but `manifest/roadmap.md:25,30` sequences "loom: convert to a Shed recipe" *before* the five producer tasks this guard exists for, replacing that literal with a recipe file; the discussion names the recipe loader as the next consumer but never says the guard migrates with it.
**Fix:** Add one sentence stating that the invariant test moves onto the recipe-assembled list when the conversion item lands, so the guard is not silently dropped at conversion time.

## Verdict

REQUEST_CHANGES
Graph model and coverage claims verify against source; finding-field mapping is under-specified.
MILL_REVIEW_END
