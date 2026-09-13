MILL_REVIEW_BEGIN
# Review: Rename hub container suffix from -HUB to -LYXHUB

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] Stale grep counts and final-sweep expectation
**Section:** Technical context → File inventory; Testing → Sentinel integrity
**Issue:** The doc says `grep -rl -- '-HUB'` returns 54 files with 3 under `_mill/`; it now returns 60 with 9 (`status.md`, `discussion.md`, `reviews/*`, `briefs/*`), and the closing-grep expectation names only `_mill/status.md` as a permitted mill hit.
**Fix:** State the exclusion as "everything under `_mill/`" rather than a pinned count and a single filename — the 51 product files and their 7/31/13 split verify correctly as written.

### [NIT:consistency] Class (b) label contradicts the per-HUB rewrite
**Section:** Decisions → Test-literal classification, class (b); File inventory
**Issue:** Class (b) is defined as "coincidental substring → leave untouched" and the inventory heading says "do NOT change the literal", yet `internal/reedcli/smoke_teardown_test.go:270` is listed there with an explicit instruction to rewrite `per-HUB` → `per-hub` (verified: it is comment prose about the tmux socket being per-hub).
**Fix:** Give the prose-rewrite case its own class label so the plan writer is not choosing between the class rule and the per-entry instruction.

### [NIT:decision] Measured-snapshot doc treated unlike class (c)
**Section:** Scope → In; Decisions → Test-literal classification, class (c)
**Issue:** `manifest/designs/reed-fabric-standalone-api.md:320` is a point-in-time measured public-surface list (verified: the section is framed as "measured", alongside `wc -l` counts and import censuses), scheduled for substitution, while `docs/research/session-fork-spike.md` is left as a historical record — the discussion never says why the two snapshots are treated differently.
**Fix:** State the distinguishing rule in one line (e.g. a live API-surface census stays current; a dated run log does not).

## Verdict

APPROVE
Decisions are complete and every source claim I checked holds; only cosmetic inconsistencies remain.
MILL_REVIEW_END
