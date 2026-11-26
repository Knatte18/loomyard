# PATTERN-test-speed

Tests stay fast: `tier0` is offline and spawns nothing, no test waits out a production duration,
and tests run in parallel.

## Four tiers

The hub declares its tiers in `gate.yaml`, numbered `tier0` through `tier3`, and `lym gate test --tier <n>` runs one.

| Tier | Needs | Costs |
|---|---|---|
| `tier0` | nothing: offline, no spawn | seconds, runs on every card gate |
| `tier1` | git, a built `lym`, subprocesses; no tmux server and no LLM, which `tmuxkit.Main` checks at the end of the package | tens of seconds, runs in the keys that list it |
| `tier2` | a real tmux server, no LLM | slower, runs in the keys that list it and by hand |
| `tier3` | a real LLM session | billed, compiled by every gate and run by hand only |

- A package builds one test binary that holds every tier's tests, and the tier is chosen at run time.
  `internal/testkit/tiergate` reads `LYM_TEST_TIER`, one tier (`1`) or a range up to a tier (`0-2`),
  and an unset variable selects `tier0`.
- Each tier in `gate.yaml` sets `LYM_TEST_TIER` in its `env`, so each test runs in exactly one tier;
  a key that wants two tiers lists both.
- Every `Test`, `Benchmark` and `Fuzz` function opens with `tiergate.Require(t, N)` for its tier.
  `Require` skips the function when the selection does not hold N.
  The functions of one test file name one tier, and no test file holds an `Example` function.
  No test file's build constraint names a tier tag or `integration`, `tmux` or `llm`,
  so every tier compiles the same files;
  a platform term stays (`!windows`).
  A support file holds no test function, such as `TestMain` and helpers, and names no tier.
  Enforced by `cmd/lym/tiertags_test.go`.
- A `//lym:guard` test names its tier like any other test.
- A test needing two substrates takes the higher tier.
- `tier3` tests, and with them `internal/testkit/llmkit`, compile into every test binary of their package,
  and no gate runs them.
- A test that reads `LYM_TEST_TIER` keys the Go test cache on it,
  so each tier's run of an unchanged binary misses the cache once.
- A tier's command hands its packages to `go test` through `{test_targets}`.
  `lym gate test` fills it with the packages holding a test of the tier, read from each test's `tiergate.Require`,
  so a pass starts no test binary without one, and a package's first test of a tier joins with no hand edit.

### Gate keys

A gate key in `gate.yaml` names a site's tiers and its scope.
A Go site runs `lym gate test --wait --gate <key>`,
and an agent's card, batch and `webster-fix` calls carry no `--wait`.
No site spells a tier list, a scope or a test command.
`internal/gateslot`'s doc holds the tier plan, which recipe row reads which key.

| Key | Site | Template scope |
|---|---|---|
| `card` | each Webster or Weaver card, through `--card`, then the comment lint and the headroom lint | `card` |
| `batch` | each Webster or Weaver batch, each tier run failing on a tmux server socket its end-of-package check finds | `card` |
| `webster-fix` | Webster-Burler-Fix's verify fixers, after the headroom and comment lints | `diff` |
| `webster` | Webster's end gate, and Weaver's plan-level verify | `all` |
| `parent-align`, `finalize` | Parent-Align and Finalize | `all` |
| `adopt` | `lym adopt check <subject>`, in-process through `gatecli.RunGate` in the subject's worktree | `all` |
| `darn-write`, `code-burler-fix` | darn's writer and the Code-Burler-Fix rows | `diff` |
| `harden-burler-fix` | Harden-Burler-Fix's `verify` gate | `diff` |
| `harden-live` | Harden-Burler-Fix's `live` gate: its merge-bar run and its concurrent copies | `diff` |
| `pr-review` | PR-Review's gate over the agent's commits | `diff` |
| By hand | the `tier3` tier | |

### No gate runs `tier3`

Every gate compiles the `tier3` tests, since every test binary holds them, and no gate runs them.
Running it is the operator's call, because a `tier3` run is billed, nondeterministic and needs a live session.

### Every gate run takes a slot

Every gate build or test run holds slots of the hub's gate-slot pool,
so the hub never runs more at once than its configured count.
A tier run takes the slots its tier asks for, and the non-tier holders `internal/gateslot`'s doc lists each hold one.
An agent runs a module-wide or `tier2` test only through `lym gate test`, and never as a raw `go` run.
A tagged gate run builds `lym` once and hands it to every package through the `LYM_PREBUILT_LYM` variable.
`lymbin` returns that binary instead of building.
A tagged hub run exports a path named by the binary's content,
and `lym gate test` strips the trace id from `go test`'s environment.
Go keys a cached test result on every variable a test reads.
Bound: a forged value misleads only an ungated raw run, since `lym gate test` sets or strips the variable itself.

## Only `tier3` tests reach an LLM

- A test file that imports `internal/testkit/llmkit` holds only `tier3` tests, or no test at all;
  a file whose tests name a lower tier, or no tier, fails.
- A test file outside `internal/testkit/llmkit` never calls `exec.LookPath` with an LLM binary name,
  as a string literal or a same-file constant;
  it calls `llmkit.Claude`.
- A file under `internal/testkit/llmkit` uses no identifier of `os/exec` but `LookPath`.
  The Testkit import check cannot see that rule.
- Enforced by `cmd/lym/llmtier_test.go`.
- Bound: the guard sees the static shape only.
  Review catches two cases:
  a `tier2` test that drives `lym` into spawning an LLM without calling `llmkit`;
  and a lower-tier test calling an `llmkit` helper of a support file.

## `tier0` tests spawn nothing

- No `gitexec.Run` or `RunGit`, and no `exec.Command` or `CommandContext`, appears in a file of `tier0` tests.
  Nor does `hubforge.NewHub`, `hubforge.CopyHub`, `hubforge.SharedHub` or a gitkit spawn.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn;
  the set is defined once in `cmd/lym/gitkitspawn_test.go`.
- Any `lymbin.` reference, which builds the `lym` binary, is likewise barred in a file of `tier0` tests.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred in a file of `tier0` tests.
  The set is defined once in `cmd/lym/tmuxkitspawn_test.go`.
  That covers the `/proc` probes although they spawn nothing.
  The bar costs nothing, because a `tier0` test has no process to probe.
- A file of `tier1` tests referencing a `tmuxkit` spawn fails too; its tests are `tier2` or `tier3`.
  An allowlist entry with a reason admits a `tier1` file that uses only the spawn-free `/proc` probes.
- `time.Sleep(...)` of one second or more in a file of `tier0` tests is flagged unless allowlisted.
- A file's tier is the tier its test functions name, read by `impactset.FileTier`;
  a support file, holding no test function, is not scanned.
- Enforced by `cmd/lym/tierpurity_test.go`.
- Bound: one test binary holds every tier.
  The compiler does not stop a `tier0` test from calling a spawning helper in a support file or a higher tier's file;
  the run-time skip and review hold that line.

## Repository-scanning tests carry `//lym:guard`

- A test that reads repository files outside its own package directory carries `//lym:guard`.
  The marker sits directly above the test's `func Test…` line, the way `//testtiming:keep` sits above a kept test.
- A narrowed gate's guard step runs every marked test by name under its file's tier, whatever the impacted set.
  A guard's result depends on files no import edge names.
  The one exception is a guard test that also carries `//lym:watch`:
  the step leaves it out when no changed file concerns it.
- A test carrying both markers stacks them as contiguous directive lines directly above its `func Test…` line.
  Either order works.
  `//lym:guard` counts as directly above when only `//testtiming:keep` and other `//lym:` lines lie between.
  `//testtiming:keep` counts as directly above when only `//lym:` lines lie between.
  Two stacked keeps stay an error.
- A test that reads only its own package directory is left unmarked.
  The impacted set selects it whenever that package changes.
- No marked test sits in a `tier2` or `tier3` file.
- Whether a scanning test carries the marker is review discipline, not a test.
- Bound: an unmarked scanning test is not run by the round gate or darn's writer gates, so a violation passes those.
  The whole verify catches it at the sites that keep it for a diff touching production code, never on `main`.
  For a tree-scoped darn diff, a site runs its row's key over the trees' targets,
  so such a violation can reach the parent.
  What the hub lists as non-production in `darn.tree_scope` bounds that.

## Reflective tests carry `//lym:seam`

- A test whose body reflects over a struct's field set carries a `//lym:seam <unit>#<Type>` line.
  The test carries one such line per struct its reflection walks, nested structs included.
  The lines sit in the comment block directly above its `func Test…` line.
- `//lym:seam`, `//lym:guard` and `//testtiming:keep` lines stack there in any order.
- The tier0 scan refuses these:
  an unmarked reflective test it sees;
  a marker whose glyph names no struct;
  and a test reaching `envkit.NilSeams` whose markers miss a struct the `Env` set holds.
- A struct declared outside the module is marked by its import path, such as `//lym:seam time#Time`.
  Such a marker, whose unit's first segment names no worktree directory, is not checked against a declaration.
- Bound: reflection through a non-test helper other than `envkit.NilSeams` is not seen.
- `lym loom validate-plan` reads the markers for `seam-test-unlisted`.

## Narrow tests carry `//lym:watch`

- A test that guards only some files, such as a cross-compile or a fixture build, carries `//lym:watch <pattern>...`.
  `path.Match` matches a pattern holding a `/` against the worktree-relative path, and any other against the base name.
- A `//lym:guard` test whose scan reads a bounded set of files carries the marker too.
  Its patterns cover every file that can change its outcome:
  the files it scans, a package declaring a value it names, and a helper in another file, `scankit` included.
  A recursive walk's patterns reach one directory level below the deepest file it scans today.
- A guard that scans all the Go code of the module, or of `internal/` and `cmd/`, stays unmarked.
  So does one whose inputs no pattern set bounds, such as a link check whose targets come from the link text.
- A narrowed gate runs the test only when a changed file matches a pattern or the test's own file changed.
  A diff-scoped key narrows over its diff, and a `card` key over the diff from the merge base with the parent branch.
- A whole gate always runs it.
- The marker stacks with `//lym:guard`, `//lym:seam` and `//testtiming:keep` lines in any order.
- A misplaced marker, or one with no pattern or a malformed one, refuses the gate.
- Bound: the skip matches by test name, so a test of the same name in another package of the run is skipped too.
- Bound: a file nested deeper than a guard's patterns reach does not run it in a narrowed gate; a whole gate does.

## Time is injectable

- A production interval, timeout or clock that a test would otherwise wait out is a struct field or a parameter.
  It is never a package-level `var`, so tests keep `t.Parallel`.
- Exception: `internal/gitexec`'s remote-attempt bounds and seams stay package-level variables,
  and the tests that replace them run serially.
- Production always passes today's value; the test passes a short one.
- No test in any tier waits out a production duration.
- No test compares a measured real time to a bound a loaded machine can reach.
  A test that proves a call does not wait on something makes that thing outlast a generous deadline, a minute,
  by a release the test controls or a duration far past the deadline.
  Where an outcome already tells the two paths apart, such as an exit code, an error or a pid,
  the test asserts that outcome instead of a time.
- Exception: a cost budget that no outcome can stand in for may compare a measured time to a bound.
  The bound sits far above what load adds to the work it measures, so only a gross regression reaches it.

## Tests run in parallel

- A test calls `t.Parallel` unless it touches process-global state: env, cwd or a shared fixture.
- A comment at the test, or once atop its file, names that state.

## No test skips by design

A skip in a gate log names a missing prerequisite, so every skip is worth reading.
The one exception is the tier selector's own skip:
a tier run skips every test of another tier, with a skip line naming both tiers.

- A test that means something on one OS alone sits in a file only that OS compiles:
  a `_windows` or `_linux` file name, or a `!windows` term in its build constraint.
- A table row or subtest bound to one OS is not generated on another; the loop passes over it before `t.Run`.
- A child-process body is no test function: the package's `TestMain` runs it when its environment variable is set.
- A skip that remains names a prerequisite the host lacks, such as a binary, a privilege or a non-root user.
- Enforcement is review discipline, not a test.

## Tree-sitter links

- Linking tree-sitter slows a test binary's link, so only the packages that call the code index for real may do it.
- `treeSitterAllowedLinks` in `cmd/lym/treesitterlink_integration_test.go` lists those packages, each with its reason.
- The `tier1` guard runs `go list -test -deps` under every tag.
  It fails, naming one import chain, when a test binary outside the list links `github.com/tree-sitter/go-tree-sitter`.
- It also fails when a listed package's test binary does not link it, so the list tracks the tree.
- The chain search and the comparison are in `cmd/lym/treesitterlink_test.go`.

## Tests run through the gate

Running a hub's tests goes through `lym gate test` over the tiers `gate.yaml` declares,
so every run takes its slots, its timeout and its log.

- Tests run only through `lym gate test` over `gate.yaml`'s tiers.
  Two things run outside it: a card `**Verify:**` line and `cmd/testtiming`'s runs in every mode.
- The redundancy runs take a tier's `env` and a gate slot as a tier run does.
  `lym gate test --map` rebuilds the per-package coverage maps over its scope at the HEAD of the worktree it runs in,
  into the hub's shared maps from the prime and into the worktree's anchor `.lym` elsewhere.
  The `redundancy` report step a gate key's `reports` lists judges beside the first run of the key's highest tier,
  takes its own gate slot beside the slots that tier holds, and writes its report after the lints.
  The `reach` report step refreshes a task worktree's own anchor maps by the same measurement, in the map run's slot ask.
  The map run, the refresh and the `redundancy` step run every test alone under a coverage binary, since coverage counters are process-wide.
  `lym gate redundant` runs each test alone under a binary holding the mutants of its package, one mutant on per run.
  `lym gate mutate` and the `mutation` report step do the same for the tests covering each mutant of a changed line,
  after they refreshed the worktree's own maps.
- A card `**Verify:**` line runs through the shared `internal/cardverify` runner,
  and `cmd/testtiming` measures outside the gate because its modes measure what a tier run hides.
- A site or stencil names a gate key, never a tier list, scope form, test command or target list.
- Darn's tree-scoped sites add the touched trees' targets to the key's call.
- The shell guard refuses a command with a prefix in `gate.refuse_raw`,
  so an agent's raw test run is steered to the gate.
- The plan gate's `verify-raw-command` finding flags a card `**Verify:**` line that starts with a `gate.refuse_raw` prefix.

## Test economy

A test exists for a behavior, not for a symbol, and no two tests cover the same behavior.
Enforcement is review discipline, not a test.
The general rules live in `scribe:testing` and `scribe:golang-testing`;
this section keeps only what is project-specific or stricter here.

- A test targets behavior and contract at a module's public surface, not each helper.
  The surface is the exported API, a CLI verb or a file contract.
  An unexported helper is tested through the surface that uses it.
- A behavior testable with a fake or an in-memory fixture is tested at tier0 that way, never through a real hub.
  Integration tests stay the exception.
- A new test function or file must cover a behavior no existing test covers.
  Otherwise it becomes a row in an existing table-driven test, or an assertion in an existing test of that behavior.
- Many items of one kind (refusals, claims, rows, verbs) are covered by one table-driven or contract-level test.
  They never get one test each.
- No test pins a stencil's static prose.
  A stencil test may pin only:
  a marker set, a Fill round-trip or a Go-filled marker's position;
  a value Go fills, reads or names, a fragment Go chooses, or a rule across a stencil family.
  Review guards the wording.
- A review fix adds a test only when the finding is a coverage gap;
  for any other finding it corrects the existing test that asserted the wrong thing.
- A redundant test is a finding exactly as a missing one is.
  The finding names the existing test that already covers the behavior,
  so it stays grounded and fixable by folding or deleting.
- `go run ./cmd/testtiming -redundancy` writes the per-test coverage report, `docs/benchmarks/test-redundancy.md`.
  A run narrowed with `-pkg` rewrites the header and only its own packages' sections of the existing report,
  so parallel tasks pruning different packages merge cleanly.
  The run judges a candidate over the blocks of its own package's files,
  and adds a "Contained module-wide" column saying whether the candidate is also contained over every block of the module.
  A candidate in it is evidence, not a verdict:
  coverage blocks do not see assertions,
  so a candidate whose assertions differ from its covering tests is kept or folded.
  A prune finding names the covering test from the report,
  and only the removable set may be deleted together:
  deleting a candidate outside it can leave a block uncovered once its partner is gone.
  A candidate that is kept carries `//testtiming:keep <reason>` directly above its `func Test…` line,
  and the reason names what it pins that its covering tests do not (an error message, an output shape, an ordering);
  the report lists it under "Kept".
  A test in the "no coverage" list is outside the report's judgment, not exempt from the rules above.
- The "no coverage" list holds:
  - a test that is skipped, failed or covers nothing of its own package;
  - a test above tier0 with a call the static scan cannot resolve;
  - a test that may run this module's code in another process:
    one that sits in a `tier2` file;
    or one that references `internal/testkit/lymbin`, `os.Executable`, `os.Args[0]` or `exec.Command("go", ...)`.
    The reference counts when it is direct or sits in a followed call.
- Spawning git, using `hubforge` or driving a tmux server through `tmuxkit` does not exclude a test,
  and a tier0 test is always judged.
- The scan does not see a hook, alias or helper a test installs so that git runs module code,
  so the hand check before deleting a candidate also asks whether the candidate makes git run module code.

### The coverage map and the gate's redundancy report

- `lym gate test --map` keeps one coverage map per package under `internal/testcover`.
  A map records, for each tier0 and tier1 test, its function hash, the blocks it covers of its own package's files,
  the functions it covers of each other in-module package its test binary links, and its spawn scan verdict.
  It also records the hashes of the package's files and of each linked package's files at the commit it was built from.
  The hub's shared maps are built only on command: no dobby, landing or gate builds them, and the gate selects no test from them.
  The `reach` step refreshes a task worktree's own anchor maps of the packages holding changed functions,
  under its budget and the map run's clean checks.
  A call measures each package of its scope's set whole at the HEAD of the worktree it runs in, and replaces the package's map.
  A failed build keeps the map's entries.
  The map run bounds its test processes by the tiers' own limits:
  a package's tier0 tests run before its tier1 tests, each tier's tests up to its `parallel` side by side,
  clamped to the memory its limit leaves and, for a tier that sets `memory_mb`, to that memory divided by `gate.redundancy_test_memory_mb`.
  A hub whose tier0 or tier1 sets a `memory_mb` below `gate.redundancy_test_memory_mb` is refused.
  A test that fails, times out or is ended by the memory guard or its cap beside others runs again alone, and that run stands.
  A test that passed only alone is listed as `passed_alone` and logged, and enters the map as a plain pass.
  The run holds the larger of tier0's and tier1's slot asks, where `slots: all` ranks above any count, and builds no `lym`.
  It writes every map that passes its checks, failed entries included,
  and exits 1 when a build failed, a package was left unmeasured or an entry records a failed or timed-out test.
  A map is written only when HEAD is still the commit the call read at its start and no file the package or a linked package owns is modified or untracked.
  The check runs once the build ended and again before the write, and guards every write path, a failed build's included.
  A package that fails it writes no map and is listed under `unmeasured` with its reason, and a moved HEAD ends the walk.
  Bound: an edit made before a package's first check, or between its two checks, and reverted before the next goes unseen,
  and a gitignored file is seen by neither check, so a gitignored Go file the build compiles enters the map under HEAD's hashes.
  It writes no run record or green record, and a green record never stands in for a map run.
- `lym gate redundant <package>` judges a whole package's map by the same package-scoped verdict,
  then confirms each candidate by mutation inside a gate slot.
  It mutates the candidates' blocks and keeps a candidate only when its covering tests kill every mutant it kills;
  one that kills a mutant they miss is listed as refuted, with those mutants.
  A run that ends at its time bound or the memory cap is no kill,
  and a candidate whose answer rests on one is listed as unconfirmed, with those mutants.
  A `//lym:guard` test and one reaching `internal/testkit/scankit` are never candidates:
  their check reads source files, which no mutant changes.
  It lists the candidates with their seconds, covering tests and kills, and the refuted, unconfirmed, keep-marked and unjudged tests.
  Bound: a mutant changes the package's own code only,
  so a candidate whose check differs from its covers only in another package's result is confirmed falsely.
- A gate key whose `reports` lists `reach` judges the report step beside its last tier and before the redundancy step, and writes its report after the lints.
  It finds the functions the diff changes against the merge base with the parent branch, or against HEAD in the prime,
  and writes `reach.json` in the worktree's gate directory with the tests of the maps that reach each one.
  It prints a note of the report.
  A task worktree first refreshes its own anchor maps of the packages holding a changed function whose map is absent or stale,
  within `gate.reach_budget_min`; a package with an uncommitted file of its own or of a linked package keeps its map and is listed.
  The prime refreshes nothing and answers from the shared maps.
  The step is advisory: it never changes the gate's result.
  Bound: a map older than the files, or a test that runs the function in another process, can make a function read as unreached.
- A gate key whose `reports` lists `mutation` runs the report step after its other report steps.
  It runs `lym gate mutate`'s refresh and check over the diff from the reach step's base,
  and writes `mutation.json` in the worktree's gate directory.
  The report lists every survivor, unjudged mutant, uncovered changed block and package not judged.
  The step prints one summary line of their counts, and it never changes the gate's result.
  Bound: a survivor is a check the diff's tests lack or a line no test can reasonably check, and review tells them apart.
- A gate key whose `reports` lists `redundancy` judges the report step beside its last tier, and writes its report after the lints.
  It takes the diff's new and changed top-level tests against the merge base with the parent branch,
  leaving out tier2 and tier3 tests, tests marked `//testtiming:keep` and tests that may run module code in another process.
  It judges each against its package's map, package-scoped like `cmd/testtiming`'s verdict,
  and writes `redundancy.json` in the worktree's gate directory with each test contained, unique or not judged.
- The step is advisory and bounded: it never changes the gate's result,
  and `gate.redundancy_max_tests` and `gate.redundancy_budget_min` cap what it judges.
  Bound: it lets a redundant test through when an edited block is uncovered or a test lies past the cap or the budget.
  Bound: it can flag a test falsely, since a map test whose own text is unchanged counts as a cover
  even when a helper or callee change stops it reaching a block.
  Bound: a map rebuilt since the report can make a contained verdict stale.
- Code-Review's rubric reads the report when it matches the reviewed HEAD's tree:
  every judged package's Go files hash the same and the step's selection over HEAD gives the recorded tests with the recorded hashes.
  A report written on a red tier never matches, and the rubric then reads a line naming that gate key.
- A gate finding is evidence for Code-Review exactly as a report candidate is:
  the reviewer checks a contained test's assertions against its covering tests,
  then files a finding to fold or delete it, naming the covering tests,
  or accepts it with a `//testtiming:keep <reason>` finding when it pins what coverage cannot see.
