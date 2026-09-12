MILL_REVIEW_BEGIN
# Review: lyx loom step + external supervisor skill

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] Kind-key premise is false — `output.ErrFields` exists
**Section:** `crash-cleanup-is-one-retry-then-hand-back`, "Scope of an error envelope"
**Issue:** The decision rests on "`internal/output`'s `Err` emits a bare message with no machine-readable kind key … widening it for one skill's convenience is a far larger change" — but `internal/output/output.go:37` already exports `ErrFields(w, msg, fields)` for exactly per-verb keys, and it is already used by `internal/loomcli/validate.go:58`, `internal/webstercli/beginbatch.go:128` (`plan_drifted`), `internal/shuttlecli/run.go:146` and `internal/fabriccli/envelope.go:68`. Adding a `kind`/refusal key to `step`'s own refusal envelopes is a one-verb change with zero repo-wide surface.
**Fix:** Re-decide with the true premise: either have `step` emit a discriminating key via `output.ErrFields` (and let the skill branch mechanically on refusal vs producer fault), or keep the blind retry and state the real reason it is preferred.

### [BLOCKING:design] "Every row attaches" is false for the Webster row
**Section:** `interrupted-step-re-invokes-on-loom-s-own-crash-resume-with-a-consecutive-cap`
**Issue:** The decision claims "the `Webster` row reaches the same property through `websterengine`'s entry-time reclaim" and "there is no orphan to warn about — the next step attaches to the agent". `internal/shedadapters/doc.go:161-162` says the opposite: "`WebsterProducer` inherits websterengine's own entry-time reclaim, **which stops a leftover Master rather than attaching to it**", implemented by `reclaimEntryTimeStrands` (`internal/websterengine/runlevel.go:277`, proven by `TestRun_EntryTimeReclaimStopsLiveMasterAndRecoveryStrandsButNotAbsent`). So re-invoking an interrupted step on the Webster row kills the in-flight Master and restarts it from `state.json` — not the attach-and-wait the rule assumes, and on the single most expensive row in the list.
**Fix:** Split the rule's Webster branch out explicitly: state that re-invocation on `current_producer == Webster` restarts rather than attaches, what that costs, and whether the skill re-invokes at all there or hands back.

### [NIT:consistency] Incomplete `on_stuck` enumeration in Technical context
**Section:** Technical context, `contracts/recipes/loom-recipe.yaml`
**Issue:** It states "`Batchifier`, `Publish`, `Finalize`, `Discussion-Write`, and `Plan-Write` carry no `on_stuck` at all"; the recipe's own header records **eight** such rows, adding `Preflight`, `Loom-Preflight`, and `Webster`. The cap arithmetic is unaffected (none of them bounce), but the paragraph is offered to the plan writer as the authority.
**Fix:** Name all eight, or drop the enumeration and point at the recipe header.

### [NIT:design] The bootstrap commit is outside every lock in `step`
**Section:** Technical context, "The extracted helpers are lock-agnostic"; `run-lock-per-step-not-held-across-steps`
**Issue:** `internal/loomcli/run.go:143` takes `LoomBootstrapLock` only at step 4, after `Seed`/`VerifySeedOwnership`/`CommitAnchoredPaths`. `step` inherits that, and its run lock is taken later still, inside `Step` — so two concurrent `lyx loom step` invocations both run `CommitAnchoredPaths` unserialised before either reaches the run lock. Inherited from `run` today, not introduced here, but the discussion presents the run lock as the mutual-exclusion story without saying what covers the commit.
**Fix:** State it as an inherited, accepted property so the plan writer does not read the run lock as covering bootstrap.

## Verdict

REQUEST_CHANGES
Two decisions rest on premises the source contradicts; both need redeciding before planning.
MILL_REVIEW_END
