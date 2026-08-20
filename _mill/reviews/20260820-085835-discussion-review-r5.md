MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own

```yaml
duration_s: 174.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [NIT:scope] loomengine package doc undisposed
**Demoted-from:** BLOCKING
**Section:** Technical context → "Files that change" **Issue:** `internal/loomengine`'s *package* doc lives in `status.go:5-12` ("Package loomengine implements loom's **Preflight** precondition validator: the four checks (worktree geometry, worktree cleanliness, fabric readiness/sync, and status.json coherence)… Callers MUST NOT invoke `Preflight`… `Preflight` is a stateless validator") — it names the deleted symbol and claims tier-1/2 checks this task moves out, and `status.go` appears nowhere in the change list. The cause is the enumeration method: the "closed grep" was run on the *qualified* spelling `loomengine.Preflight`, which is structurally blind to in-package references, so `coherence.go:1-5`/`report.go:1-7` were caught by hand-reading while the package doc was not. **Fix:** Add `internal/loomengine/status.go:5-12` to the change list with its new wording, and state that the closed sweep must also cover the unqualified in-package spelling (`\bPreflight\b` inside `internal/loomengine`) rather than only `loomengine.Preflight`.

### [NIT:consistency] "coherent seed" fixture for the row-2 test
**Section:** Testing → TDD candidate 5 **Issue:** Item 5 says the row-2 producer test uses "a `t.TempDir` status file: coherent seed → `Done`", but row 2 tells `NameLoomPreflight` as the expected `current_producer`, so a `loomshed.Seed`-shaped fresh seed (`current_producer: "Preflight"`, `seed.go:57`) yields `Stuck`, not `Done`; the correct shape is only implied by `coherence-rules-after-the-split`. **Fix:** State in item 5 that the fixture file is hand-written as the post-row-1 shape (`current_producer: "Loom-Preflight"`, `history: [{"producer":"Preflight","outcome":"done"}]`), not produced by `loomshed.Seed`.

## Verdict

APPROVE
Package doc naming the deleted `Preflight` is undisposed; grep sweep missed in-package spellings.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
