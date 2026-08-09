# Batch: code-retirement

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
batch: code-retirement
number: 1
cards: 4
verify: go build ./... && go test ./cmd/lyx/... ./internal/configreg/... ./internal/configcli/... ./internal/loomengine/... ./internal/fabricengine/... ./internal/scoutcli/... ./internal/webstercli/... && go test -tags integration ./internal/webstercli/... ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch removes builder from the compiled tree: it deletes `internal/builderengine` and `internal/buildercli`, unregisters the module from the CLI and from `internal/configreg`, repairs every test the removal breaks, retires the builder sandbox suite, and renames the loom phase token `builder` → `webster` in Go.
It is one batch because every site here is either a compile dependency or a guard test that fires the moment the module leaves `newRoot()`;
splitting it across batches would ship a non-building intermediate state.

Card 1 is deliberately oversized: it is the atomic set.
`cmd/lyx/main.go` + `internal/configreg/configreg.go` + the direct importers must land together to keep the build green, and `cmd/lyx/helptree_test.go`, `internal/configreg/configreg_test.go`, `internal/configcli/configcli_test.go` and `cmd/lyx/sandbox_coverage_test.go` all fail the instant `builder` leaves the module registry — the sandbox-suite retirement is therefore part of the same commit, not a follow-up.
Cards 2–4 are separable because each leaves the tree building and green on its own.

The external interface the later batches consume: after this batch, `builder` is not a registered module, `internal/builderengine`/`internal/buildercli` do not exist, and `webster` is the loom phase name in live validation code — the doc batches then align prose and contracts with that reality.

## Cards

### Card 1: Delete the builder packages and unregister the module

- **Context:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/perchengine/identity.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `.gitattributes`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/main.go`
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/rawgitmutation_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
  - `internal/webstercli/sync_integration_test.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `tools/sandbox/main.go`
  - `tools/sandbox/suite.go`
- **Creates:** none
- **Deletes:**
  - `internal/builderengine`
  - `internal/buildercli`
  - `sandbox/builder-suite.cmd`
  - `tools/sandbox/SANDBOX-BUILDER-SUITE.md`
- **Moves:** none
- **Requirements:** Delete the directories `internal/builderengine` and `internal/buildercli` in full.
  In `cmd/lyx/main.go`: drop the `"github.com/Knatte18/loomyard/internal/buildercli"` import, drop the `buildercli.Command(),` entry from `newRoot()`'s command list, and remove `builder, ` from the `Available modules: …` line in the root command's long help text.
  In `internal/configreg/configreg.go`: drop the `builderengine` import and the `{Name: "builder", Template: builderengine.ConfigTemplate}` entry from the module slice.
  In `cmd/lyx/helptree_test.go`: remove `"builder"` from the module-name slice and delete the `name: "builder"` / `module: "builder"` help-tree case.
  In `internal/configreg/configreg_test.go`: change `want` to `[]string{"board", "burler", "fabric", "loom", "models", "perch", "reed", "shuttle", "webster"}`.
  In `internal/configcli/configcli_test.go`: delete the `builder (default)` menu assertion and its comment, and drop `builder` from the "intentionally not seeded" comment, leaving `fabric` as the sole named example.
  In `cmd/lyx/notransients_test.go`: drop the `builderengine` import and every `builderengine.Dir`/`builderengine.ReportsDir`/`builderengine.ScratchDir` case, including the paired `builderengine.Dir/ScratchDir` mirrored-subpath case;
  update the package-list doc comment to drop `builderengine`.
  In `cmd/lyx/constructoranchoring_test.go`: drop the `builderengine` import, every `assertPath` call naming a `builderengine.*` constructor, the `"builderengine.ScratchDir"` map entry, and the two doc-comment package lists that name `builderengine`.
  In `cmd/lyx/rawgitmutation_test.go`: narrow the guard to webster alone — drop `filepath.Join("internal", "builderengine")` from the scanned roots, drop the `"internal/builderengine/gitquery.go"` grandfathered-exemption entry, rename `TestNoRawGitMutation_WebsterBuilderProductionSource` to `TestNoRawGitMutation_WebsterProductionSource`, and rewrite **every** comment in the file that names the deleted package or describes the guard as two-package so it describes a single-package walk over `internal/websterengine` — the file-header comment, the test-function doc comment, and the three var doc comments on `rawGitMutationScanPackages` ("exactly the two packages … internal/websterengine and internal/builderengine"), `rawGitMutationAllowlist` ("the two grandfathered read-only exemptions"), and `rawGitMutationMinScannedFiles` ("this guard's two-package walk … across both packages combined").
  Leave `rawGitMutationMinScannedFiles`'s value alone: the vacuous-scan floor is 4 and `internal/websterengine` alone contributes 24 non-test `.go` files, so the one-package walk still clears it — only that constant's doc comment changes.
  In `internal/webstercli/sync_integration_test.go`: replace the deleted builder sibling fixture with a `perch` sibling — drop the `builderengine` import, add `"github.com/Knatte18/loomyard/internal/perchengine"`, rename the local variables `builderDir`/`builderScratchDir` to `perchDir`/`perchScratchDir`, write the durable fixture at `<_lyx>/perch/state.json` and the machine-local one at `<.lyx>/perch/perchengine.PauseFlagName`, and update `wantPresent`/`wantAbsent` to `base + "/perch/state.json"` and `scratchBase + "/perch/" + perchengine.PauseFlagName`;
  rewrite **every** comment and message string in the file that names builder in any form, so each describes a sibling module sharing one `_lyx` geometry without naming builder — the file-header comment, the `newWarpWeftPairAt` doc comment, both in-body fixture comments, the `t.Fatalf` strings on the seeding calls ("mkdir weft builder dir", "write weft builder state.json", "mkdir weft builder scratch dir", "write weft builder pause flag"), and `TestFabricSync_CommitsAtEveryRelPathDepth`'s own doc comment, whose cross-module regression-guard sentence reads "a webster commit must hold back BUILDER's pause flag".
  The uppercase spelling matters: the acceptance sweep's bare-word pattern is case-insensitive, and card 6 forbids re-editing this file, so a site missed here is never fixed later.
  In `tools/sandbox/suite.go`: delete the `//go:embed SANDBOX-BUILDER-SUITE.md` directive with its `builderSandboxSuiteMD` var and the whole `builderSuite` `suiteSpec` var with its doc comment, and change the two doc comments listing the suites from seven to six, dropping `builder` from both lists.
  In `tools/sandbox/main.go`: delete the entire `case "builder-suite":` block and drop `builder-suite` from the two usage/doc comment suite lists.
  In `tools/sandbox/SANDBOX-CORE-SUITE.md`: delete scenario S9 in full — the span from the `### S9 -- Builder plan validate/status` heading through the `---` rule that follows its `**Verdict:**` line, inclusive, so the separator immediately preceding the heading remains as the single rule before the `reed has its own dedicated suite` paragraph;
  the two `---` lines inside the S9 fenced code block are content, not separators.
  Also change the scenario-id line to read `` - `ref` is the scenario id (`S0`-`S6`). `` and delete the `S9: <OK|WARN|FAIL> …` row from the session-log template.
  In `.gitattributes`: delete the three `internal/builderengine/*` LF pin lines.
- **Commit:** `refactor(builder): delete builderengine/buildercli and unregister the module`

### Card 2: Retarget the fabricengine builder fixtures

- **Context:**
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/fabricengine/trailer_test.go`
  - `internal/fabricengine/weftgit_exclude_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/fabricengine/weftgit_exclude_test.go`'s `TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth`, treat the three builder fixture lines differently.
  Delete the two `mustWriteFile` calls writing `filepath.Join(dotLyxDir, "builder", "run.lock")` and `filepath.Join(dotLyxDir, "builder", "pause")` — they are negative controls already proven at every depth by the adjacent `webster` `.lyx` fixtures.
  Rename the durable positive control from `builder` to `webster`: the `mustWriteFile(t, filepath.Join(lyxDir, "builder", "state.json"), "{}")` call becomes `"webster"`, and its assertion `durable := lyxRel + "/builder/state.json"` becomes `lyxRel + "/webster/state.json"` — rename the fixture and its assertion together, in the same edit, or the test asserts on a file it no longer writes.
  In `internal/fabricengine/trailer_test.go`'s `TestAppendWarpSHATrailer_SubjectIsNeverATrailerBlock`, the `builder: <label>` weft commit-subject form dies with the module: rename the `"builder_style_subject"` table case to `"webster_style_subject"` and change its `message`/`want` strings from `builder: poll 01-json-flag done` to a webster subject (`webster: record-batch 01 done`), and narrow the function doc comment so it names only the `"webster: <label>"` form.
- **Commit:** `test(fabric): retarget builder weft fixtures to webster`

### Card 3: Rename the loom phase builder to webster

- **Context:**
  - `internal/loomengine/coherence_test.go`
- **Edits:**
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/perchengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomengine/coherence.go`, change the `validPhases` map entry `"builder": true` to `"webster": true`, keeping the existing ordering position between `"plan"` and `"raddle"`.
  In `internal/loomengine/preflight_integration_test.go`, change both `Phase: "builder"` fixture literals to `Phase: "webster"`.
  In `internal/perchengine/doc.go`, change the gate name `builder-review` to `webster-review` in the review-kind list.
  No compatibility shim and no read-time migration: `CheckSeedIncoherent` will reject an on-disk `status.json` carrying `phase: "builder"`, which is accepted because `lyx loom` is unbuilt.
  If the implementer finds a real `status.json` with `phase: "builder"` in a live worktree, stop and report it as a finding rather than adding a compatibility path.
- **Commit:** `refactor(loom): rename the builder phase to webster`

### Card 4: Repair scoutcli's builder-named help example and fixtures

- **Context:**
  - `internal/scoutengine/doc.go`
  - `internal/webstercli/cli.go`
- **Edits:**
  - `internal/scoutcli/cli.go`
  - `internal/scoutcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/scoutcli/cli.go`, the user-facing help example `lyx scout refs --within internal/builderengine SomeMethod` must name a package that exists — change it to `internal/websterengine`.
  Change the comment that cites the deleted buildercli package's own cli.go so it names `internal/webstercli/cli.go` instead.
  In `internal/scoutcli/cli_test.go`, rewrite the synthetic path fixtures in `TestFilterWithin`: `inScope1`/`inScope2` become `/repo/internal/websterengine/poll.go` and `/repo/internal/websterengine/state.go`, `prefixCollision` becomes `/repo/internal/webstercli/cli.go`, the two `within:` values become `/repo/internal/websterengine` and `internal/websterengine`, and the deliberate prefix-collision `within:` becomes `/repo/internal/webster`.
  The `crossPackage` negative control must be renamed in the same edit: it is currently `/repo/internal/websterengine/poll.go`, which after the rename above would be byte-identical to `inScope1` and would classify as in-scope, contradicting every `wantRefs` in the table.
  Change it to a sibling package that stays genuinely outside the new `within` scope — `/repo/internal/perchengine/identity.go`.
  Update the accompanying comment so it explains the collision check against `internal/webster` vs `internal/websterengine`.
  The property under test — that a path-containment check never treats a shorter prefix as containment — is preserved exactly;
  only the synthetic package names change.
- **Commit:** `test(scout): rename builder-named path fixtures to webster`

## Batch Tests

`verify:` builds the whole tree and then runs the packages this batch touches, plus the two integration-tagged packages that matter here.
`go build ./...` is what proves the deletion left no dangling importer.
`./cmd/lyx/...` covers `helptree_test.go`, `notransients_test.go`, `constructoranchoring_test.go`, `rawgitmutation_test.go` and `sandbox_coverage_test.go`'s `TestSandboxCoverage_AllModulesCoveredOrExcluded` — the guard that fails on the now-removed `**Covers:** builder` tag.
`./internal/configreg/...` and `./internal/configcli/...` cover the module-list and config-menu assertions.
`./internal/loomengine/...` covers the phase rename, `./internal/fabricengine/...` the weft fixtures, `./internal/scoutcli/...` the path fixtures.
The integration-tagged run is scoped to `./internal/webstercli/...` and `./internal/loomengine/...` because `internal/webstercli/sync_integration_test.go` is the one real cross-package compile blocker invisible to an untagged run, and `preflight_integration_test.go` holds the phase fixtures.
The full untagged and integration suites run at the batch-5 acceptance gate and again at the repo-wide done gate;
running them on every implementer round here would cost minutes per round for no extra signal.
No new tests are written — the existing guards are the test.
