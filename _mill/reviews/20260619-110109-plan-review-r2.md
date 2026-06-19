This confirms the moved research docs have rich cross-references that Card 5 must fix:
- `mux-proposal.md`: links to `mux.md` (L3, L6, L136, L150 — become `../modules/mux.md`), `muxpoc.md` (L5, L146 — deleted, retarget to `../overview.md#modules`), `mux-hooks-exploration.md` (L4, L128, L148 — sibling in `research/`, stays bare).
- `mux-hooks-exploration.md`: links to `mux.md` (L3, L306), `mux-exploration.md` (L6, L94 — sibling).

Card 5 covers all of these: it specifies `mux.md` → `../modules/mux.md`, `muxpoc.md` → `../overview.md#modules`, and sibling-research links stay relative (correct since both are in `research/`). The plan's Card 5 is complete.

I have verified the plan thoroughly. The plan is accurate against all sources: line references (overview L64 PortalLink omission, L188/L205 "(planned)", paths.md, config.md L19-20/L14/L10, README L18-24, roadmap milestones 3/4/8, benchmarks L6, mux.md L21-23), the Go file-header structure for all four packages, decision alignment, DAG integrity, sequential card numbering, and complete Context/Edits/Requirements/Commit per card.

One minor observation: paths.md already carries portal-deprecation tags and `PortalLink` (Card 9 requirement appears partly satisfied already), but the card phrasing is "keep ... tag them" which reads as ensure/verify — not a contradiction. CONSTRAINTS.md L19 already lists `PortalLink` (Card 3 only needs to add deprecation tags, which it does). These are consistent with the plan; not findings.

No blocking or material issues found.

MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-19
```

## Findings

### [NIT] paths.md portal tags / PortalLink may already be present
**Location:** Batch 2 / Card 9; cross-ref Batch 1 / Card 3
**Issue:** `docs/shared-libs/paths.md` already carries the three "Deprecated … task 006" portal tags and documents `PortalLink` (L70-76), and CONSTRAINTS.md L19 already lists `PortalLink`; Card 9 / Card 3 read as ensure-consistency rather than net-new edits, so those commits may be near-empty.
**Fix:** No change needed — phrasing ("keep … tag", "add deprecation tags") already tolerates pre-satisfied state; just flag so implementer does not force a no-op rewrite.

## Verdict

APPROVE
Plan is accurate against sources, DAG-clean, fully specified, decisions faithfully implemented.
MILL_REVIEW_END