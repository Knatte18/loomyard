# Batch: warpengine-stderr-fix

```yaml
task: "CLI ergonomics from the sandbox run: config editor + warp error wrapping"
batch: warpengine-stderr-fix
number: 2
cards: 8
verify: go test ./internal/warpengine/...
depends-on: []
```

## Batch Scope

This batch fixes all 14 raw-git-stderr-leak sites inside `internal/warpengine` (8 files:
`checkout.go`, `add.go`, `cleanup.go`, `clone.go`, `junction.go`, `prune.go`,
`reconcile.go`, `weftwiring.go`) per the `## Shared Decisions` convention in the overview
(never let git's own stderr text reach an error message; compose from local context +
exit code instead). It is its own batch because it is a single, independently-verifiable
module (`go test ./internal/warpengine/...`) with zero file overlap against
`config-set-flag` or `weft-hubgeometry-stderr-fix` — all three batches are root nodes with
no dependency edges between them. One card per file (each file's sites already share
local context — same imports, same `gitexec.RunGit` call shape — so per-file is the
natural "smart unit" split; splitting further, e.g. per call site, would fragment trivially
related edits across cards for no benefit). No batch-local decisions beyond `## Shared
Decisions` in the overview.

## Cards

### Card 8: `checkout.go` — 3 sites (host switch, weft switch, fork weft branch)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/checkout.go`
  - `internal/warpengine/checkout_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Checkout` (line ~88): replace
    `fmt.Errorf("host switch failed: %s", hostSwitchStderr)` with a message composed only
    from the already-in-scope `branch` and `exitCode` variables, e.g.
    `fmt.Errorf("host switch to branch %q failed (git exit %d)", branch, exitCode)`.
  - In `switchOrForkWeft`'s branch-exists path (line ~134): replace
    `fmt.Errorf("weft switch failed: %s", stderr)` with a message from `branch` and
    `exitCode`, e.g.
    `fmt.Errorf("weft switch to branch %q failed (git exit %d)", branch, exitCode)`.
  - In `switchOrForkWeft`'s fork path (line ~165): replace
    `fmt.Errorf("fork weft branch failed: %s", stderr)` with a message from `branch`,
    `parentWeftBranch`, and `exitCode`, e.g.
    `fmt.Errorf("fork weft branch %q from %q failed (git exit %d)", branch, parentWeftBranch, exitCode)`.
  - If any `stderr`/`hostSwitchStderr` variable from `gitexec.RunGit`'s return becomes
    fully unused after these edits, replace its binding with `_` in the corresponding
    `gitexec.RunGit(...)` call to avoid an unused-variable compile error.
- **Tests:** Extend `TestCheckout_HostRollback` in `checkout_test.go` (it already triggers
  the `switchOrForkWeft` branch-exists failure at line ~134, currently asserting only that
  `err != nil`) to additionally assert `err.Error()` contains the target branch name and
  does NOT contain the substring `"fatal:"` nor git's own wording `"already checked out"`
  — mirror `TestResolve_NotAGitRepo`'s pinning style in `hubgeometry_test.go`. Add one new
  small test that triggers the host-switch failure (`Checkout` to a branch name that does
  not exist anywhere, e.g. `"nonexistent-branch-xyz"`) asserting the same no-`"fatal:"`
  pin and that the message contains the branch name. A dedicated test for the
  fork-weft-branch failure (line ~165) is optional — add one only if it is cheaply
  reproducible with the existing `setupCheckoutFixture` helper; if skipped, state the
  one-line reason in `## Batch Tests` below.
- **Commit:** `fix(warpengine): stop leaking git stderr in checkout error messages`

### Card 9: `add.go` — 3 sites (host worktree add, weft worktree adopt, host push)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/add.go`
  - `internal/warpengine/add_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `Add` (line ~147, host worktree creation): replace
    `fmt.Errorf("worktree add failed: %s", stderr)` with a message from `target` and
    `branch` and `exitCode`, e.g.
    `fmt.Errorf("create worktree %q for branch %q failed (git exit %d)", target, branch, exitCode)`.
  - In `Add` (line ~172, weft worktree adopt path): replace
    `fmt.Errorf("weft worktree add (adopt) failed: %s", stderr)` with a message from
    `branch` and `exitCode`, e.g.
    `fmt.Errorf("adopt weft worktree for branch %q failed (git exit %d)", branch, exitCode)`.
  - In `Add` (line ~202, host push): replace `fmt.Errorf("push failed: %s", stderr)` with
    a message from `branch` and `exitCode`, e.g.
    `fmt.Errorf("push branch %q failed (git exit %d)", branch, exitCode)`.
  - Drop any now-fully-unused `stderr` bindings per the same unused-variable rule as
    Card 8.
- **Tests:** Add or extend a test in `add_test.go` asserting the new message text (branch
  name present) and absence of `"fatal:"` for at least one of the three sites — reuse
  whichever existing fixture setup in `add_test.go` (e.g. the one `TestAddRollback`
  already builds) is closest to triggering one of these three failure paths cheaply
  (e.g. a target worktree directory collision for the line ~147 site, or a pre-locked weft
  branch adoption conflict for the line ~172 site). State in `## Batch Tests` below which
  of the three sites got direct test coverage vs. code-inspection-only, if not all three
  are cheaply reproducible with a real git fixture.
- **Commit:** `fix(warpengine): stop leaking git stderr in add error messages`

### Card 10: `cleanup.go` — 2 sites (list weft branches, delete weft branch)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/cleanup.go`
  - `internal/warpengine/cleanup_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `listWeftBranches` (line ~160): replace
    `fmt.Errorf("git branch exited %d: %s", exitCode, stderr)` with
    `fmt.Errorf("list weft branches failed (git exit %d)", exitCode)`.
  - In `deleteWeftBranch` (line ~183): replace
    `entry.Error = fmt.Sprintf("git branch -D %s failed: %s", branch, stderr)` with
    `entry.Error = fmt.Sprintf("delete weft branch %q failed (git exit %d)", branch, exitCode)`.
  - Drop any now-fully-unused `stderr` bindings per the same rule as Card 8.
- **Tests:** Extend whichever of `TestCleanup_ApplySkipsProtectedBranch`,
  `TestCleanup_ApplyForceDeletesTaskBranch`, or `TestCleanup_LiveBranchNeverDeleted` in
  `cleanup_test.go` most directly exercises a branch-delete path (or add a small new test
  that forces `deleteWeftBranch` to fail, e.g. deleting a branch that is checked out
  elsewhere) asserting `entry.Error` contains no `"fatal:"` substring and does contain the
  branch name.
- **Commit:** `fix(warpengine): stop leaking git stderr in cleanup error messages`

### Card 11: `clone.go` — 1 site (repo clone)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/clone.go`
  - `internal/warpengine/clone_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cloneRepo` (line ~133): replace
  `fmt.Errorf("clone failed: %s", stderr)` with a message composed from the function's
  original `url` and `dest` parameters (not the slash-normalized `gitURL`/`gitDest`
  locals, for a more readable message) and `exitCode`, e.g.
  `fmt.Errorf("clone %q to %q failed (git exit %d)", url, dest, exitCode)`. Drop the
  now-fully-unused `stderr` binding per the same rule as Card 8 (note `stdout` is already
  discarded via `_ = stdout` in this function — do not disturb that line).
- **Tests:** Add a test in `clone_integration_test.go` that calls `cloneRepo` (or the
  exported path that reaches it) with an invalid/nonexistent source URL, asserting the
  returned error contains the attempted URL/destination and no `"fatal:"` substring.
- **Commit:** `fix(warpengine): stop leaking git stderr in clone error messages`

### Card 12: `junction.go` — 1 site (git-path resolution)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
  - `internal/warpengine/add_test.go`
- **Edits:**
  - `internal/warpengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedGitExclude` (line ~137): replace
  `fmt.Errorf("git rev-parse --git-path failed: %s", stderr)` with a message from
  `worktreePath` and `exitCode`, e.g.
  `fmt.Errorf("resolve git exclude path for %q failed (git exit %d)", worktreePath, exitCode)`.
  Drop the now-fully-unused `stderr` binding per the same rule as Card 8.
- **Tests:** No dedicated test file exists for `junction.go` (`internal/warpengine` has no
  `junction_test.go`); `seedGitExclude` is exercised only indirectly through `Add`'s happy
  path, which never reaches this failure branch, and there is no fault-injection seam in
  this codebase to force `git rev-parse --git-path` to fail without a corrupted `.git`
  directory. State in `## Batch Tests` below that this site is code-inspection-only —
  acceptable since the change is a single-line message-text swap with no behavior change
  on the (already-covered) success path.
- **Commit:** `fix(warpengine): stop leaking git stderr in junction error messages`

### Card 13: `prune.go` — 1 site (stale weft worktree removal fallback)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/prune.go`
  - `internal/warpengine/prune_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `removeStalePair` (line ~179): replace
  `pe.Error = fmt.Sprintf("git worktree remove failed (%s); fallback os.RemoveAll also failed: %v", stderr, removeErr)`
  with
  `pe.Error = fmt.Sprintf("remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v", weftPath, exitCode, removeErr)`
  — keep `removeErr` as-is (it is a Go `os` error, not git stderr, and is explicitly
  in-scope to retain per the overview's Shared Decision). Drop the now-fully-unused
  `stderr` binding per the same rule as Card 8.
- **Tests:** Extend `TestPrune_StaleWeft` in `prune_test.go`, or add a small new test, that
  forces both the `git worktree remove --force` call AND the `os.RemoveAll` fallback to
  fail (e.g. by locking the weft worktree directory) asserting `pe.Error` contains no
  `"fatal:"` substring. If double-failure is not cheaply reproducible with a real
  filesystem/git fixture, state this in `## Batch Tests` below as code-inspection-only.
- **Commit:** `fix(warpengine): stop leaking git stderr in prune error messages`

### Card 14: `reconcile.go` — 1 site (adopt weft worktree)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/reconcile_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `adoptWeftWorktree` (line ~225): replace
  `fmt.Errorf("git worktree add failed: %s", stderr)` with a message from `weftPath`,
  `branch`, and `exitCode`, e.g.
  `fmt.Errorf("adopt weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)`.
  Drop the now-fully-unused `stderr` binding per the same rule as Card 8.
- **Tests:** Extend the existing reconcile test closest to an adopt-failure path (survey
  `TestReconcile_MissingWeftWorktreeRecreated`, `TestReconcile_BrokenJunctionRepointed`,
  `TestReconcile_RawHostWorktreeAdopted` in `reconcile_test.go` for the nearest analog), or
  add a small new test, asserting no `"fatal:"` substring in the resulting error/report
  field. If not cheaply reproducible, state this in `## Batch Tests` below as
  code-inspection-only.
- **Commit:** `fix(warpengine): stop leaking git stderr in reconcile error messages`

### Card 15: `weftwiring.go` — 2 sites (create weft worktree, push weft branch)

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_test.go`
- **Edits:**
  - `internal/warpengine/weftwiring.go`
  - `internal/warpengine/weftwiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In `createWeftWorktree` (line ~75): replace
    `fmt.Errorf("weft worktree add failed: %s", stderr)` with a message from `weftPath`,
    `branch`, and `exitCode`, e.g.
    `fmt.Errorf("create weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)`.
  - In `pushWeftBranch` (line ~102): replace
    `fmt.Errorf("weft push failed: %s", stderr)` with a message from `branch` and
    `exitCode`, e.g.
    `fmt.Errorf("push weft branch %q failed (git exit %d)", branch, exitCode)`.
  - Drop any now-fully-unused `stderr` bindings per the same rule as Card 8.
- **Tests:** Extend or add a test in `weftwiring_test.go` triggering at least one of these
  two failure paths asserting no `"fatal:"` substring; if both are cheaply reproducible
  with the fixtures already used by `TestWeftForkPointSubtaskIsolation` /
  `TestWeftSpawnPushesWeftBranch`, cover both, otherwise cover one and state the other as
  code-inspection-only in `## Batch Tests` below.
- **Commit:** `fix(warpengine): stop leaking git stderr in weftwiring error messages`

## Batch Tests

`verify: go test ./internal/warpengine/...` runs every test file this batch touches
(`checkout_test.go`, `add_test.go`, `cleanup_test.go`, `clone_integration_test.go`,
`prune_test.go`, `reconcile_test.go`, `weftwiring_test.go`) plus the whole package's
existing suite, catching any regression from the message-text edits across all 8 files.
`junction.go`'s single site (Card 12) has no dedicated test file and no fault-injection
seam in this codebase to force it — that site is verified by code inspection and
compilation only, which the implementer must call out explicitly in its card's commit or
PR notes if any card ends up unable to add direct coverage for a listed site (see each
card's per-site Tests note above for the specific fallback-to-inspection call).
