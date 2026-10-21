# PATTERN-test-speed

Tests stay fast: `tier0` is offline and spawns nothing, no test waits out a production duration, and tests run in parallel.

## Four tiers

The hub declares its tiers in `gate.yaml`, numbered `tier0` through `tier3`, and `lyx gate test --tier <n>` runs one.

| Tier | Tag | Needs | Costs |
|---|---|---|---|
| `tier0` | none | nothing: offline, no spawn | seconds, runs on every card gate |
| `tier1` | `integration` | git, a built `lyx`, subprocesses; no tmux server and no LLM, which `tmuxkit.Main` checks at the end of the package | tens of seconds, runs once per batch and in the final gate |
| `tier2` | `tmux` | a real tmux server, no LLM | slower, runs in the `publish` gate, at the round gate while a Publish failure record is present, and by hand |
| `tier3` | `llm` | a real LLM session | billed, compiled by every gate and run by hand only |

- Tags do not nest: `-tags integration` runs no `tmux` or `llm` file, so a tier's command names every tag it wants.
- A test needing two substrates takes the higher tier.
- Each tag compiles on its own: `go vet -tags tmux ./...` and `go vet -tags llm ./...` each pass.
- A helper file used by files of two tiers and spawning something carries the disjunction of their tags (`//go:build tmux || llm`); a helper that spawns nothing goes untagged; a helper only one tier uses lives in that tier's file.
- `llm` files are compiled by `go vet -tags llm ./...` and never run by a gate.

### Gate per tier

A gate is a named list of tiers in `gate.yaml`, and a site runs its gate's list through `lyx gate test`; no site spells a test command.

| Gate | Runs |
|---|---|
| `card` | the tiers the `card` list names over the card's own packages through `lyx gate test --card`, then the comment lint and the headroom lint |
| `batch` | the `batch` list once per batch over the batch's packages, each failing on a tmux server socket its end-of-package check finds |
| `round` | the Webster-Burler-Fix round gate: the headroom lint when told a merge base, then the comment lint, then the `round` list over the impacted set, which runs whole wherever the impacted set cannot narrow |
| `round`, while a Publish failure record is present | the failing tests the record names before the round list, and for a `publish` failure an impacted-set `tmux` pass after it, to confirm the fix |
| `final`, `landing` | webster's gate, Merge-In and Finalize: the `final` and `landing` lists over the whole tree |
| `publish` | Publish after the `landing` gate, where the repo's list is not empty: the `tmux` tier |
| By hand | the `llm` tier |

### No gate runs `llm`

Every gate compiles the `llm` tier through `go vet -tags llm`, and no gate runs it.
Running it is the operator's call, because an `llm` run is billed, nondeterministic and needs a live session.

### Every gate run takes a slot

Every gate build or test run holds slots of the hub's gate-slot pool, so the hub never runs more at once than its configured count.
A tier run takes the slots its tier asks for, and the non-tier holders `internal/gateslot`'s doc lists each hold one.
An agent runs a module-wide or `tmux`-tier test only through `lyx gate test`, and never as a raw `go` run.
A tagged gate run builds `lyx` once and hands it to every package through the `LYX_PREBUILT_LYX` variable, which `lyxbin` returns instead of building.
A tagged hub run exports a path named by the binary's content, and `lyx gate test` strips the trace id from `go test`'s environment, since Go keys a cached test result on every variable a test reads.
Bound: a forged value misleads only an ungated raw run, since `lyx gate test` sets or strips the variable itself.

## Only `llm` files reach an LLM

- A test file that imports `internal/testkit/llmkit` is constrained to `llm`, so `//go:build llm` and `//go:build llm && linux` pass while `//go:build tmux || llm`, `integration` and an untagged file fail.
- A test file outside `internal/testkit/llmkit` never calls `exec.LookPath` with an LLM binary name, as a string literal or a same-file constant; it calls `llmkit.Claude`.
- A file under `internal/testkit/llmkit` uses no identifier of `os/exec` but `LookPath`, which the Testkit import check cannot see.
- Enforced by `cmd/lyx/llmtier_test.go`.
- Bound: the guard sees the static shape only.
  A `tmux`-tier test that drives `lyx` into spawning an LLM without calling `llmkit` is caught by review.

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub`, `hubforge.CopyHub`, `hubforge.SharedHub` or gitkit spawn outside tier-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside tier-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside tier-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
  That covers the `/proc` probes although they spawn nothing, which costs nothing because an untagged test has no process to probe.
- A file referencing a `tmuxkit` spawn is further constrained to the `tmux` tier: it compiles in neither the untagged nor the `integration` build on any platform, so `tmux`, `llm`, `tmux || llm` and `tmux && !windows` pass while `integration || tmux` fails.
  An allowlist entry with a reason admits a tier-2 file that uses only the spawn-free `/proc` probes.
  The scan evaluates the build line under every assignment of its tags other than `tmux` and `llm`, so the host's GOOS plays no part.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
- Enforced by `cmd/lyx/tierpurity_test.go`.

## Repository-scanning tests carry `//lyx:guard`

- A test that reads repository files outside its own package directory carries `//lyx:guard` on the line directly above its `func Test…` line, the way `//testtiming:keep` sits above a kept test.
- The round gate runs every marked test by name whatever the impacted set, because a guard's result depends on files no import edge names.
- A test carrying both markers stacks them as contiguous directive lines directly above the `func Test…` line, in either order.
  `//lyx:guard` counts as directly above when only `//testtiming:keep` lines separate it from the func line, and `//testtiming:keep` counts as directly above when only `//lyx:` lines separate it; two stacked keeps stay an error.
- A test that reads only its own package directory is left unmarked, since the impacted set selects it whenever that package changes.
- No marked test sits in a `tmux` or `llm` file.
- Whether a scanning test carries the marker is review discipline, not a test.
- Bound: an unmarked scanning test is not run by the round gate or darn's writer gates, so a violation passes those and is caught by the whole verify at the sites that keep it for a diff touching production code, never on `main`.
  For a tree-scoped darn diff such a violation can reach the parent, bounded by what the hub lists as non-production in `darn.yaml`'s `tree_scope`.

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
- The `integration`-tier guard runs `go list -test -deps` under every tag and fails, naming one import chain, when a test binary outside the list links `github.com/tree-sitter/go-tree-sitter`.
- It also fails when a listed package's test binary no longer links it, so the list tracks the tree.
- The chain search and the comparison are in `cmd/lyx/treesitterlink_test.go`.
