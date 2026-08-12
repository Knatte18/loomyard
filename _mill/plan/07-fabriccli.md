# Batch: fabriccli

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'fabriccli'
number: 7
cards: 3
verify: go vet -tags integration ./... && go test -tags integration ./internal/fabriccli/...
depends-on: [6]
```

## Batch Scope

This batch migrates `internal/fabriccli`'s eleven `Copy*` sites.
Both files are already `package fabriccli_test`, so no moves are needed — an external test package may import `hubforge` even though `hubforge` itself imports `fabriccli`.

This is the last batch before `internal/fabricengine`'s eighty-two sites, and it is the one where the fixture and the subject are closest together: these tests drive `lyx fabric clone|add|remove|checkout` against a hub that is now built by `fabriccli.CloneAndWire` — the same code path the CLI under test uses.
Expect assertions about hub shape to change most here, and read every one rather than silencing it.

Batch-local decision: a test that clones a **new** hub as its subject must not reuse the fixture hub as the clone destination.
`hubforge.NewHub` hands back `h.WarpBare` and `h.WeftBare`, which are exactly the two URLs such a test needs;
point the subject clone at those and give it its own `t.TempDir()` destination.

## Cards

### Card 39: Migrate fabriccli's CLI suite

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the eight `gitkit.CopyPaired(t)` calls and the one `gitkit.CopyWarpHub(t)` call with `hubforge.NewHub(t, ".")` and retarget the fixture fields per the overview's mapping table.
  The sixteen `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  Where a test's subject is `fabric clone` itself, do not clone into the fixture hub: pass `h.WarpBare` and `h.WeftBare` as the URLs and a fresh `t.TempDir()` as the destination, so the fixture hub stays the arranged state and the cloned hub is the observed one.
  Where a test previously relied on the old fixture *not* being a hub — for example asserting that `fabric add` refuses outside a hub, or that a path does not exist — that assertion is now false by construction and must be re-expressed against a directory that genuinely is not a hub (a bare `t.TempDir()`), not deleted.
  Note any such re-expression in the commit message.
- **Commit:** `test(fabriccli): build the CLI fixtures with hubforge.NewHub`

### Card 40: Migrate fabriccli's push-bypass suite

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/fabriccli/pushbypass_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the one `gitkit.CopyWarpHub(t)` call and the one `gitkit.CopyWeft(t)` call with `hubforge.NewHub(t, ".")`, retargeting the fields per the overview's mapping table — the `WarpFixture.Hub` read becomes `h.PrimeWorktree()`, the `WeftFixture.WeftPath` read becomes `h.PrimeWeft()`, and both `.Bare` reads become `h.WarpBare` and `h.WeftBare` respectively.
  This file's subject is push bypass, so its remote must be a live push target: a real hub's warp and weft both have their own copied bare origin, which is exactly the substrate `CopyWeft` used to provide, so nothing about the push path needs compensating.
  The two `gitkit.MustRun(` calls stay on `gitkit` unchanged.
- **Commit:** `test(fabriccli): build the push-bypass fixtures with hubforge.NewHub`

### Card 41: Confirm fabriccli holds no stand-in hub

- **Context:**
  - `internal/fabriccli/cli_test.go`
  - `internal/fabriccli/pushbypass_integration_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Confirm by grep that `internal/fabriccli` contains zero occurrences of `gitkit.CopyPaired`, `gitkit.CopyPairedLocal`, `gitkit.CopyWeft` and `gitkit.CopyWarpHub`, and zero `gitkit.SeedConfig` calls.
  Confirm that every `hubforge.NewHub` call in the package is followed by field reads drawn from the overview's mapping table and none from the retired `PairedFixture`/`WarpFixture`/`WeftFixture` shapes.
  If any check fails, fix it under card 39 or 40 rather than here.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under `-tags integration` and runs `internal/fabriccli`'s integration suite in full — both migrated files are integration-tagged, so unlike batches 4 through 6 this batch's entire migration gets runtime proof.

The `smoke` tag is not compile-checked here because no file this batch touches is smoke-tagged.
