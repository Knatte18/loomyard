# fabric — fixer report, round 5 (`fable-high-r5`)

Companion to `_mill/fabric-review-fable-high-r5.md`. All four recorded findings fixed; nothing
deferred. Commit-per-fix on branch `fabric-crucible-hardening`, no push.

## What was implemented

### F1 (MEDIUM) — createGitWorktree symlink-directed-write escape

New helper `containedWorktreeAdd` in `internal/fabricengine/destroy.go`. `git worktree add` resolves
and follows a symlink standing at its destination-path argument, so `os.Root` cannot reach through the
subprocess the way `removeContainedPath` reaches through `os.Remove` on the delete side. The escape is
closed structurally instead:

1. `mkWorktreeStagingDir` creates a random-named staging PARENT directory through an `os.Root` rooted
   at the container (openat-atomic; refuses an intermediate-symlink escape; the crypto/rand name is
   unnameable by an adversary).
2. `git worktree add` writes the worktree to `<staging-parent>/<target-base>` — an in-container path
   the adversary cannot target. The leaf keeps target's base name so git's internal admin dir
   (`<gitdir>/worktrees/<name>`) is named after the slug, not the staging token.
3. `os.Root.Rename(stagingRel, targetRel)` moves the worktree to the real target. `renameat` refuses
   to follow a symlink planted at target (ENOTDIR), so the one adversary-controllable path is touched
   solely by an operation that cannot escape.
4. `git worktree repair <target>` rewrites git's registration to name target.

`createGitWorktree` now takes `(rec, repoDir, container, target, buildArgs func(path) []string)` and
delegates to `containedWorktreeAdd`; `add.go`'s warp-side call site passes a closure embedding the
staging path git is handed, never target. Failure paths clean up the staging tree (RemoveAll through
the root; `git worktree remove --force` for a registered staging worktree on a rename failure).

Why it closes rather than narrows: git's WRITE only ever targets a path an adversary cannot name or
symlink-redirect. The sole residual (target swapped to a symlink between rename and repair) causes at
most a failed repair and rolled-back add — repair writes only pointer files, never a worktree tree —
so no worktree is ever written outside the container.

- Files: `internal/fabricengine/destroy.go`, `internal/fabricengine/add.go`,
  `internal/fabricengine/create_containment_toctou_integration_test.go` (new),
  `internal/fabricengine/doc.go`, `CONSTRAINTS.md`.
- Verified: live re-attack 1200 toggle-race trials → 0 escapes (pre-fix escaped at attempt 12);
  new integration test + sabotage proof; full integration suite + 4× concurrent amplifier green.

### F2 (LOW) — createExclusiveDir intermediate-symlink escape

`createExclusiveDir` now creates its leaf through an `os.Root` rooted at `filepath.Dir(path)` instead
of a bare `os.Mkdir`, so an intermediate-symlink escape is refused at mkdir time (a leaf symlink was
already EEXIST-safe; the gap was an intermediate ancestor component). EEXIST semantics preserved (the
token is still only minted for a directory this call brought into being). Fixed in the same commit as
F1 (shared create-side-containment hunk and docs).

- File: `internal/fabricengine/destroy.go`. Verified: clone integration tests (which drive
  createExclusiveDir for the hub) green; direct filesystem experiment confirmed os.Root refuses the
  escape.

### F3 (LOW) — non-gated weft/board/reconcile worktree-add sites

Routed the four remaining bare `git worktree add <target>` sites through `containedWorktreeAdd`:
`createWeftWorktree` (weftwiring.go), Add's weft-adopt branch (add.go), `adoptWeftWorktree`
(reconcile.go), and `ensureBoardWorktree`'s adopt+orphan branches (boardweft.go). Same escape class,
same helper; recording is unchanged (each caller still hand-records its own mutation kinds).

- Files: `internal/fabricengine/weftwiring.go`, `add.go`, `reconcile.go`, `boardweft.go`.
- Verified: full integration suite green; live clone+add+remove+reconcile end-to-end (weft/board
  worktrees land correctly, admin dirs named after slugs, no debris).

### F4 (NIT) — rollbackAdd WARN-log regression test

Added `TestAddRollback_RefusedWarpBranchDeletionLogsWarn` (in `add_rollback_adopt_test.go`) capturing
the logger sink via `logger.SetOutput` and asserting the specific WARN line fires (naming the branch
and the ownership check) when the gate refuses the bare-slug branch deletion. Non-parallel by design
(owns the process-global sink). Sabotage-proved: removing rollbackAdd's WARN hunk fails it.

- File: `internal/fabricengine/add_rollback_adopt_test.go`.

## Deferred

None. Every recorded finding, all severities, was fixed.

## Test commands run (all green)

- `go build ./...`; `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...`
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5`
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -count=1`
- 4× concurrent compiled integration binary, `-test.parallel=8` — all rc=0, no markers.
- Guards: destructive-bypass, mutation-record, markdown-links, fabric-vocabulary, hermetic-env,
  tier-purity — all green.
- Sabotage proofs for F1 and F4 (both fail on production-hunk revert).
- Live driving (dev binary via `./deploy-dev`): F1 toggle-race re-attack (0/1200); clone+add+remove+reconcile happy path.

## SANDBOX-FABRIC-SUITE

No new live/visual behavior needs a suite scenario: the create-side containment property is a
non-visual security invariant exercised deterministically by the new integration tests and the
existing add/clone/reconcile suite coverage. Noted here rather than extending the suite.

## Changed files

- `internal/fabricengine/destroy.go` — containedWorktreeAdd + mkWorktreeStagingDir; createExclusiveDir/createGitWorktree rework; header doc.
- `internal/fabricengine/add.go` — warp create + weft-adopt call sites route through the helper.
- `internal/fabricengine/weftwiring.go`, `reconcile.go`, `boardweft.go` — remaining worktree-add sites routed through the helper.
- `internal/fabricengine/doc.go` — create-side containment twin documented.
- `CONSTRAINTS.md` — Fabric Destruction Chokepoint Invariant containment bullet extended.
- `internal/fabricengine/create_containment_toctou_integration_test.go` (new) — F1 guard + happy path.
- `internal/fabricengine/add_rollback_adopt_test.go` — F4 WARN-log guard.
- `_mill/fabric-review-fable-high-r5.md`, `_mill/fabric-review-fable-high-r5-fixer-report.md` — deliverables.
