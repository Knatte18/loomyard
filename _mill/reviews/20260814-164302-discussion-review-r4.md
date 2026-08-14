MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board

```yaml
duration_s: 245.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic), per this session's own model identification
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Deleting the only CLI refusal-object coverage
**Section:** Technical context → "The `_board` junction surface" (`cli_test.go:887-896`) and Testing → `internal/fabriccli`
**Issue:** `TestRunCLI_Unwire_RefusesDriftedBoardJunctionWithRefusalObject` (`internal/fabriccli/cli_test.go:884-939`) is one test, not "two ownership cases", and it is the repo's ONLY positive assertion that `internal/fabriccli/envelope.go:58-63`'s `"refusal"` object reaches an envelope with all four keys plus `mutations` on the failure path — `envelope_test.go:153` only asserts its absence, and no other file mentions `"refusal"`.
**Fix:** State a disposition that preserves that contract — e.g. re-home the drifted-junction refusal case onto a surviving junction (`_lyx`/`.lyx`) — rather than deleting it with `unwireBoardLink`.

### [NIT:consistency] `envelopecontract_integration_test.go` carries no envelope key
**Demoted-from:** BLOCKING
**Section:** Technical context → Tests; Testing → `internal/fabriccli`
**Issue:** Both places say `internal/fabriccli/envelopecontract_integration_test.go` holds the `board_junction_removed` key and must drop it; that file covers reconcile/prune array-and-failure properties only and contains no `unwire` or `board_junction` reference at all (verified by grep).
**Fix:** Drop that file from the inventory, or replace it with the real remaining site if one is found.

### [BLOCKING:design] Prose inventory declared complete but misses live sites
**Section:** "Prose and scenario surfaces naming the old paths" ("This list is the enumerated inventory")
**Issue:** The enumeration missed at least: `README.md:61` (hub-tree diagram showing hub-level `.lyx/`), `internal/fabriccli/fabric.go:426` (production comment: "the `_board` link fabric wires at every anchor"), `internal/reedengine/serverlog.go:3`, `internal/reedengine/lifecycle.go:747,772`, and `internal/reedcli/smoke_debuglog_test.go:5` — all naming hub-level `.lyx`/the junction, and README/`docs` staleness is a Documentation Lifecycle obligation.
**Fix:** Name the enumeration method (a repo-wide grep for the two tokens outside `AnchorPath()` context, not per-directory spot checks) and fold these sites into the list.

## Verdict

REQUEST_CHANGES
Two inventory claims are false and one deleted test is the sole refusal-envelope coverage.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
