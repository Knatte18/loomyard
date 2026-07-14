# Discussion: Reduce git spawns in warpengine integration tests

```yaml
task: Reduce git spawns in warpengine integration tests
slug: warpengine-spawn-reduction
status: discussing
parent: main
```

## Problem

The task was framed on a Windows/AV reference machine (Intel 155U + Cortex XDR),
where a git subprocess costs ~72–208 ms and warpengine's integration tests spawn
~1,430 git processes — so spawn count *was* the residual wall-clock floor. Two
angles were proposed: (1) cache/reduce `hubgeometry.Resolve` (claimed ~401
`rev-parse` spawns), and (2) consolidate overlapping add/remove test flows.

**What changed:** this discussion re-measured everything on **Linux** (the machine
we now work on). Two findings collapse most of the original premise:

1. **On Linux there is no wall-clock problem.** A full `internal/warpengine`
   Tier 2 run is **0.92 s** (vs ~96 s on the Windows/AV box). A git spawn costs
   ~0.6 ms here, so 1,435 spawns barely register. The *test-speed* motivation is
   Windows-only, and the machine that feels the pain is the one that should
   measure any test-speed fix.
2. **The "401 rev-parse ≈ `Resolve`" assumption is false.** A fresh Linux
   GIT_TRACE2 census (1,435 processes — matching the Windows census almost
   exactly, confirming spawn *count* is OS-invariant) splits the 398 `rev-parse`
   as: `--show-toplevel` **94** (this is the only bucket `Resolve` produces),
   `--abbrev-ref HEAD` 86 (per-operation branch reads), `--git-common-dir` 42,
   `--git-path info/exclude` 42, `--is-inside-work-tree` 31, and ~100
   `--verify refs/heads/*` (test-harness assertions). `Resolve` is **94, not
   ~400**.

The genuinely expensive Windows operations (`git worktree add`/`remove`, `push`,
`receive-pack`) are the real work and are **irreducible** — no caching removes a
`git worktree add`. So the task is narrowed to the one lever with durable,
OS-independent value: removing **redundant** geometry re-resolution in two
warpengine product scan-loops. This is a production-latency + code-quality fix
(it speeds real `lyx warp status` / `reconcile` CLI runs, most visibly on
Windows), not a test-speed fix.

## Scope

**In:**

- New pure method on `hubgeometry.Layout` that derives a sibling worktree's
  `Layout` from an already-resolved `Layout` **without spawning git**, with a
  documented "input must be a worktree root" precondition.
- Replace the per-iteration `hubgeometry.Resolve(hostPath)` call in
  `internal/warpengine/status.go` (`Status`) and
  `internal/warpengine/reconcile.go` (`Reconcile`) loops with the new method,
  **guarded** by a `filepath.Dir(hostPath) != l.Hub` check that falls back to
  `hubgeometry.Resolve` for any non-hub-sibling (out-of-hub raw) worktree root.
- A `hubgeometry` equivalence test (new method ≡ `Resolve(root)`).
- A trace-based spawn-count regression guard proving resolve spawns do **not**
  scale with worktree count in `Status`/`Reconcile`.
- A dated Linux benchmark/census block in
  `docs/benchmarks/fixture-copy.md` recording before/after spawn counts and the
  `Resolve`=94 correction.

**Out:**

- **Angle 2 (test consolidation) is dropped from this task.** Its only payoff is
  test wall-clock, which is invisible on Linux; consolidating tests blind here
  risks trading coverage for an unmeasurable gain. If still wanted, it is a
  separate task to be run and measured on the Windows machine.
- `internal/warpengine/clone.go`'s single `Resolve` (not in a loop) and
  `internal/warpcli`'s one-`Resolve`-per-command calls — not redundant, untouched.
- `drift.go`'s direct `rev-parse --abbrev-ref HEAD` branch reads — real work,
  untouched. No change to `readBranch`, weft-wiring, or exclude-seeding spawns.
- No Windows benchmark run required; Windows impact is projected analytically
  (spawns removed × measured per-spawn cost).
- No new production observability seam in `gitexec` (the spawn-count guard uses
  `GIT_TRACE2_EVENT`, not a production hook).

## Decisions

### narrow-to-angle-1

- Decision: Ship only the redundant-`Resolve` removal in `status.go` +
  `reconcile.go`; drop test consolidation.
- Rationale: Linux shows no test-speed problem (0.92 s); angle 1 is
  OS-independent production/code-quality value verifiable by spawn count, angle 2
  is Windows-only test-speed that cannot be measured or validated from Linux.
- Rejected: Doing both angles (angle 2 unmeasurable here); deferring the whole
  task (angle 1 is a real, low-risk clean-up worth landing now).

### sibling-layout-method

- Decision: Add `func (l *Layout) SiblingLayout(worktreeRoot string) *Layout` to
  `internal/hubgeometry/hubgeometry.go`, returning
  `{Cwd: c, WorktreeRoot: c, Hub: l.Hub, RelPath: ".", Prime: l.Prime}` where
  `c := filepath.Clean(worktreeRoot)` — no git spawn. It **cleans its input via
  `filepath.Clean`** to match `Resolve` exactly (`Resolve` sets
  `Cwd = filepath.Clean(cwd)` and `WorktreeRoot = filepath.Clean(rev-parse
  output)`); this removes any reliance on callers pre-cleaning the path.
  **Precondition (godoc): `worktreeRoot` must be an actual worktree root** (as
  returned by `hubgeometry.List`), not a subpath — `RelPath: "."` is only correct
  for a root; a subpath argument would silently mis-derive `RelPath` (unlike
  `Resolve`, which computes it). Callers only pass `List` roots.
- Rationale: In `Status`/`Reconcile`, the loop iterates worktree roots already
  enumerated by one `hubgeometry.List(l.WorktreeRoot)` call. `Resolve(hostPath)`
  then re-spawns `rev-parse --show-toplevel` (root already known) **and** a second
  `worktree list` (via `Resolve`→`List`). For a hub-sibling host worktree root,
  `Resolve` yields exactly `Cwd=root`, `WorktreeRoot=root`,
  `Hub=filepath.Dir(root)` (== `l.Hub`, since hosts are siblings in the hub),
  `RelPath="."`, and `Prime` == `l.Prime` (repo-global; identical for every
  sibling). So the derivation is provably equivalent and needs no spawn. This is
  *not* a cache — no retained state, no staleness surface; it derives within one
  operation's consistent worktree-list snapshot.
- **Non-sibling guard (byte-for-byte closure):** `Resolve(root)` sets
  `Hub = filepath.Dir(root)`, whereas `SiblingLayout` sets `Hub = l.Hub`. These
  diverge only for a worktree root that is **not** a direct child of `l.Hub` —
  i.e. a non-lyx raw `git worktree add /elsewhere` outside the hub (which
  `Reconcile`'s raw-adoption path can encounter). Every lyx-created worktree is a
  hub sibling regardless of subpath `lyx init` (subpath init is additive/local —
  it only affects where a `_lyx` junction sits *inside* a worktree; the worktree
  **root** stays `<hub>/<name>`, so `filepath.Dir(root) == l.Hub` always holds).
  To keep the equivalence absolute even for the pathological out-of-hub case, the
  guard is `if filepath.Dir(worktreeRoot) != l.Hub` → fall back to
  `hubgeometry.Resolve(worktreeRoot)` (the spawning path); else use
  `SiblingLayout`. Non-sibling worktrees (rare/degenerate) take the slow path;
  siblings take the fast path. Since `Hub`, `WeftWorktree()`, `WeftLyxDir()`, and
  `WeftRepoRoot()` all derive from `Hub`, this makes the junction-health verdict
  and weft target byte-for-byte identical to today for every entry.
  **Factoring:** the guard is lifted into **one unexported warpengine helper**
  (both call sites are warpengine — `status.go` and `reconcile.go`), not inlined
  at each site, e.g. `func hostLayoutFor(l *hubgeometry.Layout, root string)
  (*hubgeometry.Layout, error)` returning `(l.SiblingLayout(root), nil)` for a
  hub sibling and `hubgeometry.Resolve(root)` otherwise. It returns an `error` so
  the existing per-iteration `Resolve`-error handling at each call site is
  preserved unchanged (`SiblingLayout` never errors → `nil`). The helper lives in
  warpengine, keeping `SiblingLayout` a pure, always-no-spawn primitive and the
  fallback *decision* a warpengine concern; it constructs no geometry tokens, so
  the Hub Geometry Invariant is unaffected.
- Rejected: A keyed/memoized `Resolve` cache (staleness risk, overkill for a
  within-operation derivation); a free function `LayoutForKnownRoot(root, prime)`
  (the method on `Layout` reuses the already-resolved `Hub`+`Prime` and reads
  more naturally at the call site); threading `Layout` through every warpengine
  op entrypoint (unnecessary — only these two loops re-resolve); documenting
  non-sibling worktrees as out-of-scope without a guard (leaves a latent
  behavioral divergence — the guard costs two lines and closes it).

### spawn-count-regression-guard

- Decision: Add a trace-based guard test: run `Status` (and `Reconcile`) over a
  fixture hub at **two worktree counts** under `GIT_TRACE2_EVENT`, and assert the
  count of `git rev-parse --show-toplevel` invocations **does not grow** between
  them (the non-scaling property the guard exists to lock). Do not pin a single
  constant: post-fix, `--show-toplevel` from these all-sibling loops is exactly 0
  (`List` spawns `worktree list --porcelain`, not `--show-toplevel`), so a single-N
  `== 0` assertion is brittle and its "bounded by one-time `List`" rationale is
  imprecise — the two-N non-growth assertion is what actually fails if
  per-iteration `Resolve` is reintroduced (it would make the count scale with N).
- Rationale: Locks in the win and prevents a future edit from silently
  reintroducing per-iteration `Resolve`. Uses `GIT_TRACE2_EVENT` → a temp trace
  dir, parsed in-test; no production code change. GIT_TRACE2 passes through the
  hermetic git env (verified — `lyxtest` does not clear it).
- Rejected: Equivalence test alone (no permanent regression lock); a production
  spawn-counter hook in `gitexec` (adds a production var solely for tests).

### record-on-linux

- Decision: Record a dated Linux block in `docs/benchmarks/fixture-copy.md`:
  before-census (1,435 spawns, `--show-toplevel` 94), after-census (re-run
  post-change), the `Resolve`=94-not-401 correction, and the note that the
  Windows premise was re-measured on Linux and largely evaporated. Windows impact
  is projected analytically, not measured.
- Rationale: `fixture-copy.md` is the census home; the correction belongs next to
  the original claim. Linux Tier 2 timing won't move measurably (0.92 s), so no
  `test-suite-timing.md` block is warranted.
- Rejected: Requiring a real Windows before/after run (blocks the task on the
  other machine); adding a `test-suite-timing.md` block (no measurable Linux
  timing delta to record).

## Technical context

- **`internal/hubgeometry/hubgeometry.go`** — `Resolve(cwd)` (lines ~98–143) runs
  `git rev-parse --show-toplevel` then `List(cwd)` (which runs
  `git worktree list --porcelain`), and computes `Cwd/WorktreeRoot/Hub/RelPath/
  Prime`. The new `SiblingLayout` method sits alongside the existing `Layout`
  methods. `List` lives in `internal/hubgeometry/worktreelist.go`.
- **`internal/warpengine/status.go`** — `Status(l *hubgeometry.Layout)`: calls
  `hubgeometry.List(l.WorktreeRoot)` once, loops entries, and at ~line 129 calls
  `hubgeometry.Resolve(hostPath)` per entry to build `hostLayout` for
  `PairInSync(hostLayout)`, `hostLayout.HostLyxLinkHere()`,
  `hostLayout.WeftLyxDir()`. Replace with `l.SiblingLayout(hostPath)`.
- **`internal/warpengine/reconcile.go`** — `Reconcile(l *hubgeometry.Layout)`:
  same shape, `hubgeometry.Resolve(hostPath)` at ~line 113. Replace with
  `l.SiblingLayout(hostPath)`.
- **`internal/gitexec/gitexec.go`** — `RunGit(args, cwd)` is the single git-spawn
  chokepoint; no test seam today (and none added).
- **Census reproduction** — from repo root:
  `GIT_TRACE2_EVENT=<dir> go test -tags integration -count=1 ./internal/warpengine`
  produces one trace file per git process; count `"event":"cmd_name"` by `"name"`,
  and split `rev-parse` by the `"argv"` in each file's start event. Baseline this
  run: 1,435 processes; `rev-parse` 398 (of which `--show-toplevel` 94), worktree
  229, branch 104, reset 68.
- **Behavior must be byte-for-byte unchanged.** `SiblingLayout(root)` must produce
  the same `Layout` `Resolve(root)` did for a worktree root, so `Status`/
  `Reconcile` JSON output is identical. These loops are **root-scoped**: they pass
  the worktree *root* (not a subpath) to the resolver, so the host layout already
  operates at `RelPath="."` today, and `SiblingLayout` uses `RelPath="."` to match
  exactly. Per-subpath `_lyx` junctions (from a subpath `lyx init`) are not
  enumerated by these root-scoped loops — that is pre-existing behavior, unchanged
  here. The `filepath.Dir(root) != l.Hub` guard (see the sibling-layout-method
  decision) preserves equivalence for the one case where `Hub` would otherwise
  diverge (an out-of-hub raw worktree).

## Constraints

- **Hub Geometry Invariant** (`CONSTRAINTS.md`): all worktree-root/geometry
  resolution stays in `internal/hubgeometry`; geometry tokens (`_lyx`, `-weft`,
  `-HUB`, `_portals`, `_launchers`, `_board`, `_raddle`) are owned solely there.
  `SiblingLayout` living in `hubgeometry` satisfies this; the warpengine call
  sites only consume the returned `Layout`. Enforced by
  `internal/hubgeometry/enforcement_test.go`.
- **Weft Git Invariant**: all weft git goes through the engines — unchanged (no
  git-call routing changes here).
- **Parallel isolation / junction coverage**: unchanged; no test fixture topology
  changes. Windows junction paths are untouched (geometry values are identical).
- **Documentation Lifecycle** (`CLAUDE.md`): update the `hubgeometry` module doc
  if the change touches its documented API (it adds one method); record the
  benchmark block in `docs/benchmarks/fixture-copy.md` in the same commit. No new
  cross-cutting invariant is introduced.

## Testing

- **Tagging (applies to both new tests):** both go in `//go:build integration`
  files — the equivalence test calls `Resolve` (spawns git) and the guard runs
  `Status`/`Reconcile` over a fixture hub, so an untagged placement trips the Test
  Tier Purity guard (`cmd/lyx/tierpurity_test.go`). Do **not** target
  `hubgeometry`'s untagged `hubgeometry_unit_test.go`. Both packages already carry
  a `HermeticGitEnv` `TestMain`, so no new test infra is needed.
- **`internal/hubgeometry` — equivalence unit test (TDD candidate, mandatory):**
  over a real fixture worktree (or hub with a known root), assert
  `l.SiblingLayout(root)` returns a `Layout` deep-equal to `hubgeometry.Resolve(
  root)`. Write this first; it is the safety net proving the pure derivation
  matches the spawning path. Cover: main worktree root, a sibling child worktree
  root, and **all four `Layout` fields explicitly** (`Cwd`, `WorktreeRoot`, `Hub`,
  `Prime`, `RelPath`) — not just `Hub`/`Prime`/`RelPath` — so the input-cleaning
  and `WorktreeRoot` derivation are pinned too. Also cover **a non-hub-sibling
  root** (out-of-hub worktree) pinning that the guarded call site falls back to
  `Resolve` (i.e. that `SiblingLayout` and `Resolve` diverge there, which is
  exactly why the guard exists).
- **`internal/warpengine` — spawn-count regression guard (TDD candidate):** run
  `Status` (and `Reconcile`) over a fixture hub at **two hub-sibling worktree
  counts** (e.g. N=2 and N=4) under `GIT_TRACE2_EVENT` pointed at `t.TempDir()`,
  and assert the `rev-parse --show-toplevel` spawn count **does not grow** between
  the two runs (it is 0 in both post-fix). This non-scaling assertion is the guard
  that fails if per-iteration `Resolve` is reintroduced (which would make the count
  scale with worktree count); a single-N constant would be brittle.
- **`internal/warpengine` — existing `Status`/`Reconcile` behavior tests:** must
  still pass unchanged (identical JSON output). Run the full
  `go test -tags integration ./internal/warpengine` before/after and confirm the
  test-name set and results are identical.
- **Census verification:** re-run the trace census after the change and confirm
  the `--show-toplevel` count dropped by the per-iteration resolves removed from
  `Status`/`Reconcile` (record the exact delta in `fixture-copy.md`).

## Q&A log

- **Q:** Given Linux runs warpengine Tier 2 in 0.92 s, does the test-speed
  premise still hold? **A:** No — spawn *count* is OS-invariant but per-spawn cost
  is ~0.6 ms on Linux; the test-speed motivation is Windows-only.
- **Q:** Is `hubgeometry.Resolve` really ~401 `rev-parse` spawns? **A:** No — the
  Linux census shows `Resolve` (`--show-toplevel`) is 94; the rest are
  per-operation branch reads, weft geometry helpers, and test `--verify`
  assertions.
- **Q:** Is "Resolve-caching" worth it, and won't a cache go stale? **A:** The
  fix is not a cache — it derives a sibling `Layout` from an already-resolved one
  within a single operation (no retained state, no staleness). Worth it as a
  small, clean production-latency win for the scan-loop commands.
- **Q:** Aren't the slow Win11 git operations the real cost? **A:** Yes, and
  `worktree add`/`remove`/`push` are irreducible; this task only removes the
  *redundant* resolve spawns in two read-only scan loops — a modest, honest win.
- **Q:** Do both angles, or narrow? **A:** Narrow to angle 1; split angle 2 out to
  a Windows session where its test-speed payoff is measurable.
- **Q:** Regression lock on spawn count? **A:** Yes — a trace-based guard
  asserting resolves don't scale with worktree count, plus the equivalence test
  (no `gitexec` production seam).
- **Q:** Where to record the benchmark/finding? **A:** A dated Linux block in
  `docs/benchmarks/fixture-copy.md`; skip `test-suite-timing.md` (no measurable
  Linux timing delta).
- **Q:** (Review round 1 gap) `SiblingLayout` sets `Hub: l.Hub` while
  `Resolve(root)` sets `Hub = filepath.Dir(root)` — these diverge for a worktree
  not under `l.Hub`, breaking byte-for-byte. How to resolve? **A:** Guard the call
  sites: `if filepath.Dir(root) != l.Hub` fall back to `Resolve(root)`, else use
  `SiblingLayout`. Every lyx worktree is a hub sibling (confirmed: subpath
  `lyx init` is additive/local and never moves the worktree root), so only an
  out-of-hub raw worktree hits the fallback; the guard makes equivalence absolute
  at trivial cost. An equivalence-test non-sibling case pins it.
