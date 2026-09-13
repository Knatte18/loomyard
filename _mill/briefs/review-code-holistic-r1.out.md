MILL_REVIEW_BEGIN
# Review: Rename hub container suffix from -HUB to -LYXHUB — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-13
```

## Verification summary

Verified all seven batches against source, end to end:

- **Batch 1 (core-constants):** `hubSuffix`/`HubSuffix` both swapped to `-LYXHUB`, doc-comment examples updated, `TestEnforcement_GeometryLiterals`'s token/owner map/positives-table moved together, `CONSTRAINTS.md`'s new `## Hub Suffix Invariant` section lands in the exact position (after Hub Containment, before gitkit Leaf) with the exact required substance. `TestHubPath`/`TestBoardDir` updated per the literal-vs-composed split the card specifies, including the updated doc comment explaining the deliberate mix. New `reponame_test.go` pins the trim/no-trim/no-suffix trio exactly as specified, calling `buildLocation` with `applyGate=false`.
- **Batch 2 (production-prose):** All three doc-comment sites (`CloneResult.HubPath`, `resetHub`'s R4 sentence, `hubforge.Hub.Path`) and both `fabric clone` `Long`-text sites updated; no executable change; `Use`/`Short`/`Args`/`RunE` untouched as required.
- **Batch 3 (fabricengine-tests):** All six untagged fixture files swept correctly; `clone_reset_guard_test.go`'s header substituted while its `NOT-A-HUB-USER-DATA` sentinel is byte-for-byte preserved; the three integration-tagged comment files updated; `destructivegaps_integration_test.go`'s new `AcceptsLegacySuffixedHubDirectory` subtest is correctly placed immediately after `AcceptsWeftSiblingOnly`, uses only `os.MkdirAll`/`filepath.Join`, and its `OUTSIDE-PARENT-HUB-CONTENT` sentinel survives untouched.
- **Batch 4 (peripheral-tests):** Every fixture file swept (counts verified exactly against the card's stated per-file tallies for `loomengine`); `lyxcwd` geometry/comment sites correct; `tokenvocab`/`weftname` sites correct including the tenth (comment) hit; `reedengine` socket-key sites swept with the two required re-check comments retained verbatim (sanctioned survivors, confirmed in the final sweep below); `reedcli`'s conceptual comment reworded to lowercase "per-hub" exactly as specified.
- **Batch 5 (sandbox-fixture):** `hubName` constant and `warpDirName` doc comment updated in `main.go`/`suite.go`; all seven suite docs updated including both of `SANDBOX-CORE-SUITE.md`'s and `SANDBOX-FABRIC-SUITE.md`'s second hits.
- **Batch 6 (docs-prose):** `sandbox-hub.md`'s seven hits plus the new "Disposing of a pre-rename container" subsection (correct placement, correct required substance, correct semantic-line-break style); `sandbox-howto.md`/`overview.md`/`lyxcwd.md`/`reed-fabric-standalone-api.md` each updated at their single named hit.
- **Batch 7 (final-sweep-gate):** Ran the described `grep`-equivalent census myself. Outside the mill task-state tree, exactly seven files still contain the retired token, and they are precisely the seven the card names: `internal/fabricengine/clone_reset_guard_test.go` (sentinel only), `internal/fabricengine/destructivegaps_integration_test.go` (sentinel + the legacy-suffix subtest literal), `internal/shuttleengine/claudeengine/startup_test.go` (verbatim historical capture), `docs/research/session-fork-spike.md` (dated research note), `CONSTRAINTS.md` (the invariant naming the retired token by design), `internal/lyxcwd/reponame_test.go` (the degradation case), and `internal/reedengine/server_test.go` (the two re-check comments). `internal/reedcli/smoke_teardown_test.go` correctly does NOT appear — its hit was reworded, not substituted.

No out-of-plan files, no cross-batch contract mismatches, no duplicated helpers, no constraint violations found. The Shared Decisions (clean break, four-way literal classification, no physical rename, migration-prose placement, sanctioned dual declarers) are applied consistently across every batch that touches them.

## Verdict

APPROVE
Every batch's cards are fully and correctly realised; the final-sweep census matches exactly, with no drift.
MILL_REVIEW_END
