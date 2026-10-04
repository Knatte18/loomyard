# PATTERN-test-tier-purity

Untagged test files perform no expensive spawns; Tier 1 stays offline and fast.

- No `gitexec.Run` or `RunGit`, `exec.Command` or `CommandContext`, `hubforge.NewHub` or gitkit spawn outside `integration`- or `smoke`-tagged files.
- Every `gitkit` export except `gitkit.HermeticGitEnv` counts as a gitkit spawn, defined once in `cmd/lyx/gitkitspawn_test.go`.
- Any `lyxbin.` reference, which builds the `lyx` binary, is likewise barred outside `integration`- or `smoke`-tagged files.
- Every `tmuxkit` export except `tmuxkit.Main` counts as a tmux spawn and is barred outside `integration`- or `smoke`-tagged files, defined once in `cmd/lyx/tmuxkitspawn_test.go`.
- `time.Sleep(...)` of one second or more in an untagged file is flagged unless allowlisted.
