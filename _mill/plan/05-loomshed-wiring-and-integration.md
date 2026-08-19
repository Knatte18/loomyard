# Batch: loomshed wiring and integration

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'loomshed wiring and integration'
number: 5
cards: 5
verify: go test ./internal/loomshed/... ./internal/landingshed/... ./cmd/lyx/... && go test -tags integration ./internal/loomshed/... ./internal/landingshed/...
depends-on: [4]
```

## Batch Scope

Swapping rows 12 and 13 of loom's producer list off their stubs and onto the real producers, and proving both producers work against real two-worktree pairs rather than fakes.
It is one batch because the wiring edit is small and its correctness is exactly what the integration tier demonstrates: without the tier, "the rows are wired" is a claim about a constructor call rather than about behaviour.

It depends on batch 4 for the two constructors and the told-value struct the passthrough field carries.

Batch-local decisions beyond `## Shared Decisions`:

- **The integration tier is worth its cost, deliberately.** The merge primitive's own review round surfaced findings about exactly this kind of two-worktree interaction that no unit tier against fakes would have caught. This tier drives real pairs with only the model seam faked.
- **Driving both producers through a real engine run over a short producer list is explicitly not tested.** That re-tests the engine, which already has its own resume, crash-recovery, and pause suites.

## Cards

### Card 31: pass landing's told values through loom's Deps

- **Context:**
  - `internal/landingshed/deps.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/shedengine/producer.go`
- **Edits:**
  - `internal/loomshed/loomshed.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/loomshed.go`, grow `Deps` by exactly one field — a single `Landing landingshed.Deps` passthrough — rather than flattening landing's values into individual fields.
  A single passthrough keeps landing-specific fields out of this struct and gives a future sibling product the same struct to fill from its own geometry with no duplication.
  Document the field accordingly.

  In `New`, replace the two `newStub(...)` backings on the `Publish` and `Finalize` rows with the real constructors built from `deps.Landing`, keeping both rows' escalate-on-stuck setting exactly as it is: neither bounces, because nothing in the list produces what these two gate — an unresolvable conflict against the parent's current state, an unreachable remote service, a drifting parent, and a pull request awaiting human review are all things only a human fixes.
  Both constructors return an error, so `New` surfaces a construction failure rather than discarding it, alongside its existing nil-check on the preflight row.

  Update `New`'s own doc comment, which currently enumerates all thirteen rows with their backing: rows 12 and 13 now read as real producers rather than stubs.
  Update the name-constant block's comment too, which currently points a reader at the design document this task deletes — repoint it at the new package's own documentation instead.
- **Commit:** `feat(loomshed): wire the real Publish and Finalize producers into rows 12 and 13`

### Card 32: extend loomshed's import allowlist

- **Context:**
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomshed/seam_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `"github.com/Knatte18/loomyard/internal/landingshed": true,` to the `loomshedAllowedImports` map in `internal/loomshed/seam_enforcement_test.go`.
  The guard is an allowlist-membership check over every non-test file's imports, so the new production import fails the build until it is listed.
  Leave the map's existing entries and its explanatory comment otherwise untouched — the transitive-is-fine reasoning it already records applies unchanged to the new entry.
- **Commit:** `test(loomshed): allow the landingshed import in the seam guard`

### Card 33: correct the stub inventory

- **Context:**
  - `internal/loomshed/loomshed.go`
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/stub_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `stubProducer`'s doc comment in `internal/loomshed/stub.go` currently enumerates eight stubbed rows by name, two of which are no longer stubs.
  Drop `Publish` and `Finalize` from that list and correct the stated count to six.
  A doc comment that names a row the code no longer stubs is a lie a reader has no way to detect.

  Check `internal/loomshed/stub_test.go` for any assertion or fixture that names either of those two rows as stub-backed, and update it to match the new list.
  If the file turns out to name neither row, leave its body unchanged.
- **Commit:** `docs(loomshed): drop Publish and Finalize from the stub inventory`

### Card 34: wiring assertions

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/loomshed_test.go`
  - `internal/loomshed/fixture_test.go`
  - `internal/loomshed/sequence_test.go`
  - `internal/landingshed/deps.go`
- **Edits:**
  - `internal/loomshed/loomshed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/loomshed/loomshed_test.go` so the list-assembly tier proves the swap actually happened rather than merely still compiling.
  Assert that the row named `Publish` and the row named `Finalize` are each backed by the real producer type rather than the stub type, that both rows keep their escalate-on-stuck setting, and that the list's thirteen rows stay in their existing table order with their existing names.
  Assert that a `Deps` whose landing passthrough is missing a required closure makes the constructor return an error rather than yielding a list that panics at call time.
  Reuse this package's existing test fixture helpers rather than building a second set.
- **Commit:** `test(loomshed): assert rows 12 and 13 are backed by the real producers`

### Card 35: integration coverage for both producers

- **Context:**
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/config.go`
  - `internal/mergeresolve/deps.go`
  - `internal/fabricengine/mergein_integration_test.go`
  - `internal/fabricengine/merge_target_integration_test.go`
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/hermetic.go`
  - `internal/loomshed/testmain_integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/testmain_integration_test.go`
  - `internal/landingshed/finalize_integration_test.go`
  - `internal/landingshed/publish_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one integration test per producer, driving real two-worktree pairs built by the standard hub fixture with only the model seam faked.
  Each file opens with the `//go:build integration` constraint as its first line, followed by a file-header comment.

  `internal/landingshed/testmain_integration_test.go` wires this package's integration binary into the hermetic git environment with a `TestMain` calling the shared helper before running, modelled on the sibling package's own equivalent.
  It is mandatory rather than optional: this package's tests become git-spawning the moment the fixture is used, and the hermetic-environment guard is a presence check that fails without it.

  `internal/landingshed/finalize_integration_test.go` builds a parent pair and a task pair, creates a genuine conflicting change on both sides, runs the producer with a fake session that writes real resolutions to the conflicted files, and asserts that the parent pair afterwards carries the task's content on both sides, that the squash setting took effect, and that no merge record is left behind.

  `internal/landingshed/publish_integration_test.go` covers the merge-in half against real pairs with a faked GitHub client, asserting the pair is left clean and current before the create call is made.
  No test in either file contacts a real service or a real model.
- **Commit:** `test(landingshed): drive both producers against real pairs`

## Batch Tests

`verify:` runs both packages' fast tiers plus `cmd/lyx`, then the two integration tiers.

The untagged half proves the wiring: `./internal/loomshed/...` runs the list-assembly assertions (card 34), the corrected stub inventory (card 33), and the import allowlist guard (card 32), which fails until card 32 lands and is therefore the direct check that the swap is legal rather than merely compiling.
`./internal/landingshed/...` re-runs batch 4's unit tier, which must stay green under the new call site.
`./cmd/lyx/...` re-runs the whole guard family, including the hermetic-environment presence check that card 35's new `TestMain` satisfies and the tier-purity check that would fire if any of card 35's files were untagged.

The `-tags integration` half is required because card 35 creates three tagged files; an untagged-only run would compile none of them, and they are the only real evidence in this task that two live pairs merge correctly.
`./internal/loomshed/...` appears in the tagged half too, because that package already carries a tagged preflight suite whose fixture must stay green under the new `Deps` field.
