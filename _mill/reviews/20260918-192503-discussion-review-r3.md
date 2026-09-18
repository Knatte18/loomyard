MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree

```yaml
duration_s: 112.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-20250514 (best-effort; presented to me as Opus 5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Warm attach pre-flight can reap untracked panes
**Section:** `warm-attach-gains-one-apply-and-one-write`, consequence 2 + its pinning smoke test
**Issue:** The decision prices only `select-layout`, but `upLocked`'s tail calls `reconcileApplyPersistLocked` → `reconcileLocked`, which *kills* panes: with an alive header, `planReconcile` adds every non-exempt live pane to `untrackedPanesToKill` (`internal/reedengine/reconcile.go:127-131`, and its own log comment says "it destroys panes an operator may have created themselves"). Today's `Status()` pre-flight is read-only, so a warm `lyx reed attach` never destroyed anything.
**Fix:** State a disposition for operator-created untracked panes in a warm attach (accept the reap, or scope the pre-flight), and restate the "every pane alive before is alive after" test claim in terms that survive the reap.

### [BLOCKING:design] Up()'s config validation now gates a warm attach
**Section:** `attach-heals-at-the-cli-preflight` / Scope "In"
**Issue:** `ensureServerAndSessionLocked` runs `debugLogArgs`/`mouseOption`/`watchdogOption`/`ValidateHeader`/`probeCapabilityLocked` *before* its already-up early return (`lifecycle.go:164-244`). Swapping `Status()`→`Up()` therefore makes a config typo or a below-floor tmux version refuse `lyx reed attach` against a healthy live session — an operator lockout from viewing an existing session, not just a boot-time refusal. The decision lists those errors only as cold-boot failures.
**Fix:** Disposition the warm-path refusal explicitly (accept as part of "attach converges like every other entrypoint", or keep it out) and pin it with a test either way.

### [NIT:scope] Warm `reed add`'s added pre-flight cost is unpriced
**Section:** `warm-attach-gains-one-apply-and-one-write` (attach only)
**Issue:** A warm `AddStrand` gains the same tail — an extra `ensureHeaderPaneLocked` + reconcile + apply + `SaveState` ahead of its own `reconcileApplyPersistLocked` (`strand.go:441/464/503`) — plus the config validation above; no decision names this.
**Fix:** Extend the decision (or add a sentence) covering the warm `add` path's duplicated converge, so a plan writer does not read it as attach-specific.

## Verdict

REQUEST_CHANGES
Warm-path costs of routing attach through `Up()` are understated: pane reaping and config refusal are undispositioned.
MILL_REVIEW_END
