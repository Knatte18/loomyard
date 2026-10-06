# PATTERN-test-isolation

A test package never reaches the operator's global gitconfig or default tmux socket directory.

## Hermetic git

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted (`internal/proc`).
- A package is git-spawning when a test file references a gitkit spawn, by the same definition `PATTERN-test-speed` uses.
- Enforced by `cmd/lyx/hermeticenv_test.go`.

## tmux isolation

- Every test package with an `integration`-, `tmux`- or `llm`-tagged test file runs its tests through `tmuxkit.Main`, so no test touches the operator's default tmux socket directory.
- Under every tag set (untagged, each of `integration`, `tmux` and `llm` alone, and the three together) and on every platform that compile any of the package's test files, a file declaring `TestMain` compiles, and every `TestMain` in the package calls `tmuxkit.Main`.
- The rule keys on "has a tagged test file", not "starts tmux": starting tmux has no static shape, so a tagged package that never starts tmux still carries the one call, which costs one temp directory.
- In an untagged run the entry point spawns nothing.
- A package whose `TestMain` cannot call the entry point is admitted by an allowlist entry whose reason names that obstacle; "does not start tmux" is not a reason.
- Enforced by `cmd/lyx/tmuxisolation_test.go`.

## tmux socket files

- A test that starts a tmux server takes its `-L` key from `tmuxkit`: `Socket` mints one, and `KillOnCleanup` covers a key the test did not mint, such as a `reedengine.ServerName` key of a fixture hub.
- Both run `kill-server` at cleanup and then remove that key's socket file, so a run leaves none behind.
  The removal touches only the one path for the key, under the current `TMUX_TMPDIR`'s per-user directory, and only a socket that refuses connections.
- A test that builds a subprocess environment from scratch passes `TMUX_TMPDIR` through, so the subprocess's tmux lands in the isolated directory.
- Bound: socket files already in `/tmp/tmux-$UID/` from earlier runs are not cleaned.
