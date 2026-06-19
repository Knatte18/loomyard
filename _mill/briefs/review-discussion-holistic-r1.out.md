This confirms mux.md is heavily laden with registry/local-state.json references and a link to the deleted state.md. The discussion's scope says mux.md gets a "terminology sweep (Container→Hub, Prime, drop registry); fix links to moved research docs and to the relocated psmux reference."

However, there's a subtlety: mux.md describes the *future mux module's design*, where the registry/local-state.json is part of mux's own planned state document (roadmap milestone 8 references "from the worktree registry", and the state lib "will own its own state document"). The discussion's stale-term check says grep kept docs for "registry" framing → expect zero. But mux.md is a kept doc describing a not-yet-built module that legitimately may use state. This is a genuine tension: is the registry stale (the *worktree* registry that doesn't exist) or legitimate (mux's *own* future state)? The discussion isn't crisp about whether mux.md's registry references should all be purged or only the worktree-registry framing.

I have enough to produce the review.

MILL_REVIEW_BEGIN
# Review: Reconcile stale design docs (stateless + weft model)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-19
```

## Findings

### [GAP] mux.md registry sweep scope is ambiguous
**Section:** Technical context (mux.md line) / Testing (stale-term check)
**Issue:** mux.md (kept) uses "registry"/`local-state.json` ~10 times (L21,132,148,202,209,260,304,317,333) to describe mux's *own future* state document, but the stale-term check demands "zero registry framing" in kept docs and the worktree-registry is the thing being purged — these conflict for a not-yet-built module's design doc.
**Fix:** State explicitly whether mux.md's registry/`local-state.json` references are legitimate (mux's own planned state) and exempt, or must be reworded; the convention keeps unbuilt-module design but the stale-term check would flag them all.

### [GAP] README.md "Libraries" link list points at 5 deleted docs
**Section:** Technical context (kept-docs stale spots)
**Issue:** `docs/shared-libs/README.md` L18-24 links to all seven lib docs including the five being deleted (fsx, git, gitignore, lock, state); the discussion only flags README's *opening line* ("board, worktree, mux"), not that its core link list goes 5-dead after deletions.
**Fix:** Add README.md's Libraries link block (L18-24) to the explicit broken-link sweep targets so the plan rewrites it to the kept set, not just the opening sentence.

### [GAP] overview.md still marks internal/state as "(planned)"
**Section:** Technical context (kept-docs stale spots)
**Issue:** Technical context says state landed in `ba81abf` (verified: only its own test imports it), but kept doc overview.md L188 and L205 both label `internal/state` **(planned)** — this stale marker is not in the discussion's enumerated overview.md sweep items (which list only link blocks + status markers).
**Fix:** Add overview.md's `internal/state` "(planned)" markers (L188, L205) to the explicit sweep list, mirroring the roadmap milestone-3 reword.

### [NOTE] Path Invariants method list omits PortalLink
**Section:** Decisions / paths framing
**Issue:** overview.md L64 enumerates Layout methods but omits `PortalLink`/`PortalTarget` while paths.md keeps documenting all three portal methods as deprecated-but-present; the two kept docs will describe the portal surface inconsistently.
**Fix:** Note whether overview's method list should align with paths.md's deprecated-portal framing or intentionally omit portals.

## Verdict

GAPS_FOUND
Three kept-doc stale spots (mux.md registry semantics, README link list, overview "planned" state) fall outside the enumerated sweep scope.
MILL_REVIEW_END