# Batch: enforcement-and-extraction

```yaml
task: 'fabric: live-state integration harness (slice 13)'
batch: 'enforcement-and-extraction'
number: 1
cards: 4
verify: go build ./... && go test ./internal/lyxcwd/ -run TestEnforcement && go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource && go test -tags integration ./internal/fabriccli/
depends-on: []
```

## Batch Scope

This batch lands everything the `fabrictest` package needs to exist **before** its first file is written: the one production extraction the hub factory calls (`fabriccli.CloneAndWire`), the two enforcement-map owner rows and the one guard-walk exclusion that would otherwise fail `go test ./...` the moment a non-test `.go` file appears under `internal/fabricengine/fabrictest`, and the matching `CONSTRAINTS.md` text.
It is one batch because all four are the same obligation — "the tree still builds" — and because the discussion is explicit that each enforcement change must land in the same commit as the code that needs it, which for a package created in batch 2 means landing here, one batch ahead.

The external interface batch 2 consumes is `fabriccli.CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error)`.

Batch-local decision: the owner rows and the guard exclusion are added **before** the directory they name exists.
This is safe — every one of the three maps is an exact-match lookup consulted only when a scanned file's directory matches, and none of the three guards asserts that every registered owner directory exists.

## Cards

### Card 1: extract `fabriccli.CloneAndWire` from `runCloneWithReset`

- **Context:**
  - `internal/fabricengine/clone.go`
  - `internal/fabriccli/fabric.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabriccli/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an exported `CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error)` to `internal/fabriccli/clone.go`, holding the engine-y middle of the existing `runCloneWithReset` verbatim: `fabricengine.CloneHub(cwd, opts)`, then `configsync.ReconcileFabricAt(res.BoardDir, true)`, then `fabricengine.NewBolt(res.BoardDir)` with `.Commit("fabric clone: record anchor + repo-wide config", fabricengine.SyncOptions{})` and `.Push(fabricengine.SyncOptions{})`, then `lyxcwd.Resolve(res.PrimeCwd)`, then `fabricengine.WiredNames(res.BoardDir)`, then `fabricengine.WireJunctions(l, filepath.Base(l.WorktreePath()), names)`, then `configsync.ReconcileAll(res.WeftBase, true)`.
  It returns `(res, nil)` on success and `(fabricengine.CloneResult{}, err)` on the first failure, preserving the existing early-return order exactly.
  Rewrite `runCloneWithReset` to keep its own argument parsing, its `lyxcwd.Getwd()` call and its usage error, then call `CloneAndWire` once, map a non-nil error through `output.Err(out, err.Error())`, and build the same `output.Ok` envelope from the returned `CloneResult` (`hub`, `anchor`, `warp`, `warp_binding_recorded`).
  This is a pure extraction: same sequence, same order, same errors, no behaviour change, and the CLI/Cobra seam (`Command()`/`RunCLI`, every `Short`) is untouched.
  The `.lyx`-exclusion comment currently sitting above the `configsync.ReconcileAll` call moves with the code into `CloneAndWire`.
  `CloneAndWire`'s doc comment states that it is the single wiring sequence shared by the cobra handler and `internal/fabricengine/fabrictest`'s hub factory, and that a second copy of the sequence anywhere is the drift this extraction exists to prevent.
- **Commit:** `fabriccli: extract CloneAndWire from runCloneWithReset`

### Card 2: register `fabrictest` in both fabric-vocabulary owner maps

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/lyxcwd/enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the exact key `"internal/fabricengine/fabrictest"` with value `true` to **both** `fabricVocabularyOwners` and `weftnameImportOwners` in `internal/lyxcwd/enforcement_test.go`.
  Both maps are exact-match directory lookups with no prefix or subtree matching, so `internal/fabricengine/fabrictest` inherits nothing from `internal/fabricengine` and every bare `warp`/`weft` identifier, string literal, or comment in the package's non-test `.go` files would otherwise fail `TestEnforcement_FabricVocabulary`, as would its import of `internal/weftname`.
  Add a comment above each new row recording that the row exists because `fabrictest` is a directory of non-test `.go` files (the factory must be non-test to be importable across packages) that names fabric's own geometry and imports `weftname` for the weft-suffix hostile input.
  Do **not** add a `geometryTokenOwners` row — geometry literals are handled by routing through the exported accessors instead.
  Leave `configsyncOwnerDir` and the host-phrase rules untouched;
  the host rule still applies inside the new owner directory.
- **Commit:** `lyxcwd: register fabrictest in the fabric-vocabulary owner maps`

### Card 3: exclude the `fabrictest` subdirectory from the destructive-bypass guard walk

- **Context:**
  - `CONSTRAINTS.md`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `cmd/lyx/destructiveguard_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `cmd/lyx/destructiveguard_test.go`, add a directory-level exclusion so `TestNoDestructiveBypass_FabricengineProductionSource`'s `filepath.WalkDir` skips `internal/fabricengine/fabrictest` entirely.
  Implement it as a named package-level variable `destructiveGuardExcludedDirs` mapping a module-relative slash-separated directory to its reason, consulted in the walk callback's `d.IsDir()` branch: compute the directory's module-relative slash-separated path with `filepath.Rel` plus `filepath.ToSlash`, and return `fs.SkipDir` when it is a key.
  The single entry is `"internal/fabricengine/fabrictest"` with the reason that it is a different package — a test-support package whose state builders must plant and tear down hostile filesystem shapes through `fslink.Remove` and `os.Remove` — so excluding it restores the guard to exactly the scope its own invariant text already claims ("the only file in *package fabricengine*"), where per-file allowlist rows would punch a growing set of holes.
  A directory exclusion is deliberately preferred over per-file allowlist rows, and over confining every destructive token to `_test.go` files, which would strand the state builders inside the test binary where `fabricengine_test` consumers cannot reach them.
  Extend the file's header comment to state that the guard now carries both a per-file allowlist and a directory exclusion, and that the two answer different questions.
  Leave `destructiveGuardMinScannedFiles` at 30 — `internal/fabricengine` carries far more production files than that and the exclusion removes none of them.
- **Commit:** `lyx: exclude fabrictest from the destructive-bypass guard walk`

### Card 4: record both enforcement changes in `CONSTRAINTS.md`

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `cmd/lyx/destructiveguard_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits, both in the same commit as cards 2 and 3 per the Documentation Lifecycle.
  First, in the **Fabric Vocabulary Invariant**'s "Owner set carves out the bare weft/warp rule only" bullet, add `internal/fabricengine/fabrictest` to the listed owner set so the file matches `fabricVocabularyOwners`, and note that it is an owner for the `weftname` import rule too — i.e. it joins the same three-member subset as `internal/fabricengine`, `internal/fabriccli` and `internal/lyxtest` rather than the wider bare-token set.
  Second, in the **Fabric Destruction Chokepoint Invariant**, state that the guard's walk is package-scoped in intent and names the `internal/fabricengine/fabrictest` subpackage exclusion explicitly, with the reason that `fabrictest` is a separate test-support package whose state builders plant and tear down hostile filesystem shapes and that is therefore outside the invariant's subject.
  Keep the file's existing style: rules only, no rationale essays, semantic line breaks with one sentence per line.
  Do not add a new invariant section — both changes amend existing ones.
- **Commit:** `docs: record the fabrictest vocabulary owner row and destruction-guard exclusion`

## Batch Tests

`verify:` runs four scoped commands.
`go build ./...` proves the `CloneAndWire` extraction still compiles every consumer.
`go test ./internal/lyxcwd/ -run TestEnforcement` covers `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals`, the two guards card 2 touches.
`go test ./cmd/lyx/ -run TestNoDestructiveBypass_FabricengineProductionSource` covers card 3's exclusion — in particular that it still scans at least `destructiveGuardMinScannedFiles` production files, so an over-broad exclusion fails loudly instead of vacuously passing.
`go test -tags integration ./internal/fabriccli/` is the regression proof for card 1: `cli_test.go` and `pushbypass_integration_test.go` already drive `fabric clone` end to end and must stay green **unmodified** — the extraction adds no new test of its own, exactly as the discussion's testing section specifies.
The scope is deliberately narrower than `go test ./...`;
whole-tree coverage is the configured `pipeline.done_gate`, which runs once at the end.
