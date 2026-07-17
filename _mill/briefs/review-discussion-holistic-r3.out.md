MILL_REVIEW_BEGIN
# Review: loom: pin the spawn/handover status schema + discussion-format.md

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [NOTE] loom.md sibling-link enumeration is incomplete
**Section:** Scope (Relative markdown links) / Decisions: loom-md-reconciliation
**Issue:** Scope names only `(builder-contract.md)` at loom.md L117, but same-folder sibling links to the relocating files also live at L36 and L111 (`plan-format.md`) and L235 (both `plan-format.md` and `builder-contract.md`) — all resolve to `docs/modules/…` and must become `../reference/…`.
**Fix:** List L36/L111/L235 alongside L117 (Testing's `(plan-format.md)`/`(builder-contract.md)` grep already covers the class, so this is a wart, not a hole).

### [NOTE] Timestamp format unpinned under strict parse
**Section:** Decisions: status-field-set / doc-rigor-moderate
**Issue:** `ts` in `history[]` and the doc's fail-loud `KnownFields(true)` parse discipline coexist, but the schema shows `"ts": "…"` without pinning a format, so "malformed" is undefined for that field.
**Fix:** State the timestamp encoding (e.g. RFC3339 UTC) in status-schema.md's field notes / check list.

## Verdict

APPROVE
Contracts, boundaries, and decisions are pinned; only two non-blocking notes remain.
MILL_REVIEW_END
