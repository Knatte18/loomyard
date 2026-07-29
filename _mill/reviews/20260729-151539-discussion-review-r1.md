MILL_REVIEW_BEGIN
# Review: Give codeintel a persistent, session-long daemon

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.x (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Batch-mode refs/definition never thread WorktreeRoot
**Section:** Technical context ("Options already threads WorktreeRoot for supervised") / item5-dispatch-supervised
**Issue:** Only the single-arg paths set `Options.WorktreeRoot` (`cli.go:152,265`); the batch-mode closures (`cli.go:168,281`) omit it, so once dispatch flips to supervised — which anchors state/lock/socket via `Layout{WorktreeRoot}.CodeintelDaemonStateFile` — batch calls resolve a cwd-relative empty-rooted `_lyx/codeintel/go/` path, spawning/using a *different* daemon than single-arg mode and breaking the one-daemon-per-worktree thesis.
**Fix:** Add to Scope/Testing that both refs and definition batch closures must thread `WorktreeRoot` into `Options`, and correct the "already threads" claim; add a unit assertion that batch and single-arg build the same WorktreeRoot-anchored Options.

### [GAP] Wedged escalation trigger cannot tell a fresh respawn from the wedge
**Section:** item5-wedged-daemon-escalation
**Issue:** The under-lock kill criterion is "re-check staleness" (`daemonStale` = PID liveness + protocol only) and "force-kill the recorded PID." Under concurrency, a caller that failed to dial the OLD pid arrives after another caller already killed+respawned; the state now holds a NEW, healthy pid that also reads non-stale, so the literal rule force-kills a freshly-respawned healthy daemon — callers can thrash each other's restarts within the deadline.
**Fix:** Specify that the escalation kills only when the *current* recorded PID equals the one whose dial/finalize just failed (or re-dials under the lock and kills only if that fresh dial also fails), not merely on a non-stale re-read.

## Verdict

GAPS_FOUND
Two concurrency/anchoring gaps in item 5's supervised wiring must be resolved before plan writing.
MILL_REVIEW_END
