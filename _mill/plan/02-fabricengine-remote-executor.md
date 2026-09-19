# Batch: fabricengine-remote-executor

```yaml
task: 'fabric: no remote/GitHub branch deletion'
batch: 'fabricengine-remote-executor'
number: 2
cards: 4
verify: go test ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

This batch adds the sixth destructive primitive to fabric's destruction chokepoint: a new mutation kind, a new request type and its check function, the gated executor that performs the remote deletion, the two enforcement-ledger rows that make the addition machine-enforced, and the executor's own direct-call tests.
It is one batch because the executor, its mutation kind, and its ledger rows are a closed set that fails the build if split — `manifestObservableKind` refuses an unclassified `Kind`, and `destructiveGuardRecordingExecutors` is the table that proves the executor takes its recorder.

External interface batch 3 consumes: the unexported `deleteRemoteBranch(rec *Mutations, req remoteBranchRequest) (deleted bool, err error)` executor, the `remoteBranchRequest` struct, the `originRemoteName` constant, and the exported `KindRemoteBranchDeleted`.

Batch-local decisions beyond the overview's Shared Decisions:

- `remoteBranchRequest` is a distinct type, never a `remote` field added to `branchRequest`.
  `branchRequest`'s own doc comment rejects per-site empty-string sentinels for structurally-N/A fields, and a `remote` that must be `""` at the four existing local call sites is exactly that shape.
- The gate tests in this batch are direct-call tests only.
  They pin the executor's request shape, not an end-to-end guarantee — see card 6's Requirements for why the dirtiness probe cannot refuse at either real call site.

## Cards

### Card 3: KindRemoteBranchDeleted mutation kind

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/mutation.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the constant `KindRemoteBranchDeleted Kind = "remote_branch_deleted"` to the existing `const` block, placed immediately after `KindBranchDeleted` so the two branch-deletion kinds read together.
  Its doc comment records that it is appended by `deleteRemoteBranch` for a `git push <remote> --delete` that observably removed a ref, and that a distinct kind rather than a reused `KindBranchDeleted` exists because only one of the two deletions is recoverable, so a consumer switching on kind must be able to tell them apart.

  Update the `const` block's own leading comment, which currently reads:

```
// The fixed set of mutation kinds fabric records.
// Seven are auto-recorded by the destruction gate (destroy.go); the remaining eleven are
// hand-recorded at their success sites, since no chokepoint covers them.
```

  The gate-recorded count becomes eight;
  the hand-recorded count is unchanged.
  Verify the two counts against the block's actual membership before writing them rather than incrementing blind — the comment is a claim about the block it heads.

  Do not change any existing constant's string value: `Kind`'s own doc comment fixes those as part of fabric's public JSON contract.
- **Commit:** `feat(fabric): add KindRemoteBranchDeleted mutation kind`

### Card 4: remoteBranchRequest, its check, and the deleteRemoteBranch executor

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/clone.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add four things to this file, then amend its header.

  First, an unexported package-level constant `originRemoteName = "origin"`, declared in `destroy.go` with a doc comment recording that fabric's geometry already hardcodes `origin` throughout — `clone.go`'s weft adopt-or-create path uses `refs/remotes/origin/` and `origin/<branch>` — so introducing configurability at this one point alone would be inconsistent and untested against any other value.

  Second, the struct `remoteBranchRequest`, declared immediately after `branchRequest`, with exactly these fields and a per-field doc comment on each: `what string`, `repoDir string`, `remote string`, `branch string`, `ownership branchOwnership`, `dirtiness branchDirtiness`, `force bool`.
  Its type doc comment must state that it is a distinct type rather than a `remote` field on `branchRequest` because `branchRequest`'s own comment rejects a per-site `""` for a structurally-N/A field, and a `remote` that must be empty at the four existing local call sites is exactly that shape;
  and that a distinct type keeps the two executors' call-site sets disjoint and auditable.
  `force` is reserved and hardcoded false at every construction site, exactly as on `branchRequest` — say so on the field.

  Third, the function `checkRemoteBranchRequest(req remoteBranchRequest) error`, structurally mirroring `checkBranchRequest`: refuse `branchOwnershipUnset` with `CheckOwnership` and the reason `no ownership kind declared`;
  refuse `branchDirtinessUnset` with `CheckDirtiness` and the reason `no dirtiness declared`;
  then call `resolveBranchOwnership(req.ownership, req.branch)` and refuse with `CheckOwnership` on a false result;
  then run the same checked-out-at-a-worktree dirtiness probe `checkBranchDirtiness` runs.
  Reuse `branchOwnership`, `branchDirtiness`, `resolveBranchOwnership`, and the dirtiness probe unchanged — do not add a second ownership rule for a branch.
  Containment is structurally N/A and there is no container field, exactly as on `branchRequest`.

  `checkBranchDirtiness` takes a `branchRequest`, so it cannot be called with a `remoteBranchRequest` directly.
  Resolve this by extracting the probe body into a helper both call, keeping the `*lyxcwd.Location` access path through `req.ownership.location` that `checkBranchDirtiness` already uses — never by adding a `location` field to either request type.
  The helper takes the three values the probe needs (the ownership value, the act name, and the branch) and returns the same `*destructiveRefusal` shape;
  `checkBranchDirtiness` then becomes a one-line delegation to it, and its existing doc comment moves to or is referenced from the helper.
  This keeps the probe's single implementation and preserves the Location access path the file's own comment calls out.

  Fourth, the executor `deleteRemoteBranch(rec *Mutations, req remoteBranchRequest) (deleted bool, err error)`.
  It runs `checkRemoteBranchRequest(req)` first and returns `(false, checkErr)` on a refusal.
  It then calls `gitrepo.New(req.repoDir).DeleteRemoteBranch(req.remote, req.branch)` and, when that returns a nil error and `deleted == true`, appends `KindRemoteBranchDeleted` to `rec` via `AppendRef` with `req.branch` as the ref and `req.remote` as the `detail`.
  It returns the `(deleted, err)` pair through unchanged, wrapping nothing — every call site builds its own message from it, exactly as `deleteBranch` and `removeGitWorktree` already do.
  `AppendRef`, not `Append`: a remote ref is a ref and carries no hub-relative path conversion.
  The append happens only on an observed deletion — never on `deleted == false`, which is the already-absent idempotent success, and never on an error.
  State that record-only-on-observed-effect rule in the executor's doc comment, since it is what the file's recording contract requires.

  Then amend the file header in two places.
  The opening enumeration currently names five primitives:

```
// destroy.go is the only file in package fabricengine permitted to perform a destructive primitive.
// The five primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
// worktree remove), removing or re-pointing a link (fslink.Remove), deleting a branch (git branch
// -D), and resetting a warp checkout hard (ResetHard).
```

  It becomes six, with the new one named as deleting a branch on a remote (`git push <remote> --delete`) and placed immediately after the local branch deletion.
  Separately, the recording contract's sentence currently opens:

```
// Recording contract: every one of the eight executors below takes a leading `rec *Mutations`
```

  `eight` becomes `nine`.
  The same sentence later names `removeGitWorktree and deleteBranch` as the two executors recording only on a nil git error;
  extend that list to name `deleteRemoteBranch` as well, noting that it additionally requires an observed deletion rather than a nil error alone, since its own nil-error-plus-`deleted == false` case is the already-absent success.

  Do not add a `CheckForce` member to the `Check` enum and do not make `force` answer the dirtiness check — the file's header forbids both.
- **Commit:** `feat(fabric): gate remote branch deletion behind destroy.go's sixth primitive`

### Card 5: Enforcement-ledger rows

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/mutation.go`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
  - `internal/fabricengine/livestate_mutationoracle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three rows, in three closed tables that each fail loud on an unregistered addition.

  In `cmd/lyx/destructiveguard_test.go`, add `".DeleteRemoteBranch("` to `destructiveGuardBannedTokens`, placed next to the existing `"branch", "-D"` entry so the two branch-deletion tokens read together.
  This is what gives the sixth primitive any chokepoint enforcement at all: without it, any file in `internal/fabricengine` other than `destroy.go` could name the method freely.
  `internal/fabricengine/destroy.go` is already on `destructiveGuardAllowlist` with the invariant-required reason, so no allowlist row changes.
  Update that variable's own leading comment, which currently describes its contents as "the discussion's final seven tokens plus `createdToken{`", to describe the set accurately after the addition.

  In the same file, add the row `{"deleteRemoteBranch", "func deleteRemoteBranch(rec *Mutations, "}` to `destructiveGuardRecordingExecutors`, placed immediately after the existing `deleteBranch` row.
  The `declPrefix` must match the executor's real declaration byte-for-byte, since the consuming test greps for it as a raw substring.
  Update that variable's doc comment and the `destructiveGuardRecordingExecutorsMin` comment where either states the table's row count — the table goes from eight rows to nine.
  Do not change `destructiveGuardRecordingExecutorsMin`'s value: it is a vacuous-scan floor of 5, deliberately well below the table size, and is not expected to track it.

  In `internal/fabricengine/livestate_mutationoracle_test.go`, add `fabricengine.KindRemoteBranchDeleted: false` to `manifestObservableKind`, grouped with the other git-state kinds alongside `KindBranchDeleted` and `KindBranchPushed`.
  The value is `false` because a remote ref is not something the filesystem `CaptureManifest` records, exactly as local branch existence already is not.
  Declaring it is mandatory rather than optional: that map's own comment records that declaring every kind, rather than testing list membership, is what makes a newly added `Kind` fail loud through `participatesInCommission`.
- **Commit:** `test(fabric): register the remote-branch-deletion primitive in all three enforcement ledgers`

### Card 6: Direct-call gate tests for the remote executor

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/destroy_toctou_test.go`
  - `internal/fabricengine/refusalof_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/hubforge/hub.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/destroy_test.go`
- **Creates:**
  - `internal/fabricengine/destroyremote_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  These are direct-call tests: each constructs a `remoteBranchRequest` and calls `checkRemoteBranchRequest` or `deleteRemoteBranch` itself.
  What they pin is the executor's own request shape — that it cannot be reached with an undeclared predicate, and that a future call site arriving without a preceding local deletion is still gated.
  They are not end-to-end guarantees and must not be written or commented as if they were: the remote executor runs only after the local `git branch -D` already succeeded, so at both real call sites the branch is gone from the local repo by the time the dirtiness probe would read it, and the probe therefore cannot refuse there.
  Each test that exercises a refusal the real call sites cannot reach must carry a comment saying exactly that.

  Which tier each case belongs to follows from whether the assertion reaches git, not from judgment.
  `resolveManagedBranch` short-circuits on its naming predicate and only then spawns git twice, for `primaryWeftBranch` and `listWeftBranches`;
  the Test Tier Purity Invariant bans a git spawn in an untagged file.

  In `internal/fabricengine/destroy_test.go`, extend the existing `t.Run`-based zero-declaration group that already holds the `BranchZeroOwnership` and `BranchZeroDirtiness` subtests with three new subtests, mirroring their exact shape and reusing the file's existing `assertRefusalCheck` helper.
  A hand-built `*lyxcwd.Location` is safe in the first two for the reason the existing `BranchZeroDirtiness` subtest's own comment records — the zero-value declaration refuses before `resolveManagedBranch` ever runs — and safe in the third because the naming predicate refuses ahead of the first spawn.
  The three cases: an unset ownership kind is refused with `CheckOwnership`;
  an unset dirtiness kind is refused with `CheckDirtiness`;
  a branch whose name fabric's scheme does not construct is refused with `CheckOwnership`.

  Create `internal/fabricengine/destroyremote_integration_test.go` with a leading `//go:build integration` line, a file-header comment, and `package fabricengine` — the internal test package, because these tests construct the unexported `remoteBranchRequest`.
  Confirm that `internal/fabricengine/testmain_test.go` already satisfies the Hermetic Git Test Environment Invariant for this package before adding a `TestMain`;
  do not add a second one.
  Two cases, each integration-tagged because each reaches a git spawn: the repo's primary weft branch is refused with `CheckOwnership`, reaching `primaryWeftBranch`;
  and a branch still checked out at a worktree is refused with `CheckDirtiness`, reaching `listWeftBranches`.
  Build the hub these two need through `hubforge.NewHub`, per the hubforge Fabric-Fixture Invariant — no hub is hand-assembled.

  No test in this card asserts that a remote ref was actually deleted: that is batch 3's behaviour-test surface, over a real hub with a real remote.
  Card 6 asserts refusals and request shape only.
- **Commit:** `test(fabric): pin the remote-branch executor's request shape and gate refusals`

## Batch Tests

`verify:` runs three scopes.
`go test ./internal/fabricengine/...` covers card 6's untagged `destroy_test.go` additions and compiles cards 3 and 4's production changes.
`go test ./cmd/lyx/...` is what actually executes card 5's two `destructiveguard_test.go` ledgers — that file is untagged, and `TestMutationRecord_FabricengineProductionSource` is the test that consumes `destructiveGuardRecordingExecutors`, so a run scoped to `internal/fabricengine` alone would never see a broken declPrefix.
`go test -tags integration ./internal/fabricengine/...` executes card 6's new integration-tagged file and card 5's `livestate_mutationoracle_test.go` row, which is itself `integration`-tagged.

`cmd/lyx` needs no `-tags integration` half in this batch: neither file card 5 edits under that scope is tagged, and the tagged tests there cover unrelated surface.

Two further enforcement tests run inside these scopes without needing a row of their own and are expected to pass unchanged: `TestEnforcement_FabricVocabulary`, which this batch stays clear of because `fabricengine` is inside its owner set, and the help-tree tests, which this batch does not touch.
