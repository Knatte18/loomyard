# Batch: perchcli-standalone-entry

```yaml
task: "the standalone CLI path"
batch: "perchcli-standalone-entry"
number: 5
cards: 7
verify: go test ./internal/perchcli/... && go test -tags integration ./internal/perchcli/...
depends-on: [1, 2, 4]
```

## Batch Scope

This batch gives `lyx perch` its standalone entry and, in the same move, removes the `layout *lyxcwd.Location` field from `perchCLI` entirely — the one thing that makes "no `*lyxcwd.Location` survives on the receiver" true rather than nearly true.
It is one batch because the field removal in card 21 and the three call-site reroutes in card 22 are a single compile unit: neither half builds without the other.
Perch is the harder of the two producer CLIs, because it carries a real fabric relationship (which becomes nil in standalone) and a second, easily-missed stencils consumer in its nested burler engine.

**Why this batch depends on batch 4 rather than running parallel to it.**
Cards 20, 21, 24 and 25 each read a file batch 4 creates — `internal/burlercli/wiring.go`, `cli.go`, `wiring_test.go` and `cli_integration_test.go` — and card 20's nested-`burlerengine.New` requirement is stated as byte-identical to what `burlercli` builds for the same target.
That is a real semantic coupling, not stylistic mirroring: the nested burler engine and a directly-driven `lyx burler run` must produce the same geometry for the same target, and the cheapest way to keep that true is to read the shipped file rather than re-derive it from the plan.
So the edge is declared rather than left implicit.
Batches 1, 2 and 3 still parallelise; only 4 → 5 serialises.

**Batch-local decision:** one `stencilsDir` value per invocation reaches **both** consumers — the nested `burlerengine.New` and the `perchengine.Engine.Run` argument — in both modes.
Two independent resolutions would let `--stencils-dir` reach perch's own rounds but not its nested burler rounds: a half-working flag, and exactly the split a single told value exists to prevent.

## Cards

### Card 20: add `internal/perchcli/wiring.go`

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/burlercli/wiring.go`
  - `internal/perchcli/cli.go`
  - `internal/preflight/predicates.go`
  - `internal/standalonegeom/perchgeom.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/perchengine/geometry.go`
  - `internal/perchengine/config.go`
  - `internal/perchengine/identity.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/config.go`
  - `internal/modelspec/load.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/buildinfo/buildinfo.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
  - `contracts/stencils/stencils.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/open.go`
  - `internal/reedengine/config.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:** none
- **Creates:**
  - `internal/perchcli/wiring.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/perchcli/wiring.go` in `package perchcli`, mirroring `internal/webstercli/wiring.go` and `internal/burlercli/wiring.go` in structure and file-header shape, including the statement that the mode decision lives here rather than upstream so the truth-table test stays tier 1.

  Add `func (c *perchCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error`, dispatching to `c.wireHub(loc, stencilsDirFlag, targetDirFlag)` on `preflight.ModeHub` and `c.wireStandalone(cwd, stencilsDirFlag, targetDirFlag)` otherwise.
  There is no `planDirFlag` — perch parses no plan.

  Add `func (c *perchCLI) wireHub(loc *lyxcwd.Location, stencilsDirFlag, targetDirFlag string) error` reproducing today's `PersistentPreRunE` body in resolved values:
  refuse a non-empty `targetDirFlag` with an error naming the reason, mirroring `webstercli`'s wording;
  bind `anchorPath := loc.AnchorPath()`;
  load `shuttleengine.LoadConfig(anchorPath, "shuttle")`, `reedengine.LoadConfig(anchorPath, "reed")`, `modelspec.LoadRegistry(anchorPath)`, `perchengine.LoadConfigWithRegistry(anchorPath, "perch", modelReg)` and `burlerengine.LoadConfig(anchorPath)`, in that order, preserving today's comment explaining that `models.yaml` is read exactly once per invocation and reused for both `perchCfg`'s judge-model resolution and `decodeProfile`'s profile-field resolution;
  compute a single `stencilsDir := fabricengine.StencilsDir(loc.HubPath)`, overridden by `stencilsDirFlag` when non-empty;
  build `reedGeom := hubgeom.ReedGeometry(loc)`, the reed engine, the claude engine and the runner exactly as today;
  set `c.burlerEngine = burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir)` — passing the same single `stencilsDir`, never a second `fabricengine.StencilsDir` call;
  set `c.runner`, `c.perchCfg`, `c.modelReg`, `c.perchGeom = hubgeom.PerchGeometry(loc)`;
  set `c.runDirBase = perchengine.RunsDir(c.perchGeom.AnchorPath)` and `c.scratchDirBase = perchengine.ScratchDir(c.perchGeom.AnchorPath)`, carrying today's nested-init comment across verbatim in substance — it explains why both anchor at `AnchorPath` rather than `WorktreeRoot`, and `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` enforces it;
  set `c.stencilsDir = stencilsDir`, `c.anchorRel = loc.AnchorRel`, and `c.openFabric = func() (*fabricengine.Fabric, error) { return fabricengine.Open(loc) }`;
  set the reporting fields `c.mode = "hub"` and `c.stateDir = ""`.
  The fabric handle must stay a closure and must not be opened here — `fabricengine.Open` stat-checks the paired sibling and would fail the pre-run in healthy-but-unwired locations.

  Add `func (c *perchCLI) wireStandalone(cwd, stencilsDirFlag, targetDirFlag string) error`:
  `target, err := resolveStandaloneTarget(cwd, targetDirFlag)`;
  `stateDir, hash8, err := standalonestate.Derive(target)` — the only `Derive` call site in the package;
  compute `stencilsDir` exactly as burler's standalone branch does: an explicit `stencilsDirFlag` is used verbatim and never seeded, otherwise `standalonegeom.StencilsDir(stateDir)` is seeded via `stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")` with a `Reconcile` failure returned as a hard, directory-naming error;
  load all five config loaders over `stateDir` — `shuttleengine.LoadConfig(stateDir, "shuttle")`, `reedengine.LoadConfig(stateDir, "reed")`, `modelspec.LoadRegistry(stateDir)`, `perchengine.LoadConfigWithRegistry(stateDir, "perch", modelReg)` and `burlerengine.LoadConfig(stateDir)`;
  build `reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)` and reuse it as-is;
  set `c.burlerEngine = burlerengine.New(runner, standalonegeom.BurlerGeometry(target, stateDir), burlerCfg, stencilsDir)` — the nested burler's geometry is byte-identical to what `burlercli` builds for the same target, so a burler round behaves the same whether driven directly or through perch;
  set `c.perchGeom = standalonegeom.PerchGeometry(target, stateDir)`, with `c.runDirBase` and `c.scratchDirBase` computed from `c.perchGeom.AnchorPath` by the same two expressions the hub branch uses;
  set `c.stencilsDir = stencilsDir`, `c.anchorRel = ""`, and `c.openFabric = nil`;
  set `c.mode = "standalone"` and `c.stateDir = stateDir`.

  Document why `openFabric` is nil rather than a closure: standalone has no fabric repo by construction — not a broken one, an absent one — so nil is the honest representation, and a closure that would stat-fail if called is not.

  Add `func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error)` with the same three-row body and doc rationale as `webstercli`'s and `burlercli`'s.
  Three near-identical five-line functions across three CLI packages is the deliberate call over a shared package that would exist only to hold one of them; revisit if a fourth standalone CLI appears.

  Neither branch may call `os.Getwd`, spawn git, or re-derive any path.
- **Commit:** `feat(perchcli): add wiring.go with the hub and standalone engine-stack branches`

### Card 21: remove `layout` from `perchCLI` and reduce `cli.go` to resolve-probe-delegate

- **Context:**
  - `internal/perchcli/wiring.go`
  - `internal/webstercli/cli.go`
  - `internal/burlercli/cli.go`
  - `internal/preflight/predicates.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/fabricengine/open.go`
  - `internal/perchengine/engine.go`
- **Edits:**
  - `internal/perchcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Delete the `layout *lyxcwd.Location` field from the `perchCLI` struct.
  A nil-able `c.layout` guarded by `c.layout != nil` is explicitly rejected: it reintroduces the fictional-`Location` shape the design was written to eliminate and invites a later dereference no compiler catches.

  The post-card struct field inventory is exactly: `burlerEngine`, `runner`, `perchCfg`, `modelReg`, `perchGeom`, `runDirBase`, `scratchDirBase`, `stencilsDir`, `anchorRel`, `openFabric`, the two flag fields `stencilsDirFlag`/`targetDirFlag`, and the two reporting fields `mode`/`stateDir` — and no `layout`.
  Note that `stencilsDir` doubles as the third reporting field, so perch carries the same three envelope values burler does without a fourth field.
  Document each new field: `stencilsDir` is the one told stencils directory both consumers read; `anchorRel` is `loc.AnchorRel` in hub mode and `""` in standalone, and is the one thing the deleted `layout` field held that no other told replacement supplies; `openFabric` is the lazy opener, nil in standalone, which must not be called during the pre-run.
  Update the `perchGeom` field's existing comment, which today says "layout survives for the three fabric call sites in run.go, which genuinely need the Location and are genuinely hub-mode-only" — that is now false and must name the three told replacements instead.

  Replace the inline `PersistentPreRunE` closure with an extracted method
  `func (c *perchCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error`, assigned as `PersistentPreRunE: c.resolvePersistentPreRun`, with the same body shape card 14 specifies for burler: the `if cmd.Name() == "perch" { return nil }` group guard preserved exactly (pinned by `TestRunCLI_GroupGuard_OutsideGitRepo`), `lyxcwd.CwdFrom`, one `preflight.ResolveMode(cwd)` whose non-nil error is surfaced verbatim and aborts, and one `c.wire(loc, mode, cwd, c.stencilsDirFlag, c.targetDirFlag)` call.
  Move every config load, geometry build and engine construction out of this file into `wiring.go` and prune the imports that leaves unused.

  Add the two persistent flags `--stencils-dir` and `--target-dir` on the parent command with the same wording and semantics card 14 specifies for burler.
  Extend the parent `Long` with a `Modes:` section and a standalone example, keeping it accurate per the CLI/Cobra Invariant's help-accuracy obligation.
  Keep `Short`, `RunE: clihelp.GroupRunE`, both `AddCommand` calls, and the `RunCLI`/`RunCLIIn` seam functions unchanged.
  Rewrite the file-header comment's `cwd -> layout -> perch geometry -> ...` chain into the resolve-probe-delegate shape naming `wiring.go`, while preserving its explanation of why the perch engine is constructed per-invocation in the run verb rather than in the pre-run — that reasoning is unchanged and still load-bearing.
- **Commit:** `feat(perchcli): branch between hub and standalone mode and drop the layout field`

### Card 22: reroute run.go's three `c.layout` uses onto told values

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/wiring.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/open.go`
  - `internal/perchengine/engine.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/perchcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace all three `c.layout` reads, which are the only reason the field existed:

  Replace `engine.Run(profile, runDir, scratchDir, fabricengine.StencilsDir(c.layout.HubPath))` with `engine.Run(profile, runDir, scratchDir, c.stencilsDir)`.
  Add a comment recording that this is the same single value the nested `burlerengine.New` received in `wiring.go`, and that the two must never resolve it independently — that is what makes `--stencils-dir` reach perch's own rounds *and* its nested burler rounds rather than half-working.

  Replace `fabricengine.ScopedPathspec(c.layout.AnchorRel, []string{lyxdirs.LyxDirName})` with `fabricengine.ScopedPathspec(c.anchorRel, []string{lyxdirs.LyxDirName})`.
  Preserve the existing comment block above it explaining that lock files and the pause flag live in the block's `.lyx` scratch dir so no exclusion layer is involved.

  Replace `fab, syncErr = fabricengine.Open(c.layout)` with a call through `c.openFabric()`, and guard the whole block-exit fabric sync on `c.openFabric != nil` in addition to the existing `!opts.SkipGit` check.
  When `c.openFabric` is nil the entire sync is skipped and `committed` stays false, so the envelope reports `fabricCommitted: false`.
  The two conditions compose and must both be preserved: `opts.SkipGit` is a separate CI/test bypass, not a mode question, and its existing comment explaining why it is checked before `Open`'s stat-based path validation stays.
  Document that a nil `openFabric` means standalone, which has no fabric repo by construction.

  Add the three additive fields to the existing success envelope's `output.Ok` map, alongside `runDir`/`scratchDir`/`fabricCommitted`: `"mode"` from `c.mode`, `"stateDir"` from `c.stateDir` (empty in hub mode), and `"stencilsDir"` from `c.stencilsDir`.
  They are emitted in both modes for the same reason card 15 records for burler.
  Do not print any of them outside the envelope.

  Do not edit `internal/perchcli/pause.go` in this card or anywhere in this task.
  Pause runs through the same `wireStandalone` and writes under `<state>` in standalone, so the question is real — but its success envelope already reports an **absolute** `pauseFile`, which names `<state>` by construction, and pause is not where an operator first meets standalone mode.
  This omission is deliberate, recorded here so a later reader does not have to guess.
- **Commit:** `feat(perchcli): reroute run.go onto told stencils, anchorRel and a nil-able fabric opener`

### Card 23: redirect the state root in the three existing untagged tests that now reach standalone

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/wiring.go`
  - `internal/perchcli/testmain_test.go`
  - `internal/standalonestate/standalonestate.go`
- **Edits:**
  - `internal/perchcli/cli_test.go`
  - `internal/perchcli/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Three untagged tests `t.Chdir(t.TempDir())` and then drive a real subcommand, so after this batch their pre-run enters `wireStandalone`, calls `standalonestate.Derive` against the live environment, and reconciles a stencils tree into the operator's real state directory:
  `TestRunCLI_Pause_MissingRunID` in `cli_test.go`, and `TestRunCLI_Run_MissingProfile` and `TestRunCLI_Run_InvalidRunID` in `run_test.go`.

  Before running the enumeration blindly, re-derive it against the tree as it stands: the rule is every untagged test in this package that changes directory into a non-repository temp dir **and** drives a non-group subcommand.
  Tests that stop at the group guard never reach the wiring and are unaffected.
  If the re-derived set differs from the three named above, treat the re-derived set as authoritative and say so in the batch report.

  For each test in that set, add `t.Setenv("XDG_STATE_HOME", t.TempDir())` and `t.Setenv("LOCALAPPDATA", t.TempDir())` before the `RunCLI` call so both `Derive` branches land inside the test's own temp tree on every platform.
  `gitkit.HermeticGitEnv()` in `internal/perchcli/testmain_test.go` does not cover this — it isolates git config only.
  None of these tests may be marked `t.Parallel()`.

  Update the stale doc comments.
  `TestRunCLI_Pause_MissingRunID` and `TestRunCLI_Run_MissingProfile` both describe "the same documented double-failure shape", which is gone: the pre-run now succeeds and only the verb's own flag error is emitted.
  `TestRunCLI_Run_InvalidRunID`'s comment needs the same treatment for its "runs against an uninitialized directory" clause.
  Replace each with an accurate statement: the directory resolves to standalone mode, the pre-run succeeds, and the state root is redirected so the standalone wiring stays inside the test's temp tree.
  Leave every assertion exactly as it is — the exit codes and pinned output substrings still hold, and each test's flag-validation contract is what it exists to pin.
- **Commit:** `test(perchcli): redirect the state root in the three tests that now wire standalone`

### Card 24: add the tier-1 wiring truth table and pinned standalone values

- **Context:**
  - `internal/perchcli/wiring.go`
  - `internal/perchcli/cli.go`
  - `internal/webstercli/wiring_test.go`
  - `internal/burlercli/wiring_test.go`
  - `internal/preflight/predicates.go`
  - `internal/standalonegeom/perchgeom.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/standalonegeom/standalonegeom_test.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/perchengine/identity.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/perchcli/wiring_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/perchcli/wiring_test.go` in `package perchcli`, untagged, following the same shape card 18 specifies for burler and `internal/webstercli/wiring_test.go` establishes.
  Every case calls `c.wire` directly on a `&perchCLI{}` the test holds, with a told `(loc, mode)` pair.

  Cover the two-row truth table — `(loc non-nil, ModeHub)` and `(nil, ModeStandalone)` — with the same comment naming both surviving standalone causes and the same prohibition on a `(loc non-nil, ModeStandalone)` row.
  Assert that `wireStandalone` never reads `loc`.
  Record in the file header that the refuse case is pinned in `internal/preflight`'s integration table, not here.

  Pin every standalone value: `c.perchGeom.GateDir` equals the target and `c.perchGeom.AnchorPath` equals `<state>`; `c.runDirBase` equals `perchengine.RunsDir(<state>)` and `c.scratchDirBase` equals `perchengine.ScratchDir(<state>)`, both therefore under `<state>`; the config base is `<state>`; `c.stencilsDir` equals `standalonegeom.StencilsDir(stateDir)` when the flag is unset.

  **Do not assert the reed geometry's field values here, and do not add an accessor to make them assertable**, for the same reason card 18 records for burler: `shuttleengine.Runner` holds every field unexported with no geometry accessor, and `perchCLI` stores only `c.runner`, so `SocketKey`, `SessionName`, `LogsDir`, `RepoName` and `HubPath` are unreachable from this in-package test.
  They are pinned one layer down by `TestReedGeometry` in `internal/standalonegeom/standalonegeom_test.go`.
  The linkage — that `wireStandalone` passes this invocation's own `target`, `stateDir` and `hash8` to `standalonegeom.ReedGeometry` — is a review obligation, not an assertion; record that in a comment here.

  Add the two perch-only assertions the design names:
  `c.openFabric` is non-nil in hub mode and nil in standalone, and `c.anchorRel` equals `loc.AnchorRel` in hub mode and `""` in standalone;
  and — the load-bearing one — **one** `stencilsDir` value reaches **both** consumers in both modes, including when `--stencils-dir` is passed.
  Assert the nested burler engine's stencils directory and the value `run.go` would pass to `perchengine.Engine.Run` are the **same string**, not merely that each is non-empty.
  If the nested engine's stencils directory is not observable from outside `*burlerengine.Engine`, assert instead that exactly one `stencilsDir` value exists on the receiver and that `wireHub`/`wireStandalone` pass that same field to `burlerengine.New` — and state in a comment which of the two forms was used and why, rather than silently weakening the assertion.

  Cover hub mode resolving today's values given a told `Location`, `--target-dir` refused in hub mode, `--stencils-dir` honoured in both modes, the standalone default seeded, an explicit `--stencils-dir` never written to, and `resolveStandaloneTarget`'s three rows — the same set card 18 specifies.
  Every case reaching `wireStandalone` sets both `XDG_STATE_HOME` and `LOCALAPPDATA` to `t.TempDir()` values and is not `t.Parallel()`.
  No test in this file may spawn a process.
- **Commit:** `test(perchcli): pin wire's truth table, the two-consumer stencils rule and every told value`

### Card 25: extend the integration suite with the standalone pre-run case

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/wiring.go`
  - `internal/perchcli/run.go`
  - `internal/webstercli/cli_integration_test.go`
  - `internal/burlercli/cli_integration_test.go`
  - `internal/standalonestate/standalonestate.go`
- **Edits:**
  - `internal/perchcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two tests to this already-`//go:build integration`-tagged file, following the shape card 19 specifies for burler.

  `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate`: drive `RunCLIIn(<a t.TempDir() outside any git repository>, &out, []string{"run"})` and assert exit code 1 with the run verb's own `--profile is required` flag error, plus an explicit assertion that the output carries no `not a git repository` text.

  `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged`: assert the target directory is empty before, drive the same invocation, and assert it gained no entries — the two-roots split's whole point, and the only place a real `Derive` and a real filesystem can prove the absence.

  Both redirect `XDG_STATE_HOME` and `LOCALAPPDATA` to `t.TempDir()` values before the call and neither is `t.Parallel()`.
  Extend the file-header comment, which today describes the file as holding "the perchcli pause tests that build a real hub", so it also covers the standalone entry tests, which build no hub at all.

  Leave all five existing tests in this file unchanged.
  `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` in particular must keep passing untouched: it is what pins `runDirBase`/`scratchDirBase` anchoring at `AnchorPath` rather than `WorktreeRoot`, and it is exactly the kind of thing a careless wiring refactor breaks.
- **Commit:** `test(perchcli): drive RunCLIIn standalone from outside any git repository`

### Card 26: verify no `*lyxcwd.Location` survives on the perch receiver

- **Context:**
  - `internal/perchcli/cli.go`
  - `internal/perchcli/run.go`
  - `internal/perchcli/pause.go`
  - `internal/perchcli/wiring.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Grep `internal/perchcli` for `c.layout`, `layout`, and `lyxcwd.Location` and confirm the only surviving reference to `*lyxcwd.Location` in the package is `wire`'s and `wireHub`'s `loc` parameter — no struct field, no local held across a call boundary, and no synthesised value anywhere.
  Confirm the same for `fabricengine.StencilsDir`: it must appear exactly once in the package, in `wireHub`, and never in `run.go` or `pause.go`.
  Confirm `standalonestate.Derive` appears exactly once, in `wireStandalone`.

  This card writes no code and produces no commit.
  It exists because "no `*lyxcwd.Location` survives on the receiver" and "one `stencilsDir` per invocation" are properties a compiler will not check once the field is gone and a second `fabricengine.StencilsDir` call would silently reintroduce the split card 24 exists to prevent.
  Report any surviving reference as a blocking finding rather than editing here — a fix belongs in the card that introduced it, where its own tests and commit message cover it.
- **Commit:** none

## Batch Tests

`verify:` runs `go test ./internal/perchcli/...` followed by `go test -tags integration ./internal/perchcli/...`.
Both are required.
The untagged run covers cards 23 and 24 plus the package's large existing untagged surface (`cli_test.go`, `run_test.go`), which is the regression coverage proving the `layout` removal and the `cli.go` rewrite preserved the group guard, the cobra seam, and every flag-validation contract.
The tagged run compiles card 25's two new tests and, critically, the five pre-existing ones in `cli_integration_test.go` — `TestRunCLI_Pause_NestedInitAnchorsRunDirsAtCwd` among them, which is the anchoring regression this batch is most able to break and which the untagged run never executes.

It also compiles `internal/perchcli/run_integration_test.go`, which this batch never edits but which is card 22's most direct regression coverage and is named here so it is not overlooked.
Its four tests — `TestRunCLI_Run_FabricSyncRunsOnEngineError`, `TestRunCLI_Run_FabricCommitExcludesLockFiles`, `TestRunCLI_Run_FabricCommitExcludesLockFiles_NestedRelPath` and `TestRunCLI_Run_BusyBlockSkipsFabricSync` — exercise exactly the block-exit fabric sync card 22 reroutes from `fabricengine.Open(c.layout)` onto `c.openFabric()`, including the `opts.SkipGit` short-circuit and the `ErrBlockBusy` skip that must both compose unchanged with the new nil-opener guard.
They run in hub mode, where this task claims byte-identity, so all four must keep passing without modification;
a failure there is the clearest possible signal that the opener reroute changed hub behaviour rather than merely relocating it.
Re-derive the exact test names from the file rather than trusting this list.

The scope is the single package.
Card 26 is verification-only and contributes no diff; its findings, if any, are reported rather than fixed in place.
