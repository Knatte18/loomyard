# Batch: stuck packages

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'stuck packages'
number: 6
cards: 4
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/loomengine/... ./internal/treadleengine/...
depends-on: [5]
```

## Batch Scope

`internal/loomengine` and `internal/treadleengine` both sit inside `internal/fabriccli`'s dependency set, so an in-package test in either cannot import `hubforge` — it would close a compile cycle.
Each has exactly one in-package test file using a fixture, and both need unexported access, so each file becomes an external `*_test` package with an `export_test.go` shim in the original package.
`internal/fabricengine/export_test.go` is the existing precedent for the shim shape.

A fixture subpackage would not solve this: `loomtest`/`treadletest` would itself import `hubforge` → `fabriccli` → the parent package, so an in-package test still could not reach it.
Moving the test file is the fix;
the subpackage is not.

`internal/loomengine/preflight_integration_test.go` is the higher-value of the two — it hand-rolls a `seedRepoWideFabricConfig` helper that writes an uncommitted repo-wide `fabric.yaml` straight into `BoardDir`, which this batch replaces with `hubforge.SeedFabricConfig`.
Note the shape of that replacement carefully: the helper's *placement* is stand-in-hub scaffolding, but its *content* is a genuine override (`pathspec: _extra`) that two of this file's tests depend on, so it is retargeted rather than dropped — see card 36.

Batch-local decision: the two `export_test.go` files are created in their own cards, before the package flip, so each migration card can be reviewed on its own.
Neither file is renamed — moving a test file out of its package is a one-line change to its `package` clause, and a `git mv` on top of that would add churn without adding history.

## Cards

### Card 35: Add loomengine's export shim

- **Context:**
  - `internal/fabricengine/export_test.go`
  - `internal/loomengine/preflight.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/export_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `internal/loomengine/export_test.go` in `package loomengine`, untagged, re-exporting exactly what `preflight_integration_test.go` reaches for and nothing more: `var CheckResolvedForTest = checkResolved`, the unexported `checkResolved(l *lyxcwd.Location)` entry point, which the file invokes at twelve call expressions (a bare-word grep reports 26, but fourteen of those are `t.Fatalf("checkResolved: %v", err)` message literals).
  Follow `internal/fabricengine/export_test.go`'s convention: a file-header comment explaining that the shim exists so `package loomengine_test` files can drive an unexported seam directly rather than through the exported `Preflight()`, whose own `lyxcwd.Getwd()` dependency makes it unusable against an arbitrary `*lyxcwd.Location`, and a per-identifier doc comment saying why each one is re-exported.
  Add only the identifiers card 36 proves it needs;
  if the compiler shows more are required, add those and note them in the commit message rather than pre-emptively exporting the package's private surface.
- **Commit:** `test(loomengine): add the export shim for the external test package`

### Card 36: Flip loomengine's preflight suite out of package and onto hubforge

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/loomengine/export_test.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/clone.go`
- **Edits:**
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package loomengine` to `package loomengine_test`, keeping the filename and the `//go:build integration` tag.
  Retarget every `checkResolved(` call to `loomengine.CheckResolvedForTest(`, adding the `internal/loomengine` import.
  Replace the single `gitkit.CopyPaired(t)` in `setupPreflightFixture` with `hubforge.NewHub(t, ".")` and change that helper's return type from `gitkit.PairedFixture` to `*hubforge.Hub`;
  the same substitution applies to the `commitFabricStatus(t, f)` helper's parameter and to the `dirty func(t *testing.T, f …)` table-test closures, all of which name `PairedFixture` today.
  Retarget the fixture fields per the overview's mapping table: `f.WeftPrime` becomes `h.PrimeWeft()`, `f.Hub` becomes `h.PrimeWorktree()` (it is handed to `git` as a repo directory, so `h.Path` would be wrong), and `f.Layout` becomes `h.Location`.
  **Retarget `seedRepoWideFabricConfig` onto `hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")` and delete the helper** — do **not** simply drop the call.
  Its placement is stand-in-hub scaffolding (a hand-written, uncommitted file at `BoardDir`) but its **content is a genuine override**, not a duplicate of the registered template: it names `pathspec: _extra`, whereas `internal/fabricengine/template.yaml` ships `pathspec: ""`.
  The `_extra` value is load-bearing — `setupPreflightFixture` wires `_extra` as this fixture's second, non-`_lyx` junction through its explicit `fabricengine.WireJunctions(…, []string{"_lyx", lyxdirs.DotLyxDirName, "_extra"})` call, and `RepoWiredNames` must agree with what is wired on disk or `checkJunctionHealth` stops classifying `_extra` as a real optional junction at all.
  Dropping it would silently break `TestPreflight_MissingOptionalJunctionIsAJunctionFault` and the two `_extra` junction-corruption sub-tests, which is a failure mode that looks like a passing test suite with two fewer meaningful assertions.
  This is outcome 3 of the overview's three-way triage, and it mirrors batch 4 card 27's resolution of `perchcli`'s identically-named helper exactly.
  Apply the `SeedConfig` triage to the one `gitkit.SeedConfig(t, f.WeftPrime, …)` call.
  The `gitkit.MustRun(` calls stay on `gitkit` unchanged.
  **Delete `setupPreflightFixture`'s `gitkit.MustRun(t, …, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))` line.**
  This is not a "check whether it is redundant" judgement call — it is a hard requirement, because on a real hub that line **errors** rather than no-ops: `internal/fabricengine/clone.go`'s weft-primary step already runs `git checkout -b <WeftBranchName(warpBranch)>` on the weft during `CloneHub`, so the branch exists by the time the fixture returns and a second `checkout -b` onto the same name fails with "branch already exists", taking every test that calls `setupPreflightFixture` with it.
  Note the removal in the commit message.
  The two `git checkout -b "warp-only"` calls elsewhere in this file are unrelated — they create a distinct branch and stay.
- **Commit:** `test(loomengine): move the preflight suite to package loomengine_test on hubforge`

### Card 37: Add treadleengine's export shim

- **Context:**
  - `internal/fabricengine/export_test.go`
  - `internal/treadleengine/smoke_judge_test.go`
- **Edits:** none
- **Creates:**
  - `internal/treadleengine/export_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `internal/treadleengine/export_test.go` in `package treadleengine`, untagged, re-exporting exactly the two identifiers `smoke_judge_test.go` reaches for: `var RunCirclingForTest = runCircling` and `type JudgeInputsForTest = judgeInputs` — the latter must be a type **alias** (`=`), not a defined type, so the external test can construct it with the same field names.
  Follow `internal/fabricengine/export_test.go`'s convention for the file-header and per-identifier comments, recording that this exists because `internal/treadleengine` is inside `internal/fabriccli`'s dependency set and its fixture-using test therefore has to live in an external test package.
- **Commit:** `test(treadleengine): add the export shim for the external test package`

### Card 38: Flip treadleengine's judge smoke test out of package and onto hubforge

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
  - `internal/treadleengine/export_test.go`
- **Edits:**
  - `internal/treadleengine/smoke_judge_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `package treadleengine` to `package treadleengine_test`, keeping the filename and the `//go:build smoke` tag.
  Retarget `runCircling(` to `treadleengine.RunCirclingForTest(` and the `judgeInputs{` composite literal to `treadleengine.JudgeInputsForTest{`, adding the `internal/treadleengine` import.
  Update the file-header comment, which today explains that the file is in-package *because* `runCircling` and `judgeInputs` are unexported: that reason is now discharged by the shim, and the file is external because `internal/treadleengine` sits inside `fabriccli`'s dependency set and this test needs a real hub.
  Replace the `gitkit.CopyPaired(t)` call with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the `gitkit.SeedConfig(t, fixture.Hub, …)` call — that call seeds into the old fixture's container-shaped `Hub` field, so it cannot survive unchanged.
  This file is `//go:build smoke` and spawns a real Claude session;
  it is compile-checked only and must not be executed by `verify:`.
- **Commit:** `test(treadleengine): move the judge smoke test to package treadleengine_test on hubforge`

## Batch Tests

`verify:` compile-checks the repo under both tags, then runs `internal/loomengine`'s integration suite, which is where the substantive migration in this batch lives: the flipped `preflight_integration_test.go` drives `checkResolved` at twelve call sites against a real hub and moves its repo-wide `fabric.yaml` seeding onto `hubforge.SeedFabricConfig`, so a green run here proves both that the shim exposes the seam correctly and that a committed board-side override behaves as the hand-written uncommitted one did.
The `_extra` junction-health tests are the ones to watch: they are what would fail silently if card 36's override were dropped instead of retargeted.

`internal/treadleengine`'s integration suite is run too (`gate_lingering_test.go` lives there and is integration-tagged), but the file this batch moves in that package is `//go:build smoke` and spawns a real Claude session — compile-checked under `go vet -tags smoke ./...`, never executed.
