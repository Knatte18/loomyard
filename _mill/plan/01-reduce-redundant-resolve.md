# Batch: reduce-redundant-resolve

```yaml
task: "Reduce git spawns in warpengine integration tests"
batch: "reduce-redundant-resolve"
number: 1
cards: 5
verify: go test -tags integration -run 'TestSiblingLayout|TestStatus|TestReconcile|TestResolveSpawns' ./internal/hubgeometry ./internal/warpengine
depends-on: []
```

## Batch Scope

This batch delivers the entire task in one cohesive unit: a spawn-free
`hubgeometry.Layout` derivation method, a warpengine guarded call-site swap that
consumes it, and the two tests plus the benchmark record that prove and lock the
change. It is one batch because every card shares the same tight context
(`hubgeometry` geometry + the `warpengine` `Status`/`Reconcile` scan loops) and a
Sonnet session holds all of it at once. Cards are ordered so each builds on the
prior: the method (card 1) is proven by its equivalence test (card 2) before the
call sites consume it (card 3), which the spawn-count guard then locks (card 4),
with the benchmark record last (card 5). No batch depends on this one.

## Cards

### Card 1: Add `SiblingLayout` method to hubgeometry

- **Context:**
  - `internal/hubgeometry/worktreelist.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a method `func (l *Layout) SiblingLayout(worktreeRoot string) *Layout` to `internal/hubgeometry/hubgeometry.go` (place it among the other `Layout` methods, e.g. immediately after `Resolve`). It performs **no git spawn**. Body: `c := filepath.Clean(worktreeRoot)` then `return &Layout{Cwd: c, WorktreeRoot: c, Hub: l.Hub, RelPath: ".", Prime: l.Prime}`. The `filepath.Clean` call mirrors `Resolve`, which sets `Cwd = filepath.Clean(cwd)` and `WorktreeRoot = filepath.Clean(rev-parse output)`, so the two produce identical field values for a hub-sibling root. Godoc must state: (a) it derives the `Layout` for a **hub-sibling** worktree from the receiver's already-resolved `Hub` and `Prime` without spawning git; (b) the **precondition** that `worktreeRoot` must be an actual worktree root as returned by `hubgeometry.List` (not a subpath) — `RelPath` is hardcoded to `"."`, which is only correct for a root; (c) that it is byte-for-byte equivalent to `Resolve(worktreeRoot)` when `filepath.Dir(worktreeRoot) == l.Hub`, and callers must guard the non-sibling case. Do not change `Resolve`, `List`, or any other function.
- **Commit:** `feat(hubgeometry): add SiblingLayout for spawn-free sibling layout derivation`

### Card 2: Equivalence + non-sibling divergence test for `SiblingLayout`

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/hubgeometry/siblinglayout_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, first line `//go:build integration`, `package hubgeometry_test`. `TestSiblingLayout_EquivalentToResolve`: build a host-hub fixture with `lyxtest.CopyHostHub(t)`; resolve the base layout `l, _ := hubgeometry.Resolve(fix.Hub)`; create a second **hub-sibling** worktree root by running `lyxtest.MustRun(t, fix.Hub, "git", "worktree", "add", <siblingPath>, "-b", <branch>)` where `<siblingPath>` is a fresh directory under `filepath.Dir(fix.Hub)` (the container, so it is a hub sibling); for both `fix.Hub` and `<siblingPath>` assert that `l.SiblingLayout(root)` is deep-equal to `hubgeometry.Resolve(root)`, comparing **all five fields explicitly**: `Cwd`, `WorktreeRoot`, `Hub`, `RelPath`, `Prime` (use `reflect.DeepEqual` on the dereferenced structs, and additionally assert each field so failures are legible). `TestSiblingLayout_NonSiblingDiverges`: add a worktree root **outside** the hub (e.g. under `t.TempDir()`) via `git worktree add`; assert `l.SiblingLayout(outRoot).Hub != resolved.Hub` where `resolved, _ := hubgeometry.Resolve(outRoot)` — i.e. that `SiblingLayout` (which reuses `l.Hub`) and `Resolve` (which uses `filepath.Dir(outRoot)`) diverge there, documenting exactly why the card-3 guard exists. Mark tests `t.Parallel()` as the other hubgeometry integration tests do.
- **Commit:** `test(hubgeometry): equivalence + non-sibling divergence for SiblingLayout`

### Card 3: Add guarded `hostLayoutFor` helper and swap the Status/Reconcile call sites

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:**
  - `internal/warpengine/status.go`
  - `internal/warpengine/reconcile.go`
- **Creates:**
  - `internal/warpengine/hostlayout.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the new `internal/warpengine/hostlayout.go` (`package warpengine`, import `path/filepath` and `internal/hubgeometry`), add an unexported helper `func hostLayoutFor(l *hubgeometry.Layout, worktreeRoot string) (*hubgeometry.Layout, error)`: `if filepath.Dir(worktreeRoot) != l.Hub { return hubgeometry.Resolve(worktreeRoot) }` else `return l.SiblingLayout(worktreeRoot), nil`. Godoc: it returns the per-host-worktree `Layout` for a worktree enumerated by `hubgeometry.List`, using the spawn-free `SiblingLayout` fast path for hub siblings (the normal case) and falling back to the spawning `Resolve` for any out-of-hub worktree so the result is byte-for-byte identical to calling `Resolve` directly. In `internal/warpengine/status.go`, replace the per-iteration `hostLayout, layoutErr := hubgeometry.Resolve(hostPath)` (in `Status`, ~line 129) with `hostLayout, layoutErr := hostLayoutFor(l, hostPath)`. In `internal/warpengine/reconcile.go`, replace the identical `hostLayout, layoutErr := hubgeometry.Resolve(hostPath)` (in `Reconcile`, ~line 113) with `hostLayout, layoutErr := hostLayoutFor(l, hostPath)`. Leave all surrounding error handling and the rest of both loops unchanged. Do not touch `clone.go`'s single `Resolve` or any `warpcli` call site. If `hubgeometry` is no longer referenced elsewhere in `status.go`/`reconcile.go` after the swap, keep the import only if still used (it is: `hostLayoutFor` lives in warpengine, but both files may still reference `hubgeometry.Layout` in signatures — verify and keep imports correct so the package compiles).
- **Commit:** `perf(warpengine): derive sibling layouts without re-spawning git in Status/Reconcile`

### Card 4: Spawn-count regression guard

- **Context:**
  - `internal/warpengine/status.go`
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/status_test.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/warpengine/add.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/spawncount_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, first line `//go:build integration`, `package warpengine`. Add `TestResolveSpawnsDoNotScale` — a **non-parallel** test (do NOT call `t.Parallel()`, so it has an exclusive execution window and no sibling test's git spawns pollute the trace). Helper flow: given a target host-worktree count N, build a `setupStatusFixture(t)`-style paired fixture (its own prime pair counts as one), then add `N-1` extra **full host+weft pairs** via `w.Add(f.Layout, <slug_i>, AddOptions{SkipPush: true})` (as `reconcile_test.go` does), NOT bare `git worktree add`. **This pairing is mandatory for the `Status` measurement:** `Status` reaches the per-iteration `hostLayoutFor` (`status.go` ~line 129) only *after* passing the `os.Stat(weftPath)` weft-exists gate (~lines 110–116) — a host worktree with no weft sibling hits `continue` first and never resolves, so bare host worktrees would make the `Status` assertion vacuous (it would pass even with the per-iteration `Resolve` regression present). `w.Add` creates the paired weft sibling so every added worktree passes the gate and reaches `hostLayoutFor`. (`Reconcile` calls the resolver at `reconcile.go` ~line 113 *before* its weft check, so it scales with N regardless — but use the same paired fixture for both.) Then measure only the `w.Status(f.Layout)` call: point `GIT_TRACE2_EVENT` at a fresh empty `t.TempDir()` subdir immediately before the call via `os.Setenv` and restore the prior value immediately after (use `defer`/explicit restore; the test is non-parallel so this process-global env toggle is safe). After the call, count the trace files in that dir whose content contains the substring `--show-toplevel` (one file per git process; `rev-parse --show-toplevel` is exactly what `Resolve` spawns). Run the measurement at **two counts, N=2 and N=4**, and assert the `--show-toplevel` count at N=4 is **not greater than** at N=2 (post-fix both are 0; pre-fix they scaled with N, so the assertion fails if per-iteration `Resolve` is reintroduced). Add the same two-count non-growth assertion for `w.Reconcile(f.Layout)` (either a second sub-test or a second measured call). Add a comment explaining the guard: pre-change, `Status`/`Reconcile` called `hubgeometry.Resolve` once per enumerated worktree with a present weft sibling, so `--show-toplevel` scaled with paired-worktree count; this guard fails if that regression returns.
- **Commit:** `test(warpengine): guard that Status/Reconcile resolve spawns do not scale with worktree count`

### Card 5: Record the Linux census in the benchmark doc

- **Context:**
  - `internal/warpengine/status.go`
- **Edits:**
  - `docs/benchmarks/fixture-copy.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a new dated section (heading `## warpengine spawn-reduction (2026-07-14, Linux)`) to `docs/benchmarks/fixture-copy.md`. Record: (1) **Before** — this task's baseline Linux census: `internal/warpengine` Tier 2 = 0.92 s, **1,435** git processes, `rev-parse` 398 (of which `--show-toplevel` **94**), worktree 229, branch 104, reset 68; and the **correction** that the task brief's "401 rev-parse largely `hubgeometry.Resolve`" was re-measured on Linux — `Resolve` (`--show-toplevel`) is 94, not ~400; the Windows spawn-count premise (spawn count = wall-clock floor) does not hold on Linux (~0.6 ms/spawn), though spawn *count* is OS-invariant (matches the Windows census). (2) **After** — actually re-run the census with `GIT_TRACE2_EVENT=<dir> go test -tags integration -count=1 ./internal/warpengine` (write to a `.scratch/` trace dir, not the repo root), recount total processes and `--show-toplevel`, and record the real post-change numbers and the `--show-toplevel` delta attributable to removing the per-iteration `Resolve` in `Status`/`Reconcile`. (3) A note that the Windows/AV wall-clock impact is **projected analytically** (spawns removed × measured per-spawn cost from the existing process-spawn-cost table), not measured on Windows in this task. Reproduce the census-counting method inline (count trace files by `cmd_name` / by `--show-toplevel` argv). Keep the existing append-only discipline of the doc — do not edit prior blocks.
- **Commit:** `docs(benchmarks): record Linux warpengine spawn-reduction census (before/after)`

## Batch Tests

`verify:` runs the two new tests plus every existing `TestStatus*` / `TestReconcile*` integration test (the suites card 3 modifies) across `internal/hubgeometry` and `internal/warpengine`: `go test -tags integration -run 'TestSiblingLayout|TestStatus|TestReconcile|TestResolveSpawns' ./internal/hubgeometry ./internal/warpengine`. This is a Go repo, so no `PYTHONPATH=` prefix applies. The scope is deliberately limited to the two affected packages and the affected test-name prefixes — not a repo-wide run. `TestSiblingLayout*` proves byte-for-byte equivalence and the non-sibling divergence; `TestResolveSpawnsDoNotScale` locks the spawn-count win; the existing `TestStatus*`/`TestReconcile*` suites confirm no behavioral regression from the call-site swap. Card 5 (docs) has no runnable surface of its own but requires the implementer to execute the census command to fill in the after-numbers.
