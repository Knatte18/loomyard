# Batch: branch-callsites

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'branch-callsites'
number: 5
cards: 4
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [3]
```

## Batch Scope

This batch routes all four `git branch -D` sites onto the gate's `deleteBranch` executor, and converts the one remaining worktree-removal site — `rollbackAdd`'s forced warp-worktree removal — onto the gate's worktree-creation token.
It is one batch because all four branch sites declare the same single ownership kind and the same dirtiness member, and because the branch-space logic that protects the primary weft line moves *into* the gate here, which is what makes it apply at the other three sites rather than in `Cleanup` alone.

It depends on batch 3 because both edit `internal/fabricengine/weftwiring.go`.

Batch-local decisions beyond `## Shared Decisions`:

- There is no rollback-specific branch ownership kind.
  An earlier draft had one whose predicate was "the branch was created earlier in this same invocation", which the gate cannot verify and no token backs — the same trust-me the token minters exist to avoid on the path side.
  `ownedManagedBranch` covers all four sites with three checks the gate can actually run.
  The honest trade is that this licenses deleting any fabric-named, non-primary, non-checked-out branch rather than only one this invocation created, which is marginally broader — and is exactly the licence `Cleanup` already exercises.
- Ownership selects which branches are candidates at all;
  it does not license skipping the floor.
  The checked-out check and the primary-weft carve-out apply to every `deleteBranch` call including the two rollback sites.
- The configured branch prefix rides on `ownedManagedBranch`'s own constructor, not on `branchRequest`.
  Every card below therefore threads the prefix to the constructor call, and none of them adds a request field.
  Three of the four sites have no config in scope today and gain a parameter for it;
  the fourth reaches it from its receiver.

## Cards

### Card 23: gate Cleanup's branch deletion

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/worktreelist.go`
- **Edits:**
  - `internal/fabricengine/cleanup.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `deleteWeftBranch`'s `git branch -D` call onto the gate's `deleteBranch` executor, with a `branchRequest` carrying `repoDir` the resolved weft repo root, `branch` the branch name, ownership `ownedManagedBranch(l, branchPrefix)`, dirtiness `dirtyCheckedOutBranch()` and `force` false.
  `deleteWeftBranch` has no config in scope today;
  thread the branch prefix in from `Topology.Cleanup`, which holds it on its own config field, rather than reaching for a package-level default.
  Preserve every existing `entry.Error` string verbatim, including `resolve weft repo root: %v`, `git branch -D %s: %v` and `delete weft branch %q failed (git exit %d): %s`, all now built from the exit code and stderr the executor returns.
  Force is false at this site even when `Topology.Cleanup` was called with force true: `--force` there answers the folded-back-raddle gate and nothing else, and that gate is evaluated before this call is reached.
  It may not answer the primary-weft carve-out and may not answer the checked-out check.
  `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut` pins exactly this and must stay green unchanged.
  Leave the enumeration logic in `Topology.Cleanup` untouched — the `WeftWarpSlug` non-fabric-branch skip, the live-pair skip, the primary-weft carve-out, the checked-out protection and the folded-back gate all still run in both dry-run and apply mode, and the gate cannot replace them because it only ever runs in apply mode.
  The gate is a second, unconditional floor beneath them, and its value is that it now also applies at the three sites below.
- **Commit:** `refactor(fabricengine): route Cleanup's branch deletion through the gate`

### Card 24: gate rollbackSwitch's forked-branch deletion

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/worktreelist.go`
- **Edits:**
  - `internal/fabricengine/checkout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `rollbackSwitch`'s `git branch -D` call — the one deleting the weft branch this `Checkout` forked — onto `deleteBranch`, with a `branchRequest` carrying `repoDir` the weft worktree path this site already passes as the git working directory, `branch` the forked weft branch, ownership `ownedManagedBranch(l, branchPrefix)` with the prefix read from the receiver's config, dirtiness `dirtyCheckedOutBranch()` and `force` false.
  `rollbackSwitch` returns nothing and discards every error today, which is deliberate for its two `git switch` calls and must stay that way for them.
  For the branch deletion it is no longer acceptable: a gate refusal here would vanish entirely.
  Apply the `void` shape from the overview's refusal-surfacing decision — test the returned error with `errors.As` for `*destructiveRefusal` and log it via `logger.Warn` naming the branch and the refusing check, then continue.
  Do not widen `rollbackSwitch`'s signature and do not make it fatal: it runs on paths where `Checkout` is already failing, and turning a best-effort rollback into a hard failure is a behaviour change this slice declared out of scope.
  Leave both `git switch` calls and the "junction stays consistent without rewiring" comment untouched.
- **Commit:** `refactor(fabricengine): route rollbackSwitch's branch deletion through the gate`

### Card 25: gate rollbackAdd's worktree removal and branch deletion

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/worktreelist.go`
  - `internal/fabricengine/weftwiring.go`
- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** three changes in `internal/fabricengine/add.go`, in this order.
  First, in `Topology.Add`, replace the raw warp `git worktree add -b <branch> <target>` call with `createGitWorktree`, keeping the same argument slice and the same working directory, and binding the returned token to a local.
  Keep both existing error messages — the spawn-failure form and the "create worktree %q for branch %q failed (git exit %d): %s" form — built from the exit code and stderr the minter returns.
  Second, give `rollbackAdd` an additional `warpTok createdToken` parameter and pass the token at all of its call sites in `Topology.Add`;
  every one of them is after the worktree creation, so the token is always in scope.
  Convert `rollbackAdd`'s `git worktree remove --force` call onto `removeGitWorktree`, with container the hub path reached through the Location, target the warp worktree path, ownership `ownedFreshlyCreatedWorktree(warpTok)`, dirtiness `dirtinessNA("rollback of the worktree this Add created")` and `force` true.
  Third, convert `rollbackAdd`'s `git branch -D` call onto `deleteBranch`, with `repoDir` the acting warp worktree path this site already passes, `branch` the warp branch, ownership `ownedManagedBranch(l, branchPrefix)` with the prefix read from the receiver's config, dirtiness `dirtyCheckedOutBranch()` and `force` false.
  Preserve `rollbackAdd`'s `firstErr` accumulate-and-continue shape and both existing "git worktree remove failed with exit code %d" and "git branch -D failed with exit code %d" messages.
  Wrap each converted call's result in `surfaceRefusal` before folding it into `firstErr`, so an operational failure keeps today's best-effort behaviour while a refusal is always recorded.
  Leave the existing `!weftBranchAdopted` carve-out that preserves a pre-existing adopted weft branch exactly where it is — it runs ahead of the weft-side deletion and is preserved, not replaced, by the gate.
  Leave the trailing `worktree prune` call alone.
- **Commit:** `refactor(fabricengine): route rollbackAdd's worktree and branch teardown through the gate`

### Card 26: gate the weft branch deletion

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/cleanup.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** convert `removeWeftWorktree`'s `git branch -D` call — reached from `Remove` with the removed pair's weft branch and from `rollbackAdd` with the delete-branch flag set only when this `Add` created it — onto `deleteBranch`, with a `branchRequest` carrying `repoDir` the resolved weft repo root, `branch` the branch parameter, ownership `ownedManagedBranch(l, branchPrefix)`, dirtiness `dirtyCheckedOutBranch()` and `force` false.
  `removeWeftWorktree` has no config in scope;
  add a parameter for the branch prefix rather than reaching for a package-level default, and pass it from both call sites.
  Those two call sites are in `internal/fabricengine/remove.go` and `internal/fabricengine/add.go`, both inside `Topology` methods, so both pass their receiver's configured branch prefix.
  Update both in this card — the signature change leaves the package non-compiling until they are, so they are this card's own work rather than a side effect of the cards that touch those files for other reasons.
  Preserve the existing `firstErr` shape and the existing "git branch -D failed with exit code %d" message.
  Do not change the `deleteBranch` boolean parameter's meaning or its callers' existing carve-out: the flag decides whether a deletion is attempted at all, and the gate decides whether an attempted one is permitted.
  Note that the executor and this function's boolean parameter now share a name;
  rename the parameter to `alsoDeleteBranch` to keep the call sites readable, and update both callers.
- **Commit:** `refactor(fabricengine): route the weft branch deletion through the gate`

## Batch Tests

`verify:` runs both tiers of `internal/fabricengine`.
The integration tier is mandatory here because branch deletion is where the slice's third data-loss defect lived: `Cleanup` destroying the primary weft branch.
`TestCleanup_ProtectsPrimaryWeftBranchAfterCheckout` and `TestCleanup_PrimaryBranchSurvivesForceWhenNotCheckedOut` are the two named tests that pin the carve-out and its force interaction, and both must stay green unchanged.
`add_rollback_adopt_test.go` and `checkout_rollback_test.go` cover the two rollback sites, which is what proves the floor did not start refusing on paths that legitimately delete a fabric-created branch.

Scope stays the one package.
The module-wide `go build ./...` at the batch boundary covers the three signature changes this batch makes — `rollbackAdd`, `removeWeftWorktree` and `deleteWeftBranch` all gain a parameter.

New assertions for `ownedManagedBranch` itself — that it refuses the primary weft branch, refuses when the primary cannot be read, refuses a checked-out branch, refuses a non-fabric-managed name, and refuses all four of those under force as well — are written in batch 7, where they can be asserted directly against the gate rather than only through `Cleanup`'s enumeration.
