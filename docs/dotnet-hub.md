# Setting up a .NET hub

This guide sets a hub's `gate.yaml` up so a card, batch or diff gate builds once and tests only the classes it touches, and only the landing gates run the whole solution.
Its worked example is the Models repository, a .NET solution with fast tests, gated tests behind MSBuild properties and a numerical-baseline project.
Project names below are examples, so use the hub's own.

## The dotnet scoper

`scoper: dotnet` makes a computed, `--base` or `--card` scope the test projects and test classes a change can reach, derived from C# source alone by `internal/dotnetimpact`.
The derivation takes the changed `.cs` files and the types they declare, closes that set over every file whose identifiers name a type already in it, and selects each test class whose file is in the closure.
A tier whose command holds `{targets}` then runs once per selected test project, with the project as `{targets}` and the selected classes in `{filter}`.
A change the derivation cannot reason about runs whole, naming the reason on stderr:
a `.csproj`, `.props`, `.targets`, `.sln`, `global.json`, `nuget.config` or `.resx` change, a `.cs` file outside the tier's `dir`, a `.cs` file that declares no type, or a test class with no single `.csproj` above it.

Bound: the closure misses a reference that never spells a type's name, such as a reflection lookup by a computed string.
The whole gates catch those, which is why the gate keys below keep the whole solution at landing.

The `dotnet` scoper plans no guard step and no vet step, and `--guards` is refused under it.

## Tiers

A tier's command holds `{targets}`, `{filter}` and optionally `{args}`:

```yaml
tiers:
  tier0:
    about: fast tests of the whole solution
    command: dotnet test {targets} --no-build {filter}
    all_targets: NORCE.Models.Tests/NORCE.Models.Tests.csproj NORCE.Models.Gated/NORCE.Models.Gated.csproj
    counter: vstest
    shared_build: true
    filter: "Category!=Slow"
    timeout_min: 30
```

`{targets}` renders the selected test project, or the tier's `all_targets` for a whole run.
`{filter}` renders as `--filter <expression>`, quoted for the platform shell.
Under a narrowed scope the expression is the tier's `filter` ANDed with the selected classes, and for a whole run it is the tier's `filter` alone, so `{filter}` renders empty for a tier with none.
A `filter` is allowed only when the command holds `{filter}`.
A landing tier uses its `filter` only to exclude categories, such as `Category!=Slow`, since it applies at whole scope too and a filter that matches no test passes the tier vacuously and writes a green entry later gates carry.

`shared_build: true` makes the tier depend on the hub's `build` step, which lym runs once per call before it, so the tier's command carries `--no-build`.
A command without `--no-build` builds again inside its own run, since lym does not read the command.

A selected class that the tier's `filter` excludes entirely, or a project the tier does not run as a test project, runs no test and passes.
On Microsoft.Testing.Platform a session that ran zero tests exits 8, so put `--ignore-exit-code 8` in the command of every tier that holds `{filter}`, or `TESTINGPLATFORM_EXITCODE_IGNORE: "8"` in its `env`.

## The build step

`build` is the one build a call runs for every tier with `shared_build`, inside the slots of the first such tier whose run takes slots:

```yaml
build:
  about: build the solution with the gated and baseline projects
  command: dotnet build Models.sln -m -p:RunSlowTests=true -p:RunBaseline=true
  timeout_min: 30
```

It builds the whole solution whatever the tiers' scopes, and relies on MSBuild's incremental up-to-date checks for the projects a change did not touch.
It restores implicitly, so there is no separate restore step: NuGet's no-op restore does real work only when a project file or package reference changed.
The properties on the command make the gated and baseline projects test projects, so the tiers' `--no-build` runs find them built.
lym adds no MSBuild or compiler-server switch to the command, and the words after `--` never reach it.
Its output passes into the call's output and its own log.
`build` has its own `timeout_min`, counted from the slot hold, and the tier's `timeout_min` starts after the build.

A scope whose changes no test class names still runs the build inside the first `shared_build` tier's slots and runs no test command in that tier, so a compile break fails the gate.
A tier that does not opt in runs whole.
A call whose opted-in tiers are all carried, or whose scope is empty, runs no build.
A failed or timed-out build fails every opted-in tier of the call without running its command, naming the build's log, and the call exits with the build's code, or 1 for a timeout.

## Inert paths

`inert_paths` lists glob patterns of files no test reads:

```yaml
inert_paths: ["*.md", "docs/*"]
```

A pattern without a slash matches a path's base name and one with a slash matches the worktree-relative path.
A diff whose changed paths are all inert, untracked files included, runs no listed tier's command, and a whole gate carries a tier over history that changed only inert files.
Bound: a test that reads an inert file is skipped when only that file changes, so list only files no test reads.
The list joins every tier's fingerprint, so changing it voids the green entries written under the old one.

## Gate keys

The recommended keys for a .NET hub keep the whole solution to the landing gates:

```yaml
gates:
  card: {tiers: [0], scope: card}
  batch: {tiers: [0], scope: card}
  webster: {tiers: [0], scope: diff}
  webster-fix: {tiers: [0], scope: diff}
  darn-write: {tiers: [0], scope: diff}
  code-burler-fix: {tiers: [0], scope: diff}
  drift-verify: {tiers: [0], scope: diff}
  parent-align: {tiers: [0], scope: all}
  finalize: {tiers: [0], scope: all}
  adopt: {tiers: [0], scope: all}
```

A landing key may list more tiers than the card keys, such as one that runs the slow category.

Parent-Align is then the one planned whole-solution run per task, since it is the first gate over the merged parent.
Finalize carries that run from the green record when no commit landed between, and runs whole again only after a fix or a moved parent changed the tree.

## A timing tier

A tier that must run on an unchanged tree, such as a BenchmarkDotNet run, takes the pool's exclusive lock and never carries:

```yaml
tiers:
  tier2:
    about: timing run
    command: dotnet run -c Release --project NORCE.Models.Benchmarks -- {args}
    counter: vstest
    slots: all
    manual: true
    carry: false
    timeout_min: 120
```

`slots: all` waits for every running holder to finish and blocks every new gate run of the hub until the tier ends, so other worktrees' gate runs wait while their agents keep working.
`manual: true` keeps it out of `--upto`, and `carry: false` keeps the green record from carrying it or recording it, and runs it whole under every scope form but explicit targets.
Run it only through a direct call, which passes `--wait` so its drain is not dropped at the keep-cache-warm cap:

```
lym gate test --wait --tier 2 -- <benchmark arguments>
```

Three paths still skip or narrow a `carry: false` tier: a verify site's verified-tree record, explicit targets, and a listed tier whose `covers` names it.
So keep the tier out of darn's gate keys and out of every tier's `covers`, and never run it through a verify site.

## Refusing raw test runs

```yaml
refuse_raw: ["dotnet test"]
```

The shell guard then refuses an agent's raw `dotnet test` and steers it to `lym gate test`.

## Warm build processes

lym sets nothing that disables MSBuild node reuse or the shared compiler server, so the SDK's defaults keep build processes warm where the platform allows it.
On Linux, idle nodes outlive the build's process, since only a cancelled call kills its process group, and serve later gate calls until the SDK's idle timeout.
On Windows, the build runs in a job object that ends every process it started, so nodes and the compiler server end with each build or tier run.
The saving there is the one build per call and the `--no-build` test runs, not warmth across calls.
A warm node holds none of the child's output pipe, so a call ends when the build or test process does.
Set neither `MSBUILDENSURESTDOUTFORTASKPROCESSES` nor `MSBUILDNODEWINDOW` in a `build` or tier `env`: on a later SDK either leaves a Unix node's output unredirected, and the call then waits on idle nodes until `timeout_min`.
A hub that wants the opt-in MSBuild server sets `DOTNET_CLI_USE_MSBUILD_SERVER` in its `build` and tier `env`.

## The repo-rules lens

The Models review rules are lens prose in the hub's `lenses.yaml`, added to the `review-code` fan the code review reads:

```yaml
lenses:
  models-rules: |
    Check the repository's own rules.
    No test in the three test projects runs, mirrors or references a Program demonstration, so grep the test projects for Program0NN_.
    Comparisons against stored output live in NORCE.Models.NumericalBaseline alone, while an in-process test may byte-compare two arms it builds itself.
    Add no argument guards and no try/catch around the repo's own code.
    Report anything else you notice too — emphasis, never exclusion.
fans:
  review-code:
    - code-correctness
    - code-affected-set
    - code-tests
    - models-rules
```

A fan entry replaces the template's whole list, so the entry above repeats the template's lenses and appends the new one.

## Before installing

Take a `lym gate report` baseline on OSL-1046 before installing the binary that lands this feature, so the machine's tier run totals can be compared after:

```
lym gate report --slug <slug>
```

A report row totals the build step's runs and time apart, in `build_runs` and `build_seconds`.
