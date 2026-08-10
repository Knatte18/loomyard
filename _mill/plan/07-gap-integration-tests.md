# Batch: gap-integration-tests

```yaml
task: 'fabric: one ownership-and-dirtiness gate for all destruction (slice 12)'
batch: 'gap-integration-tests'
number: 7
cards: 3
verify: go test -tags integration ./internal/fabricengine/...
depends-on: [4, 5]
```

## Batch Scope

This batch adds the integration coverage the gate's git-touching half needs: the three newly-closed gaps, the ownership predicates that call real git, both dirtiness scopes proven to differ, and branch ownership on the ground where the slice's third defect lived.
Everything here is deliberately excluded from batch 2's hermetic tests because it spawns git, which the Test Tier Purity Invariant bans in an untagged file.

It runs in parallel with batch 6, which touches no file in this batch.

Batch-local decisions beyond `## Shared Decisions`:

- All three cards write into one new file rather than three, because they share fixture vocabulary — a real hub with a real warp repo, a real weft repo and at least one registered linked worktree — and three files would mean three copies of that setup.
- The file is `//go:build integration` in the external test package, matching the package's existing integration files.
  No new test package is created, so the hermetic git environment the package's existing test main installs is inherited rather than re-declared — a new package would need its own, per the Hermetic Git Test Environment Invariant.
- Coverage here is deliberately not the full per-primitive by per-state by per-verb cross product.
  That is the next slice's harness, and building it here without its hub factory would mean hand-rolled fixtures that slice would immediately delete.

## Cards

### Card 32: the three newly-closed gaps

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/prune_unowned_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** create the file with a `//go:build integration` constraint as its first line and the external test package clause, then write one test per gap, each named for the behaviour rather than the mechanism.
  Gap one: the junction-record removal refuses a junction link that sits outside the worktree it belongs to — the containment check that helper had no form of at all before this slice, reached from both the removal verb and the add rollback.
  Gap two, two assertions in one test or two adjacent tests: the hub teardown refuses a hub path outside the operator-named parent, **and** succeeds on a half-built hub carrying no board directory and no weft sibling.
  The second assertion is as important as the first and is the reason the teardown declares a token rather than the hub predicate: with the hub predicate it would have refused at nearly every early failure site and left a residual hub where teardown works today.
  Gap three: the warp worktree removal's directory fallback does not delete a registered linked worktree carrying untracked files when git refused for that reason.
  Construct that state explicitly — a registered linked worktree with an untracked file in it, removed without force, so git's own refusal is what routes into the fallback — and assert both that the call fails and that the untracked file is still on disk afterwards.
  Asserting the error alone would pass against the pre-fix code, so the on-disk assertion is the one that actually proves the gap closed.
  Follow the fixture and helper conventions in `internal/fabricengine/prune_unowned_integration_test.go`, which builds the closest equivalent hostile-ownership state.
- **Commit:** `test(fabricengine): cover the three gaps the destruction gate closes`

### Card 33: path ownership and both dirtiness scopes

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/dirtiness.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/prune_unowned_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the ownership and dirtiness coverage that needs real git.
  For the registered-linked-worktree kind: assert it accepts a real registered linked worktree, refuses the main worktree, refuses an unrelated git clone parked at a fabric-shaped path inside the hub, refuses a plain directory, and answers false rather than erroring when the repo cannot be enumerated at all.
  The unrelated-clone case is one of the eight original defects and must be a real second clone, not an empty directory.
  For the warp-checkout kind: assert it accepts the hub's prime warp worktree — the case the registered-linked-worktree kind deliberately refuses, and the reason the two kinds exist separately — and accepts a linked worktree of the same repo, and refuses a path that is not a worktree of that repo at all.
  Assert explicitly that the two kinds disagree on the main worktree;
  that single assertion is what stops a future edit collapsing them.
  For the hub kind: assert a real hub, a directory with only a board entry, a directory with only a weft sibling, a directory with neither, and an unreadable directory, with the last two refused.
  For dirtiness: assert both scopes against four states — a tracked modification, a staged change, an untracked file only, and clean — and assert them separately under each scope so the untracked-only case is *proven* to differ between them.
  That single case is the entire justification for scope being a declared parameter rather than a constant, so it must be asserted rather than assumed.
- **Commit:** `test(fabricengine): cover gate path ownership and both dirtiness scopes`

### Card 34: branch ownership on the primary-weft ground

- **Context:**
  - `internal/fabricengine/destroy.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the branch-ownership coverage, on the ground where the slice's third data-loss defect lived.
  Assert the branch ownership kind refuses the primary weft branch;
  refuses when the primary cannot be read at all, which is the fail-closed direction it inherits and the one direction that must never invert, since these deletions are irreversible;
  refuses a branch checked out at some worktree;
  and refuses a branch whose name is not one fabric's own scheme constructs.
  Assert each of those four **also** refuses when force is true.
  Force reaches only the folded-back-raddle gate in the cleanup verb;
  it may not answer the primary-weft carve-out and may not answer the checked-out check, and a force-satisfies-ownership reading is how the original defect would return behind a flag.
  Assert the positive case too: an orphaned, fabric-named, non-primary, non-checked-out weft branch is accepted, so the kind is not trivially always-refusing.
  Then assert the property that is the entire reason this logic moved into the gate rather than staying in one verb: the same refusals hold at the other three deletion sites, not only in the cleanup verb.
  Drive at least one of the other three end to end — the add rollback is the cheapest, since it already has integration cover — and assert that a branch the gate refuses is still present afterwards.
- **Commit:** `test(fabricengine): cover branch ownership at all four deletion sites`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/...` runs the tagged tier of the one package this batch touches, which is the only tier that can run these tests at all — every assertion here needs real git, which is why none of it lives in batch 2's untagged file.

The untagged tier is not re-run at this batch boundary: the batch adds no production code and no untagged test file, and batches 3, 4 and 5 each already ran it.
The module-wide `go build ./...` at the batch boundary still applies and is enough to catch a compile break, though this batch adds no non-test code that could cause one.

Scope is deliberately the three gaps plus the git-touching predicates, not the full verb cross product — that is the next slice's harness, and the discussion's `### Explicitly not tested here` section is explicit that building it here would mean fixtures that slice deletes.
