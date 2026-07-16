MILL_REVIEW_BEGIN
# Review: Built-in operator console pane in mux

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-16
```

## Findings

### [GAP] First strand can adopt the header pane
**Section:** header pane is the persistent keepalive (Q8) / Technical context (exclusion points)
**Issue:** `planPaneTarget` (spawn.go:40-58) adopts "the first alive pane in `live`" when no strand is bound; the header pane is alive and in `live` but not a strand, so the first strand added after boot can adopt (take over) the header pane — destroying the keepalive. The discussion lists status/count/reconcile exclusion points but not this adoption/split-target seam.
**Fix:** State that the header pane id must be excluded from `planPaneTarget`'s adoption candidate and split-target selection (spawn.go), alongside the reconcile exemption.

### [GAP] Header pane not threaded into the select-layout string
**Section:** top-band-in-render (Q3)
**Issue:** `applyLayoutLocked`→`planLayout`→`render.Rules(strands, ...)` emits a window_layout that must enumerate every live pane (tmux rejects a mismatched-count layout; psmux reaps the extra pane — strand.go:471, apply.go:21). The header is a real pane but explicitly not in `Strands`, and `Rules` takes only strands. The decision covers Box-shrink + a new policy case but never says how `HeaderPaneID`+height reach render so its top-band cell is emitted.
**Fix:** Specify the seam: pass `HeaderPaneID`+height from `planLayout` into `Rules` (new param, not a synthetic strand) so the emitted layout enumerates header band + shrunk strand stack.

### [NOTE] Repo derivation may add a git spawn to the Resolve hot path
**Section:** GAP-2 / Technical context (Layout.Repo)
**Issue:** `hubgeometry.Resolve` runs on nearly every lyx command; eagerly deriving `Repo` via `git remote get-url origin` adds a subprocess per invocation (repo recently invested in "reduce git spawns").
**Fix:** Prefer the spawn-free `filepath.Base(Prime)` derivation (Prime is already computed) or make Repo lazy; note the choice.

### [NOTE] Header-height clamp is new logic, not a clampToFit reuse
**Section:** top-band-in-render height clamp (NOTE-2)
**Issue:** `clampToFit`/`MinFullRows` (height.go:88) distributes rows among strands *inside* an already-shrunk Box; clamping the window→(header,strandBox) split is a distinct new step, and "retains its MinFullRows floor" is ambiguous when several strands each want MinFullRows.
**Fix:** Describe the header-vs-window clamp as new logic and state whether the floor is per-strand or a stack total.

## Verdict

GAPS_FOUND
Two unaddressed pane seams (adoption, layout enumeration) can break the keepalive; resolve before planning.
MILL_REVIEW_END
