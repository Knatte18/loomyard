MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] "reed state file absent" is not observable
**Section:** `mechanism-failures-do-not-attach-and-do-not-blindly-respawn`; Testing → `Attach` **Issue:** the decision gives "reed's state file absent or unreadable → error, never `found == false`" a distinct disposition, but `Attach`'s only seam is `ReedOps.Status()`, and `reedengine`'s `loadOrInitStateLocked` substitutes `&ReedState{}` for an absent state file — Status then succeeds with zero strands, indistinguishable from `errStrandNotTracked`, so the absent case silently takes the age-resolved respawn path (the corrupt/unreadable case does error, via `LoadState`). **Fix:** state that absent-vs-untracked cannot be separated through `Status()` and either drop the absent-file disposition (and its test bullet "reed's state file absent … error … at any dir age", which cannot pass) or name the new reed surface that would expose it.

### [BLOCKING:decision] Attached Run's Spec and the third normalization
**Section:** `attach-reconstructs-the-run-explicitly`, `attach-normalizes-the-spec-it-matches-on` **Issue:** the first says every non-persisted field "is decided here rather than left to whatever the zero value happens to be" but enumerates only `offset`/`deadline`/`clock`/`started`, never saying which `Spec` the reconstructed `Run` carries; the second says `Spec.validate` performs exactly two normalizations, while `spec.go:157-159` has a third — the `Display.Anchor` default — which `checkLivenessTick` reads in the `errStrandPaneBindingCleared` carve-out (`run.spec.Display.Anchor != render.AnchorHidden`), alongside `run.spec.ForkSubagents`/`KeepPane`, also unaddressed. **Fix:** say the attached `Run` carries the caller's normalized spec and give the `Display.Anchor` default (and `ForkSubagents`/`KeepPane` on the attach path) an explicit disposition.

### [NIT:design] Done-cleanup guarantee is weaker than the bounce claim
**Section:** `resume-discriminates-on-live-agent-evidence-only` **Issue:** "reached `OutcomeDone`, so `Run.finalize` removed its strand and deleted its run dir" holds only when `!KeepPane` and both best-effort steps succeed — `finalize` merely `logger.Warn`s a failed `RemoveStrand`/`RemoveAll`; a surviving Done run dir with a removed strand is then untracked and younger than `2 * StartupTimeoutS` on a fast bounce, so the age rule errors instead of respawning. **Fix:** qualify the claim and state the disposition for a matched record whose output files all already exist (also the `KeepPane` case, reachable via `lyx shuttle run --keep-pane`).

### [NIT:decision] New strict-side config key needs a stated migration
**Section:** Technical context → Config; Constraints → Config Strictness **Issue:** `configengine.load` hard-errors on missing keys, so every already-initialized worktree's `loom.yaml` fails `lyx loom run` with "missing keys: discussion_interactive; run \"lyx config reconcile\"" until reconciled, and nothing auto-reconciles outside the explicit verb. **Fix:** record the `lyx config reconcile --apply` obligation for existing worktrees.

### [NIT:design] Widening the shared Shuttle seam is undiscussed
**Section:** Scope; `attach-lives-in-shuttleengine` **Issue:** `shedadapters.Shuttle` is also consumed by the Bouncer/judge (`shedrecipe/entries_bouncer.go`) and landing rows via `shedrecipe.Env.Shuttle`; adding `Attach` puts a method on the seam only `SingleLLMProducer` calls, and a narrower optional interface is never weighed. **Fix:** add one line of rationale for widening the shared seam rather than type-asserting an optional `Attacher`.

## Verdict

REQUEST_CHANGES
Two blockers: an unobservable reed disposition, and an incomplete attached-Spec decision.
MILL_REVIEW_END
