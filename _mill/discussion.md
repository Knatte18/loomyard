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
- `RunCLIIn(cwd string, out io.Writer, args []string) int` on all 11 modules that expose `RunCLI` (`fabriccli`, `burlercli`, `configcli`, `idecli`, `shuttlecli`, `scoutcli`, `perchcli`, `selfreportcli`, `boardcli`, `webstercli`, `reedcli`).
  Existing `RunCLI` delegates to it with `cwd == ""`, meaning "read the process cwd".
- Swapping every production `lyxcwd.Getwd()` call site in a CLI path over to `lyxcwd.CwdFrom(cmd.Context())` — 15 sites: 7 in a `PersistentPreRunE`, 4 in `scoutcli` `RunE` bodies, and 4 in plain handler functions.
  A 16th touch point, `fabriccli/weft_verbs.go:52`, calls no `Getwd()` of its own and instead needs `cmd.Context()` threaded into its `resolveWarpLocation()` call.
  `loomengine/preflight.go:36` is the 17th and last production site, handled by its own Scope bullet below.
  All are enumerated under Technical context.
- Threading `cmd.Context()` into the plain handler functions that resolve cwd today (`fabriccli.resolveWarpLocation`, `fabriccli.runCloneWithReset`, `configcli.runReconcile`, `configcli.runConfig`).
- Changing `loomengine.Preflight()` to `Preflight(cwd string)` and deleting its `Getwd()` call.
- A `--into <dir>` flag on `lyx fabric clone`, defaulting to the resolved cwd, because clone's cwd is a *destination* argument rather than a lookup.
- Threading the caller's resolved cwd through nested CLI invocations: `configcli.go:384`'s `fabriccli.RunCLI(w, []string{"sync"})` becomes `RunCLIIn`.
- Migrating every `.Chdir` call site in the nine `//go:build integration` target files (41 occurrences) onto the new seam.
- Adding `t.Parallel()` to the four integration files that have no other blocker (13 of those 41 occurrences).
- A guard test at `cmd/lyx/cwdmutation_test.go` banning `t.Chdir(` and `os.Chdir(` in the migrated packages' test files, with a reasoned allowlist.
- Doc updates in the same commits: `CONSTRAINTS.md` (CLI/Cobra Invariant), `internal/lyxcwd` and `internal/clihelp` package docs, affected module docs under `manifest/designs/`, and a timing row in `docs/benchmarks/test-suite-timing.md`.

**Out:**

- **The eleven `//go:build smoke` files** (30 `.Chdir` occurrences across `reedcli`, `shuttlecli`, `burlerengine`, `treadleengine`).
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
- `internal/logger`'s durable-sink `sync.Once`. `sink.go:79` short-circuits under `testing.Testing()` unless `LYX_TRACE=1`, which no test sets.
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

### cwdfrom-owns-the-fallback

- Decision: `CwdFrom(ctx)` returns `(string, error)`, internally falling back to `Getwd()` when the context carries no cwd.
- Rationale: every one of the 15 CLI-path production sites becomes a one-line swap from `lyxcwd.Getwd()` to `lyxcwd.CwdFrom(cmd.Context())`, and the fallback exists in exactly one place.
- Rejected: `CwdFrom(ctx) (string, bool)` with per-caller fallback — more explicit at the call site, but duplicates the fallback twelve times and invites one site getting it wrong.

### runcli-gains-a-sibling-rather-than-changing

- Decision: add `RunCLIIn(cwd, out, args)` alongside the existing `RunCLI(out, args)` on all 11 modules;
  `RunCLI` delegates with `cwd == ""`.
  Amend the CLI/Cobra Invariant to name both shapes in the same commit.
- Rationale: `RunCLI` stays the production seam, `cmd/lyx/main.go` needs no change, and `cmd/lyx/drift_test.go` gains a second pinned shape rather than having one rewritten.
- Rejected: replacing `RunCLI` outright — one seam and no dual API, but a wide mechanical change to production wiring driven purely by a test need.

### plain-handlers-take-the-context

- Decision: the handler functions that resolve cwd outside a cobra hook (`fabriccli.resolveWarpLocation` at `fabric.go:398`, `fabriccli.runCloneWithReset` at `clone.go:119`, `configcli.runReconcile` at `configcli.go:257`, `configcli.runConfig` at `configcli.go:370`) take `cmd.Context()` as a parameter.
- Rationale: `clihelp.WrapRun` already has `cmd` in scope, so passing the context through is mechanical, and it leaves each package's existing seam shape intact.
  `fabriccli` legitimately has two seams — a plain helper for the 8 topology verbs and a scoped `PersistentPreRunE` for the weft verbs (`weft_verbs.go:52`) — and this decision does not force them to converge.
- Rejected: converting the plain handlers to `PersistentPreRunE` for a uniform seam shape — a cleaner end state, but a larger refactor that changes error-path ordering in packages this task has no other reason to disturb.

### nested-cli-calls-thread-the-callers-cwd

- Decision: where one module's CLI invokes another's, the caller passes its already-resolved cwd. `configcli.go:384`'s `fabriccli.RunCLI(w, []string{"sync"})` becomes `fabriccli.RunCLIIn(cwd, w, []string{"sync"})`, and the injected sync closure in `configcli_integration_test.go:74` follows.
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
- Rationale: at `clone.go:119` the cwd is where the hub is *created* (`CloneAndWire(cwd, …)`), not a lookup.
  A resolution-only seam would not cover the five clone tests, and leaving cwd to mean "destination here, lookup everywhere else" is an unmarked trap.
  The flag sits naturally beside the existing `--reset`, `--subpath`, and `--force-bootstrap`.
- Rejected: a third positional after an already-optional second one (a usage trap);
  and routing clone through the same context-carried cwd (smallest change, but leaves the trap unmarked).

### parallel-is-written-where-it-applies

- Decision: `t.Parallel()` is written explicitly in each migrated test function in the four env-free integration files.
  The four `t.Setenv` files get their chdir removed but stay serial, each carrying a comment naming `t.Setenv` as the remaining blocker.
- Rationale: parallelism stays visible at the test it governs, and the deferred work is documented where the next person will look for it.
- Rejected: calling `t.Parallel()` inside the shared fixture helpers (`seedPerchFixture` and friends) — fewer lines, but it hides a control-flow-changing call inside a setup function.

### guard-bans-both-chdir-spellings

- Decision: a guard test at `cmd/lyx/cwdmutation_test.go` bans both `t.Chdir(` and `os.Chdir(` in the migrated packages' `*_test.go` files, with an allowlist keyed by `(package, file)` carrying a reason per entry.
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

- Decision: build the seam across all 11 modules, migrate every integration-tier call site, and add `t.Parallel()` only where it is free.
  Defer the env seam and the whole smoke tier.
- Rationale: lands the durable architectural win with no speculative work, and stays honest about a small immediate payoff.
  The smoke tier contributes to no measured tier and carries three independent further blockers.
- Rejected: also building the `WEFT_SKIP_*` seam (unlocks 26 more chdirs, but drags a production env-var seam into a test-parallelism task);
  migrating smoke call sites without adding `t.Parallel()` (consistency, but touches 11 files for no measurable gain);
  and renegotiating the task away entirely (the seam has value independent of the timing numbers).

### three-commits-each-self-contained

- Decision: three commits, each building and testing green on its own, each carrying its own doc updates:
  1. `lyxcwd` context API + `clihelp.ExecuteIn` + `CONSTRAINTS.md` CLI/Cobra amendment + package docs.
  2. Module seams — the 11 `RunCLIIn` functions, plain-handler context threading, `Preflight(cwd)`, clone `--into` — plus module docs and help text.
  3. Test migration, `t.Parallel()`, the guard test, and the timing row.
- Rationale: CLAUDE.md requires docs in the same commit as the change that causes them, which rules out a trailing docs-only commit.
  Slicing this way isolates the risky half (2) from the mechanical half (3).
- Rejected: one commit (~40 files and two invariant amendments in a single reviewable unit);
  per-module commits (~12, of which 2–12 are near-identical, and the seam is only useful once all exist).

## Technical context

### The three populations of target file

The task brief names twenty files. They do not behave alike:

| population | files | `.Chdir` | blockers | measured tier? |
|---|---|---|---|---|
| integration, env-free | 4 | 13 | chdir only | Tier 2 |
| integration, `t.Setenv` too | 4 | 26 | chdir + `WEFT_SKIP_*` | Tier 2 |
| integration, cwd-is-the-subject | 1 | 2 | unremovable by design | Tier 2 |
| smoke | 11 | 30 | chdir + `deferHubRelease` + tmux races | neither |

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

The four files that gain `t.Parallel()` are `idecli/cli_test.go`, `reedcli/cli_integration_test.go`, `loomengine/preflight_integration_test.go`, and `perchcli/cli_integration_test.go`.

### Production `lyxcwd.Getwd()` call sites to migrate

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
- **`fabriccli/cli_test.go:493,579,609,659,716`** are the clone tests where cwd is the destination. These move onto `--into`, not onto `RunCLIIn`'s cwd.
- **`perchcli/cli_integration_test.go:35`** asserts `os.Stat(filepath.Join("..","..","escaped"))`, deliberately relative to the chdir'd hub, proving `--run-id ../../escaped` did not escape the runs area.
  Rewrite against an absolute path derived from the hub (`filepath.Join(h.PrimeWorktree(), "..", "..", "escaped")`) so the assertion survives the chdir's removal.
- **`perchcli/cli_integration_test.go:96` and `run_integration_test.go:157`** chdir into `h.Location.AnchorPath()`, a non-`"."` anchor — pass that path, not `PrimeWorktree()`.
- **`idecli/cli_test.go:95`** uses bare `os.Chdir` with a manual restore at `:98`, into a non-git tempdir for the error path.
- **`configcli_integration_test.go:55`** is not a `configcli` cwd dependence at all: `dispatch` is already given an explicit layout at `:80`, and the cwd is consumed inside the injected sync closure at `:74` that calls `fabriccli.RunCLI`. The fix reaches `fabriccli`, not `configcli`.
- **`loomengine/preflight_integration_test.go:188` and `:225`** exercise the public `Preflight()` specifically because they need it to observe a particular cwd.
  Under `Preflight(cwd string)` they pass the directory directly and parallelize;
  the `restoreCwd` helper at `:106` becomes dead and should be deleted.
- **`shuttlecli/smoke_interrupt_test.go:264`** calls `lyxcwd.Getwd()` in the test body, hand-rebuilding what the `PersistentPreRunE` does. Out of scope (smoke tier) but worth knowing it exists.

### What is already safe, and needs no work

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
  Adding `RunCLIIn` amends this and must update `CONSTRAINTS.md` and the drift test in the same commit.
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
`-race` is what catches a cwd dependence removed incorrectly;
`-count=2` catches fixture-teardown ordering bugs that only appear on a second run in the same binary;
the timing row is the honest record of a small payoff.

**TDD candidates — write these first, watch them fail:**

- `lyxcwd.CwdFrom` — returns the context-carried cwd when seeded via `WithCwd`, and falls back to `Getwd()` on a bare context.
  Pure unit test, no fixture, safe in Tier 1.
- `clihelp.ExecuteIn` — the cwd it is given reaches the command's context and is observable from a `RunE`.
  Tier 1 with a synthetic cobra command;
  no hub needed.
- The `cmd/lyx/cwdmutation_test.go` guard itself — assert it fires on a planted violation and stays silent on an allowlisted file, matching how `tierpurity_test.go` carries its banned tokens as test data.

**Scenarios that must be covered:**

- `RunCLIIn(cwd, …)` and `RunCLI(…)` from a process standing in that same cwd produce identical output and exit code, for at least one module. This is what proves the fallback path and the injected path agree.
- `RunCLIIn` driven at two different locations within a single test, proving cwd is per-call and not per-process — the property `fabriccli/cli_test.go:659→680` needs.
- The weft-sibling refusal (`fabriccli/cli_test.go:801`) still refuses when `PrimeWeft()` is passed as the cwd argument rather than chdir'd into.
- Clone with `--into <dir>` creates the hub at that directory, and clone without it still creates the hub at the process cwd.
  Both forms need coverage or the flag's default is untested.
- `Preflight(cwd)` against a non-git directory still yields exactly `CheckGeometry`, and against a subpath-anchored hub still treats the anchor as legal geometry — the two assertions currently carried by `TestPreflight_NotAGitRepo` and `TestPreflight_SubpathAnchoredHubIsNotRejectedForItsAnchor`.
- The `configcli` → `fabriccli` nested invocation resolves against the caller's cwd, driven without any process chdir.
- `perchcli`'s run-id escape assertion still fails the test if an escape were to occur — worth planting a deliberate escape once, locally, to confirm the rewritten absolute-path form is not vacuous.

**Explicitly not covered by this task:** anything requiring `-tags smoke`, and the four `t.Setenv` files remain serial, so no parallel-safety claim is made about them.

## Deferred follow-up

Worth filing as its own task once this lands;
recorded here rather than filed unilaterally.

The smoke tier (11 files, 30 `.Chdir`) needs three further things before it could go parallel, none of which is chdir removal:

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

- **Q:** Given the measured payoff is ~1–3 s on Linux, how far do we take this? **A:** Build the explicit-cwd seam across all 11 `RunCLI` modules plus `loomengine.Preflight`, migrate every integration-tier call site, and add `t.Parallel()` only where free. Defer the env seam and the smoke tier.
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
