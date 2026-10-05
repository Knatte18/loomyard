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

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub` or gitkit spawn outside tier-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside tier-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside tier-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
- Enforced by `cmd/lyx/tierpurity_test.go`.

## Time is injectable

- A production interval, timeout or clock that a test would otherwise wait out is a struct field or a parameter, never a package-level `var`, so tests keep `t.Parallel`.
- Production always passes today's value; the test passes a short one.
- No test in any tier waits out a production duration.

## Tests run in parallel

- A test calls `t.Parallel` unless it touches process-global state: env, cwd or a shared fixture.
- A comment at the test, or once atop its file, names that state.
