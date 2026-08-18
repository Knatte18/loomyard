# Batch: burlercli-standalone-entry

```yaml
task: "the standalone CLI path"
batch: "burlercli-standalone-entry"
number: 4
cards: 7
verify: go test ./internal/burlercli/... && go test -tags integration ./internal/burlercli/...
depends-on: [1, 2]
```

## Batch Scope

This batch gives `lyx burler` its standalone entry: a new `wiring.go` holding the mode decision and both engine-stack branches, a `cli.go` reduced to cwd plus one `preflight.ResolveMode` probe plus delegation, two new persistent flags, three additive envelope fields, and the two test tiers that pin all of it.
It is one batch because every card is inside `internal/burlercli` and the receiver-shape change in card 14 is what cards 15, 18 and 19 assert against — splitting it would leave a batch boundary in the middle of one struct's definition.
Burler is the simpler of the two producer CLIs: it has no fabric relationship at all, which is why it lands first and why batch 5 depends on it — perch's cards read this batch's `wiring.go`, `cli.go` and test files, and perch's nested `burlerengine.New` must build byte-identical geometry to what this batch ships for the same target.

**Batch-local decision:** `burlerCLI` carries no `*lyxcwd.Location`, no `openFabric`, and no `anchorRel`.
`internal/burlercli/run.go` references neither `layout` nor `fabricengine` today, so the wiring split must not invent a fabric relationship the module does not have.

## Cards

### Card 13: add `internal/burlercli/wiring.go`

- **Context:**
  - `internal/webstercli/wiring.go`
  - `internal/preflight/predicates.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/geometry.go`
  - `internal/burlerengine/config.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/buildinfo/buildinfo.go`
  - `internal/stencilstore/reconcile.go`
  - `internal/stencilstore/stencilstore.go`
  - `contracts/stencils/stencils.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/reedengine/config.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/claudeengine/claudeengine.go`
- **Edits:** none
- **Creates:**
  - `internal/burlercli/wiring.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/burlercli/wiring.go` in `package burlercli`, mirroring `internal/webstercli/wiring.go`'s structure so a reader who knows one of the three CLIs knows all three.
  Carry a file-header comment in the same shape, stating that the mode decision lives here inside the extracted function rather than upstream in `resolvePersistentPreRun`, precisely so a test can drive it with a told `preflight.Mode` and stay tier 1 — driving the real pre-run would reach `lyxcwd.Resolve` and its git spawn.

  Add `func (c *burlerCLI) wire(loc *lyxcwd.Location, mode preflight.Mode, cwd, stencilsDirFlag, targetDirFlag string) error`, dispatching to `c.wireHub(loc, stencilsDirFlag, targetDirFlag)` when `mode == preflight.ModeHub` and to `c.wireStandalone(cwd, stencilsDirFlag, targetDirFlag)` otherwise.
  There is no `planDirFlag` parameter — that flag is webster-only, because burler parses no plan.
  Document that `wire` never sees the refused case, which aborts upstream, and that `wireStandalone` deliberately does not take `loc`.

  Add `func (c *burlerCLI) wireHub(loc *lyxcwd.Location, stencilsDirFlag, targetDirFlag string) error` reproducing today's `PersistentPreRunE` body byte-for-byte in resolved values:
  return an error naming the reason when `targetDirFlag != ""`, mirroring `webstercli`'s wording — the worktree is already the target, and honouring any other value would strand its artifacts outside fabric's positive-only commit pathspec;
  bind `anchorPath := loc.AnchorPath()`;
  load `shuttleengine.LoadConfig(anchorPath, "shuttle")`, `burlerengine.LoadConfig(anchorPath)` and `reedengine.LoadConfig(anchorPath, "reed")`, returning each error unwrapped;
  compute `stencilsDir := fabricengine.StencilsDir(loc.HubPath)`, overridden by `stencilsDirFlag` when non-empty;
  build `reedGeom := hubgeom.ReedGeometry(loc)`, `reedEngine := reedengine.New(reedCfg, reedGeom)`, `runner := shuttleengine.NewRunner(reedEngine, claudeengine.New(), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)`;
  set `c.engine = burlerengine.New(runner, hubgeom.BurlerGeometry(loc), burlerCfg, stencilsDir)`;
  and set the three reporting fields `c.mode = "hub"`, `c.stateDir = ""`, `c.stencilsDir = stencilsDir`.
  Preserve today's comment explaining that both configs anchor at `loc.AnchorPath()` — the worktree the operator is actually standing in, never `WorktreeRoot` or any fabric sibling — and today's comment explaining that an absent `burler.yaml` is not an error but decodes to the zero Config.

  Add `func (c *burlerCLI) wireStandalone(cwd, stencilsDirFlag, targetDirFlag string) error`:
  `target, err := resolveStandaloneTarget(cwd, targetDirFlag)`;
  `stateDir, hash8, err := standalonestate.Derive(target)` — this is the only `Derive` call site in the package;
  compute `stencilsDir`: when `stencilsDirFlag != ""` use it verbatim and seed nothing, otherwise use `standalonegeom.StencilsDir(stateDir)` and seed it via
  `stencilstore.Reconcile(stencilsDir, stencils.Registry(), stencilstore.ModeFor(buildinfo.IsDev()), "")`,
  returning a wrapped hard error naming the directory when `Reconcile` fails;
  load `shuttleengine.LoadConfig(stateDir, "shuttle")`, `burlerengine.LoadConfig(stateDir)` and `reedengine.LoadConfig(stateDir, "reed")`, all anchored at `stateDir`;
  build `reedGeom := standalonegeom.ReedGeometry(target, stateDir, hash8)` — reuse it as-is, do not re-derive any of its eight values;
  build the reed engine, claude engine and runner exactly as the hub branch does;
  set `c.engine = burlerengine.New(runner, standalonegeom.BurlerGeometry(target, stateDir), burlerCfg, stencilsDir)`;
  and set `c.mode = "standalone"`, `c.stateDir = stateDir`, `c.stencilsDir = stencilsDir`.

  Comment the two asymmetries that a reader will otherwise try to "simplify":
  an explicitly-told `stencilsDirFlag` is read and never written in either mode, which is what makes the read-only characterisation literally true and protects a curated stencil set;
  and the derived default's `Reconcile` failure is a hard pre-run error rather than the root pre-run's best-effort logged seed, because nothing else will ever create this directory and a silent failure resurfaces much later as an opaque prompt-render error.
  The empty fourth `Reconcile` argument is the "no source tree here" value that keeps the port-back drift warning silent — standalone genuinely has no `contracts/stencils` source tree beside it.

  Add `func resolveStandaloneTarget(cwd, targetDirFlag string) (string, error)` reproducing `webstercli`'s shape: empty flag returns `cwd`; an absolute flag returns `filepath.Clean(flag)`; a relative flag returns `filepath.Join(cwd, flag)`.
  Document that the result is always absolute, which is `standalonestate.Derive`'s own precondition, because `Derive` normalises through `EvalSymlinks`+`Clean` and compares case-insensitively on Windows, so two spellings of the same directory must not produce different `<state>` values.

  Neither branch may call `os.Getwd`, spawn git, or re-derive any path: every value is either told by the caller or a plain filesystem read.
  Do not import `internal/fabricengine` for anything other than `StencilsDir` in the hub branch, and never call `fabricengine.Ready` or `preflight.Wired` from this package.
- **Commit:** `feat(burlercli): add wiring.go with the hub and standalone engine-stack branches`

### Card 14: reduce `cli.go` to resolve-probe-delegate, and add the two persistent flags

- **Context:**
  - `internal/burlercli/wiring.go`
  - `internal/webstercli/cli.go`
  - `internal/preflight/predicates.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/burlerengine/engine.go`
- **Edits:**
  - `internal/burlercli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend the `burlerCLI` struct from its current single `engine *burlerengine.Engine` field to exactly six fields: `engine`, the two raw flag fields `stencilsDirFlag` and `targetDirFlag`, and the three reporting fields `mode`, `stateDir` and `stencilsDir`, all strings.
  Document on the flag fields that an empty value means the flag was not passed and that each mode's own default is computed by the wiring function rather than a zero-value fallback landing here.
  Document on the reporting fields that they exist because `run.go` can only read them off the receiver for its envelope, and that they are CLI-level facts about how the stack was wired rather than results of a review round — which is why they are not threaded through `burlerengine.Result`.
  Add no `layout`, no `anchorRel`, and no `openFabric` field: burler has no fabric relationship, and `run.go` references neither `layout` nor `fabricengine` today.

  Replace the inline `PersistentPreRunE` closure with an extracted method
  `func (c *burlerCLI) resolvePersistentPreRun(cmd *cobra.Command, args []string) error`, assigned as `PersistentPreRunE: c.resolvePersistentPreRun`.
  Its body, in order: the group-command guard `if cmd.Name() == "burler" { return nil }`, preserved exactly as today because `TestRunCLI_GroupGuard_OutsideGitRepo` pins it; `ctx := cmd.Context()` and `out := cmd.OutOrStdout()`; `lyxcwd.CwdFrom(ctx)` with today's error branch unchanged; one `preflight.ResolveMode(cwd)` call whose non-nil error is surfaced verbatim via `output.Err` then `clihelp.Abort(ctx, 1)` and `return nil`; and one `c.wire(loc, mode, cwd, c.stencilsDirFlag, c.targetDirFlag)` call with the same error-surfacing shape.
  The extraction is what makes card 18's truth-table test tier 1 — state that in the method's doc comment, together with the fact that the refusal stays here rather than moving into `wire` because it is a resolution verdict, not a wiring choice.
  Delete the `lyxcwd.Resolve` call and every config load, geometry build and engine construction from this file — all of it now lives in `wiring.go`.
  Prune the imports this leaves unused; `internal/burlercli/cli.go` must no longer import `internal/burlerengine`, `internal/fabricengine`, `internal/hubgeom`, `internal/reedengine`, `internal/shuttleengine` or `internal/shuttleengine/claudeengine` unless a surviving expression still needs it.

  Add two persistent flags on the parent command, bound to the two flag fields, mirroring `webstercli`'s own block:
  `--stencils-dir`, described as read-only in both modes, hub default the hub's own stencils dir, standalone default the derived state directory's `_lyx/stencils`;
  `--target-dir`, described as standalone-only, defaulting to the current directory, and refused in hub mode where the worktree is already the target.
  Both are persistent rather than per-verb because they configure the stack the pre-run builds, before any verb runs.

  Extend the parent command's `Long` with a `Modes:` section documenting the two modes and both flags' semantics, and a standalone example invocation, following the shape `internal/webstercli/cli.go`'s `Long` already uses.
  The `Long` must stay accurate now that observable behaviour changes — this is the CLI/Cobra Invariant's help-accuracy obligation, not optional polish.
  Keep `Short` non-empty and unchanged, keep `RunE: clihelp.GroupRunE`, and keep `parent.AddCommand(c.runCmd())` and both `RunCLI`/`RunCLIIn` seam functions exactly as they are.
  Update the file-header comment, which today describes a `PersistentPreRunE` that resolves `cwd -> layout -> ... -> burlerengine.Engine`: it must describe the resolve-probe-delegate shape and name `wiring.go` as where the mode decision and both branches live, while preserving its closing note that `burlercli` is the module's `claudeengine` wiring point per the Provider-Seam Invariant.
- **Commit:** `feat(burlercli): branch between hub and standalone mode in the pre-run`

### Card 15: report `mode`, `stateDir` and `stencilsDir` in run's success envelope

- **Context:**
  - `internal/burlercli/cli.go`
  - `internal/burlerengine/engine.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/burlercli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Change `resultEnvelope`'s signature from `func resultEnvelope(result burlerengine.Result) map[string]any` to
  `func resultEnvelope(result burlerengine.Result, mode, stateDir, stencilsDir string) map[string]any`, and add the three keys `"mode"`, `"stateDir"` and `"stencilsDir"` to the returned map alongside the nine existing keys.
  Update the single call site in `runCmd`'s `RunE` to `resultEnvelope(result, c.mode, c.stateDir, c.stencilsDir)`.

  Extend `resultEnvelope`'s doc comment: the three new values are CLI-level facts about how the stack was wired, taken as parameters rather than read off a receiver so the function stays directly unit-testable without a live `c.engine.Run` call — which is the property the existing doc comment already claims and card 16 depends on.
  Record that the fields are emitted in **both** modes deliberately: a `mode` field that existed only in standalone could not be used to tell the two modes apart, which is its whole purpose, and `stencilsDir` is equally worth reporting in a hub run pointed at an experimental stencil set via the flag.
  `stateDir` is the empty string in hub mode, where no derived state directory exists.
  Note that this is the third named exception to hub byte-identity: it is an output-shape-only change, no path resolves differently and nothing new is written in hub mode, and the keys are additive so no existing consumer breaks.

  Do not print any of the three values outside the envelope: a stray `fmt.Println` would corrupt the machine-readable envelope contract every caller parses.
  Change nothing else in this file — `decodeProfile`, the flag set, and the `RunE` control flow all stay as they are.
- **Commit:** `feat(burlercli): report mode, stateDir and stencilsDir in run's envelope`

### Card 16: update the two existing untagged tests the mode change moves

- **Context:**
  - `internal/burlercli/run.go`
  - `internal/burlercli/cli.go`
  - `internal/standalonestate/standalonestate.go`
- **Edits:**
  - `internal/burlercli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Two tests in this file need work, for two unrelated reasons.

  `TestRunCLI_Run_MissingProfile` calls `t.Chdir(t.TempDir())` and then drives the real `run` subcommand.
  Today its `PersistentPreRunE` aborts because the temp dir is not a git repository; after this batch `ResolveMode` returns `ModeStandalone` there, so the pre-run enters `wireStandalone`, calls `standalonestate.Derive` against the live environment, and reconciles a stencils tree into the operator's real state directory from an untagged unit test.
  Add `t.Setenv("XDG_STATE_HOME", t.TempDir())` and `t.Setenv("LOCALAPPDATA", t.TempDir())` before the `RunCLI` call so both `Derive` branches land inside the test's own temp tree on every platform.
  The test is already not `t.Parallel()`, which `t.Setenv` requires; keep it that way.
  Rewrite the doc comment's stale rationale: it currently says this case "runs against an uninitialized (non-git) directory, so PersistentPreRunE's own abort error is also present in the captured output alongside the flag-specific error line — the same documented double-failure shape as shuttlecli's TestRunCLI_Run_FlagValidation."
  That double-failure shape is gone — the pre-run now succeeds standalone and only the verb's own flag error is emitted.
  Say instead that the directory resolves to standalone mode, that the pre-run therefore succeeds, and that the state root is redirected so the standalone wiring's `Derive` and stencil seed stay inside the test's temp tree.
  Leave both assertions (`exitCode == 1`, output contains `--profile is required`) exactly as they are: what changed is the surrounding output, not the flag-validation contract this test exists to pin.

  `TestResultEnvelope_ForkCountNilGuard` calls `resultEnvelope` directly in both of its subtests and must follow card 15's new signature.
  Pass told literal strings for the three new parameters and add one assertion per subtest that each lands in the envelope under its own key, so the new fields are covered rather than merely compiled against.
  Its existing `forkCount` and `clusterWarnings` assertions are unchanged.

  Do not touch `TestRunCLI_NoArgs`, `TestRunCLI_UnknownSubcommand`, `TestRunCLI_GroupGuard_OutsideGitRepo`, `TestCommand_EveryCommandHasShort`, or `TestDecodeProfile`.
  The first three stop at the group guard and never reach the wiring; the last two are unaffected by mode selection.
- **Commit:** `test(burlercli): redirect the state root and follow resultEnvelope's new signature`

### Card 17: add a hermetic `TestMain` to `internal/burlercli`

- **Context:**
  - `internal/perchcli/testmain_test.go`
  - `internal/webstercli/testmain_test.go`
  - `internal/gitkit/hermetic.go`
- **Edits:** none
- **Creates:**
  - `internal/burlercli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/burlercli/testmain_test.go` in `package burlercli`, mirroring `internal/perchcli/testmain_test.go` and `internal/webstercli/testmain_test.go` exactly: a `TestMain(m *testing.M)` that calls `gitkit.HermeticGitEnv()` before `os.Exit(m.Run())`.
  Leave the file untagged so it compiles into both the tagged and the untagged test binary.

  Its file-header comment must state why the file is required rather than tidy: card 19's integration test drives `RunCLIIn`, which now reaches `preflight.ResolveMode` and through it `lyxcwd.Resolve`'s real `git rev-parse` spawn, in a package that spawns git from no test today.
  The Hermetic Git Test Environment guard is token-keyed and will not catch a spawn reached indirectly through a CLI seam, so nothing else would flag the gap.
  Note that git-config isolation here and the per-test state-root redirect (`t.Setenv`) are two separate halves and this package needs both.
- **Commit:** `test(burlercli): add a hermetic TestMain for the new git-spawning integration test`

### Card 18: add the tier-1 wiring truth table and pinned standalone values

- **Context:**
  - `internal/burlercli/wiring.go`
  - `internal/burlercli/cli.go`
  - `internal/webstercli/wiring_test.go`
  - `internal/preflight/predicates.go`
  - `internal/standalonegeom/burlergeom.go`
  - `internal/standalonegeom/reedgeom.go`
  - `internal/standalonegeom/stencilsdir.go`
  - `internal/standalonegeom/standalonegeom_test.go`
  - `internal/standalonestate/standalonestate.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/burlercli/wiring_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/burlercli/wiring_test.go` in `package burlercli`, untagged, following `internal/webstercli/wiring_test.go`'s established shape.
  Every case calls `c.wire` directly on a `&burlerCLI{}` the test holds, with a told `(loc, mode)` pair — never through the real pre-run, so no case spawns a process or resolves cwd.

  Cover the two-row mode-selection truth table:
  `(loc non-nil, preflight.ModeHub)` selects hub mode, driven with a fictional, on-disk-absent `*lyxcwd.Location` exactly as `webstercli`'s hub cases do, since every config load degrades to its embedded template on a proven-absent `_lyx/` directory;
  `(nil, preflight.ModeStandalone)` selects standalone.
  Comment on the standalone row that it covers **both** surviving standalone causes — the plain downloaded repo and a genuine non-repository directory — because `ResolveMode` returns a nil `loc` for each, making them indistinguishable at `wire`'s boundary; name the plain-git-repo cause explicitly, since it is what the design's r5 review caught.
  Do not write a `(loc non-nil, ModeStandalone)` row: no caller can produce it.
  Assert directly that `wireStandalone` never reads `loc` — it takes `cwd` and the flags, nothing else — and record in the file header that the refuse case is pinned in `internal/preflight`'s integration table and `internal/burlercli/cli_integration_test.go`, not here, because `wire` never receives it.

  Pin every standalone value the design names, asserting against a `stateDir` the test derives itself via `standalonestate.Derive(target)` under a redirected state root:
  the burler geometry's `WorktreeRoot` equals the target and its `AnchorPath` equals `<state>`;
  the config base is `<state>`;
  and `c.stencilsDir` equals `standalonegeom.StencilsDir(stateDir)` when the flag is unset.
  Where a value is not observable from outside the constructed `*burlerengine.Engine`, assert it through the receiver's own reporting fields (`c.mode`, `c.stateDir`, `c.stencilsDir`) and add a comment naming what is and is not observable at this seam rather than inventing an accessor on the engine.

  **Do not assert the reed geometry's field values here, and do not add an accessor to make them assertable.**
  `burlerengine.Engine` exposes only `Run` and holds its `geom`, `cfg` and `stencilsDir` unexported; `shuttleengine.Runner` holds `reed`, `engine`, `anchorPath`, `worktreeRoot` and `cfg` unexported with no geometry accessor.
  Both are different packages from this in-package test, so `SocketKey`, `SessionName`, `LogsDir`, `RepoName` and `HubPath` are unreachable from here, and none of the three reporting fields encodes them.
  `internal/webstercli/wiring_test.go`, the file this one mirrors, asserts no reed-geometry values either.
  Those five values are already pinned one layer down, by `TestReedGeometry` in `internal/standalonegeom/standalonegeom_test.go`, which asserts all eight fields against told parameters.
  What that leaves uncovered by any test is the *linkage* — that `wireStandalone` calls `standalonegeom.ReedGeometry` with this invocation's own `target`, `stateDir` and `hash8` rather than some other triple.
  No seam exposes it, so it is a review obligation rather than an assertion: state that explicitly in a comment at this point in the file, so a later reader knows the gap is recorded rather than overlooked.

  Cover hub mode resolving the same values it does today given a told `Location`: `c.mode` is `"hub"`, `c.stateDir` is empty, and `c.stencilsDir` equals `fabricengine.StencilsDir(loc.HubPath)`.
  Cover the flags: `--target-dir` refused in hub mode with an error naming the reason; `--stencils-dir` honoured in both modes; the standalone default seeded on disk when the flag is unset; an explicit `--stencils-dir` never written to, asserted by pointing it at an empty `t.TempDir()` and checking the directory is still empty afterwards.
  Cover `resolveStandaloneTarget`'s three rows: unset returns cwd, absolute returns the cleaned path, relative returns the path joined onto cwd.

  Every case that reaches `wireStandalone` must call both `t.Setenv("XDG_STATE_HOME", t.TempDir())` and `t.Setenv("LOCALAPPDATA", t.TempDir())` first, and must not be marked `t.Parallel()`.
  State that constraint in the file header, as `webstercli`'s own wiring test does.
  No test in this file may spawn a process or call `gitexec.Run` — this file is what the Test Tier Purity Invariant's untagged rule is protecting.
- **Commit:** `test(burlercli): pin wire's truth table and every standalone told value`

### Card 19: add the integration-tier standalone entry test

- **Context:**
  - `internal/burlercli/cli.go`
  - `internal/burlercli/wiring.go`
  - `internal/burlercli/testmain_test.go`
  - `internal/webstercli/cli_integration_test.go`
  - `internal/standalonestate/standalonestate.go`
- **Edits:** none
- **Creates:**
  - `internal/burlercli/cli_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/burlercli/cli_integration_test.go` in `package burlercli`, whose first line is the `//go:build integration` constraint, following `internal/webstercli/cli_integration_test.go`'s shape and file-header style.

  Add `TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate`: drive `RunCLIIn(<a t.TempDir() outside any git repository>, &out, []string{"run"})` and assert exit code 1 with output containing `burler: --profile is required` — the run verb's own flag validation — rather than a cwd-resolution error.
  Assert explicitly that the output contains no `not a git repository` text, mirroring how the webster test guards the same distinction: reaching the verb's own gate is the property that pins the real wiring rather than the extracted helper, and a resolution failure would produce a completely different message.

  Add `TestRunCLIIn_StandalonePreRun_TargetDirectoryUnchanged`: read the target directory before the invocation, assert it is empty (failing the fixture if not), drive the same standalone invocation, then assert it gained no entries at all.
  This is the two-roots split's whole point — the operator's directory gains no hidden state tree, no lock file, and no rendered prompt — and it is the one property no untagged test in this batch can observe, since it needs a real `Derive` call and a real filesystem to assert an absence against.

  Both tests must redirect `XDG_STATE_HOME` and `LOCALAPPDATA` to `t.TempDir()` values before calling `RunCLIIn`, and neither may be marked `t.Parallel()`.
  Document in the file header that this file's tests reach the real `standalonestate.Derive`, the real standalone stencil seed, and an end-to-end pre-run, which is why they are tagged, and that card 17's `testmain_test.go` supplies the hermetic git environment the `git rev-parse` inside `ResolveMode` needs.
- **Commit:** `test(burlercli): drive RunCLIIn standalone from outside any git repository`

## Batch Tests

`verify:` runs `go test ./internal/burlercli/...` followed by `go test -tags integration ./internal/burlercli/...`.
Both are required.
The untagged run covers cards 16 and 18 plus the package's existing suite — `TestRunCLI_GroupGuard_OutsideGitRepo`, `TestRunCLI_NoArgs`, `TestRunCLI_UnknownSubcommand`, `TestCommand_EveryCommandHasShort` and `TestDecodeProfile` are the regression coverage proving the `cli.go` rewrite in card 14 preserved the group guard and the cobra seam.
The tagged run is the only one that compiles card 19's new file, which is this batch's sole end-to-end proof that the standalone path works rather than merely wires.

The scope is the single package: no card here edits a file outside `internal/burlercli`, and the cross-package enforcement suites in `cmd/lyx` (tier purity, hermetic environment, help tree, constructor anchoring) are batch 6's gate, with the overview's module-wide `go vet ./...` catching any compile-level fallout at this batch's own boundary.
