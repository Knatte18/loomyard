MILL_REVIEW_BEGIN
# Review: Shed recipe: engine registry

```yaml
duration_s: 150.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-21
```

## Findings

### [NIT:consistency] SingleLLM's Env.AnchorPath requirement contradicts itself
**Demoted-from:** BLOCKING
**Section:** "Live seams travel in a told `Env`" vs. "The `SingleLLM` entry builds its `SpecSource`"
**Issue:** The `Env` struct comment annotates `AnchorPath` as used by "Batchifier, PlanValidate, Webster" only, but the reserved-token table supplies `anchor_path` from `Env.AnchorPath` for `SingleLLM`; under the "each entry validates exactly the `Env` fields it consumes" rule it is undecided whether a `SingleLLM` entry must reject an empty/relative `AnchorPath` even when its stencil never names the token.
**Fix:** State whether `SingleLLM` reads `AnchorPath` unconditionally (and therefore validates it) or only when the stencil declares the marker, and align the `Env` field annotation with whichever answer.

### [BLOCKING:design] Config's decoded Go representation is unspecified for maps and numbers
**Section:** "`Config` is a decoded `map[string]any`" (plus the `tokens` and `profile` key decisions)
**Issue:** The named accessors are only `configString`/`configStringSlice`/`configBool`/`configInt`, yet `tokens` is described as a `map[string]string` and `profile`/`profile.target` as nested maps — no accessor or decoded-type contract covers them, and `configInt` has no stated behaviour for a `float64` (which a JSON-shaped loader would produce), even though the whole point of `map[string]any` is that piece 2 picks the format.
**Fix:** Pin the decoded representation the entries accept for nested maps (`map[string]any`) and for numbers (int and float64-with-integral-value, or int only, explicitly), and name the nested-map accessor alongside the other four.

### [NIT:decision] Registry's exported lookup surface is unnamed
**Section:** "Central table literal" / Testing item 1
**Issue:** The discussion pins the map type and the error shape but never names the exported entry point (`Select`? `Lookup`?) or says whether the name set is enumerable (`Names()`), which the coverage guard and piece 2 both consume.
**Fix:** Name the exported lookup function and say whether an enumeration accessor ships with it.

### [NIT:consistency] "signatures unchanged" contradicts the changed return type
**Section:** "`loomshed`'s six constructors become exported"
**Issue:** The decision says the six keep "their current signatures and behaviour unchanged" and, two sentences later, that each declared return type becomes `shedengine.ShedProducer` — a signature change.
**Fix:** Reword to "same parameters and behaviour; return type widened to the seam interface".

## Verdict

REQUEST_CHANGES
Two contract gaps — SingleLLM's AnchorPath requirement and Config's decoded value types.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
