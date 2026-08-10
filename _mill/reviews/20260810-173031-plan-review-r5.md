MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12) — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-10
```

## Findings

### [NIT:consistency] Batch 1's error-collapse site list omits pull.go and remove.go
**Location:** `01-dirtiness-probe.md` Batch Scope, vs. Cards 2 and 3 **Issue:** The Batch Scope paragraph names exactly "`add.go`, `checkout.go`, `warpclean.go` and `reconcile.go`" as the sites where `worktreeDirty`'s single consolidated error collapses today's separate spawn-failure/exit-code messages into one, but `pull.go`'s `warpWorktreeDirty` (lines 142-151, two distinct `fmt.Errorf` forms) and `remove.go`'s two probes (`Topology.Remove`'s warp check at lines 60-72 and `refuseDirtyWeftWorktree` at lines 127-144, both also two-form) verifiably carry the identical structure and undergo the same collapse per Card 2's and Card 3's own site-specific instructions (which correctly tell the implementer to keep only the `%w`-prefixed wording at each of those sites too). **Fix:** Widen the Batch Scope's enumeration to name all affected sites, or restate the rule as applying uniformly to every migrated probe rather than to four named files — the individual cards are already correct, only the summary undercounts.

### [NIT:design] `resolveBranchOwnership`'s `repoDir` parameter is unused by the only branch kind
**Location:** `02-the-gate.md` Card 6 **Issue:** Card 6 pins `resolveBranchOwnership(own branchOwnership, branch, repoDir string) (bool, string)`, but the sole branch-shaped kind it dispatches to, `ownedManagedBranch(l, branchPrefix)`, resolves its branch-name check, its `primaryWeftBranch(l)` comparison, and its checked-out lookup entirely from `l` (carried inside `own`) — no predicate in Card 6's text ever consults the separate `repoDir` argument, even though `branchRequest` already carries `repoDir` and it would be free to pass. **Fix:** Either drop the unused parameter from the dispatcher signature or add one sentence to Card 6 stating why it is carried anyway (e.g. future kinds).

## Verdict

APPROVE
Plan is thorough, internally consistent, and its extensive line-level source citations verify correct against the current tree; only two non-blocking nits found.
MILL_REVIEW_END
