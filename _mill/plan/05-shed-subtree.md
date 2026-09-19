# Batch: shed-subtree

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "shed-subtree"
number: 5
cards: 6
verify: go test ./internal/shedcli/... ./cmd/lyx/... ./internal/loomcli/... ./internal/lifecyclecli/... && go test -tags integration ./internal/shedcli/...
depends-on: [2, 4]
```

## Batch Scope

This batch ships the `lyx shed` subtree: `internal/shedcli`, holding the named-recipe table and the three CLI seams, registered under the lyx root.
It is a separate package from `shedverbs` because one package cannot hold both halves — the arming functions are built over the `loomCLI` and `lifecycleCLI` receivers, so whichever package owns the table must import those two `<module>cli` packages, and those two must import the verb bodies.
Splitting them puts the bodies at a leaf and the table at the composition layer, which is what the dependency direction already demands, and keeps every module's `Command()` signature argument-free so the CLI/Cobra Invariant needs no new deviation for an injected table.

It depends on batch 4 for `loomcli.Arm` and `lifecyclecli.Arm`, and on batch 2 because registering a second path into the lifecycle recipe means this batch's parity tests drive `lyx shed run --recipe lifecycle` through the renamed `InnerRun` engine — a partial rename would fail that path at `shedbuild.Build` time rather than at compile time, so the edge is real rather than defensive.

Batch-local decision: the table is a plain map literal in `shedcli`, in the same shape and for the same reason as `shedrecipe`'s own registry — one declaration site, reached through accessors, with no `init()` self-registration and no runtime `Register`.
It is a *different* table from the engine registry (recipe names, not engine names) and must never be merged into it.

## Cards

### Card 29: declare the recipe table

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/verbs.go`
  - `internal/loomcli/arm.go`
  - `internal/lifecyclecli/arm.go`
  - `internal/shedrecipe/registry.go`
  - `internal/lifecyclecli/cli.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcli/doc.go`
  - `internal/shedcli/table.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare `type entry struct` carrying three fields — `Arm func(cwd string, verb string, args []string) (shedverbs.Spec, error)`, `Args cobra.PositionalArgs`, and `Verbs []string` naming the verbs that recipe supports — and a package-level `var recipes = map[string]entry{"loom": {Arm: loomcli.Arm, Args: cobra.NoArgs, Verbs: []string{"run", "step", "status", "pause"}}, "lifecycle": {Arm: lifecyclecli.Arm, Args: cobra.ExactArgs(1), Verbs: []string{"run", "status", "pause"}}}`.
  The `Verbs` set is this table's sole authority on which verb/recipe pairs exist, read by card 30's pre-run refusal and asserted by card 31's table test.
  It is needed because the four generic verbs are registered once for every recipe while the recipes do not all support all four: lifecycle has no `step` analogue, so `lyx shed step --recipe lifecycle` would otherwise reach the generic `step` body with `StepBusyKind` unset and `PreStep` nil — emitting `kind: ""`, a sixth value outside the five the new Shed Verb-Set Invariant and `ly-drive` both pin closed, and skipping lifecycle's whole `PreRun` pre-flight.
  Reach it only through accessors — a `lookup(name string) (entry, error)` returning a clear unknown-recipe error naming the available names, and a `names() []string` returning them sorted — with no `init()` self-registration and no runtime `Register` function, mirroring `internal/shedrecipe/registry.go`'s own shape.
  Record in `doc.go` that this table is a different table from the engine registry: it maps recipe names to arming functions, the registry maps engine names to `ShedProducer` constructors, and the two must never be merged.
  Record why the table binds a name to an arming *function* rather than to a recipe file path: a recipe alone cannot arm a Shed, because `Env` carries injected seams (`Shuttle`, `Burler`, `WebsterRun`) and told roots no file can name, and a path-taking flag would also imply a runtime on-disk location for recipe files, which `internal/shedbuild`'s own doc and the Recipe-Format Sole-Parser Invariant both forbid.
  Record why each entry declares its own positional-arg contract: `loom` takes no positional argument and `lifecycle` takes exactly one slug, and sharing the same `cobra.PositionalArgs` value `lifecyclecli` assigns to its own commands is what makes the two paths refuse a wrong argument count byte-identically rather than by luck.
- **Commit:** `feat(shedcli): declare the named-recipe arming table`

### Card 30: build the shed subtree and its three seams

- **Context:**
  - `internal/shedcli/table.go`
  - `internal/shedcli/doc.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/spec.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/loomcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/lyxcwd/cwdcontext.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare `Command() *cobra.Command` returning a `shed` parent carrying a non-empty `Short`, a `Long` describing the four verbs and the `--recipe` flag, `RunE: clihelp.GroupRunE` so a bare `lyx shed` lists subcommands and `lyx shed bogus` emits a JSON error envelope, and a persistent `--recipe` string flag defaulting to `loom`.
  `--recipe` is persistent on the parent so it is never confused with a recipe's own positional arguments.
  Hold one `*shedverbs.Spec` on an unexported receiver and build all four verbs from `shedverbs.Verbs(shedVerbTexts, spec)`, where `shedVerbTexts` is this subtree's own generic help text — it is not loom's and not lifecycle's, since one `lyx shed status --help` serves every recipe.
  Set each returned command's `Args` to a closure that looks the resolved `--recipe` value up in the table and delegates to that entry's `cobra.PositionalArgs`, returning nil when the recipe is unresolvable so the unknown-recipe refusal comes from the pre-run's own envelope rather than from an argument-count error.
  This closure form is required rather than a static assignment: cobra parses flags before it validates `Args`, so `--recipe` is readable at that point, but the table entry is not known when the tree is built.
  Declare `PersistentPreRunE` on the parent, short-circuiting when `cmd.Name() == "shed"` so a bare listing needs no git repository — this subtree's own equivalent of each module's existing group guard.
  Otherwise it reads cwd through `lyxcwd.CwdFrom(ctx)` and looks the `--recipe` value up through `lookup`.
  Before arming, refuse on the envelope when `cmd.Name()` is not in that entry's `Verbs` set, with a message naming the verb, the recipe, and the verbs that recipe does support — this is what keeps `lyx shed step --recipe lifecycle` from reaching a generic body lifecycle never armed, and it happens before any resolution so it costs nothing on the refusal path.
  Then it calls that entry's `Arm(cwd, cmd.Name(), args)` and assigns `*spec = armed`, rendering any error through `output.Err` followed by `clihelp.Abort(ctx, 1)` exactly as both modules' own pre-runs do.
  It must not resolve cwd into a `*lyxcwd.Location` itself: each module's `Arm` owns its own resolution, which is what keeps lifecycle's `fabricengine.PrimeName` lookup and non-prime refusal reachable through this path.
  Record this pre-run's own failures the way both existing modules' pre-runs already do: a bare `output.Err(out, err.Error())` followed by `clihelp.Abort(ctx, 1)`.
  `Abort` is what writes the exit code here — `internal/clihelp/exec.go`'s `RunRootCtx` returns `es.code`, which only `SetExit` and `Abort` ever write — and it additionally short-circuits the leaf body, which is exactly what a failed resolution needs.
  `SetExit` is the leaf bodies' mechanism, not the pre-run's;
  `internal/loomcli/cli.go` and `internal/lifecyclecli/cli.go` both use the `output.Err` + `Abort` pair and never `SetExit`.
  Declare `RunCLI(out io.Writer, args []string) int` and `RunCLIIn(cwd string, out io.Writer, args []string) int` with the same bodies both existing modules carry: `RunCLI` delegates to `RunCLIIn("", out, args)`, and `RunCLIIn` branches on an empty cwd to `clihelp.Execute` and otherwise to `clihelp.ExecuteIn`, because `lyxcwd.WithCwd` panics on an empty directory.
  `RunCLIIn` is carried rather than skipped because card 31's parity tests need an injectable cwd.
- **Commit:** `feat(shedcli): add the lyx shed subtree and its CLI seams`

### Card 31: prove envelope parity across the two paths

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/shedcli/table.go`
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/parity_test.go`
  - `internal/loomcli/cli.go`
  - `internal/lifecyclecli/cli.go`
  - `internal/loomcli/status_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/shedengine/status.go`
  - `internal/lifecyclecli/testmain_integration_test.go`
  - `internal/lifecyclecli/lifecycle_integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/table_test.go`
  - `internal/shedcli/cli_test.go`
  - `internal/shedcli/testmain_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `parity_test.go`, assert byte-identical envelopes from the same fixture across the two invocation paths for each verb: `lyx loom step` against `lyx shed step --recipe loom`, and likewise for `run`, `status` and `pause`, plus `lyx lifecycle run <slug>` against `lyx shed run --recipe lifecycle <slug>`.
  Drive both sides through `RunCLIIn` over one hub fixture so cwd is injectable and no test reads the process working directory, and compare the captured stdout byte-for-byte rather than comparing decoded maps, since key ordering and formatting are part of what "byte-identical" means to a supervisor parsing the line.
  `parity_test.go` carries `//go:build integration` as its first line, and every case in it is tier 2.
  That tier is forced, not chosen: `RunCLIIn` reaches each module's `PersistentPreRunE` and therefore `lyxcwd.Resolve`, which spawns git through `internal/gitexec`, and the Test Tier Purity Invariant bans `gitexec.Run` outside `integration`/`smoke`-tagged files;
  loom's `run` additionally calls `c.reed.Up()` and `fabricengine.Open`.
  `internal/loomcli/parity_test.go` is the precedent for the comparison *shape* but not for the tier — its own header states "no test here calls RunCLIIn -- so both stay tier 1", which is exactly why it stays untagged and this file cannot.
  Follow its three-way verdict discipline where a comparison can produce more than a binary pass.
  Build the hub through `internal/hubforge`'s fabric-fixture entry point per the hubforge Fabric-Fixture Invariant, and create `internal/shedcli/testmain_integration_test.go` carrying `//go:build integration` and a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, in the shape `internal/lifecyclecli/testmain_integration_test.go` already uses — the Hermetic Git Test Environment Invariant requires it for any package whose tests spawn git.
  `table_test.go` and `cli_test.go` stay untagged tier 1: neither drives `RunCLIIn`, and `cli_test.go`'s bare-listing and unknown-recipe cases run through `RunCLI` against a command tree whose pre-run short-circuits before any resolution.
  Pin which arm of each verb the fixture lands on, per verb, and never leave it to the implementer — every parity case must refuse or complete *above* the substrate, because `lyx loom run`'s pre-flight reaches `c.reed.Up()` and `step`'s `PreStep` reaches `ensureStatusStrand`, which calls `c.reed.Up()` too, and a real tmux session is what every existing loomcli test touching that substrate is `//go:build smoke` for.
  The arms to use: `run` against a hub with **no** loom status file, which refuses at the status-file existence check — the first statement in loom's `PreRun`, well above `reed.Up()`;
  `step` against a hub whose run lock is already held, which refuses at the early run-lock probe with `kind: busy`, above `seedAndCommitBootstrap` and above `ensureStatusStrand`;
  `status` and `pause`, which are read-only, lightweight-wired, and never reach the substrate on any path — drive those against a seeded status file so they exercise the success envelope rather than a refusal;
  and `lyx lifecycle run <slug>` against a slug whose persisted status is `StateDone`, which refuses inside lifecycle's `PreRun` before `BuildShed` is ever called.
  State the bound explicitly in the file's own header comment: **no parity case may reach reed, tmux, an LLM producer, or `shed.Run`/`shed.Step`'s own producer call** — every case compares the envelope the two paths produce at a decision that happens strictly above the engine.
  That bound is what keeps this suite in the `integration` tier rather than pushing it to `smoke`, and it is also why it proves what it needs to: the two paths' divergence risk lives entirely in arming and pre-run resolution, which every one of these arms exercises in full.
  Assert positional-argument parity explicitly: `lyx shed run --recipe lifecycle` with no slug and with two slugs must be refused byte-identically to `lyx lifecycle run` with the same argument counts, which holds because both sides validate through the same `cobra.ExactArgs(1)` value;
  `lyx shed run --recipe loom <extra>` must be refused by `cobra.NoArgs`.
  In `cli_test.go`, assert `lyx shed step --recipe lifecycle <slug>` is refused on the envelope naming the verb, the recipe and lifecycle's three supported verbs, and that the refusal happens without resolving cwd — it must pass in a directory that is not a git worktree at all, which is what proves the check runs ahead of arming.
  In `table_test.go`, assert every entry's `Verbs` set is a subset of the four verbs `shedverbs.Verbs` returns, and that `loom` carries all four while `lifecycle` carries three;
  assert the table's key set is exactly `loom` and `lifecycle`, that `lookup` on an unknown name returns an error naming the available recipes, and — by an AST scan over this package's production files in the style of `internal/shedverbs`'s own seam test — that no `init()` function is declared and no exported `Register`-shaped function exists, which is the table clause of the new Shed Verb-Set Invariant.
  In `cli_test.go`, assert a bare `lyx shed` lists the four subcommands and needs no git repository, that `lyx shed bogus` emits a JSON error envelope, that `--recipe` defaults to `loom`, and that an unknown `--recipe` value emits the unknown-recipe error envelope rather than an argument-count error.
  Add one further case to `parity_test.go`, in the integration tier alongside the rest: over a fixture whose loom module config is deliberately broken in a way that refuses `lyx shed run --recipe loom`, assert `lyx shed status --recipe loom` still succeeds.
  That is the proof `lyx shed status` reaches loom's lightweight wiring rather than the full `wire()` — the exact hazard `wireLightweight` exists to avoid, which a verb-blind arming would silently reintroduce on this path only.
- **Commit:** `test(shedcli): prove envelope and argument parity across both invocation paths`

### Card 32: register the subtree under the lyx root

- **Context:**
  - `internal/shedcli/cli.go`
  - `internal/loomcli/cli.go`
  - `internal/lifecyclecli/cli.go`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/drift_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/jsonhelp_test.go`
  - `cmd/lyx/registration_test.go`
- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add `shedcli.Command()` to `newRoot()`'s `root.AddCommand(...)` call and the `internal/shedcli` import alongside the others.
  Place it adjacent to `loomcli.Command()` and `lifecyclecli.Command()` so the grouping reads by subject rather than by insertion order.
  Add `shed` to the root `Long`'s "Available modules:" list, which currently ends `loom, start, quarry, lifecycle`.
  `shedverbs` is never registered directly and must not appear here: it exposes no `Command()` seam at all.
  Run `cmd/lyx`'s four guards and confirm each passes: `helptree_test.go` asserts the root help names every subtree, `drift_test.go` fails CI on any command with a blank `Short`, `longlist_test.go` asserts `root.Long` names every registered top-level module, and `jsonhelp_test.go` asserts the `--json` help schema for root, `board`, `config`, `ide`, `reed` and `selfreport`.
  A `longlist_test.go` failure here means `shed` is missing from the root `Long`'s "Available modules:" list, which this card adds — it holds no snapshot of any verb's prose and derives its module set from the live tree, so it can neither catch nor be broken by a reflowed `loom` or `lifecycle` `Long`.
  There is no expectation file to regenerate in any of the four.
- **Commit:** `feat(lyx): register the shed subtree under the root`

### Card 33: add the sandbox coverage exclusion

- **Context:**
  - `cmd/lyx/main.go`
  - `internal/shedcli/cli.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
  - `_mill/discussion.md`
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add the entry `"shed": "armed re-exposure of loom's and lifecycle's own verbs; covered by those two modules' own scenarios"` to `excludedModules` in `cmd/lyx/sandbox_coverage_test.go`.
  This is the disposition `_mill/discussion.md`'s Constraints section decided, and it uses the same reasoning the existing `"start"` entry already gives for loom's aliased bootstrap verb: a scenario of its own would exercise the identical code path twice.
  The alternative — a `**Covers:** shed` tag in a `tools/sandbox/*SUITE.md` file — is deliberately not taken, and no sandbox suite file is edited by this card.
  Confirm `TestSandboxCoverage_AllModulesCoveredOrExcluded` passes after the addition, which is what proves card 32's registration was seen by the guard.
- **Commit:** `test(lyx): exclude the shed module from sandbox coverage with a reason`

### Card 34: confirm both subtrees still agree end to end

- **Context:**
  - `internal/shedcli/parity_test.go`
  - `internal/shedcli/cli_test.go`
  - `internal/shedcli/table_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/helptree_test.go`
  - `internal/loomcli/parity_test.go`
  - `internal/lifecyclecli/run_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** run this batch's `verify:` command and confirm every package passes.
  Confirm specifically that `internal/loomcli` and `internal/lifecyclecli` still pass unchanged by this batch — this batch edits no file in either package, so any failure there means card 32's registration changed shared state it should not have.
  Confirm no parity case reached reed, tmux or a producer — the arms card 31 pins are what keep the suite in the `integration` tier, and a case that drifted into the substrate would hang or fail on a machine with no tmux rather than reporting cleanly.
  Confirm card 31's lightweight-wiring case passed — the one asserting `lyx shed status --recipe loom` still succeeds against a fixture whose loom module config is broken enough to refuse `lyx shed run --recipe loom`.
  A green run that skipped it is not evidence, since it is the only case proving the `shed` path did not silently reacquire the full `wire()`.
  This card changes no file.
- **Commit:** none

## Batch Tests

`verify:` runs `internal/shedcli` (the new parity, table and CLI suites), `cmd/lyx` (the four help-tree guards plus the sandbox coverage guard, all of which pick the new subtree up automatically once registered — confirmed rather than assumed, per card 32), and both `internal/loomcli` and `internal/lifecyclecli`, which this batch edits no file in but which the registration could regress through shared root state.

Parity is the proof the extraction preserved behaviour across the two invocation paths, and it is asserted on captured stdout bytes rather than on decoded maps, because a supervisor parses the line.

`verify:` chains `go test -tags integration ./internal/shedcli/...` because `parity_test.go` and `testmain_integration_test.go` are both tagged — without that chain the whole parity suite, which is this batch's entire proof, would never compile in-batch and the batch would pass green having run none of it.
