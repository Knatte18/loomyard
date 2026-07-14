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
  `Layout` from an already-resolved `Layout` **without spawning git**.
- Replace the per-iteration `hubgeometry.Resolve(hostPath)` call in
  `internal/warpengine/status.go` (`Status`) and
  `internal/warpengine/reconcile.go` (`Reconcile`) loops with the new method.
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
  `{Cwd: worktreeRoot, WorktreeRoot: worktreeRoot, Hub: l.Hub, RelPath: ".",
  Prime: l.Prime}` — no git spawn.
- Rationale: In `Status`/`Reconcile`, the loop iterates worktree roots already
  enumerated by one `hubgeometry.List(l.WorktreeRoot)` call. `Resolve(hostPath)`
  then re-spawns `rev-parse --show-toplevel` (root already known) **and** a second
  `worktree list` (via `Resolve`→`List`). For a host worktree root, `Resolve`
  yields exactly `Cwd=root`, `WorktreeRoot=root`, `Hub=filepath.Dir(root)` (==
  `l.Hub`, since hosts are siblings in the hub), `RelPath="."`, and `Prime` ==
  `l.Prime` (repo-global; identical for every sibling). So the derivation is
  provably equivalent and needs no spawn. This is *not* a cache — no retained
  state, no staleness surface; it derives within one operation's consistent
  worktree-list snapshot.
- Rejected: A keyed/memoized `Resolve` cache (staleness risk, overkill for a
  within-operation derivation); a free function `LayoutForKnownRoot(root, prime)`
  (the method on `Layout` reuses the already-resolved `Hub`+`Prime` and reads
  more naturally at the call site); threading `Layout` through every warpengine
  op entrypoint (unnecessary — only these two loops re-resolve).

### spawn-count-regression-guard

- Decision: Add a trace-based guard test: run `Status` (and `Reconcile`) over a
  fixture hub with N host worktrees under `GIT_TRACE2_EVENT`, assert the count of
  `git rev-parse --show-toplevel` invocations does **not** grow with N (O(1), not
  O(N)).
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
  `Reconcile` JSON output is identical. Note the existing code already operates at
  `RelPath="."` for host layouts (it passes the worktree *root*, not a subpath, to
  `Resolve`), so `SiblingLayout` uses `RelPath="."` to match exactly.

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

- **`internal/hubgeometry` — equivalence unit test (TDD candidate, mandatory):**
  over a real fixture worktree (or hub with a known root), assert
  `l.SiblingLayout(root)` returns a `Layout` deep-equal to `hubgeometry.Resolve(
  root)`. Write this first; it is the safety net proving the pure derivation
  matches the spawning path. Cover: main worktree root, a sibling child worktree
  root, and the `Hub`/`Prime`/`RelPath` fields explicitly.
- **`internal/warpengine` — spawn-count regression guard (TDD candidate):** build
  a fixture hub with ≥2 host worktrees, run `Status` (and `Reconcile`) under
  `GIT_TRACE2_EVENT` pointed at `t.TempDir()`, and assert the number of
  `rev-parse --show-toplevel` spawns is constant w.r.t. worktree count (e.g.
  bounded by the one-time `List` setup, not `≥ N`). This is the guard that fails
  if per-iteration `Resolve` is reintroduced.
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
