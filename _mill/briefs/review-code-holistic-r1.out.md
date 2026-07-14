MILL_REVIEW_BEGIN
# Review: Reduce git spawns in warpengine integration tests — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-14
```

## Findings

None. Verified end-to-end against the single batch (`01-reduce-redundant-resolve.md`):

- `hubgeometry.Layout.SiblingLayout` (`internal/hubgeometry/hubgeometry.go:161-170`) matches
  card 1's exact spec: no git spawn, `filepath.Clean`, correct field derivation, and godoc
  covering all three required points (spawn-free derivation, worktree-root precondition,
  byte-for-byte equivalence + caller-guard obligation).
- `internal/hubgeometry/siblinglayout_test.go` implements both required tests
  (`TestSiblingLayout_EquivalentToResolve`, `TestSiblingLayout_NonSiblingDiverges`), asserts
  all five `Layout` fields individually plus `reflect.DeepEqual`, uses `t.Parallel()`, and
  correctly constructs a hub-sibling vs. an outside-hub worktree.
- `hostLayoutFor` (`internal/warpengine/hostlayout.go`) implements the exact guard
  (`filepath.Dir(worktreeRoot) != l.Hub` → `Resolve`, else `SiblingLayout`), and both
  `status.go:130` and `reconcile.go:114` swap in `hostLayoutFor(l, hostPath)` in place of the
  prior per-iteration `hubgeometry.Resolve(hostPath)`, with surrounding error handling
  untouched. `clone.go`'s unrelated `Resolve` call (line 72) is correctly left alone.
  `hubgeometry` imports remain used (both files still reference `hubgeometry.Layout` in
  signatures).
- `internal/warpengine/spawncount_test.go` builds the paired fixture via `w.Add(...,
  AddOptions{SkipGit: true})` (not bare `git worktree add`), correctly required since `Status`
  gates on `os.Stat(weftPath)` before reaching `hostLayoutFor`; measures N=2 vs. N=4 for both
  `Status` and `Reconcile` via `GIT_TRACE2_EVENT` + `--show-toplevel` substring counting; is
  non-parallel with no `t.Parallel()` anywhere in the file, correctly isolating the trace
  window (all other tests in the package call `t.Parallel()` as their first line, before any
  git spawn, so they pause before this test's measurement window).
- `docs/benchmarks/fixture-copy.md`'s new dated section is strictly appended after the prior
  content, records before/after/isolated-delta numbers with the reverted-vs-shipped
  `hostLayoutFor` A/B (−30 total processes / −15 `--show-toplevel`, with the 2× relationship
  correctly explained by `Resolve`'s internal `List` call), and explicitly reconciles the
  larger post-change test population against the isolated-delta methodology.
- No out-of-plan files: `git grep`-style scan of `internal/warpengine` for `hubgeometry.Resolve`
  confirms only test files and the untouched `clone.go` call site reference it directly;
  production callers all route through `hostLayoutFor`. "All Files Touched" in the overview
  (7 files) matches exactly what was reviewed.
- Constraints: Hub Geometry Invariant preserved (`SiblingLayout` lives in `hubgeometry`;
  `hostLayoutFor` only calls into it and uses `filepath.Dir` for the guard, constructing no
  geometry token). Test Tier Purity and Hermetic Git Env invariants respected (`//go:build
  integration` on both new files; existing `TestMain` reused per the plan's stated decision).

## Verdict

APPROVE
Implementation matches the plan precisely; no deviations, duplication, or constraint violations found.
MILL_REVIEW_END
