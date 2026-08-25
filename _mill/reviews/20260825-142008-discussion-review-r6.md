MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
duration_s: 176.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] LoadState-as-gate blocks the ordinary first call
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`
**Issue:** `LoadState` is declared "read **first**, as a gate" and an absent/unreadable `reed.json` is "error, never `found == false`, at any dir age" — but the same list says "no matching `run.json` at all → `found == false`, nil error", and Testing asserts "No run dirs at all, and a root that does not exist: `found == false`, nil error"; with `Attach` probed unconditionally on *every* `SingleLLMProducer.Call`, a worktree whose `.lyx/reed.json` does not yet exist (verified: `reedengine.LoadState` returns `(nil, nil)` for absent) would fail the very first `Discussion-Write`/`Plan-Write` call, with nothing to attach to.
**Fix:** State the precedence explicitly — e.g. the reed-state gate applies only once a candidate `run.json` has matched, so a scan finding no candidate returns `found == false` regardless of reed state — and make the Testing bullet name that precondition.

### [BLOCKING:design] Tracked-with-dead-pane leftover has no disposition
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`, `leftover-run-dir-from-a-completed-run`
**Issue:** The enumeration claims to cover reed's answers, but `checkLivenessTick` (`wait.go:297-317`) distinguishes a **fifth** state the decision never names: strand tracked, `Live == false` (the `OutcomeDied` case). Since `finalize` cleans up only on `OutcomeDone`, a died or timed-out run leaves exactly that record behind, and `leftover-run-dir-from-a-completed-run` scopes its output-files tie-breaker to "untracked, or binding-cleared" only — so this common leftover falls through an enumeration presented as exhaustive.
**Fix:** Give tracked-but-not-live its own line (respawn-eligible `found == false`, or the same age/output-files rule), and drop the "three answers … a fourth" framing.

### [BLOCKING:design] Attach to a live-but-idle agent replaces progress with a full timeout
**Section:** `attach-is-unconditional-not-interactive-only`, `attach-restarts-the-deadline`
**Issue:** In autonomous mode an `OutcomeAsking` (and an `OutcomeTimeout`) finalizes without cleanup while its pane stays live and tracked; neither `Discussion-Write` nor `Plan-Write` carries an `on_stuck` in `contracts/recipes/loom-recipe.yaml`, so the run blocks and the operator's `lyx loom run` resume re-enters the same row — where `Attach` now takes the attachable branch and waits on an idle agent nothing in loom ever sends input to, burning a freshly restarted full `run_timeout_min` where today it respawns and makes progress.
**Fix:** State the disposition for a matched record that is live but whose previous run already reached a terminal non-`Done` outcome (e.g. respawn, or attach only when no terminal classification is recorded), rather than leaving it inside the unqualified "tracked, with a live pane → attach" rule.

### [NIT:consistency] "Attach mirrors the sweep's refusal" is not what the sweep does
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`
**Issue:** `sweepOrphansOpportunistic` (`run.go:260-267`) treats absent/unreadable reed state as a `logger.Warn` and **proceeds with the new run**; `Attach` is specified to hard-error and block the run, which is a strictly stronger response than the cited precedent.
**Fix:** Reword the rationale to own the asymmetry instead of claiming the sweep as precedent for erroring.

### [NIT:scope] `mergeresolve.Shuttle` never dispositioned
**Section:** Scope "Out", `attach-is-unconditional-not-interactive-only`
**Issue:** `internal/mergeresolve/deps.go:39-46` defines a second one-method `Shuttle` seam over the same `*shuttleengine.Runner` for the landing rows; the Out list disposes of `Bouncer`/`Burler` but never mentions it, so a plan writer cannot tell whether it is deliberately left with the respawn-over-live-agent path.
**Fix:** Name it in Out beside the `Bouncer`/`Burler` carve-out.

## Verdict

REQUEST_CHANGES
Attach's reed-answer enumeration has three unresolved states that block or stall resume.
MILL_REVIEW_END
