I have enough context to write the review. Let me note the key findings.

The implementation is thorough and well-tested. I found one real integration bug in the standalone `resume`-after-crash path, plus a minor side-effect note on `status`.

MILL_REVIEW_BEGIN
# Review: Build internal/mux: the window to the world (overlay + strands + render) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/ + source
date: 2026-07-02
```

## Findings

### [BLOCKING] Resume-after-crash never adopts the new-session pane
**Location:** `internal/muxengine/lifecycle.go:190-233` (Resume) with `internal/muxengine/spawn.go:26-33` (planLaunch)
**Issue:** When `resume` is called standalone after a server death (a supported flow — Resume boots the server itself, and "server dead" is one of the three states in `TestPlanResumeLaunches`), the persisted strands still carry stale non-empty `PaneID`s. Resume launches strands *before* reconcile runs, so `planLaunch(st)` sees those stale bindings, returns `false`, and every strand `split-window`s instead of the first strand adopting the fresh new-session pane. The initial pwsh pane is left orphaned/untracked, so the final `reconcileApplyPersistLocked` builds a layout enumerating N panes while psmux holds N+1 — the exact GAP2 pane-count mismatch, causing `select-layout` (and thus `Resume`) to fail. (The `add` path avoids this only because `up` reconciles first; Resume has no such precondition.)
**Fix:** Run `reconcileLocked` (clear stale bindings) at the start of Resume, after `ensureServerAndSessionLocked` and before `planResumeLaunches`/the launch loop — then `planLaunch` correctly adopts. (Equivalently, key `planLaunch` on the live-pane set rather than on `PaneID != ""`.)

### [NIT] `status` mutates the live layout as a side effect
**Location:** `internal/muxengine/lifecycle.go:263-295`
**Issue:** Card 20 specifies Status "reconcile against live panes and report," but `Status` calls `reconcileApplyPersistLocked`, which additionally re-issues `select-layout`/`select-pane` and re-persists on every query — so `lyx mux status` can move input focus and rewrite the layout, a surprising side effect for a read verb (attach's pre-flight reuse is fine).
**Fix:** For the report path, reconcile + list-panes without the `applyLayoutLocked`/persist tail, or document the apply-on-status behaviour as intentional.

## Verdict

REQUEST_CHANGES
One integration bug breaks standalone resume-after-crash; otherwise the batches compose cleanly.
MILL_REVIEW_END
