MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-class (self-assessed; exact build not introspectable)
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:design] Unreachability rule stated for one branch only
**Section:** `checkseedmissing-stays-but-is-unreachable-through-shed`
**Issue:** `run.go:77-86`'s step-1 gate reads the *same* told path with the *same* `state.ReadJSONStrict[shedengine.Status]` decoder and additionally rejects an invalid `st.State`, so row 2's decode-failure branch (`preflight.go:119-127` → `CheckSeedIncoherent`) and `checkCoherence`'s invalid-state rule (`coherence.go:57-61`) are unreachable through Shed for exactly the reason the discussion gives for `CheckSeedMissing` — yet the decision covers only `CheckSeedMissing` and the TOCTOU guard, leaving two neighbouring branches with no stated disposition.
**Fix:** State the reachability rule once, generally ("every verdict step 1 already hard-errors on is unreachable at row 2"), naming the branches it covers, and pin what `contracts/specs/loom-status-spec.md:31` ("loom's Preflight **requires the file to exist** and fails loud if it is missing") becomes — it is listed as a changing line with no stated new wording.

### [BLOCKING:decision] Row-1 integration test left as two alternatives
**Section:** Technical context, "Tests that change" → `internal/loomshed/preflight_integration_test.go`
**Issue:** The disposition is "the file either moves to `internal/preflightshed` or is rewritten against row 2", and its external-`loomshed_test` rationale (lines 3-8: an in-package `hubforge` import would close a cycle through `internal/loomengine`) is deferred as "must be re-evaluated" — while Testing item 4 already assumes the file's two tests land as `internal/preflightshed`'s Tier-2 outcome-mapping coverage.
**Fix:** Choose the move to `internal/preflightshed` explicitly, and state whether it lands in-package or as `preflightshed_test` (`preflightshed` is not inside `internal/fabriccli`'s dependency set, so the loomshed cycle rationale does not carry over unexamined).

### [NIT:consistency] "No Tier-1 route to cancelErr" rests on a wrong reason
**Section:** Testing, item 4, second cancellation case
**Issue:** The stated reason — "reaching `cancelErr` requires `preflight.Check` to have run to completion with a non-OK report, which spawns git and needs a `hubforge` hub" — is half wrong: `loomshed/preflight.go:50-60` also reaches `cancelErr` on `preflight.Check`'s error return, and a plain non-git `t.TempDir` yields a determined non-OK geometry report (`preflight/preflight.go:37-41`) with no hub at all.
**Fix:** Restate the reason as the transitive `git rev-parse` spawn inside `lyxcwd.Resolve`, which is what actually forces Tier 2.

### [NIT:scope] Fixture seed step's fate after the split not stated
**Section:** Technical context, "Tests that change"
**Issue:** `setupPreflightWrapperFixture` (`internal/loomshed/preflight_integration_test.go:31-40`) seeds a real `status.json` via `loomshed.Seed` solely because today's row 1 includes check 4; post-split row 1 never reads it, and the discussion does not say whether that step is dropped or kept.
**Fix:** Name the seeding step's disposition when the file moves.

## Verdict

REQUEST_CHANGES
Two branches lack a reachability disposition; the row-1 integration test's home is unchosen.
MILL_REVIEW_END
