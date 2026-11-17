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
| `card` | each Webster card, through `--card`, then the comment lint and the headroom lint | `card` |
| `batch` | each Webster batch, each tier run failing on a tmux server socket its end-of-package check finds | `card` |
| `webster-fix` | Webster-Burler-Fix's verify fixers, after the headroom and comment lints | `coverage` |
| `webster` | Webster's end gate | `all` |
| `parent-align`, `finalize` | Parent-Align and Finalize | `all` |
| `adopt` | `lym adopt check <subject>`, in-process through `gatecli.RunGate` in the subject's worktree | `all` |
| `darn-write`, `code-burler-fix` | darn's writer and the Code-Burler-Fix rows | `coverage` |
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
  What the hub lists as non-production in `darn.yaml`'s `tree_scope` bounds that.

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
- Two coverage runs take a tier's `env` and a gate slot as a tier run does:
  `lym gate testmap`, which rebuilds the per-package coverage maps from the hub's prime worktree,
  and the `redundancy` report step a gate key's `reports` lists, which runs after the key's tiers released their slots.
  Each runs every test alone under a coverage binary, since coverage counters are process-wide.
- A card `**Verify:**` line runs through the shared `internal/cardverify` runner,
  and `cmd/testtiming` measures outside the gate because its modes measure what a tier run hides.
- A site or stencil names a gate key, never a tier list, scope form, test command or target list.
- Darn's tree-scoped sites add the touched trees' targets to the key's call.
- The shell guard refuses a command with a prefix in `gate.yaml`'s `refuse_raw`,
  so an agent's raw test run is steered to the gate.
- The plan gate's `verify-raw-command` finding flags a card `**Verify:**` line that starts with a `refuse_raw` prefix.

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

- `lym gate testmap` keeps one coverage map per package under `internal/testcover`.
  A map records, for each tier0 and tier1 test, its function hash, the blocks it covers of its own package's files,
  the functions it covers of each other in-module package its test binary links, and its spawn scan verdict.
  It also records the hashes of the package's files and of each linked package's files at the commit it was built from.
  Each pass plans every map at the prime's HEAD through the coverage selector, building nothing:
  a current map moves to HEAD with its entries, an affected one drops the affected tests' entries, which are re-measured,
  and a miss rebuilds the package whole.
  A failed build keeps the map's entries and is not retried at the same HEAD.
  A self-check re-measures a kept entry a fresh build cannot place, and an audit round re-measures the oldest entries.
  While `test_map_upkeep` is on, the prime's dobby starts a `--upkeep` pass whenever work is left:
  no exit record, a HEAD past the last pass's plan, or queued work on a hub of two or more slots.
- `lym gate redundant <package>` judges a whole package's map by the same package-scoped verdict.
  It lists the candidates with their seconds and covering tests, the keep-marked tests and the tests not judged.
  It runs no test, so its candidates are evidence exactly as the report's are.
- A gate key whose `reports` lists `redundancy` runs the report step after its tiers and lints.
  It takes the diff's new and changed top-level tests against the merge base with the parent branch,
  leaving out tier2 and tier3 tests, tests marked `//testtiming:keep` and tests that may run module code in another process.
  It judges each against its package's map, package-scoped like `cmd/testtiming`'s verdict,
  and writes `redundancy.json` in the worktree's gate directory with each test contained, unique or not judged.
- The step is advisory and bounded: it never changes the gate's result,
  and `redundancy_max_tests` and `redundancy_budget_min` cap what it judges.
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
