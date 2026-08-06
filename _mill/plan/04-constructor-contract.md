# Batch: constructor contract (unexport)

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'constructor contract (unexport)'
number: 4
cards: 3
verify: go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
depends-on: [2, 3]
```

## Batch Scope

The contract step of the expand-migrate-contract sequence: with every external caller migrated (batch 03) and the typed-reason work landed (batch 02), `New` becomes private `newPaired`, `Fabric.Warp`/`Fabric.Weft` go private, and `PartialCommitError`'s fields go private.
The four `package fabricengine_test` files that reached these from outside move onto an `export_test.go` shim (decision `export-test-shim`);
`fabric_test.go`'s missing-path contract is restated through `Open`.
`WeftWorktree` stays exported — `fabriccli` (an owner) still calls it at `weft_verbs.go:244`;
batch 07's enforcement test polices non-owner use.
No external interface changes for later batches: from outside fabric, `Open` is now the only constructor.

## Cards

### Card 14: unexport `New` and the `Fabric` fields, add the test shim

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/diff.go`
  - `internal/fabricengine/pull.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/warpforward.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
  - `internal/fabricengine/warpforward_integration_test.go`
  - `internal/fabricengine/checkout_index_refresh_test.go`
- **Creates:**
  - `internal/fabricengine/export_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `fabric.go`: rename `New` → `newPaired` and the struct fields `Fabric.Warp` → `Fabric.warp`, `Fabric.Weft` → `Fabric.weft`.
  Retarget every in-package reference — grep confirmed the field references live in `fabric.go`, `commit.go`, `diff.go`, `index.go`, `pull.go`, `revert.go`, `warpforward.go`, `weftgit.go`, plus comment references in `doc.go`;
  the in-package `New` callers are `open.go` (card 1's forwarding call), `index.go:307`, and `unwire.go:100`.
  Create `export_test.go` in `package fabricengine` re-exporting the constructor and both fields for external-package tests, e.g. `var NewPairedForTest = newPaired` plus accessor funcs `WarpForTest(f *Fabric) *gitrepo.Repo` / `WeftForTest(f *Fabric) *gitrepo.Repo` (match the actual field types).
  Migrate `weftgit_exclude_test.go` (4 uses), `warpforward_integration_test.go` (4), and `checkout_index_refresh_test.go` (2) onto the shim — these build fixtures from raw scratch paths no `lyxcwd.Location` describes, so they keep the raw-path constructor via the shim rather than `Open`.
  `fabric_test.go` is card 15's file — leave it compiling by whatever minimal shim references it needs only if the compiler forces it;
  otherwise do not touch it here.
- **Commit:** `refactor(fabricengine): unexport New as newPaired, make Fabric.warp/weft private`

### Card 15: restate `fabric_test.go` through `Open`

- **Context:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/export_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/fabric_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Restate `fabric_test.go`'s missing-path contract tests (`:25,44`) through `Open(l)` — that is the constructor the contract now belongs to;
  the assertions (host checked first, `*ErrMissingPath` naming the absent side) carry over from card 1's `open_integration_test.go` shape.
  Any remaining `New`/`.Warp`/`.Weft` uses in the file (7 total today) move onto the `export_test.go` shim where a raw-path fixture makes `Open` unusable.
  Deduplicate rather than double-cover: if a contract case is now fully covered by `open_integration_test.go`, keep exactly one home for it and note the move in the test comment.
- **Commit:** `test(fabricengine): restate missing-path contract through Open`

### Card 16: `PartialCommitError` fields private

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `partial-commit-error-fields-private`: the `PartialCommitError` struct fields (`commit.go:31`) `WarpSHA` → `warpSHA`, `WeftSHA` → `weftSHA`, `WeftCommitted` → `weftCommitted` go private;
  its `Error()` string is byte-identical before and after.
  Grep verified zero readers outside `internal/fabricengine` (production and test);
  the type is still returned to external callers and matched with `errors.As` — only field access goes away.
  Re-verify with `grep -rn 'PartialCommitError' internal cmd | grep -v fabricengine` before committing.
- **Commit:** `refactor(fabricengine): make PartialCommitError fields private`

## Batch Tests

`verify:` runs `go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/` — the four migrated external-package test files plus `fabriccli` (the one remaining production `WeftWorktree`/`Open` caller) prove the contract change compiles and behaves from both inside and outside the package.
