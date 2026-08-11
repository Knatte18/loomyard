# Batch: result-types-carry-record

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'result-types-carry-record'
number: 3
cards: 5
verify: go test ./internal/fabricengine/ ./internal/fabriccli/ ./internal/boardengine/ && go vet -tags integration ./internal/fabricengine/...
depends-on: [1, 2]
```

## Batch Scope

This batch makes the record *survive the error return*, which is the literal fix for the reported defect: every mutating result type embeds `MutationRecord`, every mutating entry point declares named results and installs the `defer` that populates the record on every path, and `push` — which has no result type at all today — gets one.
It is one batch because the embed, the named-result conversion, and the `PushResult` introduction all change the same twelve signatures and must compile together.
Nothing records anything yet: after this batch every mutating verb returns an empty-but-present record on every path, including the failure paths that return a zero result today.
That is deliberate — it isolates the mechanical signature churn from the recording work in batches 4 and 5.

The external interface batches 4-7 consume is: `res.Mutated()` on every mutating result type, a `*Mutations` recorder in scope at every verb body, and `PushResult`.

Batch-local decision: the existing `return XResult{}, err` sites are **left exactly as they are**. The `defer` assigns onto the named result after the return value has been set, so a zero-result return still carries the record — and leaving them alone is what keeps a *newly added* early return from needing to remember anything.

## Cards

### Card 7: `Mutations.Extend` for composed records

- **Context:**
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/mutation_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/fabricengine/mutation.go`:

  ```go
  func (m *Mutations) Extend(other Mutations)
  ```

  It appends every entry of `other` to `m` verbatim, performing no path conversion — `other`'s entries were already converted by whichever `Mutations` produced them.
  This is the composition primitive for a verb that calls another recording entry point (`Unwire` over `UnwireJunctions`), and for the CLI layer's concatenate-engine-record-then-CLI-entries rule in batch 6.
  A nil `*Mutations` receiver is safe, and an empty `other` is a no-op.

  Also make `Snapshot()` and `Len()` safe on a nil `*Mutations` receiver: `Snapshot()` returns the zero `Mutations` and `Len()` returns 0.
  `CloneHub` constructs its recorder only once the hub path is derived, so its `defer` observes a nil recorder on the earliest failure paths.

  Extend `internal/fabricengine/mutation_test.go` with cases for `Extend` (order preserved, entries unconverted, nil receiver safe, empty source a no-op) and for the nil-receiver behaviour of `Snapshot` and `Len`.
- **Commit:** `feat(fabricengine): add Mutations.Extend for composed records`

### Card 8: embed the record in every mutating result type

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/fabrictest/verbs.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Embed `MutationRecord` as the **first** field of each of these eleven result types, so `res.Mutated()` is available on all of them: `AddResult`, `RemoveResult`, `CheckoutResult`, `PruneResult`, `CleanupResult`, `UnwireVerbResult`, `UnwireResult`, `ReconcileResult`, `CommitResult`, `PullResult`, `CloneResult`.

  `UnwireResult` (`internal/fabricengine/junction.go`) is on this list even though it is the *inner* type rather than a verb boundary: `internal/fabricengine/fabrictest/verbs.go`'s unwire cells drive `UnwireJunctions` directly, so a record that stopped at `UnwireVerbResult` would leave those cells unable to compile in batch 7.
  The general rule the implementer applies when in doubt: any result type a harness cell reads carries the record.

  `StatusResult` (`internal/fabricengine/status.go`) and `DiffResult` (`internal/fabricengine/diff.go`) are the read-only verbs' types and must **not** gain the embed — the scope decision is machine-held by batch 8's guard, and a record on a verb that cannot mutate is noise.

  Each result type's doc comment gains one sentence naming the embed and what it carries.
  Do not change any other field, tag, or ordering of the existing fields.
- **Commit:** `feat(fabricengine): embed the mutation record in every mutating result type`

### Card 9: named results plus the populating defer, Topology verbs

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/topology.go`
- **Edits:**
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Convert each of these entry points to named results and install the recorder:

  - `(*Topology).Add(l *lyxcwd.Location, slug string, opts AddOptions) (res AddResult, err error)` — `internal/fabricengine/add.go`
  - `(*Topology).Remove(l *lyxcwd.Location, slug string, force bool) (res RemoveResult, err error)` — `internal/fabricengine/remove.go`
  - `(*Topology).Checkout(l *lyxcwd.Location, branch string) (res CheckoutResult, err error)` — `internal/fabricengine/checkout.go`
  - `(*Topology).Prune(l *lyxcwd.Location, apply, force bool) (res PruneResult, err error)` — `internal/fabricengine/prune.go`
  - `(*Topology).Cleanup(l *lyxcwd.Location, apply, force bool) (res CleanupResult, err error)` — `internal/fabricengine/cleanup.go`
  - `(*Topology).Reconcile(l *lyxcwd.Location) (res ReconcileResult, err error)` — `internal/fabricengine/reconcile.go`
  - `UnwireJunctions(l *lyxcwd.Location, slug string, names []string) (res UnwireResult, err error)` — `internal/fabricengine/junction.go`
  - `Unwire(cwd string) (res UnwireVerbResult, err error)` — `internal/fabricengine/unwire.go`

  In each body, immediately after the entry point has a `*lyxcwd.Location` in hand, construct the recorder against the hub root and install the defer:

  ```go
  rec := NewMutations(l.HubPath)
  defer func() { res.Mutations = rec.Snapshot() }()
  ```

  `Unwire(cwd)` resolves its own `*lyxcwd.Location` before it can build the recorder;
  declare `var rec *Mutations` at the top, install the defer immediately (it is nil-safe per card 7), and assign `rec = NewMutations(l.HubPath)` as soon as `l` is resolved.

  `Unwire` additionally folds its inner call's record into its own, immediately after the `UnwireJunctions` call at `internal/fabricengine/unwire.go:69` returns:
  `rec.Extend(junctionResult.Mutated())` — placed so it runs on the error path too, since a partially-completed unwire is exactly the mutated-then-errored shape this slice exists to represent.

  Do **not** rewrite the existing `return XResult{}, err` sites;
  do **not** change any verb's observable behaviour, error text, or field population.
  The only permitted body changes are the recorder construction, the defer, and `Unwire`'s one `Extend` call.
  Where a body already declares a local named `res`, `result`, or `err` that would collide with the new named results, rename the *local*, never the result.
- **Commit:** `feat(fabricengine): give every Topology verb a recorder and a populating defer`

### Card 10: named results plus the populating defer, Fabric verbs and CloneHub

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Convert to named results and install the recorder:

  - `(*Fabric).Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (res CommitResult, err error)` — `internal/fabricengine/commit.go`
  - `(*Fabric).Pull(opts SyncOptions) (res PullResult, err error)` — `internal/fabricengine/pull.go`
  - `CloneHub(cwd string, opts CloneOptions) (res CloneResult, err error)` — `internal/fabricengine/clone.go`

  For the two `*Fabric` methods the hub root is `filepath.Dir(f.warpPath)` — the same derivation `(*Fabric).ResetHard` already uses in `internal/fabricengine/destroy.go`, and correct because `Fabric` holds no `*lyxcwd.Location` and the parent of the warp worktree path is the hub.
  Construct `rec := NewMutations(filepath.Dir(f.warpPath))` and install `defer func() { res.Mutations = rec.Snapshot() }()` as the first statements of each body.

  `CloneHub` mints the hub, so its hub root is not known at entry.
  Declare `var rec *Mutations` and install the nil-safe defer as the first statements, then assign `rec = NewMutations(hubPath)` at the point `hubPath` is first derived — before the `createExclusiveDir(hubPath)` call at `internal/fabricengine/clone.go:225`, so the hub-minting record itself is captured by batch 4.

  `(*Fabric).Pull`'s `if opts.SkipGit { return PullResult{}, nil }` early return stays exactly as written;
  the defer still populates it with the (empty) record.

  Do not rewrite the existing zero-result returns, and do not change any behaviour, error text, or field population.
- **Commit:** `feat(fabricengine): give Commit, Pull and CloneHub a recorder and a populating defer`

### Card 11: introduce `PushResult`

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/spawn_test.go`
  - `internal/fabricengine/coalesce_integration_test.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare in `internal/fabricengine/weftgit.go`:

  ```go
  type PushResult struct {
  	MutationRecord
  }
  ```

  Its doc comment states that it exists solely to carry the mutation record — `push` had no result type at all before this slice, which is why its envelope had nowhere to put one — and that all three push entry points return the same type so a composed `push` verb concatenates homogeneous records.

  Change all three push entry points to return it, each with named results and the populating defer:

  - `(*Fabric).PushWeft(opts SyncOptions) (res PushResult, err error)` — `internal/fabricengine/weftgit.go`, hub root `filepath.Dir(f.warpPath)`
  - `PushWarpAt(warpPath string, opts SyncOptions) (res PushResult, err error)` — `internal/fabricengine/spawn.go`, hub root `filepath.Dir(warpPath)`
  - `CoalescePushBothAt(warpPath, weftPath string, opts SyncOptions) (res PushResult, err error)` — `internal/fabricengine/coalesce.go`, hub root `filepath.Dir(warpPath)`

  `CoalescePushBothAt` accepts an empty `warpPath` as a true no-op today (see `internal/fabricengine/coalesce_integration_test.go`'s own assertion);
  in that case build the recorder with an empty hub root, which card 1's `Append` already handles by recording the absolute slashed path.

  Update the four call sites:

  - `internal/fabriccli/weft_verbs.go:179` — `CoalescePushBothAt` in the `push --bypass` branch, currently `if err := ...; err != nil`
  - `internal/fabriccli/weft_verbs.go:192` — `fab.PushWeft(opts)` in the ordinary `push` branch
  - `internal/fabricengine/spawn_test.go:50` — `PushWarpAt("/nonexistent-warp-path", tt.opts)`
  - `internal/fabricengine/coalesce_integration_test.go` — its `CoalescePushBothAt` call sites. Do not work from a count: several further hits on that token in the file are `t.Fatalf` message text rather than calls, and the chained `go vet -tags integration` is the authority, exactly as cards 13, 18 and 30 treat their own lists.

  At the two `internal/fabriccli/weft_verbs.go` sites, discard the result with `_` for now — batch 6 is what threads it into the envelope.
  Preserve every existing assertion's meaning in the two test files;
  this is a mechanical signature repoint, not a behaviour change.
  `SpawnDetachedPush` keeps its current `error`-only signature: the push it launches happens in another process, so there is nothing for it to record.
- **Commit:** `feat(fabricengine): introduce PushResult so push can carry a record`

## Batch Tests

`verify: go test ./internal/fabricengine/ ./internal/fabriccli/ ./internal/boardengine/` runs the untagged suites of the three packages that consume the changed signatures.
`internal/boardengine` is included because it is `fabricengine`'s other in-process consumer (through `Bolt`, which this slice does not reshape) — running it is what proves the embed and the signature changes did not reach it.
`internal/fabricengine/coalesce_integration_test.go` is edited by card 11 but sits behind the `integration` tag, which neither `go test` untagged nor the module-wide `go build ./...` gate compiles — so the verify chains `go vet -tags integration ./internal/fabricengine/...` to type-check the tagged files (including `internal/fabricengine/fabrictest`) without paying for a full tagged run.
That chained vet is the only thing that catches a card-11 signature repoint that missed a tagged call site.
