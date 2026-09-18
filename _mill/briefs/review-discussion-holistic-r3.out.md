MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 4.x-class model (self-assessment; brief states "opusmedium")
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Grep set misses bare-backtick verb mentions
**Section:** §Scope (the grep set) **Issue:** Scope declares the grep set authoritative ("a file is in scope because it contains a hit"), but its four patterns match none of the many prose sites that name a verb as bare `` `drive` ``/`` `run` `` without the word "loom" and without quotes — verified at `docs/overview.md:244,331`, `manifest/roadmap.md:154`, `manifest/designs/loom.md:440`, `manifest/designs/loom-step.md:48,49`, `manifest/designs/self-report-tier2.md:42`, `internal/loomcli/drive.go:1`. **Fix:** Add backticked bare-verb and "<verb> verb" patterns to the grep set (or state that the whole-tree pass is `drive`/`run` word-boundary with a read-and-classify step), so the rule matches what it claims to.

### [BLOCKING:decision] Driver-spawn argv and /proc argv probe have no disposition
**Section:** §Technical context (`internal/loomcli` structure) **Issue:** The behaviourally load-bearing string is `exec.Command(exe, "loom", "drive")` at `internal/loomcli/run.go:137` — the detached driver the bootstrap spawns — and the discussion never names it; `internal/loomcli/smoke_test.go:246,272` finds that process by scanning argv for `"drive"`, and lines 464–1023 invoke `runLoomCLINoFatal(exe, …, "loom", "run")` in ~12 places. Split-argv forms match none of the grep patterns, and because `run` is *reused*, a missed `"loom","run"` smoke call silently invokes the new foreground driver instead of erroring as an unknown verb. **Fix:** State the disposition explicitly — after the rename `start` spawns `"loom","run"`, the argv probe matches `"run"`, and every split-argv `"loom","<verb>"` site is a scope target.

### [NIT:consistency] Meaning-inverting prose sites named incompletely
**Section:** §Decisions → historical-prose-rewritten-not-glossed **Issue:** The discussion flags `drive.go`'s `Long` as the high-risk "a blind replace makes it a self-reference" site, but `manifest/designs/self-report-tier1.md:13-20` has the same shape — its exemption turns on `lyx loom drive` vs `lyx loom run` being different verbs, so a token swap inverts the stated rule. **Fix:** Name it alongside `drive.go`'s `Long` as prose requiring rewriting, not substitution.

## Verdict

REQUEST_CHANGES
Enumeration rule under-matches prose verbs; the driver-spawn argv has no stated disposition.
MILL_REVIEW_END
