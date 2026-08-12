# Batch: fabricengine in-package hub

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'fabricengine in-package hub'
number: 10
cards: 4
verify: go vet -tags integration ./... && go test -tags integration ./internal/fabricengine/...
depends-on: [9]
```

## Batch Scope

This batch finishes `internal/fabricengine`'s migration: the last seven in-package `Copy*` sites, all of them hub-shaped rather than weft-shaped — four `CopyWarpHub` plus two `CopyPairedLocal` in `hook_test.go`, and one `CopyWarpHub` in `warplayout_test.go`.
Both files move to `package fabricengine_test` for the same forced reason as batch 9.

These five `CopyWarpHub` sites are the ones the `gitkit.CopyRepo` caller-set guard exists to catch: reading batch 8's disposition table as "all `fabricengine` primitives stay on `gitkit`" would leave them calling a helper pinned to `internal/lyxcwd` alone, and `TestCopyRepoCallerSet_LyxcwdOnly` would fail the build.
`CopyWarpHub` is hub-shaped — its `WarpFixture.Hub` field names a directory that is not a hub — so all five migrate to `hubforge`, not to `CopyRepo`.

Batch-local decision: no file is renamed, matching batch 9.

## Cards

### Card 63: Relocate and migrate hook_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
- **Edits:**
  - `internal/fabricengine/hook_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference with `fabricengine.` or its `ForTest` alias, extending `internal/fabricengine/export_test.go` under card 65 for anything not yet aliased.
  Replace the four `gitkit.CopyWarpHub(t)` calls and the two `gitkit.CopyPairedLocal(t)` calls with `hubforge.NewHub(t, ".")`, retargeting the fixture fields per the overview's mapping table.
  Never retarget a `CopyWarpHub` site onto `gitkit.CopyRepo`: `CopyRepo` is pinned to `internal/lyxcwd` and this file is not it.
  The nine `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  This file installs and asserts on git hooks;
  the old templates had their `*.sample` hooks stripped and so does a real hub's clone (`hubforge`'s `initScratchRepo`/`initBareRepo` carry the same `stripHookSamples` step), so hook-directory assertions should carry over — verify rather than assume, since a clone's hook directory is created by `git clone`, not by the template builder.
- **Commit:** `test(fabricengine): relocate the hook suite onto hubforge`

### Card 64: Relocate and migrate warplayout_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/warplayout_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package fabricengine` to `package fabricengine_test`, keeping the `//go:build integration` tag, and qualify every now-external reference per card 63's pattern.
  Replace the one `gitkit.CopyWarpHub(t)` call with `hubforge.NewHub(t, ".")` and retarget the `.Hub` field read per the overview's mapping table.
  This file's subject is warp layout, so it is the clearest case in the repo of a test that was asserting against an invented shape: the old fixture had no `_board`, no `_portals`, no `_launchers`, no anchor marker and no hub-level `.lyx`, and every one of those now exists.
  Re-express each layout assertion through `fabricengine`'s own name accessors and `HubReservedNames()`, and treat a "this path should not exist" assertion that still passes as something to re-read rather than to leave alone.
- **Commit:** `test(fabricengine): relocate the warp-layout suite onto hubforge`

### Card 65: Extend the export shim for the two relocating hub suites

- **Context:**
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/warplayout_test.go`
- **Edits:**
  - `internal/fabricengine/export_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add an exported alias to `internal/fabricengine/export_test.go` for every unexported `fabricengine` identifier `hook_test.go` and `warplayout_test.go` reach for after their package flip, following card 53's conventions exactly: `var`/`type`-alias/`const` per kind, a wrapper function where the seam takes unexported parameter types, and a doc comment per identifier naming the consuming file and why the exported surface does not cover it.
  Export nothing without a consumer.
  This card is listed after the two migration cards because the identifier set is discovered by attempting the flips, but the implementer will interleave in practice — the ordering constraint that matters is that the shim and its consumers land in the same batch.
- **Commit:** `test(fabricengine): extend the export shim for the hook and warp-layout suites`

### Card 66: Confirm fabricengine has no in-package fixture left

- **Context:**
  - `internal/fabricengine/hook_test.go`
  - `internal/fabricengine/warplayout_test.go`
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/fabricengine/clone_test.go`
  - `internal/fabricengine/bolt_integration_test.go`
  - `internal/fabricengine/commitweftat_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Confirm that no `package fabricengine` file in `internal/fabricengine/` contains any of `gitkit.CopyPaired`, `gitkit.CopyPairedLocal`, `gitkit.CopyWeft`, `gitkit.CopyWarpHub` or `gitkit.CopyRepo`, and that the in-package files still legitimately on `gitkit` use only `MustRun` and `HermeticGitEnv` — `internal/fabricengine/testmain_test.go`, `internal/fabricengine/clone_test.go`, `internal/fabricengine/bolt_integration_test.go` and `internal/fabricengine/commitweftat_test.go` are the expected survivors, and any other in-package file naming `gitkit` is a miss.
  Confirm `TestCopyRepoCallerSet_LyxcwdOnly` passes, which is the machine half of this check.
  Confirm every alias card 65 added has a consumer.
  If any check fails, fix it under the card that owns the file rather than here.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under `-tags integration` and runs `internal/fabricengine`'s integration suite in full, for the same reason as batch 9: two more files change package membership, relinking the package's test binary.

This batch also closes the migration's arithmetic: 141 `Copy*` call sites measured, 9 kept in `internal/lyxcwd` on `gitkit.CopyRepo`, 132 migrated to `hubforge.NewHub` across batches 1 and 4 through 10.
Batch 11's grep gate is what proves that claim rather than asserting it.
