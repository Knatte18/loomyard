# Batch: small consumers

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'small consumers'
number: 4
cards: 7
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/webstercli/... ./internal/configcli/... ./internal/idecli/... ./internal/boardengine/... ./internal/perchcli/...
depends-on: [3]
```

## Batch Scope

This batch migrates the seventeen `Copy*` sites and fourteen `SeedConfig` sites in the seven smallest consumer packages, smallest first, so the migration pattern is settled and reviewed before `reedcli`'s twenty sites and `fabricengine`'s eighty-two.
Every one of these packages sits **outside** `internal/fabriccli`'s dependency set, so their in-package test files may import `hubforge` directly — no file moves, no export shims.

The pattern this batch establishes, and batches 5 through 10 repeat, is in `## Shared Decisions` in the overview: the fixture-field mapping table and the three-way `SeedConfig` triage.

Batch-local decision: the `smoke`-tagged files in `internal/burlerengine` and `internal/shuttlecli` are migrated and compile-checked but never executed by `verify:` — they spawn live tmux sessions and LLM agents.
Their runtime correctness is the operator's to confirm out of band;
this task does not run them.

## Cards

### Card 21: Migrate internal/webstercli

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the single `gitkit.CopyWarpHub(t)` call with `hubforge.NewHub(t, ".")` and retarget every field read on the returned fixture per the overview's mapping table.
  Apply the `SeedConfig` triage to the single `gitkit.SeedConfig(` call: if the YAML it writes is the module's plain registered template, delete the call outright, because `fabriccli.CloneAndWire` already materializes it;
  otherwise replace it with `hubforge.SeedConfig(t, h, map[string]string{...})`, unchanged map argument.
  Add the `internal/hubforge` import and drop `internal/gitkit` if nothing else in the file uses it.
  Read every assertion the change breaks rather than silencing it: a real hub is ~155 files against the old template's ~36, so directory listings, file counts and "this path should not exist" assertions will move.
  Re-express each against the real shape;
  if an assertion is deleted rather than re-pointed, say why in the commit message.
- **Commit:** `test(webstercli): build fixtures with hubforge.NewHub`

### Card 22: Migrate internal/configcli

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/configcli/configcli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the single `gitkit.CopyPaired(t)` call with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the single `gitkit.SeedConfig(` call.
  This package's `SeedConfig` site is the highest-value drop candidate in the batch: `configcli` seeds config in order to read it back, so if the seeded YAML equals the module's registered template the call goes away entirely and the test then proves something stronger — that a real hub arrives with materialized config.
  The `gitkit.MustRun(` call in this file stays on `gitkit`, unchanged;
  `MustRun` is not part of this migration.
- **Commit:** `test(configcli): build fixtures with hubforge.NewHub`

### Card 23: Migrate internal/idecli

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/idecli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace both `gitkit.CopyWarpHub(t)` calls with `hubforge.NewHub(t, ".")` and retarget the `.Hub` field reads per the overview's mapping table — for `idecli`, whose subject is opening an IDE at a path, the replacement for `.Hub` is almost certainly `h.PrimeWorktree()` rather than `h.Path`, since the old fixture's `Hub` field held a git repo and `h.Path` holds the `<name>-HUB` container, which is not one.
  Confirm per call site by reading what the test does with the path.
  This file has no `SeedConfig` call.
- **Commit:** `test(idecli): build fixtures with hubforge.NewHub`

### Card 24: Migrate internal/boardengine/boardtest

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/boardengine/boardtest/sync_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the single `gitkit.CopyWeft(t)` call with `hubforge.NewHub(t, ".")`, mapping the old `WeftFixture` fields per the overview's table: `.WeftPath` becomes `h.PrimeWeft()` and `.Bare` becomes `h.WeftBare`.
  The old `CopyWeft` fixture arrived with upstream tracking already established;
  a real hub's weft worktree is cloned from `h.WeftBare` by `CloneHub`, so tracking exists there too — confirm it rather than assuming, since this test syncs against that remote.
  `internal/boardengine/boardtest` is a test-only sibling package outside `fabriccli`'s dependency set, so importing `hubforge` here is legal;
  `internal/boardengine` itself is inside that set and must never gain such an import.
- **Commit:** `test(boardtest): build the weft fixture with hubforge.NewHub`

### Card 25: Migrate internal/burlerengine

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/burlerengine/smoke_cluster_test.go`
  - `internal/burlerengine/smoke_round_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the one `gitkit.CopyPaired(t)` call in each file with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the one `gitkit.SeedConfig(` call in each.
  Both files are `package burlerengine_test` already, so no file move is needed even though `internal/burlerengine` sits inside `fabriccli`'s dependency set — an external test package may import `hubforge`, and that is exactly why the two stuck files in batch 6 have to move and these two do not.
  Both files carry `//go:build smoke` and are compile-checked only;
  do not attempt to run them.
- **Commit:** `test(burlerengine): build fixtures with hubforge.NewHub`

### Card 26: Migrate internal/shuttlecli

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/shuttlecli/smoke_guardrail_test.go`
  - `internal/shuttlecli/smoke_interrupt_test.go`
  - `internal/shuttlecli/smoke_run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace all four `gitkit.CopyPaired(t)` calls across the three files with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to all four `gitkit.SeedConfig(` calls.
  These files call `t.Chdir` on a fixture path;
  keep the `t.Chdir` calls exactly as they are — removing `t.Chdir` and enabling `t.Parallel` is explicitly out of scope for this task and is filed as the follow-up wiki task `hubforge-parallel-chdir`.
  Only the path handed to `t.Chdir` changes, from the old fixture's `Hub` field to whichever `Hub` accessor names the same role (`h.PrimeWorktree()` for a warp-side cwd).
  All three files carry `//go:build smoke` and are compile-checked only.
- **Commit:** `test(shuttlecli): build fixtures with hubforge.NewHub`

### Card 27: Migrate internal/perchcli

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
  - `internal/perchcli/run_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the two `gitkit.CopyPaired(t)` calls in `internal/perchcli/cli_integration_test.go` and the four `gitkit.CopyPairedLocal(t)` calls in `internal/perchcli/run_integration_test.go` with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to all six `gitkit.SeedConfig(` calls.
  `CopyPairedLocal` omitted the weft bare because its callers ran with `SkipPush: true`;
  a real hub always has both bares, so the distinction disappears and nothing at these call sites needs to compensate for it.
  This is the largest package in the batch (six `Copy*` sites, six `SeedConfig` sites) and is deliberately last, so the pattern is settled on the five smaller ones first.

  **Two of the three ad-hoc seeding sites named in the overview's triage decision live in this card, and each gets an explicit resolution rather than the general rule.**
  Both are nested-anchor tests that hand-build the anchor the old fixture could not produce, and both resolve the same way: build the hub at the real anchor and delete the hand-rolled scaffolding.

  *`TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` in `internal/perchcli/cli_integration_test.go`.*
  Build it with `hubforge.NewHub(t, "nested")` instead of `hubforge.NewHub(t, ".")`.
  Delete all three pieces of scaffolding it currently carries: the `os.MkdirAll` of `<fixture.Hub>/nested`, the `os.MkdirAll` of `fabricengine.BoardDir(...)`, and the `os.WriteFile` of `lyxcwd.AnchorFileName` recording `"nested"` — `fabriccli.CloneAndWire` records that anchor at `BoardDir` for real, which is the entire point of the migration, and a test that writes its own `.lyx-anchor` on top of a real hub is asserting against an invented shape again.
  Its `gitkit.SeedConfig(t, nested, …)` call passes three plain registered templates (`shuttleengine.ConfigTemplate()`, `reedengine.ConfigTemplate()`, `perchengine.ConfigTemplate()`), so it is triage outcome 1 — delete the call outright.
  Every later reference to the local `nested` variable becomes `h.Location.AnchorPath()`, the anchored warp path.

  *`TestRunCLI_Run_FabricCommitExcludesLockFiles_NestedRelPath` in `internal/perchcli/run_integration_test.go`.*
  Build it with `hubforge.NewHub(t, "wts/some-task")`, matching the `relPath` constant the test already declares, and delete the local `seedFabricAnchor` helper entirely — this test is its only caller.
  Its `gitkit.SeedConfig(t, warpSubdir, …)` call is again three plain registered templates, so it too is triage outcome 1 and the call is deleted;
  `warpSubdir` and the `t.Chdir(warpSubdir)` argument both become `h.Location.AnchorPath()`, and the run-dir path built from `fixture.WeftPrime` plus `relPath` becomes `h.WeftBase`, which is that join done by fabric rather than by the test.
  `lyxcwd.ValidateAnchorRel` accepts a multi-segment relative anchor and imposes no existence requirement, so `"wts/some-task"` is a legal `Subpath`;
  if the anchored directory does not exist inside the freshly cloned warp, `os.MkdirAll(h.Location.AnchorPath(), 0o755)` after `NewHub` is the correct arrangement — that is ordinary test arrangement on a real hub, not stand-in-hub scaffolding, and the difference is that the anchor itself is recorded by fabric either way.

  *The `seedRepoWideFabricConfig` helper, four call sites across `internal/perchcli/run_integration_test.go`.*
  Unlike the module-config sites it writes a genuine **override** — `branch_prefix: ""` and `pathspec: _lyx`, not the registered `fabric` template — so it is triage outcome 3, not a deletion: replace all four calls with `hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _lyx\n")` and delete the helper.
  This matters beyond tidiness: the hand-rolled helper writes the file and never commits it, whereas `SeedFabricConfig` commits through `fabricengine.NewBolt`, and an uncommitted seed leaves the `weft:main` checkout dirty — which `Fabric.Commit`, this file's own subject, observes.
- **Commit:** `test(perchcli): build fixtures with hubforge.NewHub`

## Batch Tests

`verify:` compile-checks the repo under both the `integration` and `smoke` tags, then runs the integration suites of the five packages whose migrated files are integration-tagged: `internal/webstercli`, `internal/configcli`, `internal/idecli`, `internal/boardengine` (covering the `boardtest` sibling) and `internal/perchcli`.

`internal/burlerengine` and `internal/shuttlecli` are compile-checked under `-tags smoke` and never executed: their suites spawn live tmux sessions and LLM agents, which a per-batch verify must not do.
That is a real coverage gap for those six call sites, and it is stated rather than hidden — the whole-repo `done_gate` does not close it either, since it runs no `smoke`-tagged tests.
