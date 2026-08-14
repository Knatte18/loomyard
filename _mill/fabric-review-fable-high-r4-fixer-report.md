# fabric — fixer report, round 4 (`fable-high-r4`)

Companion to `_mill/fabric-review-fable-high-r4.md`. This round found 5 findings (4 LOW + 1 NIT, 0 BLOCKING/MEDIUM) and fixed 4 of them in code + docs; the 5th is a documented design tradeoff recorded with its reason. Commit-per-fix on the `fabric-crucible-hardening` branch, no push.

## What was implemented

### F1 (LOW) — `applyStaleRemoval` reports only convergence that landed
Commit: `fabric: fix F1 — applyStaleRemoval reports only convergence that landed`.
`internal/fabricengine/reconcile.go`: the per-name stale-removal loop now `unseedGitExclude`s and tallies a junction into `removed` ONLY after a nil-error removal. A refused OR operational-failure removal is logged (distinctly) and skipped — neither strips the still-present junction's `.git/info/exclude` entry (which left it as untracked dirt) nor counts it as removed. The `removed`-detail append and the `Action`→`stale_removed` flip now run only when at least one junction actually came off disk. Test: `internal/fabricengine/reconcile_stale_removal_test.go`'s new `TestReconcile_RefusedStaleRemovalReportsNothing` plants a drifted stale junction (fabric-owned but `RawTarget`-mismatched, so `ownedWiredJunction` refuses), and asserts Action stays `already_healthy`, no removed-detail, junction still present, and a clean `git status` (exclude preserved). **Sabotage-proved:** reverting the reconcile.go hunk makes the test fail at all three assertions (`Action=stale_removed`, empty removed-detail, and `?? _other` untracked dirt).

### F2 (LOW) — surface `rollbackAdd`'s swallowed warp-branch-deletion refusal + honest doc
Commit: `fabric: fix F2 — surface rollbackAdd's swallowed warp-branch-deletion refusal, correct over-claiming doc`.
`internal/fabricengine/add.go`: under the default empty `branch_prefix` the gate cannot prove the bare-slug warp branch is fabric's, so step-5 `deleteBranch` is refused and the branch is left behind. That refusal was captured into a `firstErr` every caller discards, so it never reached the trace. Now `logger.Warn`ed (mirroring `rollbackSwitch`'s identical best-effort-void handling). The file header and `rollbackAdd`'s doc are corrected: the "full rollback ... never leave partial state" claim now states the warp branch is left behind under an empty prefix, self-healing via the "already exists" remedy Add's own re-add error names. Test: `internal/fabricengine/add_rollback_adopt_test.go`'s new `TestAddRollback_WarpBranchLeftBehindUnderEmptyPrefix` forces a post-creation Add failure (portal blocker) under the default prefix and asserts the worktree pair is rolled back while the bare-slug warp branch is left behind. (Confirmed live during Job 1: a failed `lyx fabric add` left branch `my-task` behind and blocked re-add.)

### F3 (LOW) — correct round-3 fixer report's overstated M1 coverage (carried item 1)
Commit: `fabric: fix F3 — correct round-3 fixer report's overstated M1 integration-test coverage claim`.
`_mill/fabric-review-fable-high-r3-fixer-report.md`: appended a Round-4 correction. Traced that `destroy_containment_toctou_integration_test.go` plants an already-live escaping symlink, which `checkPathRequest`→`containmentFailure`→`containmentPath` refuses at the check phase (M3, round 2) before M1's `os.Root` act is reached — so reverting M1's production code alone leaves that test green. The hermetic `TestRemoveContainedPath_RefusesEscapingIntermediate` is M1's sole regression guard. No production behaviour affected. Chose option (b) (correct the claim) over (a) (a genuine M1-specific live test), because the window is closed by design and the deterministic unit test already sabotage-proves M1 authoritatively.

### F4 (LOW) — Add's dir-exists error names the leftover-cleanup remedy (carried item 4)
Commit: `fabric: fix F4 — Add's dir-exists error names the leftover-cleanup remedy`.
`internal/fabricengine/add.go`: a leftover directory stranded at `<hub>/<slug>` by an interrupted remove/reconcile blocks a re-add yet is invisible to `list`/`prune` (which enumerate only git-registered worktrees). The dir-exists error now names the recovery (different slug for a live pair, or remove the leftover directory and retry) instead of a bare "already exists". Test: `internal/fabricengine/add_branch_exists_test.go`'s new `TestAdd_LeftoverWorktreeDirErrorNamesRemedy` plants a bare directory (no branch) and asserts the guidance substrings surface. Reasoning basis: `add.go`'s `os.Stat` guard blocks re-add, while every List-based verb only sees git-registered worktrees — so round 2's "blocks nothing" is inaccurate for that placement.

### P2 / carried item 3 — create-side symlink-directed write: CONFIRMED NOT A DEFECT (no fix)
Live-verified: `createExclusiveDir`'s `os.Mkdir` refuses any planted symlink via EEXIST; `createGitWorktree`'s non-racing cases are refused (symlink→dir caught by `os.Stat`-follow → "already exists"; dangling symlink → git's own lstat refusal). Only a same-process [`os.Stat`, `git worktree add`] race window remains, with no concurrent fabric writer expected at the unique slug path — the identical accepted-residual class as N4's dirtiness-probe TOCTOU. No code change.

## Deliberately deferred (with reasons)

### F5 (NIT) — symlink-loop / operational failure at a launcher path is best-effort-swallowed (carried item 2)
Not changed. `surfaceRefusal`'s documented policy is that an operational failure (git nonzero, filesystem said no — including an ELOOP at a launcher path) stays discardable while only a gate refusal is non-discardable. `Remove` therefore reports `ok:true`/`partial:false` and leaves that one launcher entry unremoved. This is the module's uniform, documented tradeoff (doc.go, `surfaceRefusal`), narrower in scope than M2 (which was about a *refusal*, not an operational failure). Making launcher ELOOP specifically surface would be inconsistent with every other best-effort operational swallow in the module and is a broader behaviour change than this final round should make. Reason for leaving it: it is an intended documented design policy applied uniformly, not a local defect; changing it safely is a module-wide error-surfacing decision beyond a hardening round's scope.

### F2's deeper functional fix — a "freshly-created branch" ownership token
Not implemented. A functional fix that would let `rollbackAdd` delete the bare-slug warp branch under an empty prefix requires a new branch-ownership kind (analogous to `createdToken` for paths/worktrees), touching the closed `branchOwnershipKind` enum, `resolveBranchOwnership`, and the guard test's companion table. Reason for leaving it: the leftover branch is recoverable via a clearly-named remedy and the gate's conservative refusal (never delete a branch it cannot prove is fabric's) is correct-by-design; introducing a new ownership kind is a larger change than a final hardening round should make when the surgical fix (surface the refusal + honest doc) fully closes the actual defect (the silence + the over-claim).

## Tests run + results

- `go build ./...`: clean.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...`: clean.
- Hermetic `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=3`: all packages `ok`.
- Live integration `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... -count=1`: `ok` (fabricengine 16.8s, fabriccli 1.6s).
- 4× concurrent full integration suites (`-test.parallel=8`): all 4 `PASS`, no `FAIL`/`being used by another process`/`permission denied`/`panic:` markers.
- Guard tests `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` + `TestTierPurity_UntaggedTestsSpawnNothing`: `ok`.
- Each new test sabotage-checked where meaningful (F1 explicitly reverted-and-confirmed-failing; F2/F4 assert the intended observable behaviour and the WARN log surfaced under the concurrent amplifier).
- Live driving after redeploy (`./deploy-dev`): full clone→pairs→add→list→status→reconcile→prune→cleanup sweep clean; failed-Add path live-confirmed F2's WARN log and leftover branch.

## Changed files

- `internal/fabricengine/reconcile.go` (F1)
- `internal/fabricengine/reconcile_stale_removal_test.go` (F1 test)
- `internal/fabricengine/add.go` (F2 + F4)
- `internal/fabricengine/add_rollback_adopt_test.go` (F2 test)
- `internal/fabricengine/add_branch_exists_test.go` (F4 test)
- `_mill/fabric-review-fable-high-r3-fixer-report.md` (F3 correction)
- `_mill/fabric-review-fable-high-r4.md`, `_mill/fabric-review-fable-high-r4-fixer-report.md` (deliverables)

No `SANDBOX-FABRIC-SUITE.md` change: none of the findings is a visual/interactive scenario the black-box suite exercises — all are error/repair-path honesty and message-guidance gaps, covered by the deterministic tests above.

No `doc.go`/`docs/overview.md`/`CONSTRAINTS.md`/`roadmap.md` change: no invariant, module-table entry, or observable-CLI contract moved. F1/F2/F4 are honesty/operability fixes on existing error paths; the doc corrections that were needed live in the fabric source file headers (add.go) and the round-3 report (F3), updated in the same commits as their fixes.

## Merge-readiness

MERGEABLE. No BLOCKING or MEDIUM findings; the four LOW findings and one NIT were error/repair-path honesty and operability gaps, all fixed (4) or recorded as documented tradeoffs (1) with regression tests where practical. The destruction chokepoint's containment/TOCTOU property, ownership predicates, and concurrent-race behaviour were spot-checked and hold. Standing limit, unchanged from every prior round: Windows path/junction behaviour is out of scope (unreachable from a Linux host).
