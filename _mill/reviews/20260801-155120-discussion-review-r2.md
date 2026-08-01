MILL_REVIEW_BEGIN
# Review: fabric: warp-rebase / remote-reconcile recovery

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] SHAExists cannot detect a rebase or find the anchor
**Section:** rebase-detection-scope; warp-refresh-primitives; Technical context (SHAExists)
**Issue:** `gitrepo.Repo.SHAExists` is an object-existence lookup (verified `gitrepo.go:410` — peels to a commit via go-git, no reachability), but a force-pushed/rebased old tip's commit object survives a `git fetch` (fetch adds objects and moves refs; it never prunes), so `SHAExists(latestWarpSHA)` returns true post-fetch and detection sees no drift — and the newest-to-oldest re-anchor walk finds every dead SHA still "exists," anchoring on the wrong (orphaned) commit.
**Fix:** Base detection and the anchor walk on ancestry/reachability from the new remote ref (e.g. `merge-base --is-ancestor` against the fetched ref — the same primitive as the divergence classifier), not on `SHAExists`; the injectable predicate in Testing should be reachability, not object-existence.

### [GAP] New anchor is never persisted — detection is not idempotent
**Section:** safe-vs-unsafe-reconcile; pattern-conflict-reporting; Constraints (anchor commit)
**Issue:** After reconcile, warp is reset to new remote HEAD but weft HEAD's trailer still names the old (dead) Warp-SHA, so the "record a new anchor point" commit is only hedged ("any commit this slice makes"), never decided; without it, every subsequent `Fabric.Pull` on the already-reconciled clean repo re-detects the same drift.
**Fix:** Decide explicitly whether reconcile writes a new weft correspondence commit binding the new warp HEAD (restoring idempotency) or leaves the stale entry, and state the resulting re-detection behavior.

### [NOTE] Weft state during reconcile (reset vs preserve) left implicit
**Section:** safe-vs-unsafe-reconcile
**Issue:** Reusing `RevertWithWeft`'s nearest-older walk invites reusing its weft-reset (which discards post-anchor commits), yet this slice must preserve those commits to report PATTERN residue; only pattern-conflict-reporting implies preservation.
**Fix:** State plainly that reconcile resets warp only and leaves weft history untouched, so the walk is shared but the weft-reset is not.

## Verdict

GAPS_FOUND
Detection/anchor primitive is semantically wrong and anchor persistence is undecided.
MILL_REVIEW_END
