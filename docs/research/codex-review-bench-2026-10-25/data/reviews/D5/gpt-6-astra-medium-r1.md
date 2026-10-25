---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: _lyx/discussion/decision-record.md:75
    summary: The merge-record exception still permits a cleanup instruction for another merge's staged changes
    fix: Amend D2 and D4 to give an in-progress merge reason precedence over the ordinary dirty-parent cleanup guidance at Finalize. Preserve the aggregated error and typed dirty-path details, but suppress commit-or-discard guidance when the error also reports a merge in progress; direct the recipient to the parent merge's owner to finish its recovery before retrying. Record the aggregation evidence from fabricengine's current Merge and MergeGuardError implementation, and add an acceptance scenario with both an existing parent merge record and tracked staged changes that verifies neither the goto response nor the subsequent Finalize halt recommends discarding those changes.
  - id: F2
    severity: MEDIUM
    class: design
    location: _lyx/discussion/decision-record.md:43
    summary: The suggested fabric commit does not clean the parent's code checkout
    fix: Replace the lyx fabric commit example in D2's shared D2/D4 recovery text with instructions to commit or intentionally discard the listed changes in the named parent code checkout. For the commit example use ordinary code-side git add -- <paths> followed by git commit -m <message>, with the parent operator performing it when the receiving task session cannot modify that checkout, then retain the task-worktree goto and resume commands. Record that fabric commit synchronizes the configured records pathspec, and require the recovery acceptance scenario to demonstrate that the named code path becomes clean before goto is retried.
---

### [BLOCKING] An in-progress merge can receive the ordinary discard advice

**Location:** `_lyx/discussion/decision-record.md:75`, together with D2 at lines 40–45.

**Issue:** D4's exception rests on Finalize refusing with the in-progress reason without telling the recipient to discard the staged merge. The current `internal/fabricengine/merge.go` explicitly aggregates `mergeInProgressReason` and `pairDirtyReason` before returning. `MergeGuardError.WorktreeDirty()` in `internal/fabricengine/mergeerrors.go` tests membership, not exclusivity. Consequently, when that combined error reaches Finalize's steps 6–7, it enters the dirty arm that D2 instructs Plan to decorate with commit-or-discard advice. Admitting the goto does not provide the promised protection. This is a concrete unsafe recovery instruction for the very staged changes D4 intends to protect, not a request to change the closed error vocabulary.

### [MEDIUM] The recovery example commits the wrong side

**Location:** `_lyx/discussion/decision-record.md:43`.

**Issue:** `internal/fabriccli/weft_verbs.go` builds the CLI commit pathspec from Fabric's routing names and describes the command as committing weft changes. It takes no file arguments and invokes `fab.Commit` with that records pathspec. Thus, for the support log's example of dirty code-side `docs/overview.md`, the suggested command can succeed while leaving the actual blocker untouched; D4 will refuse the following goto again. In addition, `internal/fabriccli/roleguard.go` refuses this mutating fabric verb from sandboxed task roles, so changing directory to the parent does not make it a usable task-agent recovery. The named goto and resume commands themselves exist; the defect is the proposed cleanup step.

## Rejected

- D4's unreadable-parent admission and advisory-at-goto-time limitation are explicitly bounded by Finalize's authoritative checks. No additional finding for those choices. The inspected goto wiring passes the recipe guard through CLI/driver goto, tackler and arbiter validation, and their consume paths; no bypass of those guarded targets was established.
- D1 states a researched answer, names the dirty guard and parent-record commit step, and does not assign the factual question to Plan. The current `fabricengine/mergeguards.go` and `fabricengine/junction.go` support the code-only guard and linked records topology. No missing exclusion finding.
- D3 explicitly retains recognition and skipping of the legacy parent-status section, names the removed writer seam, and describes the affected task-side surface in Scope. No necessary constraint was found only in the support log; removal of test references is Plan's enumeration work.
- The Decisions contain actionable requirements and short rationales; the Notes carry path-conversion and parent-opening hints. No deliberation-only passage warranted relocation after applying completeness-before-leanness.
