MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] No-surviving-anchor case undefined
**Section:** Decisions § safe-vs-unsafe-reconcile / rebase-detection-scope
**Issue:** The re-anchor walks "newest-to-oldest until finding the first `Warp-SHA` still reachable," but behavior is undefined when the rewrite leaves *zero* reachable entries (whole recorded warp history rebased away, or a single-entry index whose one `Warp-SHA` is gone).
**Fix:** Specify the terminal case — e.g. abort loudly like the double-conflict path, or treat all weft commits as post-anchor PATTERN residue — so `/mill-plan` and the walk's pure-function contract have a defined result.

### [NOTE] Warp-first ordering vs. reconcile's weft anchor commit
**Section:** Decisions § unified-pull-dispatch / safe-vs-unsafe-reconcile
**Issue:** Warp-first means reconcile writes a new weft anchor commit *before* the weft `--ff-only` pull; if weft's remote also advanced, the local anchor commit makes weft diverged and the ff-pull fails (self-induced), and the anchor walk also ran against a pre-pull, staler weft index.
**Fix:** Surface the concrete ordering consideration (pull weft first for a fresh index and to avoid self-induced divergence) so `/mill-plan`'s "unless a concrete reason to order otherwise" hook is decided deliberately.

### [NOTE] Cross-machine propagation of the anchor commit
**Section:** Decisions § safe-vs-unsafe-reconcile
**Issue:** Idempotency is scoped to "a subsequent `Fabric.Pull` on the now-reconciled repo" (same machine); the new anchor commit is local/unpushed, so a second collaborator pulling the rebased warp without it re-detects and writes its own anchor commit, diverging weft across machines.
**Fix:** Note whether cross-machine anchor propagation is out of scope for this slice or whether the anchor commit is expected to reach other machines via a later push/sync.

## Verdict

GAPS_FOUND
One failure-mode gap (no surviving anchor) must be pinned before plan writing.
MILL_REVIEW_END