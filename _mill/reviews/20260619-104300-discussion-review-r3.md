Confirmed the outbound-link problem. The moved research docs link to:
- `muxpoc.md` (being DELETED) at mux-proposal.md L5, L146 — these become dead links regardless of the move
- `../psmux-tui-behavior.md` at mux-exploration.md L159 — after move to `docs/research/`, `../psmux-tui-behavior.md` resolves to `docs/psmux-tui-behavior.md` which is still correct (research/ → ../ = docs/). Actually that one survives.
- `mux.md` (staying in modules/) — from `docs/research/` the link `mux.md` would break; needs `../modules/mux.md`

This is a real gap. The discussion's link-fix scope and broken-link test only cover "kept docs" and don't address that the *moved research docs* themselves contain links to a deleted doc (`muxpoc.md`) and to `mux.md` which stays behind.

```
MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [GAP] Moved research docs' own outbound links unhandled
**Section:** Scope ("Fix every internal link") + Testing (broken-link check)
**Issue:** The three `mux-*` docs move to `docs/research/` but each links to `mux.md`, which *stays* in `docs/modules/` (e.g. mux-exploration.md L3, mux-hooks-exploration.md L3/L306, mux-proposal.md L150); after the move `[..](mux.md)` resolves to a non-existent `docs/research/mux.md`. The link-fix scope is framed "across all kept docs (overview.md, roadmap.md, README.md, mux.md, etc.)" and the broken-link grep only targets links *pointing at* deleted/moved files — so a relative-path shift inside the moved files themselves is not in scope.
**Fix:** State that the moved research docs' internal links must be re-pointed (`mux.md` → `../modules/mux.md`, `../psmux-tui-behavior.md` re-verified), and add the moved files to the link-fix targets.

### [NOTE] muxpoc.md links inside moved research docs go dead
**Section:** Scope (delete muxpoc.md) / mux-proposal.md
**Issue:** mux-proposal.md L5 and L146 link `[muxpoc.md](muxpoc.md)`, which is deleted by this task; the broken-link grep catches it but the discussion gives no resolution (retarget to overview.md? drop?), unlike the explicit board-performance.md L6 retarget.
**Fix:** Note the muxpoc.md references inside moved research docs and say to retarget them (e.g. to overview.md's muxpoc entry) or drop, mirroring the benchmarks-link decision.

## Verdict

GAPS_FOUND
Moved research docs' internal links (esp. to the staying mux.md) are not in the link-fix scope.
MILL_REVIEW_END
```