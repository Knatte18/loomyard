# Discussion: Unblock t.Parallel on hub-fixture tests that currently t.Chdir

```yaml
task: Unblock t.Parallel on hub-fixture tests that currently t.Chdir
slug: hubforge-parallel-chdir
parent: main
status: discussing
```

## Problem

`internal/hubforge` is the repo-wide real-hub fixture factory landed by `lyxtest-real-hubs`, and it is parallel-safe by construction — a `sync.Once` bare-repo template built once per test binary, copied fresh into each test's own `tb.TempDir()`.
What blocks the tests that use it from calling `t.Parallel()` is not hubforge: roughly twenty fixture-using test files call `t.Chdir`, and Go makes `t.Chdir` panic inside a parallel test.
They chdir because every lyx CLI resolves its geometry from the *process* working directory — each `<module>cli` calls `lyxcwd.Getwd()` inside a cobra hook or handler, so the only way a test can point `RunCLI` at a fixture hub is to move the whole process into it.

Measurement taken during this discussion says the immediate speed payoff is small, and the task should be understood as architectural rather than performance work.
On bare-metal Linux the seven chdir-heavy CLI packages sum to ~3.3 s of test time, and `go test` already runs those packages concurrently with each other, so intra-package parallelism recovers at most ~3 s of CPU and close to zero wall-clock.
`internal/fabricengine` — the 15.5 s package that actually dominates Tier 2 — is already 228/395 parallel and contains only two `.Chdir` occurrences, which are one chdir plus its restore inside a test whose entire premise is the unrelated cwd.

**Why now, and why do it anyway:** the durable defect is that a CLI resolving geometry from process-global state cannot be driven at two locations in one process, which is what makes the tests contort.
Fixing that removes a whole class of future contortion, and the payoff scales with environment — Tier 2 is 4.97 s on bare-metal Linux against 131.7 s on the Cortex-XDR Windows laptop (~26×), so the same serialized git work is plausibly 60–85 s there.
The task was explicitly deferred out of `lyxtest-real-hubs` so that task could land the factory and its 154-call-site migration without also touching CLI signatures;
touching those signatures is the substance of this task.

## Scope

**In:**

- A cwd-injection seam owned by `internal/lyxcwd`: `WithCwd(ctx, dir) context.Context` and `CwdFrom(ctx) (string, error)`, the latter falling back to `Getwd()` when the context carries no cwd.
- `clihelp.ExecuteIn(cmd, cwd, out, args) int` beside the existing `Execute`, seeding the cwd into the invocation context.
  This also requires reaching `clihelp.RunRoot`, which hardcodes `context.Background()` at `exec.go:150` and is the shared implementation behind both `Execute` and `cmd/lyx`.
  Adding `ExecuteIn` alone would not work.
- `clihelp.RunRootCtx(ctx context.Context, cmd *cobra.Command, out io.Writer) int` — **a sibling, not a new parameter on `RunRoot`.**
  `RunRoot` is retained as exactly `RunRootCtx(context.Background(), cmd, out)`.
  This is decided, not an open alternative: `cmd/lyx/main.go:42` and `:54` are the only two `clihelp.RunRoot` call sites in the repo, and adding a parameter would force both to change, contradicting this task's rationale that `cmd/lyx/main.go` is untouched.
  With the sibling, `main.go` genuinely needs no change, and `clihelp` gains one consistent idiom across all four seams: `Execute`/`ExecuteIn`, `RunRoot`/`RunRootCtx`, `WrapRun`/`WrapRunCtx`, `RunCLI`/`RunCLIIn`.
- `clihelp.WrapRunCtx(fn func(ctx context.Context, out io.Writer, args []string) int)` beside the existing `WrapRun`, because `WrapRun`'s handler signature is `(out, args)` and carries no context — see the `plain-handlers-take-the-context` decision.
- `RunCLIIn(cwd string, out io.Writer, args []string) int` on the 10 modules that both expose `RunCLI` and actually resolve a cwd (`fabriccli`, `burlercli`, `configcli`, `idecli`, `shuttlecli`, `scoutcli`, `perchcli`, `boardcli`, `webstercli`, `reedcli`).
  `internal/selfreportcli` is deliberately excluded: it references `lyxcwd` nowhere, so a `RunCLIIn` there would accept a cwd argument nothing reads.
  Eleven modules expose `RunCLI`; ten gain the sibling.
  Existing `RunCLI` delegates to it with `cwd == ""`, meaning "read the process cwd".
- Swapping every production `lyxcwd.Getwd()` call site in a CLI path over to `lyxcwd.CwdFrom(cmd.Context())` — 15 sites: 7 in a `PersistentPreRunE`, 4 in `scoutcli` `RunE` bodies, and 4 in plain handler functions.
  A 16th touch point, `fabriccli/weft_verbs.go:52`, calls no `Getwd()` of its own and instead needs `cmd.Context()` threaded into its `resolveWarpLocation()` call.
  `loomengine/preflight.go:36` is the 17th and last production site, handled by its own Scope bullet below.
  All are enumerated under Technical context.
- Threading `cmd.Context()` into the plain handler functions that resolve cwd today (`fabriccli.resolveWarpLocation`, `fabriccli.runCloneWithReset`, `configcli.runReconcile`, `configcli.runConfig`).
- Changing `loomengine.Preflight()` to `Preflight(cwd string)` and deleting its `Getwd()` call.
- A `--into <dir>` flag on `lyx fabric clone`, defaulting to the resolved cwd, because clone's cwd is a *destination* argument rather than a lookup.
- Threading the caller's resolved cwd through nested CLI invocations: `configcli.go:383`'s `fabriccli.RunCLI(w, []string{"sync"})` becomes `RunCLIIn`.
- Migrating every `.Chdir` call site in the **eight** `//go:build integration` target files that can be migrated — **39 occurrences** — onto the new seam.
  The ninth integration file, `fabricengine/coalesce_integration_test.go` (2 occurrences), is untouched: see Out.
- Adding `t.Parallel()` to the **three** integration files whose only blocker is chdir — **9 of those 39 occurrences** (`reedcli/cli_integration_test.go` 4, `loomengine/preflight_integration_test.go` 3, `perchcli/cli_integration_test.go` 2).
- A guard test at `cmd/lyx/cwdmutation_test.go` banning `t.Chdir(` and `os.Chdir(` in a **named per-file subject set** — the eight migrated files plus `coalesce_integration_test.go` as the one allowlisted exemption, never their whole packages.
- Doc updates in the same commits: `CONSTRAINTS.md` (CLI/Cobra Invariant, including its corrected "Enforced by" line), `docs/overview.md:253` (which states the seam verbatim as `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)` and becomes incomplete once `RunCLIIn`/`ExecuteIn` exist), `internal/lyxcwd` and `internal/clihelp` package docs, affected module docs under `manifest/designs/`, a `LYX_TRACE=1` note in `docs/benchmarks/running-tests.md`, and a timing row in `docs/benchmarks/test-suite-timing.md`.

**Out:**

- **The twelve `//go:build smoke` files** (33 `.Chdir` occurrences across `reedcli`, `shuttlecli`, `burlerengine`, `treadleengine`).
  Note this is one file and three occurrences more than the task brief's list, which named eleven: the brief omits `reedcli/smoke_test.go`, the shared smoke helper carrying `mustChdir` (`:790`) and `deferHubRelease`'s two `os.Chdir` calls.
  They are in neither measured tier — `docs/benchmarks/running-tests.md:29-30` documents `-tags smoke` as requiring a live logged-in `claude` session — so parallelizing them moves no measured number, and they carry three further blockers of their own (see Deferred follow-up).
- **The `WEFT_SKIP_GIT` / `WEFT_SKIP_PUSH` / `BOARD_SKIP_GIT` config seam.**
  `t.Setenv` panics under `t.Parallel()` exactly as `t.Chdir` does, and it lands on four of the nine integration target files.
  Those files get their chdir removed but stay serial.
- **`internal/fabricengine/coalesce_integration_test.go`.**
  Its only chdir belongs to `TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd`, whose premise *is* the unrelated process cwd (it pins the `headOrEmpty("")` fix against `gitrepo.New("")` resolving `""` through `filepath.Abs`).
  Removing it would delete the test's meaning.
  Nothing to migrate; the file is untouched.
- Any change to `internal/hubforge` itself. It is already documented, machine-tested (`TestNewHub_Concurrent`, `BenchmarkNewHubParallel`) and pinned by `CONSTRAINTS.md:79` as safe under concurrent use.
- A user-facing global `--cwd`/`-C` flag.
- `internal/logger`'s durable-sink `sync.Once` — no code change, but with a stated disposition rather than a dismissal.
  `sink.go:79` short-circuits under `testing.Testing()` unless `LYX_TRACE=1`;
  no test sets that variable today, but `CONSTRAINTS.md`'s Live-Substrate Spawn Observability documents `LYX_TRACE=1` as the supported under-`go test` gate, so "unused" is not by itself a disposition.
  The disposition is this: `sinkOnce` is process-wide, so the **first** call to log in a test binary pins the trace directory for every subsequent call in that process.
  Per-test, hub-accurate trace directories were therefore never actually delivered — under `LYX_TRACE=1` today the sink already resolves against whichever fixture hub happened to log first, which is arbitrary.
  After this migration it resolves against the repo worktree instead, deterministically.
  That is a change from one arbitrary answer to one predictable answer, and it is recorded in `docs/benchmarks/running-tests.md` beside the existing `LYX_TRACE=1` documentation.
  Not attempted: having fixtures call `logger.SetDurableSinkDir` per test — that function reassigns `sinkOnce` and `headerOnce` wholesale (`sink.go:196-208`), which is itself a data race against any concurrent logger call, so it would have to be made safe before it could be used that way.
- The seventeen further `.Chdir`-using test files outside the twenty named in the task brief.
- `manifest/roadmap.md` — this is hardening, not a planned-item completion.

## Decisions

### seam-mechanism-is-context-carried-cwd

- Decision: explicit cwd travels as a context value owned by `internal/lyxcwd` — `lyxcwd.WithCwd(ctx, dir)` to seed, `lyxcwd.CwdFrom(ctx) (string, error)` to read, falling back to `Getwd()` when unset.
  `clihelp.ExecuteIn` seeds it; each module's `RunCLIIn` calls `ExecuteIn`.
- Rationale: keeps `lyxcwd` the sole owner of cwd resolution, as the Cwd Resolution Invariant requires, and keeps `Getwd` the single raw `os.Getwd` site.
  `context` is stdlib, so `lyxcwd`'s import cap (stdlib plus `internal/gitexec`) is unaffected and the `fabricengine → logger → lyxcwd` acyclicity argument is untouched.
  Cwd becomes a *per-call* value, which two test files need because they drive the CLI at two different locations within one test.
  No user-visible CLI surface is added.
- Rejected: a `--cwd`/`-C` persistent flag — real operator value and git parity, but it adds user-visible surface needing `Short`/`Long`, help-tree test changes, docs, and a ruling on the invariant's "geometry is structural, never config/env-overridable" clause, none of which this task needs.
  Also rejected: a package-level settable cwd, which is not parallel-safe and would defeat the purpose.

### the-injected-cwd-contract

- **Value contract.** The injected cwd MUST be absolute.
  `lyxcwd.Resolve` hands it to `gitexec.RunGit` as `cmd.Dir` and then gates on an `EvalSymlinks`-normalised absolute comparison (`anchor.go:103-140`), so a relative value would itself resolve against the process cwd and would most likely trip `ErrCwdOutsideAnchor` — a confusing failure far from its cause.
  `WithCwd(ctx, "")` is not legal;
  the empty string is a sentinel **only** in `RunCLIIn(cwd, …)`, where it means "seed nothing, let `CwdFrom` fall back to `Getwd()`", which is exactly how `RunCLI` delegates.
  `CwdFrom` therefore never returns a relative path.
- **Enforcement mechanism (decided — a precondition with no detection is exactly the confusing-failure-far-from-its-cause this section argues against).** `WithCwd(ctx, dir)` **panics** when `dir` is empty or not absolute (`!filepath.IsAbs(dir)`).
  It does not silently normalise via `filepath.Abs`, and it does not defer the complaint to `CwdFrom`.
- Rationale: `WithCwd` returns only a `context.Context` and has nowhere to put an error without making every seeding site handle one.
  Seeding a relative cwd is a programmer error at a call site the programmer controls, never user input — the CLI's own cwd comes from `Getwd()`, which is always absolute — so failing loudly at the seeding site puts the diagnostic at the cause.
  Silent `filepath.Abs` normalisation was rejected precisely because it would resolve against the process cwd, reintroducing the dependency this task removes, and would do so invisibly.
  Returning an error from `CwdFrom` was rejected because it reports the fault at the reading site, arbitrarily far from the seeding site that caused it.
- **Governance contract — what the injected cwd does and does not control.** It governs **geometry resolution**: every `lyxcwd.Resolve`/`ResolveWorktree` input, and anything derived from the resulting `Location`.
  It does **not** automatically govern how a command resolves a *relative path supplied as a flag or positional argument*;
  those resolve against whatever base the individual handler chooses, which is the process cwd unless the handler is changed.
- **Consequence, and why it is not left implicit.** Two verbs take a relative path as an argument and must be brought onto the seam cwd explicitly, or `RunCLIIn` would honour the injection for lookup while silently ignoring it for the argument:
  - `fabric clone --into <dir>` — covered by the `clone-gets-an-explicit-destination` decision.
  - `scoutcli`'s relative **value-entry points** — see the decision immediately below, and note the enumeration unit.
- **Enumerate by value-entry point, not by `filepath.Abs` occurrence.** An earlier draft listed four `filepath.Abs` call sites inside `scoutcli` (`cli.go:446`, `:695`, `:784`, `:800`) and treated rebasing those as sufficient.
  That is wrong, and `cli.go:446` is only the out-of-hub fallback branch rather than the main path.
  The raw `--target-dir` value leaves the package unresolved: `cli.go:142-145` computes `dir := targetDir` (defaulting to `cwd` when empty), passes it to `lookupContext(cwd, dir)` at `:147` and `buildOptions(registry, dir, …)` at `:173`, from where it becomes `scoutengine.Options.TargetDir` (`refs.go:50`) and is finally absolutised by `rootURIFor`'s `filepath.Abs(targetDir)` at `scoutengine/ensureserver.go:120` — reached from `:182` and `:308` — plus a `DetectLanguage(opts.TargetDir, …)` tree read.
  Both resolve against the **process** cwd, outside `scoutcli` entirely.
- Decision: rebase at the flag's **defaulting point** — make `dir` absolute against the seam cwd before `lookupContext`/`buildOptions` — so every downstream consumer inside *and* outside `scoutcli` inherits a correct absolute value with no further change.
  **There are four such defaulting points, one per `RunE`, matching the four enumerated `Getwd()` sites** (`:136`, `:266`, `:371`, `:563`): the `dir := targetDir` / `if dir == "" { dir = cwd }` block recurs at `cli.go:142-145`, `:272-274`, `:377-379`, and `:569-571`.
  All four are rebased.
  A missed one does not fail to build — it silently returns an answer resolved against the wrong directory — so the named `--target-dir` scenario must cover more than one subcommand.
  A relative value becomes `filepath.Join(seamCwd, v)`;
  an absolute value is used as given.
- **`parseQuery` and `inFileQuery` take an explicit base parameter.** `parseQuery(arg string)` (`cli.go:774`) and `inFileQuery(inFilePath, name string)` (`:794`) are package-level functions with no base today, reached from six call sites through the `buildQuery` closure.
  They become `parseQuery(base, arg string)` and `inFileQuery(base, inFilePath, name string)`, with `base` the seam cwd.
  This is the same gap that forced `WrapRunCtx` — semantics without a signature — and it gets the same treatment.
- Rationale for a parameter over a closure: an explicit base is directly testable and matches the `Preflight(cwd)` decision, where the callee is *told* its geometry rather than deriving it.
  A closure capturing the seam cwd would work but hides the dependency at exactly the sites a reader needs to see it.
- **`internal/scoutcli/cli_test.go` is therefore touched by commit 2, contrary to the "not touched" list below.** `TestInFileQuery_ResolvesRelativePathToAbsolute` (`:622-635`) calls `t.Chdir(cwd)` and asserts `filepath.Join(cwd, "relative/bar.go")`, pinning the current process-cwd behaviour;
  it must move to passing an explicit base, or commit 2 does not compile.
  The file is untagged (Tier 1), so this also keeps it inside the Test Tier Purity Invariant — passing a base is cheaper than the chdir it replaces.
  It does **not** join the guard's subject set: its remaining chdirs are unrelated to hub fixtures and are deferred with the rest.
- Rationale: a seam that is honoured for geometry but ignored for arguments is worse than no seam, because it returns a confidently wrong answer rather than an error.
  Rebasing once at entry is also strictly safer than rebasing at each `filepath.Abs`, because it cannot miss a consumer in another package — which is exactly the class of miss the earlier draft made.
  `Reference.File` comparisons in `filterWithin` require an absolute path, and joining onto the seam cwd still produces one, so the invariant its comment protects is preserved.
- Rejected: documenting the limitation and leaving `scoutcli`'s bases on the process cwd (ships a known trap);
  rebasing at the `filepath.Abs` sites (misses `scoutengine`);
  and omitting `scoutcli` from the seam entirely as `selfreportcli` is omitted (it genuinely resolves cwd at four sites, so it belongs).

### cwdfrom-owns-the-fallback

- Decision: `CwdFrom(ctx)` returns `(string, error)`, internally falling back to `Getwd()` when the context carries no cwd.
- Rationale: every one of the 15 CLI-path production sites becomes a one-line swap from `lyxcwd.Getwd()` to `lyxcwd.CwdFrom(cmd.Context())`, and the fallback exists in exactly one place.
- Rejected: `CwdFrom(ctx) (string, bool)` with per-caller fallback — more explicit at the call site, but duplicates the fallback twelve times and invites one site getting it wrong.

### runcli-gains-a-sibling-rather-than-changing

- Decision: add `RunCLIIn(cwd, out, args)` alongside the existing `RunCLI(out, args)` on the 10 cwd-resolving modules;
  `RunCLI` delegates with `cwd == ""`.
  Amend the CLI/Cobra Invariant to name both shapes in the same commit, **and add the signature-pinning test the invariant currently claims but does not have** (see below).
- Rationale: `RunCLI` stays the production seam and `cmd/lyx/main.go` needs no change.
- **Correction — the seam is not machine-enforced today.** An earlier draft of this document claimed `cmd/lyx/drift_test.go` would gain a second pinned shape.
  That is false: `drift_test.go` asserts only that every cobra command carries a non-empty `Short` (`TestDriftGuard_AllCommandsHaveShort`), and no test under `cmd/lyx` references `RunCLI` at all — the only mentions are comments in `main_test.go:61` and `main_integration_test.go:188`.
  The CLI/Cobra Invariant's "Seam" clause is therefore a review obligation wearing an "Enforced by" label, and `CONSTRAINTS.md`'s enforcement list overstates its coverage on this specific point.
- Decision (follow-on): commit 1 adds a compile-time signature-pinning assertion covering both seam shapes across the 10 modules — the cheapest form is a `var _ = []func(io.Writer, []string) int{...}` / `[]func(string, io.Writer, []string) int{...}` pair in a `cmd/lyx` test file, which fails to compile if any module's signature drifts.
  `CONSTRAINTS.md`'s "Enforced by" line is corrected in the same commit to name it.
- Rationale: the invariant is being amended anyway, and closing a hole in a machine-checked rule while standing in it is cheaper than filing it.
  A compile-time assertion costs nothing at runtime and cannot rot silently.
- Rejected: replacing `RunCLI` outright — one seam and no dual API, but a wide mechanical change to production wiring driven purely by a test need.

### plain-handlers-take-the-context

- Decision: the handler functions that resolve cwd outside a cobra hook (`fabriccli.resolveWarpLocation` at `fabric.go:398`, `fabriccli.runCloneWithReset` at `clone.go:119`, `configcli.runReconcile` at `configcli.go:257`, `configcli.runConfig` at `configcli.go:370`) take `cmd.Context()` as a parameter.
- **Mechanism (corrected — an earlier draft got this wrong).** `clihelp.WrapRun` has signature `WrapRun(fn func(out io.Writer, args []string) int)` (`exec.go:123`): the wrapper closes over `cmd`, but the wrapped handler receives only `(out, args)` and therefore has no context today.
  Every registration site passes an `(out, args)` closure — `configcli.go:325,343` and ten sites in `fabriccli/fabric.go` (`:125,159,171,196,227,243,264,303,346,372`).
  So "pass `cmd.Context()` through" is not mechanical at the registration site;
  it needs a seam of its own.
- Decision: add a context-aware sibling `clihelp.WrapRunCtx(fn func(ctx context.Context, out io.Writer, args []string) int)` beside `WrapRun`, and migrate the affected registration sites onto it.
  `WrapRun` stays for handlers that need no cwd.
- Rationale: additive and shaped exactly like the `RunCLI`/`RunCLIIn` decision already taken, so the codebase gains one consistent "…Ctx/…In sibling" idiom rather than two different escape hatches.
  Converting those `RunE`s to raw `func(*cobra.Command, []string) error` was rejected: it would rewrite twelve registration sites into a different shape from every other command in the tree, for no gain over the sibling.
  `fabriccli` legitimately has two seams — a plain helper for the 8 topology verbs and a scoped `PersistentPreRunE` for the weft verbs (`weft_verbs.go:52`) — and this decision does not force them to converge.
- Rejected: converting the plain handlers to `PersistentPreRunE` for a uniform seam shape — a cleaner end state, but a larger refactor that changes error-path ordering in packages this task has no other reason to disturb.

### nested-cli-calls-thread-the-callers-cwd

- Decision: where one module's CLI invokes another's, the caller passes its already-resolved cwd.
  The production closure `realSync` at `configcli.go:382-384` calls `fabriccli.RunCLI(w, []string{"sync"})` at `:383` and becomes `fabriccli.RunCLIIn(cwd, w, []string{"sync"})`.
  The test's `injectedSync` closure at `configcli_integration_test.go:72-74` calls `fabriccli.RunCLI(w, []string{"commit"})` at `:73` — note the different verb, deliberate per the comment at `:70-71` ("sync calls a detached spawnPush that cannot run in-process, so we use commit") — and follows the same change.
- Rationale: `configcli` already resolves and holds a cwd;
  letting the nested call re-derive it from process state is precisely the bug being removed, and it is the reason `configcli_integration_test.go:55` chdirs even though `dispatch` is already given an explicit layout.
- Rejected: leaving nested calls on `RunCLI` — the outer test would still need a process chdir, so the file could not migrate at all.

### preflight-is-told-its-cwd

- Decision: `loomengine.Preflight()` becomes `Preflight(cwd string)`;
  the `lyxcwd.Getwd()` call at `preflight.go:36` is deleted.
- Rationale: there is no production caller — grep finds only doc references and `export_test.go`, and there is no `loomcli` module yet (loom is still Planned on the roadmap) — so the signature change is free today and only gets more expensive later.
  It also matches the Treadle Runner-Seam precedent, where the engine is *told* its geometry and never derives it.
- Rejected: adding `PreflightAt(cwd)` beside `Preflight()` — non-breaking, but it preserves a process-cwd read that has no caller to justify it.

### clone-gets-an-explicit-destination

- Decision: add `--into <dir>` to `lyx fabric clone`, defaulting to the resolved cwd.
- **Relative-path base (must be stated, or parallel tests hit exactly this):** a relative `--into` resolves against the **seam cwd** — the value returned by `lyxcwd.CwdFrom(cmd.Context())` — never against the process cwd.
  Under `RunCLIIn` those two differ by construction, which is the whole point;
  resolving against the process cwd would reintroduce the process-global dependency this task removes, in the one verb where cwd is a destination rather than a lookup.
  Absolute `--into` values are used as given.
  `runCloneWithReset` passes the result straight into `CloneAndWire(dest, …)` (`clone.go:119,133`), so the resolution must happen before that call, not inside `CloneAndWire`.
- Rationale: at `clone.go:119` the cwd is where the hub is *created* (`CloneAndWire(cwd, …)`), not a lookup.
  A resolution-only seam would not cover the five clone tests, and leaving cwd to mean "destination here, lookup everywhere else" is an unmarked trap.
  The flag sits naturally beside the existing `--reset`, `--subpath`, and `--force-bootstrap`.
- Rejected: a third positional after an already-optional second one (a usage trap);
  and routing clone through the same context-carried cwd (smallest change, but leaves the trap unmarked).

### parallel-is-written-where-it-applies

- Decision: `t.Parallel()` is written explicitly in each migrated test function in the **three** files whose only blocker is chdir — `reedcli/cli_integration_test.go`, `loomengine/preflight_integration_test.go`, `perchcli/cli_integration_test.go`.
  Five further integration files get their chdir removed but stay serial, each carrying a comment naming its remaining blocker: the four `t.Setenv` files, plus `idecli/cli_test.go`.
- **`idecli/cli_test.go` is not free, contrary to an earlier draft.** `TestRunCLISpawnDispatch` swaps the package-level `ideengine.CodeLauncher` (`ideengine/spawn.go:17`, an exported injectable seam) at `cli_test.go:27-29` and restores it in a `defer`.
  Under `t.Parallel()` that is both a data race on a production package-level var and a restore that fires while sibling tests are still running.
  Disposition: migrate its chdir, leave it serial, comment the seam swap as the blocker.
  Converting `CodeLauncher` to per-invocation injection would fix it properly but widens scope into `ideengine`'s public API for one test — deferred with the rest.
- **Safety audit of the three parallelized files.** Verified directly, not assumed: none of `reedcli/cli_integration_test.go`, `loomengine/preflight_integration_test.go`, or `perchcli/cli_integration_test.go` assigns to any cross-package exported variable, and none calls `t.Setenv` or `os.Setenv`.
  Their only process-global mutation is the chdir this task removes.
  The audit method is a per-file sweep for cross-package var assignment and env mutation, and it must be re-run for any file later added to the parallelized set — a package-level safety claim is not sufficient, because the blocker can live in a *production* package the test stubs.
- Rationale: parallelism stays visible at the test it governs, and the deferred work is documented where the next person will look for it.
- Rejected: calling `t.Parallel()` inside the shared fixture helpers (`seedPerchFixture` and friends) — fewer lines, but it hides a control-flow-changing call inside a setup function.

### guard-bans-both-chdir-spellings

- Decision: a guard test at `cmd/lyx/cwdmutation_test.go` bans both `t.Chdir(` and `os.Chdir(` across an explicitly named **per-file subject set**, not per-package.
- **Subject set (the guard's allowlist-of-what-is-guarded, not of what is excused):** the eight migrated files — `fabriccli/cli_test.go`, `perchcli/run_integration_test.go`, `perchcli/cli_integration_test.go`, `configcli/configcli_integration_test.go`, `webstercli/verbs_test.go`, `idecli/cli_test.go`, `reedcli/cli_integration_test.go`, `loomengine/preflight_integration_test.go` — plus `fabricengine/coalesce_integration_test.go`, which is guarded with a single allowlisted exemption.
- **Why per-file and not per-package:** the eight packages that gain a seam change carry roughly fourteen further chdir-using test files this task deliberately does not touch (`boardcli/cli_test.go` and `cli_unit_test.go`, `burlercli/cli_test.go`, `shuttlecli/cli_test.go`, `scoutcli/cli_test.go`, `perchcli/cli_test.go` and `run_test.go`, `webstercli/cli_test.go`, `reedcli/cli_test.go`, `configcli/reconcile_test.go` and `reconcile_integration_test.go`), plus the twelve deferred smoke files in those same packages.
  `scoutcli/cli_test.go` is a partial exception: commit 2 edits one test in it (`TestInFileQuery_ResolvesRelativePathToAbsolute`) because `inFileQuery` gains a base parameter, but the file does not join the guard's subject set and its chdirs are otherwise untouched.
  A per-package subject would make the allowlist larger than the guarded set, which inverts the point of a guard.
- **The one allowlist entry:** `fabricengine/coalesce_integration_test.go`, reason `"cwd is the assertion: TestCoalescePushBothAt_EmptyWarpPath_PushesWeftFromUnrelatedCwd pins gitrepo.New(\"\") against a non-git process cwd"`.
- **Growth rule:** a file joins the subject set when it is migrated, never by default. The deferred files are outside the guard entirely and carry no allowlist entry, so the guard stays silent about work this task chose not to do.
- Rationale: every package-walking guard in this repo lives in `cmd/lyx` (`tierpurity_test.go`, `hermeticenv_test.go`, `destructiveguard_test.go`, `ghguard_test.go`, `gitrepoboundary_test.go`), so placement follows the house pattern.
  Banning `os.Chdir` as well is what catches the wrapper pattern already present in three of these files (`restoreCwd`, `mustChdir`) which a `t.Chdir`-only ban would miss.
- Rejected: banning `t.Chdir(` only (blind to the wrappers);
  and placing it in `internal/lyxcwd/enforcement_test.go` — defensible on ownership, but `CONSTRAINTS.md:290-292` already flags that file's placement as a convenience rather than an ownership claim, and this guard walks other packages.

### export-test-shim-stays-with-a-corrected-rationale

- Decision: keep `loomengine.CheckResolvedForTest`, but rewrite `export_test.go`'s doc comment, which becomes false once `Preflight` takes a cwd.
- Rationale: the shim still serves tests that build a synthetic `*lyxcwd.Location` with no backing directory on disk.
  Its stated justification — "Preflight(), whose own lyxcwd.Getwd() dependency makes it unusable against an arbitrary Location" — is exactly what this task removes, so leaving the comment would be a live contradiction.
- Rejected: collapsing the shim and migrating all its users to `Preflight(cwd)` — tests more of the real path, but a 45-test refactor in a package this task otherwise touches in one function.

### scope-is-the-integration-tier-only

- Decision: build the seam across the 10 cwd-resolving modules, migrate every integration-tier call site, and add `t.Parallel()` only where it is free.
  Defer the env seam and the whole smoke tier.
- Rationale: lands the durable architectural win with no speculative work, and stays honest about a small immediate payoff.
  The smoke tier contributes to no measured tier and carries three independent further blockers.
- Rejected: also building the `WEFT_SKIP_*` seam (unlocks 26 more chdirs, but drags a production env-var seam into a test-parallelism task);
  migrating smoke call sites without adding `t.Parallel()` (consistency, but touches 11 files for no measurable gain);
  and renegotiating the task away entirely (the seam has value independent of the timing numbers).

### three-commits-each-self-contained

- Decision: three commits, each building and testing green on its own, each carrying its own doc updates:
  1. `lyxcwd` context API + the three `clihelp` siblings (`ExecuteIn`, `RunRootCtx`, `WrapRunCtx`) + the seam signature-pinning assertion + `CONSTRAINTS.md` CLI/Cobra amendment (wording and corrected "Enforced by") + `docs/overview.md:253` + package docs.
     `cmd/lyx/main.go` is not touched in this commit or any other.
  2. Module seams — the 10 `RunCLIIn` functions, plain-handler context threading onto `WrapRunCtx`, `Preflight(cwd)` **together with its two call sites and the `export_test.go` comment rewrite**, clone `--into`, and `scoutcli`'s entry-point rebase — plus module docs, `docs/overview.md:253`, and help text.
  3. Test migration (chdir removal), `t.Parallel()`, the guard test, and the timing row.
- **Commit 2 carries `Preflight`'s two call-site updates, or it does not compile.** `Preflight()` → `Preflight(cwd string)` is the one *breaking* signature change in this task (`RunCLIIn`, `--into`, and the three `clihelp` siblings are all additive).
  Its only two callers are `preflight_integration_test.go:192` and `:229`, so if those moved to commit 3, commit 2 would fail to build under `-tags integration` and the "each commit green on its own" property would be false.
  Commit 2 therefore includes both call-site updates and the `export_test.go` comment rewrite, leaving commit 3 with chdir removal and `t.Parallel()` only.
- Rationale: CLAUDE.md requires docs in the same commit as the change that causes them, which rules out a trailing docs-only commit.
  Slicing this way isolates the risky half (2) from the mechanical half (3).
- Rejected: one commit (~40 files and two invariant amendments in a single reviewable unit);
  per-module commits (~11, of which the last 10 are near-identical, and the seam is only useful once all exist).

## Technical context

### The three populations of target file

The task brief names twenty files. They do not behave alike:

| population | files | `.Chdir` | blockers | measured tier? |
|---|---|---|---|---|
| integration, chdir-only | 3 | 9 | chdir only | Tier 2 |
| integration, global-stub swap | 1 | 4 | chdir + `ideengine.CodeLauncher` | Tier 2 |
| integration, `t.Setenv` too | 4 | 26 | chdir + `WEFT_SKIP_*` | Tier 2 |
| integration, cwd-is-the-subject | 1 | 2 | unremovable by design | Tier 2 |
| smoke | 12 | 33 | chdir + `deferHubRelease` + tmux races | neither |

Per-file counts, measured on this branch:

| file | tag | `.Chdir` | `t.Setenv` |
|---|---|---|---|
| `internal/fabriccli/cli_test.go` | integration | 18 | 2 |
| `internal/perchcli/run_integration_test.go` | integration | 4 | 4 |
| `internal/configcli/configcli_integration_test.go` | integration | 3 | 2 |
| `internal/webstercli/verbs_test.go` | integration | 1 | 5 |
| `internal/idecli/cli_test.go` | integration | 4 | 0 |
| `internal/reedcli/cli_integration_test.go` | integration | 4 | 0 |
| `internal/loomengine/preflight_integration_test.go` | integration | 3 | 0 |
| `internal/perchcli/cli_integration_test.go` | integration | 2 | 0 |
| `internal/fabricengine/coalesce_integration_test.go` | integration | 2 | 0 |

The three files that gain `t.Parallel()` are `reedcli/cli_integration_test.go`, `loomengine/preflight_integration_test.go`, and `perchcli/cli_integration_test.go`.
`idecli/cli_test.go` is migrated but stays serial — its `ideengine.CodeLauncher` swap is a second blocker (see `parallel-is-written-where-it-applies`).

### Production `lyxcwd.Getwd()` call sites to migrate

**Enumeration method — read this before sizing the work.** The list below counts `lyxcwd.Getwd()` *occurrences*, which is the right inventory for the one-line `CwdFrom` swap but **understates the touch-point surface**.
The real surface is every function on the path from a cobra `RunE` to a cwd resolution, because each of those must carry a context it does not carry today.
`fabriccli` is the sharp case: `resolveWarpLocation()` holds a single `Getwd()` call but has **10 callers** — `fabric.go:427,459,485,531,563,666,691,715`, `unwire.go:18`, and `weft_verbs.go:76` — every one a `WrapRun`-wrapped handler with no context.
So `fabriccli` is one `Getwd()` site but eleven functions that change signature.
mill-plan must size per-module work from the transitive path, not from the `Getwd()` count.

Cobra `PersistentPreRunE`, calling `lyxcwd.Getwd()` directly (resolve, then build the module's engine/config) — 7 sites:
`internal/reedcli/cli.go:56`, `internal/shuttlecli/cli.go:58`, `internal/perchcli/cli.go:77`, `internal/webstercli/cli.go:123`, `internal/idecli/cli.go:37`, `internal/boardcli/cli.go:71`, `internal/burlercli/cli.go:59`.

Per-command `RunE` bodies — 4 sites: `internal/scoutcli/cli.go:136`, `:266`, `:371`, `:563`.

Plain handler functions (no `cmd` in scope today) — 4 sites:
`internal/fabriccli/fabric.go:398` (`resolveWarpLocation`, serving the 8 topology verbs), `internal/fabriccli/clone.go:119` (`runCloneWithReset`), `internal/configcli/configcli.go:257` (`runReconcile`), `internal/configcli/configcli.go:370` (`runConfig`).

Context-threading only, no `Getwd()` of its own — 1 site: `internal/fabriccli/weft_verbs.go:52`, a `PersistentPreRunE` scoped to `weftVerbNames` that reaches cwd indirectly by calling `resolveWarpLocation()` at `:76`.
Its migration action is threading `cmd.Context()` into that call, independent of the `Getwd()` swap inside `resolveWarpLocation` itself — so it is a second, real touch point in `fabriccli`, not a double-count of `fabric.go:398`.

Engine-level, no CLI — 1 site: `internal/loomengine/preflight.go:36`.

Not migrated: `internal/logger/sink.go:88`, inert under `go test` per `sink.go:79`.

### Per-call-site notes that will bite during migration

- **`fabriccli/cli_test.go:659→680` and `:716→738`** chdir twice within one test, into a hub and then into a second worktree inside it.
  These need cwd as a per-call value, which `RunCLIIn` provides naturally — do not hoist the cwd to a per-test variable.
- **`fabriccli/cli_test.go:801`** chdirs into `h.PrimeWeft()`, where the cwd's *classification* is the assertion (the weft-sibling refusal path through `fabricengine.RequireWarpWorktree`).
  Pass `PrimeWeft()` as the cwd argument;
  the assertion is preserved, not weakened.
- **`fabriccli/cli_test.go:89` and `:404`** chdir into a bare `t.TempDir()` to exercise error paths (`unknown` subcommand, and clone's usage error).
  At `:89` the `PersistentPreRunE` guard returns before any resolution, so the chdir is already dead weight.
- **`fabriccli/cli_test.go:493,579,609,659,716`** are the chdirs where cwd is a clone *destination*. These move onto `--into`.
  Note `:659` and `:716` appear in the previous bullet too, and that is not a contradiction: those two tests use **both** seams.
  `:659` chdirs into `cloneParent` for a `clone` (destination → `--into`), then `:680` chdirs into `filepath.Join(hubPath, "reconcilecli-warp")` for a `reconcile` (lookup → `RunCLIIn`);
  `:716`/`:738` is the same pairing.
  Each test therefore needs one `--into` argument and one per-call `RunCLIIn` cwd, not a choice between them.
- **`perchcli/cli_integration_test.go:35`** asserts `os.Stat(filepath.Join("..","..","escaped"))`, deliberately relative to the chdir'd hub, proving `--run-id ../../escaped` did not escape the runs area.
  Rewrite against an absolute path derived from the hub (`filepath.Join(h.PrimeWorktree(), "..", "..", "escaped")`) so the assertion survives the chdir's removal.
- **`perchcli/cli_integration_test.go:96` and `run_integration_test.go:157`** chdir into `h.Location.AnchorPath()`, a non-`"."` anchor — pass that path, not `PrimeWorktree()`.
- **`idecli/cli_test.go:95`** uses bare `os.Chdir` with a manual restore at `:98`, into a non-git tempdir for the error path.
- **`configcli_integration_test.go:55`** is not a `configcli` cwd dependence at all: `dispatch` is already given an explicit layout at `:78`, and the cwd is consumed inside the `injectedSync` closure at `:72-74`, whose `fabriccli.RunCLI(w, []string{"commit"})` call sits at `:73`.
  The fix reaches `fabriccli`, not `configcli`.
- **`loomengine/preflight_integration_test.go:192` and `:229`** — the two `loomengine.Preflight()` call sites, whose `os.Chdir` calls sit at `:188` and `:225` respectively — exercise the public `Preflight()` specifically because they need it to observe a particular cwd.
  Cite `:192`/`:229` for the calls throughout; the earlier `:188`/`:225` pair names the chdirs, not the calls.
  Under `Preflight(cwd string)` they pass the directory directly and parallelize;
  the `restoreCwd` helper at `:106` becomes dead and should be deleted.
- **`shuttlecli/smoke_interrupt_test.go:264`** calls `lyxcwd.Getwd()` in the test body, hand-rebuilding what the `PersistentPreRunE` does. Out of scope (smoke tier) but worth knowing it exists.

### What is already safe, and needs no work

**Scope limitation, stated honestly:** this appendix covers *infrastructure* packages and per-invocation closure locals.
It does **not** by itself clear any given test file for `t.Parallel()`, because a test can stub a package-level var in a *production* package the appendix never looks at — which is exactly how `idecli/cli_test.go`'s `ideengine.CodeLauncher` swap was initially missed.
Per-file clearance comes from the audit described under `parallel-is-written-where-it-applies`, not from this list.

- **`internal/hubforge`** — `bareTemplateOnce` writes its two template paths only inside `Once.Do`, then they are read-only;
  `copyBares` writes only into `tb.TempDir()`;
  teardown walks only its own `hubPath` and logs rather than fails.
  `TestNewHub_Concurrent` (`hub_test.go:340`) already fires 8 concurrent `NewHub` calls, and `BenchmarkNewHubParallel` runs it under `b.RunParallel`.
  One known wart, not a race: the `os.MkdirTemp("", "hubforge-bare-*")` template at `hub.go:56` is never removed, leaking one temp tree per test binary.
- **`internal/configreg`** — `Modules()` returns a freshly built slice each call;
  no mutable package state.
- **`Command()` in every module** builds a fresh cobra tree per call, and closure-locals like `idecli`'s `l *lyxcwd.Location` (`cli.go:24`) are per-invocation, not package-level.
- **`gitkit.HermeticGitEnv`** mutates global env but is called from `TestMain` before any test runs. It must stay in `TestMain` and never move into a test body.
- **`internal/state`, `internal/lock`, `internal/proc`, `internal/fslink`, `internal/output`** carry no mutable package-level vars.

### Baseline measurements taken during this discussion

Bare-metal Linux, `go test -tags integration -count=1`, this branch:

| package | wall | sum of per-test time | `.Chdir` |
|---|---|---|---|
| `internal/fabricengine` | 15.50 s | — | 2 |
| `internal/fabriccli` | 1.36 s | 1.08 s | 18 |
| `internal/perchcli` | 0.76 s | 0.58 s | 6 |
| `internal/loomengine` | 0.57 s | 0.79 s | 3 |
| `internal/webstercli` | 0.29 s | 0.37 s | 1 |
| `internal/reedcli` | 0.21 s | 0.26 s | 4 |
| `internal/idecli` | 0.19 s | 0.13 s | 4 |
| `internal/configcli` | 0.18 s | 0.13 s | 3 |

`internal/fabricengine` already calls `t.Parallel()` in 228 of its 395 test functions;
its slowest single test is 1.51 s.
Set expectations accordingly: this task should not be sold on the Linux numbers.

## Constraints

From `CONSTRAINTS.md`, in order of how much they bear on this task:

- **Cwd Resolution Invariant.** `internal/lyxcwd` owns cwd resolution and nothing else.
  All cwd/worktree-root queries go through `lyxcwd.Getwd()`/`Resolve()`;
  raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lyxcwd` and `cmd/lyx/main.go`.
  `lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — `context` is stdlib, so the new API is admissible and the `fabricengine → logger → lyxcwd` acyclicity argument is unaffected.
  Also binding: `root` always means the git worktree root and `cwd` the working directory — never name the new parameter `root`.
  Geometry is structural, never config/env-overridable, which is part of why no `--cwd` flag is being added.
- **CLI / Cobra Invariant.** The seam is pinned as `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`, enforced by `cmd/lyx/drift_test.go`, `helptree_test.go`, `registration_test.go`, `longlist_test.go`.
  **Note the enforcement list overstates its coverage on the seam clause:** `drift_test.go` asserts only non-empty `Short`, and no test under `cmd/lyx` references `RunCLI` at all, so the seam signature is unpinned today.
  Adding `RunCLIIn` amends this invariant, and commit 1 both corrects the "Enforced by" line and adds the compile-time signature-pinning assertion that makes it true.
  `Short` is required on every command, and help accuracy is an explicit review obligation — the new `--into` flag needs a description and a `Long` example, and any `Short`/`Long` touching clone's behaviour must be re-checked.
  Errors stay on the `internal/output` JSON envelope.
- **hubforge Fabric-Fixture Invariant.** `hubforge.NewHub` is safe under concurrent use;
  no package inside `internal/fabriccli`'s dependency set may import `hubforge`, so any new fixture use in those packages must be in an external `*_test` package or use `gitkit`.
  Fixture teardown removes junctions via `internal/fslink` before `tb.TempDir()` cleanup, and that LIFO ordering is load-bearing on Windows.
- **Test Tier Purity Invariant.** Untagged test files must not call `gitexec.RunGit`, `exec.Command`, `gitkit.Copy*`, or `hubforge.NewHub`.
  Every file this task touches is already `//go:build integration`, so no migration may move a fixture call into an untagged file.
- **Hermetic Git Test Environment Invariant.** Every git-spawning test package needs a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`. No package gains or loses git-spawning status here, but the guard must stay green.
- **Fabric Vocabulary Invariant.** Fabric/warp/weft usage is machine-checked in production Go under `internal/` and `cmd/`;
  `host` in its fabric senses is banned everywhere. New doc prose and the `--into` help text are bound by this.
- **Documentation Lifecycle** and CLAUDE.md's task-completion rule: a change to observable CLI behaviour or cross-cutting infrastructure updates its module doc, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for any new invariant — in the same commit.
- **Markdown Link Integrity** — any new `.md` link under `manifest/` or `docs/` must resolve, including its `#anchor`.
- Repo prose style: semantic line breaks, one sentence per line, no fixed-column hard wrap.

## Testing

The governing principle here is that this task must not silently weaken an assertion.
Several chdir sites exist because the cwd *is* the thing under test, and a careless migration would turn a real assertion into a vacuous one.

**Verification protocol (decided):** run the affected packages with `-race -count=2` under both `-tags integration` and untagged, before and after the change, and record the wall-clock delta as a row in `docs/benchmarks/test-suite-timing.md`.
Each flag earns its place for a specific reason, and it is worth being precise about what `-race` does **not** buy here:
- `-race` covers the **parallel-safety of the three newly parallelized files** — concurrent access to shared memory once those tests run together.
  It does **not** catch a cwd dependence removed incorrectly: the process working directory is not race-detectable memory, and a wrongly-rebased path produces a failed assertion or a wrong answer, never a race report.
- Assertion preservation across the migration is bought instead by the per-call-site notes above and by the named scenarios below — particularly the weft-sibling refusal, the run-id escape check, and the relative-`--target-dir` scoutcli case.
  Those are the checks that would actually fail if a chdir were replaced with the wrong value.
- `-count=2` catches fixture-teardown ordering bugs that only surface on a second run in the same binary.
- The timing row is the honest record of a small payoff.

**TDD candidates — write these first, watch them fail:**

- `lyxcwd.CwdFrom` — returns the context-carried cwd when seeded via `WithCwd`, and falls back to `Getwd()` on a bare context.
  Pure unit test, no fixture, safe in Tier 1.
- `lyxcwd.WithCwd`'s precondition — panics on an empty `dir` and on a relative `dir`, and accepts an absolute one.
  Tier 1, no fixture. This is what makes "the injected cwd must be absolute" a rule rather than a wish.
- `clihelp.ExecuteIn` — the cwd it is given reaches the command's context and is observable from a `RunE`.
  Tier 1 with a synthetic cobra command;
  no hub needed.
- The `cmd/lyx/cwdmutation_test.go` guard itself — assert it fires on a planted violation and stays silent on an allowlisted file, matching how `tierpurity_test.go` carries its banned tokens as test data.
- The seam signature-pinning assertion — a compile-time `var _ = []func(io.Writer, []string) int{…}` over the 11 `RunCLI` functions and `[]func(string, io.Writer, []string) int{…}` over the 10 `RunCLIIn` functions.
  It has no runtime body: the test is that the package compiles, so a drifted signature is a build failure rather than a silent divergence from `CONSTRAINTS.md`.

**Scenarios that must be covered:**

- `RunCLIIn(cwd, …)` and `RunCLI(…)` from a process standing in that same cwd produce identical output and exit code, for at least one module. This is what proves the fallback path and the injected path agree.
- `RunCLIIn` driven at two different locations within a single test, proving cwd is per-call and not per-process — the property `fabriccli/cli_test.go:659→680` needs.
- The weft-sibling refusal (`fabriccli/cli_test.go:801`) still refuses when `PrimeWeft()` is passed as the cwd argument rather than chdir'd into.
- Clone with `--into <dir>` creates the hub at that directory, and clone without it still creates the hub at the process cwd.
  Both forms need coverage or the flag's default is untested.
- `Preflight(cwd)` against a non-git directory still yields exactly `CheckGeometry`, and against a subpath-anchored hub still treats the anchor as legal geometry — the two assertions currently carried by `TestPreflight_NotAGitRepo` and `TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor`.
- The `configcli` → `fabriccli` nested invocation resolves against the caller's cwd, driven without any process chdir.
- `scoutcli.RunCLIIn(cwd, …)` with a **relative** `--target-dir` (and `--within`, and a `file:line:col` query) resolves against the injected cwd, not the process cwd.
  This is the test that would have caught the round-3 defect, so it must exist;
  an absolute value must still be honoured unchanged, and `filterWithin`'s comparisons must still see an absolute path.
- `perchcli`'s run-id escape assertion still fails the test if an escape were to occur — worth planting a deliberate escape once, locally, to confirm the rewritten absolute-path form is not vacuous.

**Explicitly not covered by this task:** anything requiring `-tags smoke`, and the four `t.Setenv` files remain serial, so no parallel-safety claim is made about them.

## Deferred follow-up

Worth filing as its own task once this lands;
recorded here rather than filed unilaterally.

The smoke tier (12 files, 33 `.Chdir`) needs three further things before it could go parallel, none of which is chdir removal:

1. **`deferHubRelease` redesign.** Defined identically in four packages (`reedcli/smoke_test.go:493`, `shuttlecli/smoke_run_test.go:141`, `burlerengine/smoke_round_test.go:147`, `treadleengine/smoke_judge_test.go:141`), it registers a `t.Cleanup` that calls `os.Chdir(os.TempDir())` and can hold for up to 100 s while polling for directory release.
   Note this is a *cascade*, not an independent blocker: that chdir exists only because the test moved the process into the hub, so removing the test-side chdir lets those two lines be deleted rather than worked around.
   The rename-probe loop below them addresses tmux holding handles and is a separate concern that must survive.
2. **The `WEFT_SKIP_GIT` / `WEFT_SKIP_PUSH` / `BOARD_SKIP_GIT` config seam.** Read via `os.Getenv` in `fabricengine/fabric.go:106-107` and `spawn.go:38`;
   `t.Setenv` blocks roughly twelve files repo-wide.
3. **Substrate hazards.** `reedcli/smoke_attach_test.go:38` derives its harness socket per-PID (`lyx-attach-harness-%d`) with a fixed session name `"h"`, and its cleanup does `kill-server` on that socket — latent today because only one test uses that shape.
   Separately, `reedengine/lifecycle.go:319-334` documents a tmux race under concurrent server startup where a spawned server never becomes reachable on its socket;
   parallelism drives exactly that pattern.
   Reed's per-hub content-hashed socket names (`reedengine/server.go:19-25`) do mean concurrent tests never collide on a server or socket.

## Q&A log

- **Q:** Given the measured payoff is ~1–3 s on Linux, how far do we take this? **A:** Build the explicit-cwd seam across 10 of the 11 `RunCLI` modules (`selfreportcli` resolves no cwd) plus `loomengine.Preflight`, migrate every integration-tier call site, and add `t.Parallel()` only where free. Defer the env seam and the smoke tier.
- **Q:** Review round 2 found `clihelp.WrapRun` hands the handler only `(out, args)`, so "thread `cmd.Context()` in" was not mechanical. What is the mechanism? **A:** Add a `clihelp.WrapRunCtx` sibling taking `(ctx, out, args)`, matching the `RunCLI`/`RunCLIIn` idiom; `WrapRun` stays for handlers needing no cwd. `clihelp.RunRoot` also needed reaching, since it hardcodes `context.Background()` — resolved in round 3 as a `RunRootCtx` sibling rather than a new parameter.
- **Q:** Review round 2 found `idecli/cli_test.go` swaps the package-level `ideengine.CodeLauncher`, so it is not free for `t.Parallel()`. **A:** Migrate its chdir but leave it serial with a comment naming the seam swap; the parallelized set drops from four files to three. Converting `CodeLauncher` to per-invocation injection is deferred.
- **Q:** Review round 3 found the `RunRoot` change was left as two unchosen alternatives, and that a parameter would force `cmd/lyx/main.go:42,54` to change. **A:** Decided: a `RunRootCtx(ctx, cmd, out)` sibling, with `RunRoot` retained as `RunRootCtx(context.Background(), cmd, out)`. `main.go` stays untouched and `clihelp` gets one consistent sibling idiom across all four seams.
- **Q:** Review round 3 found `scoutcli` resolves relative flag/positional values against the process cwd at four sites, so `RunCLIIn` would honour the injection for lookup but not for `--target-dir`/`--within`/`file:line:col`/`--in-file`. **A:** State the seam's governance contract explicitly, and rebase those four bases onto the seam cwd in the same commit. A seam honoured for geometry but ignored for arguments returns a confidently wrong answer instead of an error.
- **Q:** Must the injected cwd be absolute, and is `WithCwd(ctx, "")` legal? **A:** Absolute always; `WithCwd(ctx, "")` is illegal. The empty string is a sentinel only in `RunCLIIn`, meaning "seed nothing, fall back to `Getwd()`" — which is how `RunCLI` delegates.
- **Q:** Review round 4 asked how the absolute-cwd precondition is actually enforced, since `WithCwd` returns no error. **A:** `WithCwd` panics on an empty or relative dir. Not silent `filepath.Abs` normalisation (which would resolve against the process cwd, reintroducing the very dependency being removed), and not an error from `CwdFrom` (which reports the fault far from the seeding site that caused it).
- **Q:** Review round 4 found the scoutcli rebase was enumerated by `filepath.Abs` occurrence and so stopped at the package edge — the raw `--target-dir` reaches `scoutengine.Options.TargetDir` and is absolutised in `ensureserver.go:120`. **A:** Rebase at the flag's defaulting point (`cli.go:142-145`) instead, so every consumer inside and outside `scoutcli` inherits it; enumerate by value-entry point, never by `filepath.Abs` site.
- **Q:** Review round 4 found commit 2 would not compile, since it changes `Preflight`'s signature while commit 3 migrates its only two callers. **A:** Commit 2 carries both call-site updates and the `export_test.go` comment rewrite; commit 3 keeps only chdir removal and `t.Parallel()`.
- **Q:** Does `-race` catch an incorrectly removed cwd dependence? **A:** No — process cwd is not race-detectable memory. `-race` covers parallel-safety of the newly parallelized files; assertion preservation is covered by the per-site notes and the named scenarios.
- **Q:** Review round 5 found the scoutcli rebase named one defaulting point of four, and gave `parseQuery`/`inFileQuery` semantics without a signature. **A:** All four defaulting points are rebased (`cli.go:142-145`, `:272-274`, `:377-379`, `:569-571`), and both functions take an explicit `base` parameter — the same "semantics without a signature" gap that forced `WrapRunCtx`, given the same treatment.
- **Q:** Does the `inFileQuery` base parameter touch `scoutcli/cli_test.go`, which the guard section lists as untouched? **A:** Yes — `TestInFileQuery_ResolvesRelativePathToAbsolute` (`:622-635`) pins the process-cwd behaviour and must move to an explicit base in commit 2, or commit 2 does not compile. The file does not join the guard's subject set.
- **Q:** Review round 5 disputed the out-of-scope inventory counts. **A:** Partly upheld. The smoke tier is 12 files / 33 occurrences, not 11 / 30 — the brief's list omits `reedcli/smoke_test.go`. The claim of "12 further files / 33 total / 111 occurrences" was not upheld: a comment-filtered repo census gives 37 files / 118 occurrences (smoke 12/33, integration 13/54, untagged 12/31), so "seventeen further files outside the twenty named" stands.
- **Q:** How does explicit cwd reach the resolution sites? **A:** Context-carried, owned by `lyxcwd` (`WithCwd`/`CwdFrom`), seeded by `clihelp.ExecuteIn` and per-module `RunCLIIn`. No user-visible `--cwd`/`-C` flag.
- **Q:** `fabriccli` has a plain helper for topology verbs and a `PersistentPreRunE` for weft verbs, and `configcli` uses plain handlers. Unify them? **A:** No — thread `cmd.Context()` into the plain handlers and leave each package's seam shape alone.
- **Q:** Clone's cwd is a destination, not a lookup. What replaces it? **A:** An explicit `--into <dir>` flag defaulting to the resolved cwd.
- **Q:** Do we guard against regression? **A:** Yes — a guard test with a named allowlist carrying a reason per entry.
- **Q:** `loomengine.Preflight()` has no production caller. Change the signature or add a sibling? **A:** Change it to `Preflight(cwd string)` and delete the `Getwd` call.
- **Q:** Adding `RunCLIIn` amends the machine-checked CLI/Cobra Invariant. Add or replace? **A:** Add it alongside `RunCLI`, which delegates with `cwd == ""`; amend the invariant in the same commit.
- **Q:** `perchcli`'s `os.Stat("../../escaped")` is deliberately relative to the chdir'd cwd. **A:** Rewrite against an absolute path derived from the hub, so the assertion survives and the test parallelizes.
- **Q:** How do we prove the migration is correct rather than merely compiling? **A:** `-race -count=2` across the affected packages under both tag sets, before and after, plus a recorded timing row.
- **Q:** Which docs land? **A:** `CONSTRAINTS.md`, the `lyxcwd`/`clihelp` package docs, affected module docs under `manifest/designs/`, and a timing row. No `manifest/roadmap.md` move — this is hardening, not a planned-item completion.
- **Q:** `CwdFrom`'s fallback — inside the function or at each caller? **A:** Inside, so the fallback exists once and every call site is a one-line swap.
- **Q:** `t.Parallel()` per test, or inside the shared fixture helpers? **A:** Per test, written explicitly; the four `t.Setenv` files stay serial with a comment naming the blocker.
- **Q:** Where does the guard live and what does it ban? **A:** `cmd/lyx/cwdmutation_test.go`, banning both `t.Chdir(` and `os.Chdir(` so the `restoreCwd`/`mustChdir` wrappers are caught too.
- **Q:** How is the work sliced into commits? **A:** Three, each self-contained with its own docs: infrastructure, module seams, then test migration plus guard.
- **Q:** Nested CLI invocations (`configcli` calling `fabriccli`) — how do they get a cwd? **A:** The caller passes its already-resolved cwd via `RunCLIIn`.
- **Q:** Does `loomengine`'s `export_test.go` shim survive? **A:** Yes, but its doc comment must be rewritten — its stated rationale is exactly what this task removes.
- **Q:** What happens to the deferred smoke-tier work? **A:** Recorded as a named follow-up section in this file rather than filed as a wiki task unilaterally.
- **Q:** Review round 1 found the seam signature is not machine-pinned — `drift_test.go` only checks `Short`, and nothing under `cmd/lyx` references `RunCLI`. What do we do? **A:** State plainly that the seam is unpinned today, correct `CONSTRAINTS.md`'s "Enforced by" line, and add a compile-time signature-pinning assertion for both shapes in commit 1.
- **Q:** Review round 1 found the `LYX_TRACE=1` exclusion rested on "no test sets it", which is not a disposition given `CONSTRAINTS.md` documents that mode. What is the disposition? **A:** No code change. `sinkOnce` is process-wide, so the trace directory was always pinned by the first logging call and never per-test; the migration replaces an arbitrary result with a deterministic one, recorded in `docs/benchmarks/running-tests.md`.
