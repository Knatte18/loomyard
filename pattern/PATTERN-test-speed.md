# PATTERN-test-speed

Tests stay fast: Tier 1 is offline and spawns nothing, no test waits out a production duration, and tests run in parallel.

## Four tiers

| Tier | Tag | Needs | Costs |
|---|---|---|---|
| 1 | none | nothing: offline, no spawn | seconds, runs on every `go test ./...` |
| 2 | `integration` | git, a built `lyx`, subprocesses; no tmux server and no LLM | tens of seconds, runs under the card gates |
| 3 | `tmux` | a real tmux server, no LLM | slower, runs under the plan verify and by hand |
| 4 | `llm` | a real LLM session | billed, run by hand only |

- Tags do not nest: `-tags integration` runs no `tmux` or `llm` file, so a run names every tag it wants.
- A test needing two substrates takes the higher tier.
- Each tag compiles on its own: `go vet -tags tmux ./...` and `go vet -tags llm ./...` each pass.
- A helper file used by files of two tiers and spawning something carries the disjunction of their tags (`//go:build tmux || llm`); a helper that spawns nothing goes untagged; a helper only one tier uses lives in that tier's file.
- `llm` files are compiled by `go vet -tags llm ./...` and never run by a gate.

## Only `llm` files reach an LLM

- A test file that imports `internal/testkit/llmkit` is constrained to `llm`, so `//go:build llm` and `//go:build llm && linux` pass while `//go:build tmux || llm`, `integration` and an untagged file fail.
- A test file outside `internal/testkit/llmkit` never calls `exec.LookPath` with an LLM binary name, as a string literal or a same-file constant; it calls `llmkit.Claude`.
- A file under `internal/testkit/llmkit` uses no identifier of `os/exec` but `LookPath`, which the Testkit import check cannot see.
- Enforced by `cmd/lyx/llmtier_test.go`.
- Bound: the guard sees the static shape only.
  A `tmux`-tier test that drives `lyx` into spawning an LLM without calling `llmkit` is caught by review.

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub` or gitkit spawn outside tier-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside tier-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside tier-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
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
- Bound: an unmarked scanning test is not run by the round gate, so a violation passes that round and is caught by the full plan verify at Publish or Finalize, never on `main`.

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
