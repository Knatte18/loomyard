MILL_REVIEW_BEGIN
# Review: Producer gates: mechanical gates before session release

```yaml
duration_s: 167.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-20
```

## Findings

### [BLOCKING:design] Burler gate failure has no reporting shape
**Section:** "The four sites…" / Testing → `internal/burlerengine`
**Issue:** `burlerengine.Result` (engine.go:56-80) has no gate field and `Run`'s contract is "nil error for every non-done outcome, errors for hard failures"; the discussion says only that a gate-failed round "reports back without parsing the review file", never how — new `Result` field, an error, or a synthesised Outcome — nor how `shedadapters.BurlerProducer.Call` maps it (Stuck? which bounce budget? attempt-2 retry?), while the SingleLLM mapping is spelled out precisely.
**Fix:** Decide the `burlerengine.Result` carrier for a failed gate and `BurlerProducer.Call`'s outcome/budget mapping for it, as explicitly as the SingleLLM `Stuck`-with-empty-pointer decision.

### [BLOCKING:design] Resumed Burler round bypasses `RunOpts.Gate`
**Section:** "The four sites…" / Scope "Gated entry points … so a resumed run is gated exactly as a fresh one is"
**Issue:** `BurlerProducer` holds its own live-round probe `attach Shuttle` (burler.go:65, doc 83-91) and calls `Attach` directly on the shuttle seam, bypassing `burlerengine.Engine.Run` entirely — so a resumed Discussion/Plan fix round never sees `RunOpts.Gate` and completes ungated, which is the exact hole the task exists to close.
**Fix:** State who supplies the `GateSpec` on the burler attach path (a second carrier into `NewBurlerProducer`, or routing the probe through burlerengine) and reconcile it with the "one value, `GateSpec`, at every hop" claim.

### [NIT:scope] Stale-mention enumeration is hand-listed and incomplete
**Demoted-from:** BLOCKING
**Section:** "Row removal and resume" → **Careful**; **Docs**
**Issue:** The method is a hand-written list of ~8 sites; a grep for the three row names hits 55 files, including deployed normative stencils that are actively misleading in exactly the way the discussion flags (`contracts/stencils/loom/loom-rubric-plan-review.md:17,30-31,52`, `loom-rubric-discussion-review.md:12`, `loom-rubric-webster-review.md:38`), `docs/overview.md:338-339` (which the Docs section calls "likely untouched"), plus `contracts/specs/loom-plan-spec.md`, `README.md`, `manifest/designs/shed.md`/`shed-recipe.md`.
**Fix:** Replace the hand list with a stated mechanical sweep (grep the three row names + two engine names repo-wide, triage every hit), and note that editing a shipped stencil interacts with `stencilstore`'s no-overwrite-on-hash-mismatch rule.

### [NIT:consistency] `Attempts` on a deadline-expiry Done is specified two ways
**Demoted-from:** BLOCKING
**Section:** "A Done with no live session still runs the gate" vs Testing bullet "The run deadline expires between attempts"
**Issue:** The decision says a Done via `classifyDeadlineExpiry` returns `GateOutcome{Passed:false, Attempts: 0}`, but the test bullet describes a deadline expiring *after* attempts were spent and only requires the result to "report honestly" — 0 and N are both writable from the artefact.
**Fix:** State the rule once: `Attempts` counts re-prompts actually sent for this run, so a deadline-expiry Done after N sends reports N, and 0 only when none was sent.

### [NIT:consistency] Wrong constructors named for the two writer rows
**Section:** "The four sites and how each gets its gate"
**Issue:** `Discussion-Write`/`Plan-Write` resolve to `discussionWriteEntry` (`internal/shedrecipe/entries_discussionwrite.go:26`) and `planWriteEntry` (`entries_planwrite.go:33`), not the "`singleLLMEntry`-family in `internal/shedrecipe/entries_simple.go`"; neither row carries a `config:` block today.
**Fix:** Name the two real constructors and their files, and note that both rows gain a `config:` block for the first time.

### [NIT:scope] Widening `shedadapters.Shuttle` reaches Bouncer and the burler probe
**Section:** Scope **Out** ("The `Bouncer` rows … untouched")
**Issue:** `Shuttle` (singlellm.go:38-41) is shared by `SingleLLMProducer`, `BurlerProducer`'s probe and `Bouncer` (bouncer.go:338, 364); widening `Run`/`Attach` forces edits in all three plus every fake. Separately, the burler row's `configRejectUnknown` allowlist (entries_burler.go:189) must gain the new key.
**Fix:** Say whether the gate rides a widened `Shuttle` or an additional gated method, and record the Bouncer/fake churn as in-scope mechanical work.

## Verdict

REQUEST_CHANGES
Burler gate reporting and resume path undecided; stale-mention sweep incomplete; attempts count contradictory.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
