# Batch: cli-envelope

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
batch: 'cli-envelope'
number: 6
cards: 5
verify: go test ./internal/fabriccli/ ./internal/output/ && go test -tags integration ./internal/fabriccli/
depends-on: [5]
```

## Batch Scope

This batch is where the record becomes visible to the operator: `internal/fabriccli` emits `mutations` and `partial` on both the success and the failure path of all twelve mutating verbs, plus a structured `refusal` object when the returned error carries a gate refusal.
It also records the CLI's own layer — `CloneAndWire` and `runReconcile` mutate substantially above the engine, and `CloneAndWire` alone returns a bare zero `CloneResult` at eight sites today, which is the same defect one layer up.
It is one batch because the emission helper, the CLI-layer recording, and the assertions all describe one envelope contract, and a half-applied contract is worse than none: a consumer that finds `mutations` on eight verbs and not on the other four learns nothing it can rely on.

Batch-local decision: the emission goes through one small helper pair in `internal/fabriccli` rather than being open-coded at each of the twenty-odd `output.Ok`/`output.Err` sites, so the fixed key set is declared once and cannot vary by verb.

## Cards

### Card 23: the envelope helper

- **Context:**
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
  - `internal/output/output.go`
  - `internal/fabriccli/fabric.go`
- **Edits:** none
- **Creates:**
  - `internal/fabriccli/envelope.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/fabriccli/envelope.go` in `package fabriccli`, declaring the two functions every mutating verb's handler routes its output through:

  ```go
  func okWithRecord(w io.Writer, rec fabricengine.Mutations, fields map[string]any) int
  func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int
  ```

  `okWithRecord` sets `fields["mutations"] = rec.Entries()` and `fields["partial"] = false`, then delegates to `output.Ok`. `partial` is unconditionally false on the success path because its one derivation rule is `error ≠ nil ∧ record non-empty`, and there is no error here.

  `errWithRecord` builds a fields map carrying `"mutations": rec.Entries()` and `"partial": rec.Len() > 0`, adds a `"refusal"` key when `fabricengine.RefusalOf(err)` reports `true` — a map with exactly the four keys `check`, `what`, `target`, `reason`, the `check` value being the `Check`'s string form — and delegates to `output.ErrFields(w, err.Error(), fields)`. The flattened `error` string is retained unchanged: dropping it would break the "every failure carries an `error` string" contract every other module's envelope holds.

  Both helpers rely on `Entries()` never returning `nil`, so `mutations` is always a JSON array and never `null`, and both always set `partial`, so it is always a bool and never absent.
  A consumer therefore never has to distinguish absent from false, and the key set does not vary by outcome — which is the property that lets a test assert the shape once per verb instead of once per path.
  State that in the file's doc comment, alongside the note that `internal/fabricengine`'s read-only verbs (`list`, `pairs`, `status`, `diff`) deliberately do **not** route through these helpers.
- **Commit:** `feat(fabriccli): add the record-carrying envelope helpers`

### Card 24: emit the record from the Topology verb handlers

- **Context:**
  - `internal/fabriccli/envelope.go`
  - `internal/fabricengine/mutation.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Route these handlers' verb-result output through the card-23 helpers, keeping every existing top-level field byte-identical:

  - `runAdd`, `runCheckout`, `runPruneWithFlags`, `runCleanupWithFlags`, `runRemoveWithFlag` — `internal/fabriccli/fabric.go`
  - the unwire handler — `internal/fabriccli/unwire.go`

  For each: the `output.Ok(out, map[string]any{...})` success return becomes `okWithRecord(out, r.Mutated(), map[string]any{...})` with the same fields, and the `output.Err(out, err.Error())` return that follows the **verb call** becomes `errWithRecord(out, r.Mutated(), err)`.

  The `output.Err` sites that precede **any mutation** — cwd/location resolution failures, `LoadConfig` failures, and the `usage: ...` argument errors — stay `output.Err` exactly as they are.
  Nothing has been mutated at those points, there is no result to read a record from, and inventing an empty record for them would put `mutations` on an envelope that is not a verb outcome at all.
  Add a one-line comment at the first such site in each handler saying so, so the asymmetry reads as a decision.

  **The carve-out's boundary is "before any CLI-layer mutation", not "before the verb call".** The two differ in `runReconcile`, where `configsync.ReconcileFabricAt` runs *before* `LoadConfig` and may already have written a file — card 25 enumerates that handler's returns explicitly for this reason.

  `runReconcile` is deliberately absent from that list: it does its own CLI-layer mutating, so card 25 both records and converts it in one place rather than this card converting an emission card 25 would immediately rewrite.

  The read-only handlers `runList` and `runPairs` (`internal/fabriccli/fabric.go`) are **not** touched — no `mutations`, no `partial`.
- **Commit:** `feat(fabriccli): emit mutations and partial from the topology verb handlers`

### Card 25: record and emit the CLI layer's own mutations

- **Context:**
  - `internal/fabriccli/envelope.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/bolt.go`
  - `internal/configsync/configsync.go`
  - `internal/configengine/config.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/fabrictest/verbs.go`
- **Edits:**
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two verbs mutate substantially *above* the engine, and their zero-result returns are the same defect one layer up.
  The rule for both: the CLI owns a recorder for its own layer, seeds it with the engine call's record, appends its own steps in execution order, and returns the accumulated record on every path — including the failure paths that return a bare zero result today.

  **`CloneAndWire`** — `internal/fabriccli/clone.go`. Convert it to named results (`func CloneAndWire(cwd string, opts fabricengine.CloneOptions) (res fabricengine.CloneResult, err error)`), build `rec := fabricengine.NewMutations(res.HubPath)` once `CloneHub` has returned, seed it with `rec.Extend(res.Mutated())`, and install `defer func() { res.Mutations = rec.Snapshot() }()` so all eight `return fabricengine.CloneResult{}, err` sites carry the accumulated record without being individually rewritten.
  Note the ordering constraint: the defer can only be installed after `rec` exists, so the first return (the `CloneHub` failure itself) instead returns `res, err` directly — `CloneHub`'s own record is already in `res` at that point, and dropping it there would discard the hub the clone had just minted.
  Hand-record the CLI-layer steps at their success sites.
  The two `configsync` calls do **not** write one file each: `configsync.ReconcileAll(baseDir, apply)` returns a `[]Result`, one per registered module, and each `Result` carries an `Applied bool` reporting whether that module's file was written.
  Record **one `KindFileWritten` per `Result` whose `Applied` is true**, with the target `configengine.ConfigFile(baseDir, r.Module)`.
  That is a **new import** for `internal/fabriccli/clone.go` and `internal/fabriccli/fabric.go` — add it. (The package's `integration`-tagged external test file imports `configengine` today, but an external test package's imports establish nothing for production code beside it.)
  `configsync.ReconcileFabricAt` returns a single `Result` and records at most one entry, on the same `Applied` condition.
  A single entry per call would leave every other materialised config file as an uncovered addition, which batch 7's omission direction fires on.
  Then the `Bolt.Commit` call site as `KindCommitCreated` with the **board worktree path** (`res.BoardDir`) as its `Target` and the SHA `Commit` already returns as its `Detail`.
  The `Target` matters beyond consistency with card 19's "the worktree the commit landed in" rule: it is what supplies the omission-direction coverage for the board-side `.lyx-warp` and config writes in the clone and reconcile cells.
  The `WireJunctionsWith` call needs no entry of its own — it records `link_created` internally from batch 5.
  `fabricengine.Bolt` itself is **out of scope as a type** — it keeps its `(sha, committed, err)` signature untouched;
  what records is its *call site* here, which already has the SHA and the committed flag in hand.
  Record the commit only when the returned `committed` flag is true.

  **`Bolt.Push` records nothing, and that is deliberate.** It returns a bare `error` and reaches `gitrepo.PushCoalesced`, which returns `nil` both when a push landed and when `HasUnpushed` was already false and nothing was contacted — so a `KindBranchPushed` there would assert an outcome this call did not observe.
  That is the lie of commission this slice exists to eliminate, and it is worse than the entry's absence: the commit is already recorded, and `branch_pushed` is a git-state kind exempt from the oracle's commission direction, so omitting it costs the cross-check nothing.
  The before/after `HasUnpushed` predicate batch 5 card 19 uses is not available here without reaching past `Bolt` to a `gitrepo` handle, which would widen this card into the type the discussion scoped out.
  Write that reasoning into a comment at the call site so the next reader does not "fix" it by adding the entry.

  `runCloneWithReset` then emits through the card-23 helpers, keeping its four existing fields (`hub`, `anchor`, `warp`, `warp_binding_recorded`).
  Its `output.Err` **after** the `CloneAndWire` call becomes `errWithRecord(out, res.Mutated(), err)` — `CloneAndWire`'s defer has populated the result by then, and that failure path is precisely the mutated-then-errored case this slice exists to represent.
  The two returns before it — the `lyxcwd.Getwd()` failure and the `usage: ...` argument error — stay bare `output.Err`: nothing has been mutated at either.

  **`runReconcile`** — `internal/fabriccli/fabric.go`. **Build the recorder before the first mutation, not after the engine call**, or the emitted array misstates the order: `configsync.ReconcileFabricAt` runs *before* `top.Reconcile(l)` in this handler, so seeding from `r.Mutated()` first and appending the config rewrite afterwards would claim the rewrite happened after the engine's mutations — and array order is the only thing carrying ordering in this vocabulary.
  The sequence is: construct `rec := fabricengine.NewMutations(l.HubPath)` as soon as `l` resolves;
  append the `configsync.ReconcileFabricAt` entry at its own call site as `KindFileWritten` when its returned `Result.Applied` is true;
  then `rec.Extend(r.Mutated())` once `top.Reconcile(l)` returns;
  then record the `Bolt.Commit` backfill step as `KindCommitCreated` — **only when the returned `committed` flag is true**, exactly as the `CloneAndWire` half requires, since `Bolt.Commit` returns `("", false, nil)` on a no-op and an unconditional entry would record a commit that never happened — with the board worktree path (`fabricengine.BoardDir(l.HubPath)`) as its `Target` and the returned SHA as its `Detail`.
  Its `Bolt.Push` records nothing either, for the same unobservable-outcome reason spelled out above.
  Its existing "a failed backfill commit or push is non-fatal and downgrades the reported outcome, never the exit code" behaviour is unchanged — the record simply carries whichever steps did land.
  Emit through the card-23 helpers, keeping `pairs`, `warp_binding` and the conditional `warp_binding_detail`.

  **The three post-`l` `output.Err` sites become `errWithRecord`, carrying `rec`'s accumulated entries** — the `ReconcileFabricAt` failure, the `LoadConfig` failure, and the `Reconcile` failure.
  The handler has **four** `output.Err` sites in total;
  the first, the `resolveWarpLocation()` failure, stays a bare `output.Err` under card 24's pre-mutation carve-out — it precedes both `l` and the recorder, so there is nothing to carry.
  None of them qualifies for card 24's pre-flight carve-out: `ReconcileFabricAt` runs first in this handler and may have written a file before any of the three is reached, so all three are post-mutation returns even though two of them look like ordinary pre-flight failures in every other handler.
  The `ReconcileFabricAt` failure site is the subtle one — it can fail *after* a partial write, so it emits whatever `rec` holds rather than a bare error.
  This is the one handler where "pre-flight" and "pre-mutation" come apart, and it is why the carve-out is stated as the latter.

  This is not optional polish: `internal/fabricengine/fabrictest/verbs.go`'s `CloneHubReset`/`RealHub` cell drives `CloneAndWire`, so batch 7's unfiltered honesty diff contains those junctions, config writes and commits, and the omission direction fires on them if they go unrecorded.
- **Commit:** `feat(fabriccli): record and emit the CLI layer's own mutations`

### Card 26: emit the record from the weft verb handlers

- **Context:**
  - `internal/fabriccli/envelope.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/coalesce.go`
  - `internal/fabricengine/spawn.go`
  - `internal/fabriccli/spawn.go`
- **Edits:**
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/fabriccli/weft_verbs.go`, route `commit`, `push`, `pull` and `sync` through the card-23 helpers.
  The composition rule for the two composed verbs: concatenate the composed calls' records into **one flat** `mutations` array in execution order, and `partial` is true when any composed call returned an error and the **combined** record is non-empty.
  Build a local `rec := fabricengine.NewMutations(<hub root>)` per invocation and `rec.Extend(...)` each composed call's record as it returns, so a failure after the first call still emits the first call's record.

  - **`commit`** — one call, `fab.Commit(...)`. Keep the existing `committed` and `sha` fields.
  - **`push`** — two in-process calls in the default branch: `fab.Commit(...)` then `fab.PushWeft(opts)`. Its envelope is therefore already a concatenation — the commit record followed by the push record. A commit that lands followed by a push that fails is exactly "mutated, then errored", and the combined record is what makes it visible. The `--bypass` branch instead calls `CoalescePushBothAt` alone and carries that one record. Both branches keep their currently-empty field maps, which now gain `mutations` and `partial` and nothing else.
  - **`pull`** — one call, `fab.Pull(...)`. Keep every field `pullResultMap` produces.
  - **`sync`** — `fab.Commit(...)` then `spawnPush`, which delegates to `fabricengine.SpawnDetachedPush`, a **detached child process**. The push happens in another process after this one returns, so its outcome is unobservable here. `sync`'s envelope therefore carries the commit record plus exactly one `KindPushSpawned` entry — `Target` the weft worktree path, `Detail` the literal `"detached"` — appended after `spawnPush` returns nil. It must **never** emit `branch_pushed`, which would assert an outcome this process did not observe: that is precisely the control-flow-derived lie this slice exists to eliminate. Recording the spawn honestly is the point — the operator learns a push is in flight, not that a push succeeded.

  Neither verb needs a `PartialSyncError` or `PartialPushError`;
  the combined record plus `partial` carries the same information without a new error type.

  **The hub root differs by branch, and the bypass branch cannot use the ordinary derivation.** In the four ordinary branches the command's `PersistentPreRunE` has resolved a `*lyxcwd.Location`, so the hub root is `l.HubPath` — the canonical accessor, not a hand-rolled parent-of-warp computation.
  In `push --bypass` it has **not**: that branch takes the injected `--warp-path`/`--weft-path` values and returns before `resolveWarpLocation()`, leaving `l` nil, and `spawnPush` always injects an empty warp path (`fabricengine.SpawnDetachedPush("", weftPath)`), so any parent-of-warp derivation yields `"."` at best and dereferences nil at worst.
  Build the bypass branch's recorder with an **empty hub root**: it appends no path-targeted entries of its own — it only `Extend`s `CoalescePushBothAt`'s already-converted record, and `Extend` performs no conversion — so the hub root is never consulted on that path.
  Do not introduce a new geometry helper for either branch.
  `internal/fabriccli/spawn.go` is deliberately **not** edited: `spawnPush` keeps its `error`-only signature, and the `push_spawned` entry is appended by its caller in `internal/fabriccli/weft_verbs.go`, which owns the recorder.

  The read-only `status` and `diff` subcommands in this file are **not** touched.
- **Commit:** `feat(fabriccli): emit mutations and partial from the weft verb handlers`

### Card 27: envelope shape assertions

- **Context:**
  - `internal/fabriccli/envelope.go`
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabriccli/unwire.go`
  - `internal/fabricengine/mutation.go`
  - `internal/fabricengine/destroy.go`
  - `internal/fabriccli/testmain_test.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:**
  - `internal/fabriccli/envelope_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  The assertions split across two files, because `okWithRecord` and `errWithRecord` are unexported while `internal/fabriccli/cli_test.go` is an **external** test package (`package fabriccli_test`) behind the `integration` build tag:

  - `internal/fabriccli/envelope_test.go` — a **new, untagged** internal test file in `package fabriccli`, carrying every helper-level shape assertion. It calls the two helpers directly against an `io.Writer` buffer, spawns nothing, and contains no `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy*` token anywhere including comments (Test Tier Purity Invariant, raw substring match).
  - `internal/fabriccli/cli_test.go` — the existing `integration`-tagged external suite, extended with the end-to-end per-verb envelope assertions that need a real hub, following the file's existing fixture conventions.

  Assert:

  - **Success path:** `mutations` present (an empty JSON array when nothing was mutated, never `null`), `partial` present and `false`, and the verb's existing top-level fields unchanged. Backward compatibility of the success envelope is an explicit assertion, not an assumption — name at least `slug`, `path` and `links_removed` for `remove`, and the four clone fields.
  - **Failure path, empty record:** `ok:false`, `error` present, `mutations` present and empty, `partial` present and `false`.
  - **Failure path, non-empty record:** `ok:false`, `error` present, `mutations` populated, `partial:true`.
  - **Failure path carrying a refusal:** the `refusal` object present with all four keys (`check`, `what`, `target`, `reason`), *and* the flattened `error` string still present. Build the error with a real gate refusal reached through `fabricengine.RefusalOf`'s own contract rather than a hand-rolled stub.
  - **Read-only verbs:** `list`, `pairs`, `status` and `diff` assert `mutations` is **absent** from their envelope, so the which-verbs scope decision is machine-held rather than a convention.
  - **Reserved keys**, asserted per helper rather than jointly, because the two inject different sets: for `okWithRecord`, a caller supplying `ok`, `mutations` or `partial` is overridden — **not** `error`, which `output.Ok` never touches and which `okWithRecord` therefore does not reserve;
    for `errWithRecord`, a caller supplying `ok`, `error`, `mutations` or `partial` is overridden.
    The envelope's invariant fields cannot be shadowed by accident on either path.

  The first two bullets and the reserved-keys bullet belong in `internal/fabriccli/envelope_test.go` (no hub needed);
  the read-only-verb bullet and the refusal bullet belong in `internal/fabriccli/cli_test.go`, which can drive a real verb against a real hub.
  Place the remaining bullets in whichever file can assert them without a fixture it does not need.

  Every assertion decodes the emitted line as JSON and inspects keys;
  do not string-match the serialised envelope, which would pin key ordering `encoding/json` does not guarantee for a map.
- **Commit:** `test(fabriccli): assert the mutations/partial/refusal envelope shape`

## Batch Tests

`verify: go test ./internal/fabriccli/ ./internal/output/ && go test -tags integration ./internal/fabriccli/` runs the untagged suites of the package whose handlers this batch rewrites and of the package whose new `ErrFields` they depend on, then the tagged `internal/fabriccli` suite.
The tagged run is required rather than optional: `internal/fabriccli/cli_test.go` is `integration`-tagged, so card 27's end-to-end per-verb assertions are invisible to an untagged run, and so is any compile break cards 24-26's handler rewrites introduced in it.
The new untagged assertions live in `internal/fabriccli/envelope_test.go`;
`internal/output/output_test.go` is unchanged here and runs as a regression check that the additive `ErrFields` still has not disturbed `Ok` or `Err`.
`internal/fabricengine` is not in scope — this batch changes no engine code.
