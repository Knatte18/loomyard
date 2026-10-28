# PATTERN-test-speed

Tests stay fast: `tier0` is offline and spawns nothing, no test waits out a production duration, and tests run in parallel.

## Four tiers

The hub declares its tiers in `gate.yaml`, numbered `tier0` through `tier3`, and `lym gate test --tier <n>` runs one.

| Tier | Tag | Needs | Costs |
|---|---|---|---|
| `tier0` | `tier0` | nothing: offline, no spawn | seconds, runs on every card gate |
| `tier1` | `tier1` | git, a built `lym`, subprocesses; no tmux server and no LLM, which `tmuxkit.Main` checks at the end of the package | tens of seconds, runs in the keys that list it |
| `tier2` | `tier2` | a real tmux server, no LLM | slower, runs in the keys that list it and by hand |
| `tier3` | `tier3` | a real LLM session | billed, compiled by every gate and run by hand only |

- Tags are disjoint: `-tags tier1` compiles no `tier0`, `tier2` or `tier3` test file, so each test runs in exactly one tier, and a key that wants two tiers lists both.
- A test file holding a `Test`, `Benchmark`, `Fuzz` or `Example` function carries exactly one tier tag, a platform term staying beside it (`tier2 && !windows`); an untagged test file holds support only, such as `TestMain` and helpers.
  Enforced by `cmd/lym/tiertags_test.go`.
- A file holding a `//lym:guard` test takes its one `tierN` tag like any other test file; `cmd/lym/tiertags_test.go` refuses a constraint naming `integration`, `tmux` or `llm` in any test file.
- A test needing two substrates takes the higher tier.
- Each tag compiles on its own: `go vet -tags tier2 ./...` and `go vet -tags tier3 ./...` each pass.
- A helper file used by files of two tiers carries the disjunction of their tags (`//go:build tier2 || tier3`), and holds no test; a helper only one tier uses lives in that tier's file, and a file whose helpers another tier's tests use is split.
- `tier3` files are compiled by `go vet -tags tier3 ./...` and never run by a gate.

### Gate keys

A gate key in `gate.yaml` names a site's tiers and its scope, and a site runs `lym gate test --wait --gate <key>`; no site spells a tier list, a scope or a test command.
`internal/gateslot`'s doc holds the tier plan, which recipe row reads which key.

| Key | Site | Template scope |
|---|---|---|
| `card` | each Webster card, through `--card`, then the comment lint and the headroom lint | `card` |
| `batch` | each Webster batch, each tier run failing on a tmux server socket its end-of-package check finds | `card` |
| `webster-fix` | Webster-Burler-Fix's verify fixers, after the headroom and comment lints | `diff` |
| `webster` | Webster's end gate | `all` |
| `parent-align`, `finalize` | Parent-Align and Finalize | `all` |
| `adopt` | `lym adopt check`, in-process through `gatecli.RunGate` | `all` |
| `darn-write`, `code-burler-fix` | darn's writer and the Code-Burler-Fix rows | `diff` |
| `drift-verify` | `Drift-Verify`'s test gate over its agent's fixes | `diff` |
| By hand | the `tier3` tier | |

### No gate runs `tier3`

Every gate compiles the `tier3` tier through `go vet -tags tier3`, and no gate runs it.
Running it is the operator's call, because a `tier3` run is billed, nondeterministic and needs a live session.

### Every gate run takes a slot

Every gate build or test run holds slots of the hub's gate-slot pool, so the hub never runs more at once than its configured count.
A tier run takes the slots its tier asks for, and the non-tier holders `internal/gateslot`'s doc lists each hold one.
An agent runs a module-wide or `tier2` test only through `lym gate test`, and never as a raw `go` run.
A tagged gate run builds `lym` once and hands it to every package through the `LYM_PREBUILT_LYM` variable, which `lymbin` returns instead of building.
A tagged hub run exports a path named by the binary's content, and `lym gate test` strips the trace id from `go test`'s environment, since Go keys a cached test result on every variable a test reads.
Bound: a forged value misleads only an ungated raw run, since `lym gate test` sets or strips the variable itself.

## Only `tier3` files reach an LLM

- A test file that imports `internal/testkit/llmkit` is constrained to `tier3`, so `//go:build tier3` and `//go:build tier3 && linux` pass while `//go:build tier2 || tier3`, `tier1` and an untagged file fail.
- A test file outside `internal/testkit/llmkit` never calls `exec.LookPath` with an LLM binary name, as a string literal or a same-file constant; it calls `llmkit.Claude`.
- A file under `internal/testkit/llmkit` uses no identifier of `os/exec` but `LookPath`, which the Testkit import check cannot see.
- Enforced by `cmd/lym/llmtier_test.go`.
- Bound: the guard sees the static shape only.
  A `tier2` test that drives `lym` into spawning an LLM without calling `llmkit` is caught by review.

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub`, `hubforge.CopyHub`, `hubforge.SharedHub` or gitkit spawn outside tier-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lym/gitkitspawn_test.go`.
- Any `lymbin.` reference, which builds the `lym` binary, is likewise barred outside tier-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside tier-tagged files, defined once in `cmd/lym/tmuxkitspawn_test.go`.
  That covers the `/proc` probes although they spawn nothing, which costs nothing because an untagged test has no process to probe.
- A file referencing a `tmuxkit` spawn is further constrained to the `tier2` tier: it compiles in neither the untagged nor the `tier1` build on any platform, so `tier2`, `tier3`, `tier2 || tier3` and `tier2 && !windows` pass while `tier1 || tier2` fails.
  An allowlist entry with a reason admits a tier-2 file that uses only the spawn-free `/proc` probes.
  The scan evaluates the build line under every assignment of its tags other than `tier2` and `tier3`, so the host's GOOS plays no part.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
- Enforced by `cmd/lym/tierpurity_test.go`.

## Repository-scanning tests carry `//lym:guard`

- A test that reads repository files outside its own package directory carries `//lym:guard` on the line directly above its `func Test…` line, the way `//testtiming:keep` sits above a kept test.
- A narrowed gate's guard step runs every marked test by name under its file's tier, whatever the impacted set, because a guard's result depends on files no import edge names.
- A test carrying both markers stacks them as contiguous directive lines directly above the `func Test…` line, in either order.
  `//lym:guard` counts as directly above when only `//testtiming:keep` and `//lym:seam` lines separate it from the func line, and `//testtiming:keep` counts as directly above when only `//lym:` lines separate it; two stacked keeps stay an error.
- A test that reads only its own package directory is left unmarked, since the impacted set selects it whenever that package changes.
- No marked test sits in a `tier2` or `tier3` file.
- Whether a scanning test carries the marker is review discipline, not a test.
- Bound: an unmarked scanning test is not run by the round gate or darn's writer gates, so a violation passes those and is caught by the whole verify at the sites that keep it for a diff touching production code, never on `main`.
  For a tree-scoped darn diff, a site runs its row's key over the trees' targets, so such a violation can reach the parent, bounded by what the hub lists as non-production in `darn.yaml`'s `tree_scope`.

## Reflective tests carry `//lym:seam`

- A test whose body reflects over a struct's field set carries one `//lym:seam <unit>#<Type>` line per struct its reflection walks, nested structs included, in the comment block directly above its `func Test…` line.
- `//lym:seam`, `//lym:guard` and `//testtiming:keep` lines stack there in any order.
- The tier0 scan refuses an unmarked reflective test it sees, a marker whose glyph names no struct, and a test reaching `envkit.NilSeams` whose markers miss a struct the `Env` set holds.
- A struct declared outside the module is marked by its import path, such as `//lym:seam time#Time`.
  Such a marker, whose unit's first segment names no worktree directory, is not checked against a declaration.
- Bound: reflection through a non-test helper other than `envkit.NilSeams` is not seen.
- `lym loom validate-plan` reads the markers for `seam-test-unlisted`.

## Time is injectable

- A production interval, timeout or clock that a test would otherwise wait out is a struct field or a parameter, never a package-level `var`, so tests keep `t.Parallel`.
- Production always passes today's value; the test passes a short one.
- No test in any tier waits out a production duration.

## Tests run in parallel

- A test calls `t.Parallel` unless it touches process-global state: env, cwd or a shared fixture.
- A comment at the test, or once atop its file, names that state.

## No test skips by design

A skip in a gate log names a missing prerequisite, so every skip is worth reading.

- A test that means something on one OS alone sits in a file only that OS compiles: a `_windows` or `_linux` file name, or a `!windows` term beside its tier tag.
- A table row or subtest bound to one OS is not generated on another; the loop passes over it before `t.Run`.
- A child-process body is no test function: the package's `TestMain` runs it when its environment variable is set.
- A skip that remains names a prerequisite the host lacks, such as a binary, a privilege or a non-root user.
- Enforcement is review discipline, not a test.

## Tree-sitter links

- Linking tree-sitter slows a test binary's link, so only the packages that call the code index for real may do it.
- `treeSitterAllowedLinks` in `cmd/lym/treesitterlink_integration_test.go` lists those packages, each with its reason.
- The `tier1` guard runs `go list -test -deps` under every tag and fails, naming one import chain, when a test binary outside the list links `github.com/tree-sitter/go-tree-sitter`.
- It also fails when a listed package's test binary no longer links it, so the list tracks the tree.
- The chain search and the comparison are in `cmd/lym/treesitterlink_test.go`.

## Tests run through the gate

Running a hub's tests goes through `lym gate test` over the tiers `gate.yaml` declares, so every run takes its slots, its timeout and its log.

- Tests run only through `lym gate test` over `gate.yaml`'s tiers, apart from a card `**Verify:**` line and `cmd/testtiming`'s runs in every mode.
- A card `**Verify:**` line runs through the shared `internal/cardverify` runner, and `cmd/testtiming` measures outside the gate because its modes measure what a tier run hides.
- A site or stencil names a gate key, never a tier list, scope form, test command or target list.
- Darn's tree-scoped sites add the touched trees' targets to the key's call.
- The shell guard refuses a command with a prefix in `gate.yaml`'s `refuse_raw`, so an agent's raw test run is steered to the gate.
- The plan gate's `verify-raw-command` finding flags a card `**Verify:**` line that starts with a `refuse_raw` prefix.

## Test economy

A test exists for a behavior, not for a symbol, and no two tests cover the same behavior.
Enforcement is review discipline, not a test.
The general rules live in `scribe:testing` and `scribe:golang-testing`; this section keeps only what is project-specific or stricter here.

- A test targets behavior and contract at a module's public surface (exported API, CLI verb, file contract), not each helper; an unexported helper is tested through the surface that uses it.
- A behavior testable with a fake or an in-memory fixture is tested untagged that way, never through a real hub, so integration tests stay the exception.
- A new test function or file must cover a behavior no existing test covers.
  Otherwise the case becomes a row in an existing table-driven test, or an assertion in an existing test of that behavior.
- Many items of one kind (refusals, claims, rows, verbs) are covered by one table-driven or contract-level test, not one test each.
- No test pins a stencil's static prose.
  A stencil test may pin only a marker set, a Fill round-trip or a Go-filled marker's position, a value Go fills, reads or names, a fragment Go chooses, or a rule across a stencil family;
  review guards the wording.
- A review fix adds a test only when the finding is a coverage gap; for any other finding it corrects the existing test that asserted the wrong thing.
- A redundant test is a finding exactly as a missing one is.
  The finding names the existing test that already covers the behavior, so it stays grounded and fixable by folding or deleting.
- `go run ./cmd/testtiming -redundancy` writes the per-test coverage report, `docs/benchmarks/test-redundancy.md`.
  A run narrowed with `-pkg` rewrites only its own packages' sections of the existing report, so parallel tasks pruning different packages merge cleanly.
  A candidate in it is evidence, not a verdict: coverage blocks do not see assertions, so a candidate whose assertions differ from its covering tests is kept or folded.
  A prune finding names the covering test from the report, and only the removable set may be deleted together: deleting a candidate outside it can leave a block uncovered once its partner is gone.
  A candidate that is kept carries `//testtiming:keep <reason>` directly above its `func Test…` line, and the reason names what it pins that its covering tests do not (an error message, an output shape, an ordering); the report lists it under "Kept".
  A test in the "no coverage" list is outside the report's judgment, not exempt from the rules above.
- The "no coverage" list holds a test that is skipped, failed or covers nothing, a tier-tagged test with a call the static scan cannot resolve, and a test that may run this module's code in another process: it references `internal/testkit/lymbin`, `os.Executable`, `os.Args[0]` or `exec.Command("go", ...)` directly or through a followed call, or it sits in a `tier2` file.
  Spawning git, using `hubforge` or driving a tmux server through `tmuxkit` does not exclude a test, and an untagged test is always judged.
- The scan does not see a hook, alias or helper a test installs so that git runs module code, so the hand check before deleting a candidate also asks whether the candidate makes git run module code.
