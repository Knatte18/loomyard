MILL_REVIEW_BEGIN
# Review: Extend worktree module: portals and launchers — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-14
```

## Findings

### [NIT] muxpoc cli_test.go not in card 15 Context/Edits
**Location:** Batch 5 / Card 15
**Issue:** Card 15 changes `RunCLI` to call `paths.Resolve` (now requiring a git repo) but `internal/muxpoc/cli_test.go` (which asserts `out.Len()==0` on no-subcommand / unknown-subcommand) is not listed in Context or Edits; the early resolve writing a JSON error to stdout in a non-git cwd would trip those assertions.
**Fix:** Add `internal/muxpoc/cli_test.go` to card 15 Context and note the dispatch-error tests still pass because the package dir is a git worktree; or place the `len(rest)<1` usage check before `paths.Resolve`.

### [NIT] Smoke test worktree-root vs rev-parse normalization
**Location:** Batch 5 / Card 18
**Issue:** Card 18 sets `cfg.WorktreeRoot` to the literal `t.TempDir()` and asserts `LoadState` against that same value, but on Windows `%TEMP%` resolves to a short/symlinked path differing from `git rev-parse --show-toplevel`; since the smoke path bypasses `RunCLI`/`paths.Resolve` the literal stays internally consistent, but the card should state it must use the raw temp dir (not a resolved one) on both sides to stay consistent.
**Fix:** Make card 18 explicit that the smoke test sets `WorktreeRoot` and the `LoadState` assertion to the identical raw temp-dir string, not a `paths.Resolve`-derived value.

### [NIT] socketName basename collision across sibling worktrees
**Location:** Batch 5 / Card 16-17 (and Decision: paths)
**Issue:** Post-migration `socketName(l.WorktreeRoot)` derives from `filepath.Base(worktreeRoot)`; two sibling worktrees whose leaf dir names collide (e.g. both named `hub`) would share a psmux socket. This is inherent to the existing basename scheme and out of scope, but the muxpoc doc/state_test could note it.
**Fix:** Optionally document that socket identity is the worktree-root basename; no code change required.

## Verdict

APPROVE
Plan is complete, correctly sequenced, DAG-valid, decision-faithful; only minor test-context nits.
MILL_REVIEW_END
