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
- `PackageServer` returns one key per test binary, with a hermetic server started on first use and no cleanup of its own, because `Main`'s sweep kills every server under the kit's directory.
  A test takes it only when it names its own sessions, kills no server and asserts nothing about the server's whole session set; a test that drives reed through a hub keeps the hub's `reedengine.ServerName` key, and a test that runs `Down` or `kill-server`, or compares `list-sessions` to an exact set, keeps `Socket`.
- A `tmux`- or `llm`-tier test that boots reed registers its key through `KillOnCleanup` where the key is known and before the first `Up`, `EnsureSession`, `Resume` or `lyx` run that arms reed, so reed's boot finds the hermetic server.
  The registration sits in the `tmux`- or `llm`-constrained file or a helper only such files compile, never in a helper an `integration` build shares, since that would start a server in tier 2.
  A test that asserts on the log files of a server reed starts itself registers its key after its first boot instead.
- Both `Socket` and `KillOnCleanup` run `kill-server` at cleanup and then remove that key's socket file, so a run leaves none behind.
  The removal touches only the one path for the key, under the current `TMUX_TMPDIR`'s per-user directory, and only a socket that refuses connections.
- A test that builds a subprocess environment from scratch passes `TMUX_TMPDIR` through, so the subprocess's tmux lands in the isolated directory.
- Bound: socket files already in `/tmp/tmux-$UID/` from earlier runs are not cleaned.

## Hermetic servers

- `Socket` and `KillOnCleanup` pre-start a server on the key before returning, from `tmux -f <config> -L <key> start-server`, so no pre-started server reads `~/.tmux.conf`.
  The kit writes the config once per test binary, with four options: `default-shell` and an equal `default-command` (a non-empty `default-command` makes tmux start a pane's shell without the login flag, so the operator's rc files still run and the login profile does not), `exit-empty off` (the server outlives having no session) and `@lyx_test_server on`.
- The pre-start applies the environment hygiene reed's own server spawn applies: the server's environment is the test process's less `CLAUDECODE`, `CLAUDE_CODE_*`, `LYX_TRACE_ID`, `LYX_STRAND_NAME` and `LYX_PARENT`, because every session and pane on the key inherits from that server.
  The Claude filter is a copy of `reedengine.CleanClaudeEnv`, which the kit cannot import because reed's own tests import the kit; a `tmuxkit` test pins the two together.
- Reed's `sessionlessSocketHolderPersists` is the one production accommodation: before its grace loop it asks the holder `show-options -gqv @lyx_test_server`, and a holder answering `on` is never stale.
  Without it, reed would reap the session-less pre-started server after `staleSocketGrace` and replace it with a config-less one.
  Bound: only the kit's config sets the option, and reed's own boots never do; an operator config that sets it disables the reap for its own servers, and a server that sets `exit-empty off` without it is reaped as before.
- Bound: a server reed starts itself carries no `-f`, so it reads the operator's `~/.tmux.conf` and its strand panes start login shells.
  That covers a test that registers its key after reed's boot, and every boot after a test's own `down` or `kill-server` on its key, as those tests pin reed's own server start, teardown or recovery, which a pre-started server would bypass.
  The pre-started server also runs without reed's `-v` debug flags and with the kit's directory as its cwd, so it writes no `tmux-server-<pid>.log` into the hub logs dir; a test that asserts on those artefacts registers its key after reed's boot.
