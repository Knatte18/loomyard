MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies

```yaml
duration_s: 226.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] ErrShedBusy does not close the false-crash race
**Section:** Technical context → "Concurrency", and `### detection-site`
**Issue:** The claim "Detect at entry, file after `Run` returns cleanly" is asserted as closing the two-driver race, but it does not: if a healthy driver holds the lock when drive #2 takes its entry observation (`state: running`, non-empty history) and then terminates normally before drive #2 reaches `lock.TryAcquireWriteLock` (`internal/shedengine/run.go:59-65`) — a window that spans `reed.Up()`, `fabricengine.Open`, branch/origin resolution and `loomrecipe.New` in `drive.go:73-127` — drive #2 gets no `ErrShedBusy`, `Run` short-circuits on `StateDone` (`run.go:88`), and a crash-resume issue is filed for a run that never crashed.
**Fix:** Either add a second condition that discriminates it (the discussion must name it) or record it explicitly as a third accepted imprecision alongside the two in `### trigger-list-and-thresholds`, and drop the "gating on `ErrShedBusy` is what closes it" wording from both the Concurrency paragraph and the "Discovered during discussion" list.

### [NIT:decision] New ledger accessor's round-mismatch disposition unstated
**Section:** `### ledger-access`, and Testing → `internal/shedadapters`
**Issue:** The existing in-package ledger reader fails closed when a ledger's frontmatter `round` disagrees with the round its filename encodes (`internal/shedadapters/bouncerfiles.go:98-107`), while `parseLedger` itself does not check it (`bouncerfiles.go:147-189`); the discussion never says whether the new exported accessor keeps that check, which matters because "keep the highest `round`" in `### filing-pass-order` keys on the frontmatter value.
**Fix:** State the disposition — accessor inherits the filename/round agreement check, or deliberately does not and the collapse trusts frontmatter `round` as-is.

## Verdict

REQUEST_CHANGES
One stated concurrency closure is false; the residual false-positive needs a disposition.
MILL_REVIEW_END
