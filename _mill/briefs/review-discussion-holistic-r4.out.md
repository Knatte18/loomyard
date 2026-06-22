MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-20
```

## Findings

### [GAP] portals_test.go missing from build-tag classification
**Section:** Decisions → build-tag gating (explicit classification)
**Issue:** `internal/worktree/portals_test.go` calls `newTestRepo` (git spawns) and `createPortal` (creates junctions) — verified at portals_test.go:22/48 — but it appears in neither the "Tagged integration" nor "Untagged" list, leaving its classification undecided for the plan writer.
**Fix:** Add `portals_test.go` to the `//go:build integration` (spawning) list, consistent with the stated "spawns a git/cmd subprocess" criterion.

### [NOTE] cli.go:66 conflated with cwd-path Push call sites
**Section:** Decisions → parallelism via layered env→param
**Issue:** The discussion lists weft `cli.go` line 66 among the call sites that "gain a NEW env→option read," but line 66 is the detached `--weft-path push` child branch — that path's skip semantics are already governed by the spawn-time env check, so adding another read there risks double-handling.
**Fix:** Clarify that only the cwd-resolved Commit/Push/Pull calls (lines 106/113/117/123/129) gain the new env→option read; the detached child at line 66 either keeps reading env or is documented as the spawn-layer's responsibility.

## Verdict

GAPS_FOUND
One spawning test file (portals_test.go) is unclassified in the build-tag decision.
MILL_REVIEW_END
