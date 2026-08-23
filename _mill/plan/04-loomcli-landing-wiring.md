# Batch: loomcli-landing-wiring

```yaml
task: 'landing: parent-fabric resolution chain'
batch: loomcli-landing-wiring
number: 4
cards: 8
verify: go test ./internal/loomcli/...
depends-on: [1, 2]
```

## Batch Scope

This batch fills `shedrecipe.Env.Landing` — the whole `landingshed.Deps` struct — in `internal/loomcli/drive.go`, immediately before `loomrecipe.New(c.env, c.shedPaths)`, closing the gap that makes `lyx loom drive` fail construction on every invocation today.
It consumes batch 1's `fabricengine.OpenParent`/`Fabric.OriginURL`/`Fabric.PushBranch` and batch 2's `loomengine.LoomScratchDir` as external interfaces — neither of those two batches is edited by anything here.

Two pure helpers are extracted rather than written inline in `drive.go`'s `RunE` closure, so both stay unit-testable at Tier 1 with no `hubforge` fixture, per the `assembly-seam-takes-plain-values` design decision:

- `resolveLandingParent` (seedinput.go), beside the existing `resolveParentBranch` it wraps, resolves the two refusal clauses (unrecorded/empty parent, self-parent) from plain values.
- `landingDeps` (new file, `landingdeps.go`), performs the struct population alone, taking every value already resolved and doing no I/O of any kind.

`drive.go` itself owns everything with I/O: the one `fabricengine.Open` handle this batch adds (used for `CurrentBranch()`, `OriginURL()`, and the push closure — never a second or third open), `fabricengine.ReadOrigin`, `fabricengine.EnvSyncOptions()`, and the calls into the two pure helpers above.

`wire()` gains exactly one new config load (`landingshed.LoadConfig`, per the `landing-config-loads-in-wire` decision, so an unreconciled hub's absent-`landing.yaml` error reaches the operator's own terminal on every verb rather than only inside `drive`'s detached driver log) and stores the two values (`registry`, `runner`) it already builds locally onto the `loomCLI` struct, so `drive.go` can reach them without a second `modelspec.LoadRegistry`/`shuttleengine.NewRunner` call.

No card in this batch has a non-empty `Moves:`.

## Cards

### Card 12: `resolveLandingParent`

- **Context:**
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomcli/seedinput.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new function to `seedinput.go`, placed after `resolveParentBranch` (the function it wraps): `func resolveLandingParent(recorded fabricengine.Origin, found bool, taskBranch string) (parentBranch string, err error)`.
  It calls `resolveParentBranch(recorded, found, "")` — an empty flag, exactly as `run.go`'s own call passes `parentFlag` (Context, read-only) does with a real flag value; here the caller (drive.go, card 19) has no `--parent` flag of its own, so it passes empty, which `resolveParentBranch`'s own table treats as "no override," landing on its final row for an absent-or-empty record (`` "no recorded parent branch for this worktree pair; pass --parent once to record it" ``).
  On a `resolveParentBranch` error, return `"", err` unchanged — propagate its message verbatim, per the plan's `drive-refuses-an-unrecorded-parent` decision, which deliberately reuses that message rather than writing a `drive`-specific one.
  On success, apply the second refusal clause: when the resolved parent equals `taskBranch`, return `` "", fmt.Errorf("loom: recorded parent branch %q equals the task's own branch %q; a task may not be its own parent", parent, taskBranch) `` — this is the `self-parent-is-loom-policy-not-fabric-policy` decision's refusal, deliberately placed here rather than inside `fabricengine.OpenParent` (batch 1), which special-cases nothing.
  Otherwise return the resolved parent branch and a nil error.
  Write a doc comment in this file's existing style (see `resolveParentBranch`'s own comment) stating both refusal clauses and that a present-but-empty recorded value is treated exactly as absent, inherited unchanged from `resolveParentBranch`.
- **Commit:** `loom: add resolveLandingParent pure helper`

### Card 13: Test `resolveLandingParent`

- **Context:** none
- **Edits:**
  - `internal/loomcli/seedinput_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a new table-driven test function, `TestResolveLandingParent`, to `seedinput_test.go`, following `TestResolveParentBranch`'s existing table shape (same file, same package, `fabricengine.Origin`/`found`/error-text-substring fields).
  Cover exactly these three scenarios:
  - Unrecorded parent (`found: false`) → error, with `wantErrText` containing `"--parent"` (the message `resolveParentBranch` already emits for an absent record, propagated unchanged).
  - Recorded parent equals `taskBranch` → error, with `wantErrText` containing both the parent branch value and `"own branch"` (matching card 12's new message).
  - An ordinary recorded parent that differs from `taskBranch` → no error, `wantParent` equals the recorded value.
- **Commit:** `loom: test resolveLandingParent`

### Card 14: `landingDeps` assembly seam

- **Context:**
  - `internal/loomcli/drive.go`
  - `internal/landingshed/deps.go`
  - `internal/websterengine/geometry.go`
  - `internal/loomengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/landingdeps.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `landingdeps.go`, package `loomcli`, with a file-header comment stating this function performs no I/O and exists so the drift-guard test (card 15) stays Tier 1, mirroring `wiring.go`'s own header-comment convention for why `wire` is extracted.

  Add exactly this function:

  ```go
  func landingDeps(
  	l *lyxcwd.Location,
  	geom websterengine.Geometry,
  	taskBranch, originURL, parentBranch string,
  	pushSkipped bool,
  	pushBranch func() error,
  	registry modelspec.Registry,
  	runner *shuttleengine.Runner,
  	cfg landingshed.Config,
  ) landingshed.Deps {
  	return landingshed.Deps{
  		WorktreeRoot: l.WorktreePath(),
  		TaskBranch:   taskBranch,
  		ParentBranch: parentBranch,
  		WebsterDir:   geom.WebsterDir,
  		StencilsDir:  geom.StencilsDir,
  		ScratchDir:   loomengine.LoomScratchDir(l),
  		OriginURL:    originURL,
  		PushSkipped:  pushSkipped,
  		PushBranch:   pushBranch,
  		OpenFabric: func() (*fabricengine.Fabric, error) {
  			return fabricengine.Open(l)
  		},
  		OpenParentFabric: func() (*fabricengine.Fabric, error) {
  			return fabricengine.OpenParent(l, parentBranch)
  		},
  		Shuttle:  runner,
  		Registry: registry,
  		Config:   cfg,
  	}
  }
  ```

  Import `"github.com/Knatte18/loomyard/internal/fabricengine"`, `"github.com/Knatte18/loomyard/internal/landingshed"`, `"github.com/Knatte18/loomyard/internal/loomengine"`, `"github.com/Knatte18/loomyard/internal/lyxcwd"`, `"github.com/Knatte18/loomyard/internal/modelspec"`, `"github.com/Knatte18/loomyard/internal/shuttleengine"`, and `"github.com/Knatte18/loomyard/internal/websterengine"`.
  The function performs no I/O and returns no error, matching the `assembly-seam-takes-plain-values` decision's stated signature exactly — every value arrives already resolved from `drive.go` (card 19).
  `runner` assigns into the `Shuttle mergeresolve.Shuttle` field directly (`*shuttleengine.Runner` already satisfies that interface, per the existing compile-time assertion at `internal/mergeresolve/deps.go:46`); do not add a new assertion here.
- **Commit:** `loom: add landingDeps assembly seam`

### Card 15: Drift-guard test for `landingDeps`

- **Context:**
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/landingdeps.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/landingdeps_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `landingdeps_test.go`, package `loomcli`, untagged (no fixture, no `hubforge`, no `git init` — this test stays Tier 1 because `landingDeps` does no I/O).
  Write `TestLandingDeps_EveryFieldPopulated`: build a `*lyxcwd.Location` the same bare way `wiring_test.go`'s `hubLocation` helper does (Context, read-only — reuse the existing pattern, do not import `hubLocation` itself since this test needs no seeded config), call `landingDeps` with every scalar argument set to a distinct non-zero value (`pushSkipped: true`, so the one bool argument also reads as intentionally-set rather than a zero-value false), and use `reflect` to walk every field of the returned `landingshed.Deps` struct, failing the test by field name for any field that is the zero value for its type.
  A reflection-based walk is required, not an enumerated list of field assertions — per the plan's own testing note, an enumerated list silently keeps passing when a fifteenth field is added later; `reflect.Value.IsZero()` per field is what actually catches that.
  `OpenFabric`/`OpenParentFabric`/`PushBranch` are `func` fields — `IsZero()` on a `reflect.Value` of kind `Func` reports whether the func value itself is nil, which is exactly the failure mode worth catching (a field left unset), so no special-casing is needed for the three closure fields.
- **Commit:** `loom: drift-guard test for landingDeps`

### Card 16: New `loomCLI` struct fields

- **Context:** none
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `"github.com/Knatte18/loomyard/internal/landingshed"` and `"github.com/Knatte18/loomyard/internal/modelspec"` to `cli.go`'s import block (`"github.com/Knatte18/loomyard/internal/shuttleengine"` is already imported there).

  Add three new fields to the `loomCLI` struct, placed after the existing `runDeps websterengine.RunDeps` field:

  ```go
  	// registry is the resolved model-spec registry, carried onto the struct so drive.go can pass
  	// it to landingDeps without a second modelspec.LoadRegistry call.
  	registry modelspec.Registry
  	// runner is the constructed shuttle runner, carried onto the struct so drive.go can pass it to
  	// landingDeps as the landing seam's Shuttle value.
  	runner *shuttleengine.Runner
  	// landingCfg is the loaded landing.yaml configuration, loaded once in wire() per the
  	// landing-config-loads-in-wire decision, so an unreconciled hub's absent-config error reaches
  	// the operator's own terminal on every verb, not only inside drive's detached driver log.
  	landingCfg landingshed.Config
  ```

  Write each field's doc comment in this struct's existing per-field comment style (see the surrounding fields for the pattern).
- **Commit:** `loom: add registry, runner, and landingCfg fields to loomCLI`

### Card 17: Load `landing.yaml` in `wire()`, carry `registry`/`runner` onto the struct

- **Context:**
  - `internal/landingshed/config.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `"github.com/Knatte18/loomyard/internal/landingshed"` to `wiring.go`'s import block.

  Add a new config load in `wire()`, placed after the existing `websterCfg, err := websterengine.LoadConfig(anchorPath, "webster")` block and before `registry, err := modelspec.LoadRegistry(anchorPath)`: `` landingCfg, err := landingshed.LoadConfig(anchorPath, "landing") `` followed by the same `if err != nil { return err }` shape every other load in this function already uses.

  At the bottom of `wire()`, alongside the existing `c.location = location`, `c.cwd = cwd`, `c.cfg = loomCfg`, `c.reed = reedEngine`, `c.runDeps = runDeps` assignments, add three more: `c.registry = registry`, `c.runner = runner`, `c.landingCfg = landingCfg` — `registry` and `runner` are already local variables `wire()` builds earlier in the function (`registry, err := modelspec.LoadRegistry(anchorPath)` and `runner := shuttleengine.NewRunner(...)`); this card stores the two existing values onto the struct rather than reloading or reconstructing them.
  This card is sequenced after card 16 specifically because it assigns into the three fields card 16 adds — a `loomCLI` struct literal with no such fields would not compile if this card's commit landed first.

  Replace the existing comment block that currently reads:

  ```
  // StencilsDir, RunRoot, Shuttle, Burler, and Now are left zero -- only SingleLLM, Bouncer,
  // and BurlerRound read them, and no row in loom's recipe uses those engines yet.
  //
  // Landing is left unfilled, per the landing-parity Shared Decision: internal/landingshed's
  // own account of the gap (see internal/landingshed/deps.go's OpenFabric/OpenParentFabric
  // field doc) is what this omission preserves, and the parent-fabric resolution chain that
  // closes it is a roadmap item this task adds (see manifest/roadmap.md), not something this
  // conversion may build.
  ```

  with:

  ```
  // StencilsDir, RunRoot, Shuttle, Burler, and Now are left zero -- only SingleLLM, Bouncer,
  // and BurlerRound read them, and no row in loom's recipe uses those engines yet.
  //
  // Landing is deliberately left unfilled here too, but for a different reason than the other
  // five: Env.Landing is assembled in drive.go, immediately before loomrecipe.New, because
  // NewPublish/NewFinalize both open their fabric pair eagerly at construction, and wire()
  // runs for every verb including "status"/"pause" -- the same OpenBisector hazard the
  // comment above already guards against. See landingDeps (landingdeps.go) and the
  // env-landing-filled-in-drive-not-wire design decision.
  ```

  This is an exact text replacement inside the existing `c.env = shedrecipe.Env{...}` struct literal's trailing comment block (wiring.go:105-112) — locate it by the quoted text above, which is a byte-exact substring of the current file.
- **Commit:** `loom: load landing.yaml in wire(), carry registry and runner onto the struct`

### Card 18: Seed `landing.yaml`, assert the new fields populate

- **Context:**
  - `internal/landingshed/configtemplate.go`
- **Edits:**
  - `internal/loomcli/wiring_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `"github.com/Knatte18/loomyard/internal/landingshed"` to `wiring_test.go`'s import block.

  Add a new helper, `seedLandingConfig(t *testing.T, anchorPath string)`, mirroring `seedLoomConfig`'s exact shape (same `os.MkdirAll`/`os.WriteFile` pattern, `0o644`), writing `<anchorPath>/_lyx/config/landing.yaml` with `landingshed.ConfigTemplate()`'s contents — `landingshed.LoadConfig` is strict (an absent file is an error), so `wire()` fails on every existing test in this file without this seed once card 17 lands.
  Call `seedLandingConfig(t, loc.AnchorPath())` from `hubLocation` (this file's shared fixture builder), immediately after the existing `seedLoomConfig(t, loc.AnchorPath())` call — every existing test in this file goes through `hubLocation`, so this one seed addition keeps them all green.

  Add a new test function, `TestWire_LandingSeamFieldsPopulated`: build a location via `hubLocation`, call `c.wire(loc, cwd)`, then assert `c.registry` is non-nil, `c.runner` is non-nil, and `c.landingCfg` equals the value returned by calling `landingshed.LoadConfig(loc.AnchorPath(), "landing")` directly in the test — the same "compare against the accessor's own direct call" pattern `TestWire_PathFieldsMatchLoomengineAccessors` already uses for its own path-field assertions, but compared via `reflect.DeepEqual(c.landingCfg, want)`, not `!=`: `landingshed.Config` carries a `RequirePRToBase []string` field, which makes the struct non-comparable, so a plain `!=` does not compile here the way it does for `TestWire_PathFieldsMatchLoomengineAccessors`'s plain-string field comparisons.
  Import `"reflect"` for this comparison.
- **Commit:** `loom: seed landing.yaml, assert wire() populates the new fields`

### Card 19: Fill `Env.Landing` in `drive.go`

- **Context:**
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `internal/loomcli/drive.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `"github.com/Knatte18/loomyard/internal/fabricengine"` to `drive.go`'s import block.

  Insert the block below immediately before the existing `` `shed, err := loomrecipe.New(c.env, c.shedPaths)` `` line (drive.go, inside the status-file-exists branch, after the pre-flight `os.Stat` check and before the `shedbuild`/`loomrecipe.New` call):

  ```go
  handle, err := fabricengine.Open(c.location)
  if err != nil {
  	clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
  	return nil
  }
  taskBranch, err := handle.CurrentBranch()
  if err != nil {
  	clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
  	return nil
  }
  originURL, err := handle.OriginURL()
  if err != nil {
  	// scalar-read-errors-refuse-or-defer-by-consumer: only Publish reads OriginURL, and only
  	// when a pull request is actually required, so an unusable origin URL passes through as
  	// an empty string rather than refusing drive itself.
  	originURL = ""
  }
  recorded, found, err := fabricengine.ReadOrigin(c.location)
  if err != nil {
  	clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
  	return nil
  }
  parentBranch, err := resolveLandingParent(recorded, found, taskBranch)
  if err != nil {
  	clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
  	return nil
  }
  syncOpts := fabricengine.EnvSyncOptions()
  pushBranch := func() error {
  	_, err := handle.PushBranch(syncOpts)
  	return err
  }
  c.env.Landing = landingDeps(
  	c.location,
  	c.runDeps.Geom,
  	taskBranch,
  	originURL,
  	parentBranch,
  	syncOpts.SkipPush,
  	pushBranch,
  	c.registry,
  	c.runner,
  	c.landingCfg,
  )
  ```

  This is the only `fabricengine.Open` call in `drive.go`; `handle` backs `CurrentBranch()`, `OriginURL()`, and the `pushBranch` closure, and is never opened a second time in this function.
  `landingDeps`'s own `OpenFabric`/`OpenParentFabric` closures (card 14) open their own handles lazily, only when a producer actually calls them — per the `two-opens-in-drive-rather-than-a-shared-handle` decision, exactly two `Open` sites exist in this path: this one, and whatever `OpenFabric`/`OpenParentFabric` open when `NewPublish`/`NewFinalize`/`Finalize.Call` reach them.
  `fabricengine.EnvSyncOptions()` is called exactly once here and feeds both `syncOpts.SkipPush` (the `pushSkipped` argument) and the push closure's own `opts` argument, so the two can never disagree.
- **Commit:** `loom: fill Env.Landing in drive.go before loomrecipe.New`

## Batch Tests

`verify: go test ./internal/loomcli/...` — every file this batch touches or creates is untagged Tier 1 (no `hubforge`, no `git init`, no `exec.Command`), per the `assembly-seam-takes-plain-values` decision that shapes cards 12 and 14 specifically to keep this package's tests fixture-free.
Covers: `TestResolveLandingParent` (card 13), `TestLandingDeps_EveryFieldPopulated` (card 15), and the extended `wiring_test.go` suite (card 18), plus every pre-existing test in the package staying green against the new `landing.yaml` seed.
