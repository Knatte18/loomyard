# Batch: merge-stage-resolved verb

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'merge-stage-resolved verb'
number: 1
cards: 6
verify: go test ./internal/gitrepo/... ./internal/fabricengine/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/... ./internal/fabricengine/...
depends-on: []
```

## Batch Scope

This batch delivers the one Fabric primitive without which the whole task cannot work: `fabricengine.MergeStageResolved`, the narrow verb that stages agent-resolved conflict paths so `MergeContinue`'s index guard can pass.
`MergeContinue` refuses while either side's `ConflictedFiles()` is non-empty, and that is an *index* probe (`git diff --name-only --diff-filter=U`) — editing a file's content never clears its unmerged index entry, so without a staging verb no agent resolution can ever conclude.
It is one batch because the verb, its `gitrepo` staging counterpart, its new mutation `Kind`, and the three guard-table/invariant-list updates that pin all of it are a single indivisible unit: omitting any one of them fails the build.

The external interface batch 3 consumes is `MergeStageResolved(paths []string) (StageResult, error)` on `*fabricengine.Fabric`.

Batch-local decisions beyond `## Shared Decisions`:

- **No lock is taken.** `MergeStageResolved` stages the index of a pair the caller already holds mid-merge under its own sequential control; the irreversible half (`MergeContinue`) takes the weft write lock itself, exactly as it does today. Adding a second lock acquisition around a pure index write would buy nothing the caller's own sequencing does not already provide.
- **The discriminator is index membership, never a path prefix.** `weftPathVisible` is a total function, so "the inverse of `unifyConflictPaths`" has no third outcome and could never produce a "maps to neither side" error. The verb reads each side's own `ConflictedFiles()` instead.

## Cards

### Card 1: gitrepo staging primitive that also stages removals

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/doc.go`
- **Edits:**
  - `internal/gitrepo/merge.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) StageResolved(paths []string) error` to `internal/gitrepo/merge.go`, declared immediately after the existing `ConflictedFiles` method.
  An empty or nil `paths` returns `nil` without invoking git at all — a no-op, never an error.
  Otherwise it builds `args := append([]string{"add", "-A", "--"}, paths...)` and calls `r.runChecked(args...)`, wrapping a non-nil error as `fmt.Errorf("gitrepo: git add -A in %s: %w", r.path, err)`.
  The `-A` form is required rather than the plain `add --` form `StageAndCommit` uses: a delete/modify conflict is legitimately resolved by the file being gone, and the plain form errors on a missing pathspec while `-A` stages the removal.
  Never introduce a `-f` flag on this or any other path, per the Never Force-Add Invariant.
  Give the method a godoc comment stating that it stages caller-supplied repo-root-relative paths including removals, and that an empty slice is a no-op.
- **Commit:** `feat(gitrepo): add StageResolved for staging resolved conflict paths`

### Card 2: pin StageResolved on the gitrepo Client Boundary Invariant

- **Context:**
  - `internal/gitrepo/merge.go`
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `StageResolved` reaches the git CLI through `r.runChecked`, so it must join both halves of the gitrepo Client Boundary Invariant in this same commit or the guard fails set-equality.
  In `cmd/lyx/gitrepoboundary_test.go`, add `"StageResolved": true,` to the `gitrepoPinnedRunBoundMethods` map.
  In `CONSTRAINTS.md`, under the "## gitrepo Client Boundary Invariant" heading, add `StageResolved` to the prose list of methods `gitexec` is the only path for — the list that currently ends `…, MergeHeadPresent, MergeFFOnly`.
  It uses the checked form (`runChecked`), so it adds no `//gitexec:raw` site and the gitexec Checked-Call Invariant's per-package pinned raw-site counts stay unchanged; leave that invariant's numbers exactly as they are.
- **Commit:** `chore(constraints): pin gitrepo.StageResolved on the Client Boundary Invariant`

### Card 3: KindMergeResolvedStaged mutation kind

- **Context:**
  - `internal/fabricengine/mergelifecycle.go`
- **Edits:**
  - `internal/fabricengine/mutation.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new member to the `Kind` const block in `internal/fabricengine/mutation.go`: `KindMergeResolvedStaged Kind = "merge_resolved_staged"`, declared immediately after the existing `KindMergeStaged` member and before `KindMergeCommitted`.
  Its doc comment states that it records one side's staging of agent-resolved conflict paths, that `Target` is that side's checkout path, and that it is recorded only after the staging call observably succeeded — never on the empty-paths no-op.
  Update the const block's own leading comment, which currently reads "Seven are auto-recorded by the destruction gate (destroy.go); the remaining ten are hand-recorded at their success sites", so the hand-recorded count reads eleven rather than ten.
  This member's recording site lands in card 4 and its guard-table entry in card 5, all three in the same commit series of this batch, per the Mutation Record Invariant's same-commit rule.
- **Commit:** `feat(fabricengine): add KindMergeResolvedStaged mutation kind`

### Card 4: MergeStageResolved verb and StageResult

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/mergelifecycle.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/mutation.go`
  - `internal/gitrepo/merge.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergestage.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/mergestage.go` in `package fabricengine`, carrying a file-header comment in this package's established style and two declarations.

  First, `StageResult`, the mutating result type this verb returns, embedding `MutationRecord` as the Mutation Record Invariant requires of every mutating verb's result type:

  ```go
  type StageResult struct {
  	MutationRecord
  }
  ```

  Second, `func (f *Fabric) MergeStageResolved(paths []string) (res StageResult, err error)`.
  Its body follows the established verb shape in this package: open with `rec := NewMutations(filepath.Dir(f.warpPath))` and `defer func() { res.Mutations = rec.Snapshot() }()`, exactly as `MergeContinue` does.

  Behaviour, in order:

  1. An empty or nil `paths` returns `StageResult{}, nil` immediately — a no-op with an empty mutation record, matching the invariant's "never on a no-op" rule.
  2. Read `f.warp.ConflictedFiles()` and `f.weft.ConflictedFiles()`, returning the error unwrapped-but-wrapped in this package's usual `fmt.Errorf("fabricengine: …: %w", err)` style on either failure.
  3. Build a set from each side's result and partition `paths`: a path the warp side lists goes to the warp batch, otherwise a path the weft side lists goes to the weft batch.
     A path **neither** side lists is the error condition — return an error naming that path and stating it is not conflicted on either side, since the caller passed something that is not conflicted and silently staging it would hide a caller bug.
     A path listed by both sides cannot occur: `unifyConflictPaths` already treats that collision as `unmappable` and self-aborts the merge before any caller could see the path. State that in a comment rather than adding a branch for it.
     Partition every path before staging anything, so a bad path fails the whole call rather than leaving one side half-staged.
  4. For each side whose batch is non-empty, call that side's `StageResolved` with its batch, and on success append `rec.Append(KindMergeResolvedStaged, <that side's path field>, "")` — warp first, then weft, matching `concludeMergeSides`' fixed side ordering.
  5. Return `StageResult{}, nil`.

  The godoc comment on the verb states: it takes unified, worktree-relative paths; it stages each on whichever side's index actually lists it unmerged; a path listed by neither side is an error rather than a silent skip; deletions stage rather than erroring, because a delete/modify conflict is legitimately resolved by the file being gone; and it exists because `MergeContinue`'s conflict gate is an index probe that no amount of content editing can clear.
  Note in the comment that `MergeContinue`'s own guard is deliberately left untouched, giving two independent gates rather than one relocated one.
- **Commit:** `feat(fabricengine): add MergeStageResolved verb`

### Card 5: pin StageResult on the Mutation Record guard table

- **Context:**
  - `internal/fabricengine/mergestage.go`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the row `{"StageResult", "internal/fabricengine/mergestage.go"},` to the `destructiveGuardMutatingResultTypes` table in `cmd/lyx/destructiveguard_test.go`, appended after the existing `{"MergeResult", "internal/fabricengine/merge.go"},` row.
  Leave `destructiveGuardReadOnlyResultTypes` untouched — `StageResult` is a mutating verb's result type, and that companion table's two-rows-by-construction property is documented in its own comment.
- **Commit:** `test(cmd/lyx): pin StageResult on the mutation-record guard table`

### Card 6: MergeStageResolved integration coverage

- **Context:**
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/mergestage.go`
  - `internal/fabricengine/mutation.go`
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/mergestage_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/fabricengine/mergestage_integration_test.go` in `package fabricengine_test`, opening with the `//go:build integration` constraint as its first line, followed by a file-header comment.
  It reuses `newMergePairFixture` and the sibling commit helpers already declared in this test package rather than building its own hub fixture.

  Scenarios, each against a real conflicted pair:

  - a conflicted pair whose files are resolved on disk, then `MergeStageResolved` with exactly the conflicted paths → both sides' `ConflictedFiles()` are empty afterwards and a subsequent `MergeContinue` succeeds;
  - a path listed in neither side's `ConflictedFiles()` → an error naming that path, not a silent skip;
  - a delete/modify conflict resolved by deleting the file → staging succeeds rather than erroring on the missing path;
  - an empty `paths` slice → a no-op returning a nil error;
  - the record assertion the Mutation Record Invariant demands: a real staging call populates `StageResult`'s `Mutated()` record, and the empty-slice no-op leaves it empty.

  This package's integration binary already wires the hermetic git environment through its existing `TestMain`, so this file adds no second one.
- **Commit:** `test(fabricengine): cover MergeStageResolved against real conflicted pairs`

## Batch Tests

`verify:` compiles and runs the three affected packages' fast tiers plus the two integration tiers this batch edits.
The untagged half covers `cmd/lyx`'s two guard tests (`TestGitrepoBoundary_PinnedRunCallSites` and `TestMutationRecord_FabricengineProductionSource`), both of which are set-equality checks that fail on an omitted table entry, and `internal/gitrepo`'s own `TestNoForceAdd_GitrepoSourceHasNoForceAddBranch`.
The `-tags integration` half is required because this batch creates a tagged test file (`internal/fabricengine/mergestage_integration_test.go`) whose scenarios are the real proof the verb works; an untagged-only run would compile none of it.
`internal/gitrepo`'s integration tier is included because card 1's new method is exercised transitively there through the fabric-level scenarios and its own package's merge integration suite must stay green.
