# PATTERN-test-speed

Tests stay fast: `tier0` is offline and spawns nothing, no test waits out a production duration, and tests run in parallel.

## Four tiers

The hub declares its tiers in `gate.yaml`, numbered `tier0` through `tier3`, and `lyx gate test --tier <n>` runs one.

| Tier | Tag | Needs | Costs |
|---|---|---|---|
| `tier0` | `tier0` | nothing: offline, no spawn | seconds, runs on every card gate |
| `tier1` | `tier1` | git, a built `lyx`, subprocesses; no tmux server and no LLM, which `tmuxkit.Main` checks at the end of the package | tens of seconds, runs in the keys that list it |
| `tier2` | `tier2` | a real tmux server, no LLM | slower, runs in the keys that list it and by hand |
| `tier3` | `tier3` | a real LLM session | billed, compiled by every gate and run by hand only |

- Tags are disjoint: `-tags tier1` compiles no `tier0`, `tier2` or `tier3` test file, so each test runs in exactly one tier, and a key that wants two tiers lists both.
- A test file holding a `Test`, `Benchmark`, `Fuzz` or `Example` function carries exactly one tier tag, a platform term staying beside it (`tier2 && !windows`); an untagged test file holds support only, such as `TestMain` and helpers.
  Enforced by `cmd/lyx/tiertags_test.go`.
- A file holding a `//lyx:guard` test takes its one `tierN` tag like any other test file; `cmd/lyx/tiertags_test.go` refuses a constraint naming `integration`, `tmux` or `llm` in any test file.
- A test needing two substrates takes the higher tier.
- Each tag compiles on its own: `go vet -tags tier2 ./...` and `go vet -tags tier3 ./...` each pass.
- A helper file used by files of two tiers carries the disjunction of their tags (`//go:build tier2 || tier3`), and holds no test; a helper only one tier uses lives in that tier's file, and a file whose helpers another tier's tests use is split.
- `tier3` files are compiled by `go vet -tags tier3 ./...` and never run by a gate.

### Gate keys

A gate key in `gate.yaml` names a site's tiers and its scope, and a site runs `lyx gate test --wait --gate <key>`; no site spells a tier list, a scope or a test command.
`internal/gateslot`'s doc holds the tier plan, which recipe row reads which key.

| Key | Site | Template scope |
|---|---|---|
| `card` | each Webster card, through `--card`, then the comment lint and the headroom lint | `card` |
| `batch` | each Webster batch, each tier run failing on a tmux server socket its end-of-package check finds | `card` |
| `webster-fix` | Webster-Burler-Fix's verify fixers, after the headroom and comment lints | `diff` |
| `webster` | Webster's end gate | `all` |
| `merge-in`, `finalize` | Merge-In and Finalize | `all` |
| `darn-write`, `code-burler-fix` | darn's writer and the Code-Burler-Fix rows | `diff` |
| By hand | the `tier3` tier | |

### No gate runs `tier3`

Every gate compiles the `tier3` tier through `go vet -tags tier3`, and no gate runs it.
Running it is the operator's call, because a `tier3` run is billed, nondeterministic and needs a live session.

### Every gate run takes a slot

Every gate build or test run holds slots of the hub's gate-slot pool, so the hub never runs more at once than its configured count.
A tier run takes the slots its tier asks for, and the non-tier holders `internal/gateslot`'s doc lists each hold one.
An agent runs a module-wide or `tier2` test only through `lyx gate test`, and never as a raw `go` run.
A tagged gate run builds `lyx` once and hands it to every package through the `LYX_PREBUILT_LYX` variable, which `lyxbin` returns instead of building.
A tagged hub run exports a path named by the binary's content, and `lyx gate test` strips the trace id from `go test`'s environment, since Go keys a cached test result on every variable a test reads.
Bound: a forged value misleads only an ungated raw run, since `lyx gate test` sets or strips the variable itself.

## Only `tier3` files reach an LLM

- A test file that imports `internal/testkit/llmkit` is constrained to `tier3`, so `//go:build tier3` and `//go:build tier3 && linux` pass while `//go:build tier2 || tier3`, `tier1` and an untagged file fail.
- A test file outside `internal/testkit/llmkit` never calls `exec.LookPath` with an LLM binary name, as a string literal or a same-file constant; it calls `llmkit.Claude`.
- A file under `internal/testkit/llmkit` uses no identifier of `os/exec` but `LookPath`, which the Testkit import check cannot see.
- Enforced by `cmd/lyx/llmtier_test.go`.
- Bound: the guard sees the static shape only.
  A `tier2` test that drives `lyx` into spawning an LLM without calling `llmkit` is caught by review.

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub`, `hubforge.CopyHub`, `hubforge.SharedHub` or gitkit spawn outside tier-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside tier-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside tier-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
  That covers the `/proc` probes although they spawn nothing, which costs nothing because an untagged test has no process to probe.
- A file referencing a `tmuxkit` spawn is further constrained to the `tier2` tier: it compiles in neither the untagged nor the `tier1` build on any platform, so `tier2`, `tier3`, `tier2 || tier3` and `tier2 && !windows` pass while `tier1 || tier2` fails.
  An allowlist entry with a reason admits a tier-2 file that uses only the spawn-free `/proc` probes.
  The scan evaluates the build line under every assignment of its tags other than `tier2` and `tier3`, so the host's GOOS plays no part.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
- Enforced by `cmd/lyx/tierpurity_test.go`.

## Repository-scanning tests carry `//lyx:guard`

- A test that reads repository files outside its own package directory carries `//lyx:guard` on the line directly above its `func Test…` line, the way `//testtiming:keep` sits above a kept test.
- A narrowed gate's guard step runs every marked test by name under its file's tier, whatever the impacted set, because a guard's result depends on files no import edge names.
- A test carrying both markers stacks them as contiguous directive lines directly above the `func Test…` line, in either order.
  `//lyx:guard` counts as directly above when only `//testtiming:keep` lines separate it from the func line, and `//testtiming:keep` counts as directly above when only `//lyx:` lines separate it; two stacked keeps stay an error.
- A test that reads only its own package directory is left unmarked, since the impacted set selects it whenever that package changes.
- No marked test sits in a `tier2` or `tier3` file.
- Whether a scanning test carries the marker is review discipline, not a test.
- Bound: an unmarked scanning test is not run by the round gate or darn's writer gates, so a violation passes those and is caught by the whole verify at the sites that keep it for a diff touching production code, never on `main`.
  For a tree-scoped darn diff, a site runs its row's key over the trees' targets, so such a violation can reach the parent, bounded by what the hub lists as non-production in `darn.yaml`'s `tree_scope`.

## Time is injectable

- A production interval, timeout or clock that a test would otherwise wait out is a struct field or a parameter, never a package-level `var`, so tests keep `t.Parallel`.
- Production always passes today's value; the test passes a short one.
- No test in any tier waits out a production duration.

## Tests run in parallel

- A test calls `t.Parallel` unless it touches process-global state: env, cwd or a shared fixture.
- A comment at the test, or once atop its file, names that state.

## Tree-sitter links

- Linking tree-sitter slows a test binary's link, so only the packages that call the code index for real may do it.
- `treeSitterAllowedLinks` in `cmd/lyx/treesitterlink_integration_test.go` lists those packages, each with its reason.
- The `tier1` guard runs `go list -test -deps` under every tag and fails, naming one import chain, when a test binary outside the list links `github.com/tree-sitter/go-tree-sitter`.
- It also fails when a listed package's test binary no longer links it, so the list tracks the tree.
- The chain search and the comparison are in `cmd/lyx/treesitterlink_test.go`.
