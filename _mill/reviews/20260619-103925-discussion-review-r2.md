Interesting: CONSTRAINTS.md L19 enumerates the full `Layout` method list including `PortalLink(slug)`, `PortalTarget(slug)` — all three portal methods. The discussion says overview.md L64 omits `PortalLink`/`PortalTarget` and should align with paths.md. But note: overview L64 actually *includes* `PortalTarget(slug)` and omits only `PortalLink`. More importantly, the discussion never mentions that CONSTRAINTS.md itself lists these portal methods — and the discussion adds "a one-line pointer to the doc-lifecycle convention from CONSTRAINTS.md" without flagging that CONSTRAINTS already documents the deprecated portal surface. That's a minor consistency note, not a gap.

I have enough to assess. Let me verify the L64 portal omission claim precisely — discussion says it omits both `PortalLink` and `PortalTarget`, but L64 shows `PortalTarget(slug)` is present. This is a factual inaccuracy in the discussion's sweep instruction worth noting.

I now have all I need to render the review.

MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [GAP] benchmarks doc keeps a broken link to deleted board.md
**Section:** Scope (Out: "Untouched files") + Testing (broken-link check)
**Issue:** `docs/benchmarks/board-performance.md` L6 links `[board.md](../modules/board.md#background-sync)`, but board.md is deleted; benchmarks is declared untouched and the link sweep is scoped only to "kept docs + CONSTRAINTS + README", so this link is left dangling.
**Fix:** Either include benchmarks in the broken-link fix scope (the L6 link is the only fix needed there, not a content edit) or state explicitly the dangling link is accepted.

### [GAP] L64 portal-method sweep instruction is factually off
**Section:** Technical context (overview.md, review-r1 item ii)
**Issue:** The sweep says overview.md L64 "omits `PortalLink`/`PortalTarget`," but L64 actually lists `PortalTarget(slug)` and omits only `PortalLink`; following the instruction literally could mis-edit.
**Fix:** Correct the instruction to "add `PortalLink`, tag the portal methods deprecated" so the plan writer edits the right token.

### [NOTE] CONSTRAINTS.md already enumerates deprecated portal methods
**Section:** Decisions (doc-lifecycle-convention) / weft-framing
**Issue:** CONSTRAINTS.md L19's `Layout` method list includes `PortalsDir`/`PortalLink`/`PortalTarget`; the deprecated-portal framing decided for paths.md/overview.md is not reconciled with this third kept location, which the discussion only touches to add a convention pointer.
**Fix:** State whether CONSTRAINTS.md L19 should also tag the three portal methods deprecated for consistency, or is intentionally left as a raw capability list.

### [NOTE] mux.md own-state vs worktree-registry boundary needs a concrete rule
**Section:** Decisions (mux-registry-semantics)
**Issue:** mux.md tangles both notions on adjacent lines — L21-22 ("worktree registry from `internal/state`" + dead `state.md` link) is worktree-registry coupling to drop, while L132/L157 phrase mux's own layout as "from the registry"; the plan writer must judge each `registry` token individually.
**Fix:** Note that L21-22 (link + worktree-registry framing) is the drop/relink target while `local-state.json` session/pane refs stay, so the boundary is unambiguous.

## Verdict
GAPS_FOUND
Two scoping gaps (benchmarks broken link; inaccurate L64 sweep token) must resolve before planning.
MILL_REVIEW_END