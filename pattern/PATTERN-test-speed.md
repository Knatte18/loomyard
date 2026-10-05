# PATTERN-test-speed

Tests stay fast: Tier 1 is offline and spawns nothing, no test waits out a production duration, and tests run in parallel.

## Untagged tests spawn nothing

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub` or gitkit spawn outside `integration`- or `smoke`-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside `integration`- or `smoke`-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside `integration`- or `smoke`-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
- Enforced by `cmd/lyx/tierpurity_test.go`.

## Time is injectable

- A production interval, timeout or clock that a test would otherwise wait out is a struct field or a parameter, never a package-level `var`, so tests keep `t.Parallel`.
- Production always passes today's value; the test passes a short one.
- No test in any tier waits out a production duration.

## Tests run in parallel

- A test calls `t.Parallel` unless it touches process-global state: env, cwd or a shared fixture.
- A comment at the test, or once atop its file, names that state.
