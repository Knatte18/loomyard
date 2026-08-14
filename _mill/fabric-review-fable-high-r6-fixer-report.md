# fabric — fixer report (round 6, `fable-high-r6`)

Companion to `_mill/fabric-review-fable-high-r6.md`. What was implemented, how it was verified, and
what was deferred.

## Findings and disposition

| ID | Severity | Status | What |
|----|----------|--------|------|
| F1 | MEDIUM | FIXED | create-side staging-leaf observation escape (false success + out-of-hub worktree) |
| NIT-F2 | NIT | FIXED (folded into F1) | `containedWorktreeAdd` repair-failure left a half-placed worktree with a stale registration |
| NIT-F3 | NIT | DEFERRED (with reason) | round-4 F2's WARN-on-refused-branch-deletion has no sabotage-proving test |

## F1 + NIT-F2 — implemented

Single logical change to `containedWorktreeAdd` in `internal/fabricengine/destroy.go`, plus its
regression test and the docs it touches.

Root cause (reproduced 12/12 against a same-UID observing planter, see review): git's `worktree add`
follows a symlink planted at the predictable staging leaf and writes the worktree outside the
container; `os.Root.Rename` renames a symlink standing at the SOURCE's own final component as a link
(it refuses only the destination and intermediate-source cases), so the escaped worktree's symlink was
renamed onto the target and the call returned nil — false success.

Fix shape (kept the two-level staging so the different-UID posture does not regress — a single-level
staging dir is directly rmdir+swap-able under the hub, reproduced 121/600):
- `mkWorktreeStagingDir` now creates the random parent at mode **0700** (was 0755) with a **16-byte**
  random name — a different-UID planter can neither list nor write the leaf.
- After `git worktree add`, a **pre-rename** `stagedWorktreeContained(root, stagingRel)` check (new
  helper) confirms the staging leaf git wrote to is a real directory reached through container's own
  `os.Root` handle without traversing a symlink. A symlink there means git followed a planted link and
  wrote outside — the add fails closed, removing the escaped worktree (`git worktree remove --force`
  through the staging path) and the staging parent, then pruning.
- After `os.Root.Rename`, a **post-rename** `stagedWorktreeContained(root, rel)` check confirms the
  placed target is a real directory. If a leaf was swapped in the pre-check→rename window, its symlink
  was renamed onto target; the add unlinks that symlink via `os.Root.Remove` (which never follows the
  final component) after removing the worktree it points at, then fails closed. Target is left absent,
  never a dangling out-of-hub symlink.
- The success-path staging-parent removal is now `root.RemoveAll` (was `root.Remove`), so a planter
  that polluted the parent with a stray entry cannot leave an empty-but-non-empty staging dir behind —
  `os.Root` removes each entry as a link without following it; the real leaf has already moved to target.
- **NIT-F2:** on `git worktree repair` failure the placed worktree is now removed (`git worktree remove
  --force <target>` + prune) so the caller's rollback sees a clean slate rather than a target whose
  stale registration `git worktree remove` would itself choke on.

Honest residual (documented in doc.go, CONSTRAINTS.md, and the review): a same-UID or root planter
actively racing the add can still make git transiently write a checkout into a directory it already
controls; no staging location prevents that, since such a planter can substitute any path fabric writes
to. The guarantee the fix makes and meets: the add is never REPORTED as success while the worktree
escaped, and never leaves the target a dangling out-of-hub symlink.

### Files changed
- `internal/fabricengine/destroy.go` — `containedWorktreeAdd` rewrite, new `stagedWorktreeContained`
  helper, `mkWorktreeStagingDir` 0700/16-byte, header-comment + function-doc updates.
- `internal/fabricengine/create_containment_toctou_integration_test.go` — new
  `TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap`: a single-plant observing planter races
  `containedWorktreeAdd`, and the loop asserts the never-false-success invariant on every attempt while
  requiring at least one attempt where the planter won and the add failed closed (no escape, no dangling
  target symlink, no staging debris). Polls on actual state (the staging dir appearing) with a deadline;
  no fixed sleeps.
- `internal/fabricengine/doc.go` — "the two CREATE-side minters" paragraph rewritten to describe the
  two fail-closed containment checks and the honest same-UID residual (the old text claimed the random
  staging path + os.Root.Rename alone closed the window, which the false-success repro disproves).
- `CONSTRAINTS.md` — Fabric Destruction Chokepoint Invariant's `containedWorktreeAdd` sentence updated
  to the same, plus the residual note.

### Verification
- `go build ./...`, `go vet ./internal/fabricengine/... ./internal/fabriccli/...` — clean.
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5` — green (includes `destructiveguard_test.go`, `checkedcall_test.go`, help-tree/registration guards).
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...` — green (fabricengine 16.4s; the live-state mutation oracle and prune tests pass unchanged — slug admin naming preserved, zero blast radius).
- `go test ./internal/lyxcwd/ -run 'TestEnforcement_MarkdownLinks|TestEnforcement_FabricVocabulary|TestEnforcement_GeometryLiterals'` — green (doc.go + CONSTRAINTS.md edits pass the vocabulary/link guards).
- New regression test: `-count=20` green; 6 concurrent compiled copies × 5 counts — 0 FAIL.
- **Sabotage-proof:** neutering `stagedWorktreeContained` to `return true` makes
  `TestContainedWorktreeAdd_FailsClosedOnStagingLeafSwap` fail at "false success — nil error but target
  is not a real directory"; restored, green.
- **Live driving (deployed dev binary, real git, local-filesystem remotes):** `lyx fabric clone` →
  ok; `lyx fabric add` → ok with slug-named git admin dir preserved and no staging debris; `lyx fabric
  remove`/`reconcile`/`pairs` → ok. Adversarial: a fast Go planter racing 20 real `lyx fabric add`
  calls on a fresh hub → **20/20 fail-closed, 0 false-success, 0 hub debris, escaped worktrees fully
  removed by the fail-closed cleanup**. (An earlier measurement showed leftover debris — traced to a
  stale staging dir left by the PRE-fix binary that later planters piled onto, not a fix defect; a
  fresh hub with the fixed binary is clean.)

## NIT-F3 — deferred, with reason

Round-4 F2's `rollbackAdd` WARN-on-refused-branch-deletion (`add.go:316-323`) fires live (confirmed in
this round's live driving — the trace shows `msg="...rollbackAdd's warp-branch deletion was refused by
the destructive gate..." check=ownership`) but has no test that sabotage-proves the log line. Deferred
because it is orthogonal to F1's mechanism, is a log-line assertion of the lowest value, and closing it
well needs a `logger`-capture harness this package's create-containment test file does not have — not
something to bolt onto F1's fix without its own focused change. Recorded here so it is not lost; it is a
test-coverage gap, not a behavior defect.

## Teardown
Zero stray git processes. All scratch hubs/locks live under the session scratchpad temp tree
(auto-torn-down); no lock files or processes left outside a temp dir. The dev binary under `.dev-bin`
is gitignored and separate from any prod install.

## Merge-readiness
Mergeable after this fix, subject to: the standing **Windows-path** limit (never executed from a Linux
host); the accepted **N4 dirtiness-probe TOCTOU** residual; and the honest **same-UID/root transient-write**
residual F1 documents (unpreventable by any staging location; the fix guarantees no false success and no
dangling out-of-hub target). NIT-F3 is a deferred test-coverage gap, not a behavior risk.
