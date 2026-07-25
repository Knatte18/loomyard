MILL_REVIEW_BEGIN
# Review: git-native-library: feasibility spike

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Kept write-up location contradicts doc lifecycle
**Section:** Scope / Decisions (keep-the-poc-code) / Constraints (Documentation Lifecycle)
**Issue:** The plan keeps a durable write-up at `manifest/designs/git-native-library.md` and moves the roadmap item to **Done** "linking the write-up," but `docs/overview.md#documentation-lifecycle` says `manifest/designs/<module>.md` docs are for *not-yet-built* modules and are **deleted when the work lands**, and `roadmap.md` Maintenance states Done entries deliberately do **not** link ("that's why Done entries above don't link anywhere").
**Fix:** Decide a *kept* home for the durable evidence (e.g. `docs/reference/` or the `gitnativepoc` package godoc) and either delete/no-link the designs doc per convention or record an explicit spike carve-out; resolve the "Done entry links" contradiction before plan writing.

### [NOTE] Design doc's "read-only subset" scope is now stale
**Section:** full-surface-including-rebase / Technical context
**Issue:** `manifest/designs/git-native-library.md` scopes the spike to "the read-only subset" and calls writes an explicit non-goal, while the discussion expands to the **full write surface incl. rebase-retry**; the discussion's claim that the design "explicitly mandates verifying" rebase overstates a doc that lists rebase only as a "Known cost."
**Fix:** Have the write-up explicitly supersede the old read-only framing so the widened scope is not read as contradicting the design it replaces.

### [NOTE] go-git version left unpinned for a kept main dependency
**Section:** go-git-primary / Scope (go.mod dependency)
**Issue:** "latest stable that `go get` resolves" is fine for the spike, but the dependency lands on `main` and the harness is meant to be re-runnable (incl. later Win11), so a floating verdict-bearing version undercuts reproducibility.
**Fix:** State that the resolved go-git version is pinned in `go.mod`/`go.sum` and recorded in the write-up alongside each MIGRATE/CLI-BOUND verdict.

### [NOTE] Build-order prerequisite satisfied but unacknowledged
**Section:** Technical context (Reference points) / roadmap dependency
**Issue:** The design and roadmap gate this spike on `board-use-gitrepo` landing first; that item is now in roadmap **Done** and `StageAllAndCommit` exists in `gitrepo.go`, so the gate is met — but the discussion never states this, leaving a plan writer to re-check.
**Fix:** Note that the `board-use-gitrepo` prerequisite has landed (wildcard-stage `StageAllAndCommit` present), so the surface is stable to mirror.

## Verdict

GAPS_FOUND
One unresolved doc-lifecycle conflict on where the kept write-up lives and Done-entry linking.
MILL_REVIEW_END
