MILL_REVIEW_BEGIN
# Review: Optimise and slim the test suite — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-20
```

## Findings

### [NIT] lyxtest.go risks tripping the paths enforcement guard
**Location:** Batch 1, Cards 2–3
**Issue:** `internal/lyxtest/lyxtest.go` is non-test production code; `enforcement_test.go` fails the build if any non-`_test.go` file outside `internal/paths`/`cmd/lyx` contains `os.Getwd` or `--show-toplevel`, and lyxtest does git-fixture work where an implementer might reach for `git rev-parse --show-toplevel`.
**Fix:** Card 3 already mandates `paths.Resolve(hub)` for geometry; add an explicit note that lyxtest must never use the two banned primitives so the enforcement guard stays green.

### [NIT] host-hub template pushes `main`, diverging from addRemote semantics
**Location:** Batch 1, Card 2(a); overview "Decision: template-once"
**Issue:** The existing worktree/paths `addRemote` deliberately leaves the bare empty (`Add` populates it via `push -u <newbranch>`), whereas the host-hub template runs `git push -u origin main`; this is harmless for `TestAdd`/`TestAddRollback` (their `ls-remote`/branch checks still pass) but is an unstated behavioural change to the host fixture shape.
**Fix:** Confirm in Card 2 that pushing `main` to the host-hub bare is intentional and assert `TestAddRollback`'s `ls-remote origin` superset check still holds; or keep the host-hub bare empty and only push `main` on the weft template (where upstream tracking is actually required).

### [NIT] "All Files Touched" omits the three deletions
**Location:** Batch overview, "All Files Touched"
**Issue:** The list enumerates created/edited files but not the deleted `internal/{paths,worktree}/helpers_test.go` and `internal/worktree/testhelpers_test.go`, which are load-bearing for the drain (Cards 12, 13).
**Fix:** Add the three deleted paths to the manifest (or a "Deleted" subsection) for traceability.

## Verdict

APPROVE
Plan is complete, correctly sequenced, DAG-valid, and faithful to source; only minor nits.
MILL_REVIEW_END
