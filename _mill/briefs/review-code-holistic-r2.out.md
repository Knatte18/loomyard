MILL_REVIEW_BEGIN
# Review: reed: extract Selvage-pane lifecycle — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

No findings.

Both batches were checked end-to-end against the plan and against each other:

- `selvagepane.go` carries exactly the nine seams the plan specifies (`reapPolicy`/`newReapPolicy` + its three methods, `planPaneTarget` with the new `insertAbove` return, `selvageRenderParams`, `seedSelvageClaim`, `clearSelvagePaneBinding`, `ensureSelvagePaneLocked`, `bottommostPaneID`, `splitSelvagePaneAtBottomLocked`, `splitPaneBelowLocked`), each carrying its doc comment verbatim per the `comments-move-verbatim-with-their-code` decision.
- The four host files (`lifecycle.go`, `reconcile.go`, `spawn.go`, `apply.go`) plus `generation.go` now touch Selvage only through call-position seam invocations or composite-literal field keys — verified directly by grep against every host file, matching `selvagepane_enforcement_test.go`'s own two exemptions.
- `selvagepane_enforcement_test.go` correctly implements the AST scan, the allowlist, and both required tests; its doc-comment residual and precedent citations (`gitkit`'s directory allowlist, `cliwire`'s ban-list) check out against those two files' actual contents.
- `planReconcile`'s and `planPaneTarget`'s signature changes are threaded consistently across `reconcile.go`/`reconcile_test.go` and `spawn.go`/`selvagepane_test.go`, with `launchStrandLocked` consuming `insertAbove` exactly as specified.
- Card 7–9 test relocations/additions are complete and hermetic; `selvagepane_test.go` covers all four new helpers plus the moved `ensureSelvagePaneLocked`/`bottommostPaneID`/`planPaneTarget` tests.
- Batch 2's doc updates are accurate: `doc.go`'s two repointed passages, the design doc's Status section (its before/after per-file counts were independently re-verified against the actual files and match exactly, including `selvagepane.go`:92, `lifecycle.go`:19, `doc.go`:33), the `reed-header-selvage.md` attribution fix, and the roadmap Done-move all match their cards' requirements.
- No out-of-plan files; no cross-batch or global-utility duplication found.

## Verdict

APPROVE
Both batches fully realize the plan; audited figures were independently verified and match; no constraint violations found.
MILL_REVIEW_END
