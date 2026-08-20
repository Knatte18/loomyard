MILL_REVIEW_BEGIN
# Review: shedadapters: Burler-round producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Errored round can still leave `round-N-review.md`
**Section:** "Round resolution from disk…" / "Always `Stuck`, never `Done`"
**Issue:** The rationale rests on "a failed round writes no review file", but `burlerengine.Run` returns a hard error *after* the run reached `OutcomeDone` in two branches — cluster-audit violation (`internal/burlerengine/engine.go:176-179`) and verdict parse failure (`engine.go:188-191`) — and `OutcomeDone` means the agent already wrote every `OutputFiles` entry (`internal/shuttleengine/engine.go:14`), so `round-N-review.md` exists while `Call` returns an error; the next `Call` then advances to `N+1` and the `Bouncer` reads a broken round as completed.
**Fix:** State the disposition of the errored-but-artifact-present case (e.g. archive the round's review before returning the error, or define the discriminator as "exists AND parsed") so the two-sided protocol holds on that path.

### [BLOCKING:consistency] Hydration scan shape contradicts the `<N>` naming decision
**Section:** "Prior-round hydration"
**Issue:** It says the producer collects `round-<token>-review.md` / `round-<token>-fixer-report.md`, but the artifact decision replaced `<token>` with a strict `round-<N>-…` shape; `archiveStaleOutputs` writes dead attempts as `round-<N>-review-<stamp>.md` (`internal/shedadapters/archive.go:36-40`), so a loose scan would hydrate archived partial reviews into `PriorReviews`.
**Fix:** Restate hydration as the same exact-shape `round-<N>-` match the round scan uses, explicitly excluding stamped archive siblings.

### [BLOCKING:design] Absent `runDir` has no stated creator
**Section:** "Round resolution…" / "Seam shape and constructor validation"
**Issue:** The constructor validates only that `runDir` is non-empty and absolute, and round resolution explicitly tolerates an absent `runDir`, but nothing says whether the producer creates it before a round — the agent must write `round-1-review.md` into a directory that may not exist, and `archiveStaleOutputs` silently skips absent paths.
**Fix:** Decide and state whether `Call` creates `runDir` (and with what mode) or the constructor requires it to exist.

### [NIT:consistency] Doc-update count disagrees with itself
**Section:** Scope "In" vs Technical context
**Issue:** Scope says `docs/overview.md`'s *two* "three adapters" statements; Technical context names three sites and `docs/overview.md:235,316,318` confirms three.
**Fix:** Make Scope say three.

### [NIT:consistency] Two citations do not say what is attributed to them
**Section:** "Round resolution…" rejected alternatives; `ClusterExclude` rationale
**Issue:** "no adapter reads `Shed`'s status file (stated in `internal/shedadapters/doc.go`)" — that statement is absent from `doc.go`; and `engine.go:101` is a `logger.Info` line, not `composePrompt` (which is `engine.go:126`).
**Fix:** Drop the `doc.go` attribution (the seam argument stands alone) and re-point the `composePrompt` citation.

### [NIT:scope] Unbounded prior-round hydration growth
**Section:** "Prior-round hydration"
**Issue:** Every prior round's review and fixer report is hydrated, and the segment cap is `MaxBounces` (default ten), so a late round carries ~18 prior artifacts with no stated windowing.
**Fix:** Say explicitly that full-history hydration is intended, or name a window.

## Verdict

REQUEST_CHANGES
Artifact-existence protocol has an errored-but-written hole; hydration shape and runDir creation undefined.
MILL_REVIEW_END
