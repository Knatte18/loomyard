# PATTERN-test-isolation

A test package never reaches the operator's global gitconfig or default tmux socket directory.

## Hermetic git

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted (`internal/proc`).
- A package is git-spawning when a test file references a gitkit spawn, by the same definition `PATTERN-test-speed` uses.
- Enforced by `cmd/lyx/hermeticenv_test.go`.

## tmux isolation

- Every test package with an `integration`- or `smoke`-tagged test file runs its tests through `tmuxkit.Main`, so no test touches the operator's default tmux socket directory.
- Under every tag set (untagged, `integration`, `smoke`) and on every platform that compile any of the package's test files, a file declaring `TestMain` compiles, and every `TestMain` in the package calls `tmuxkit.Main`.
- The rule keys on "has a tagged test file", not "starts tmux": starting tmux has no static shape, so a tagged package that never starts tmux still carries the one call, which costs one temp directory.
- In an untagged run the entry point spawns nothing.
- A package whose `TestMain` cannot call the entry point is admitted by an allowlist entry whose reason names that obstacle; "does not start tmux" is not a reason.
- Enforced by `cmd/lyx/tmuxisolation_test.go`.
