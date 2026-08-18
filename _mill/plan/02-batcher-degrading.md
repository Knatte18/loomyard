# Batch: batcher moves to the degrading side

```yaml
task: websterengine + webstercli told-geometry, and Webster standalone entry
batch: batcher moves to the degrading side
number: 2
cards: 3
verify: go test ./internal/batcher/...
depends-on: []
```

## Batch Scope

`batcher.Active` is the last config read on webster's batching path that still uses the strict `configengine.Load`, and a standalone webster reaches it on every verb outside a hub, where `_lyx/` does not exist.
This batch moves `Active` to `configengine.LoadOrTemplate` and updates the Config Strictness Invariant's pinned sets in the same commit sequence, resolving the invariant's own "watch item for T7/T10" paragraph rather than leaving it describing a condition that has already fired.
It is independent of every other batch and lands early so batch 8's standalone integration test has a `batcher.Active` that degrades instead of hard-failing.

## Cards

### Card 6: `batcher.Active` loads through `LoadOrTemplate`

- **Context:**
  - `internal/configengine/config.go`
  - `internal/websterengine/config.go`
  - `internal/reedengine/config.go`
  - `internal/batcher/template.go`
- **Edits:**
  - `internal/batcher/config.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Active`, replace `configengine.Load(baseDir, moduleName, []byte(ConfigTemplate()))` with `configengine.LoadOrTemplate(baseDir, moduleName, []byte(ConfigTemplate()))`.
  Delete the `strings.Contains(err.Error(), "not initialized")` rewrap branch and its `fmt.Errorf("not initialized here; run \"lyx fabric reconcile\"")` result: `LoadOrTemplate` resolves the embedded template on an absent `_lyx/` directory and on an absent config file alike, so that branch is unreachable for the absent-`_lyx` case it was written for.
  Drop the now-unused `strings` import;
  keep `fmt` only if it still has a user (the `unmarshal batcher config` wrap).
  Rewrite `Active`'s doc comment: the two sentences documenting the strict absent-`_lyx` error and the absent-`batcher.yaml` error must be replaced by a statement that both absences resolve `ConfigTemplate()` instead, matching the shape `internal/reedengine/config.go` and `internal/websterengine/config.go` already use.
  Keep the sentence stating that `baseDir` must already be resolved by the caller and that `Active` never resolves cwd itself, per the Cwd Resolution Invariant.
- **Commit:** `refactor(batcher): resolve batcher.yaml through configengine.LoadOrTemplate`

### Card 7: Pin the degrading behaviour

- **Context:**
  - `internal/batcher/config.go`
  - `internal/batcher/identity.go`
  - `internal/batcher/registry.go`
- **Edits:**
  - `internal/batcher/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a test to `internal/batcher/config_test.go` that calls `Active` against a `t.TempDir()` that contains no `_lyx/` directory at all and asserts it returns the template-selected batchifier with a nil error, rather than the old "not initialized here" failure.
  Add a second case for a `baseDir` whose `_lyx/` exists but holds no `batcher.yaml`, asserting the same template resolution.
  Assert the returned `Batcher` is the identity batchifier the shipped `ConfigTemplate()` selects, mirroring the per-loader tests that already exist for the other degrading modules.
  Update or replace any existing case in this file that asserts the strict absent-`_lyx` error text, since that behaviour is gone.
- **Commit:** `test(batcher): pin Active's template fallback for an absent _lyx and an absent batcher.yaml`

### Card 8: Move `batcher` to the degrading pinned set

- **Context:**
  - `internal/batcher/config.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Config Strictness Invariant, the "two pinned sets" bullet currently reads degrading `{shuttleengine, reedengine, perchengine, websterengine}` and strict `{fabricengine, boardengine, loomengine, batcher}`.
  Move `batcher` into the degrading set and out of the strict set.
  Rewrite the "A watch item for T7/T10" bullet: it asserts `batcher` "sits on the strict side because it has no standalone entry of its own" and predicts that a standalone Webster reaching `batcher.Active` would move it — that condition has now fired, so the bullet must record the move as done and name this task as what fired it, rather than continuing to describe it as pending.
  Change no other bullet in this invariant, and change no other invariant in this file — batch 8 owns the Stencil Ownership, Durable-vs-Ephemeral and Fabric Git rewords.
  Use semantic line breaks throughout, per CLAUDE.md.
- **Commit:** `docs(constraints): move batcher to the Config Strictness degrading set`

## Batch Tests

`verify:` is `go test ./internal/batcher/...` — the only package with a runnable surface in this batch.
Card 7 is the batch's own regression net: it pins both absence branches `LoadOrTemplate` now degrades on, which is exactly what the strict loader used to reject.
Card 8 is documentation with no runnable surface;
its correctness is a review obligation, as the Config Strictness Invariant's own "Enforced by" bullet already states.
The module-wide `verify:` in the overview (`go vet ./... && go vet -tags integration ./...`) catches any caller of `batcher.Active` that this signature-stable change would otherwise disturb.
