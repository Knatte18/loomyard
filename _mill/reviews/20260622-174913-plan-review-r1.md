All fold-target func names verified to exist. The plan is well-grounded. Let me check one consistency concern: the muxpoc cli_test fold names. Card 16 says fold `TestRunCLINoSubcommandFails`, `TestRunCLIUnknownSubcommandFails`, `TestRunCLIUnknownFlagFails` — all three confirmed present. Good.

One minor concern: the ide menu_test `TestMenuRequiresLyxDir` is in the baseline and the keeper, while `TestMenuZeroWorktreeMessage` is the drop/fold candidate — both confirmed present.

Let me verify the muxpoc coverage floor reasoning isn't a constraint issue, and check whether the `TestParsePaneOrderSortsByTop` (in muxpoc baseline line 10) is left untouched correctly. The muxpoc scope says it's "cosmetic-only, optional" — and it stays as a baseline name not folded, which is fine for the name-map (it survives as a top-level func). No issue.

I have enough to render the verdict. The plan is thorough, well-grounded against source, the DAG is valid, card numbering is sequential and gapless, completeness fields are present, and constraint decisions are faithfully reflected. I'll note a couple of minor items.

MILL_REVIEW_BEGIN
# Review: Prune and consolidate the test suite (board first) — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-22
```

## Findings

### [NIT] Card 13 leaves drop-vs-fold choice to implementer
**Location:** Batch 4 (ide) / Card 13
**Issue:** Requirements say "Either drop `TestMenuZeroWorktreeMessage` or fold both into a `TestMenuDiscovery` table" — an open choice that propagates ambiguity into Card 17's name-map ("if it was dropped rather than folded").
**Fix:** Acceptable (counts-are-not-quotas + name-map handles both outcomes); no change required, but picking one keeps the audit diff deterministic.

### [NIT] Card 17 "before" counts hard-coded alongside derivation rule
**Location:** Batch 6 (doc) / Card 17
**Issue:** Card states both "derive before from baseline line counts" and pins the literals (board 61, worktree 22, weft 20, ide 20, muxpoc 19); the literals match the baseline files (verified), so the redundancy is harmless but could drift if a baseline is re-captured.
**Fix:** Treat the baseline files as the single source of truth; the pinned literals are correct as of now.

## Verdict

APPROVE
Plan is source-grounded, DAG-valid, sequential, complete; only cosmetic nits.
MILL_REVIEW_END