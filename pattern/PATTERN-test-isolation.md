# PATTERN-test-isolation

A test package never reaches the operator's global gitconfig or default tmux socket directory.

## Hermetic git

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted (`internal/proc`).
- A package is git-spawning when a test file references a gitkit spawn, by the same definition `PATTERN-test-speed` uses.
- Enforced by `cmd/lyx/hermeticenv_test.go`.

## tmux isolation

- Every test package runs its tests through `tmuxkit.Main`, so no test touches the operator's default tmux socket directory and its `t.TempDir` trees live in the process run directory.
- Under every tag set (untagged, each of `tier0` through `tier3` alone, and all four together) and on every platform that compile any of the package's test files, a file declaring `TestMain` compiles, and every `TestMain` in the package calls `tmuxkit.Main`.
- The rule covers every test package, not only those that start tmux: starting tmux has no static shape, so every package carries the one call, which costs one run directory per test binary.
- In an untagged run the entry point spawns nothing.
- A package whose `TestMain` cannot call the entry point is admitted by an allowlist entry whose reason names that obstacle; "does not start tmux" is not a reason.
  That `TestMain` removes the process run directory after `m.Run` through `lyxtemp.Remove`.
  The entry lets through the package's tmux isolation and its `t.TempDir` trees left in the bare system temp directory after a kill.
- Enforced by `cmd/lyx/tmuxisolation_test.go`.

## tmux socket files

- A test that starts a tmux server takes its `-L` key from `tmuxkit`: `Socket` mints one, and `KillOnCleanup` covers a key the test did not mint, such as a `reedengine.ServerName` key of a fixture hub.
- `PackageServer` returns one key per test binary, with a hermetic server started on first use and no cleanup of its own, because `Main`'s sweep kills every server under the kit's directory.
  A test takes it only when it names its own sessions, kills no server and asserts nothing about the server's whole session set; a test that drives reed through a hub keeps the hub's `reedengine.ServerName` key, and a test that runs `Down` or `kill-server`, or compares `list-sessions` to an exact set, keeps `Socket`.
- A `tier2` or `tier3` test that boots reed registers its key through `KillOnCleanup` where the key is known and before the first `Up`, `EnsureSession`, `Resume` or `lyx` run that arms reed, so reed's boot finds the hermetic server.
  The registration sits in the `tier2`- or `tier3`-constrained file or a helper only such files compile, never in a helper a `tier1` build shares, since that would start a server in tier 1.
  A test that asserts on the log files of a server reed starts itself registers its key after its first boot instead.
- Both `Socket` and `KillOnCleanup` run `kill-server` at cleanup and then remove that key's socket file, so a run leaves none behind.
  Every kit kill of a server, at cleanup or in `Main`'s sweep, then kills each process still in a session one of its panes led, because the hangup `kill-server` sends misses a pane child that ignores SIGHUP or sits outside the terminal's foreground.
  The removal touches only the one path for the key, under the current `TMUX_TMPDIR`'s per-user directory, and only a socket that refuses connections.
- A test that builds a subprocess environment from scratch passes `TMUX_TMPDIR` through, so the subprocess's tmux lands in the isolated directory.
- Bound: socket files already in `/tmp/tmux-$UID/` from earlier runs are not cleaned.

## Private temp directory and leftovers

- `tmuxkit.Main` runs in the process run directory of `internal/lyxtemp` and points `TMPDIR` (and `TMP` and `TEMP` on Windows) at it, as it does `TMUX_TMPDIR`, so every `t.TempDir`, `os.MkdirTemp("")`, `lyxbin` build and child temp file of a test package lands there.
  `Main` also sets `GIT_CEILING_DIRECTORIES` to that directory on every platform, so neither a repository-root walk nor a spawned git from a temp fixture climbs above it.
  A test binary re-executed as a helper inherits that directory, and its own run directory is made beside it by the temp package's beside-the-parent rule rather than inside it, so nesting never pushes a socket path past the `sun_path` limit.
- After the run `Main` removes the run directory on every platform, Windows included.
  A removal that fails, such as one a lingering child's cwd blocks on Windows, is reported on stderr and leaves the exit code unchanged.
  A binary killed by a panic or timeout skips the removal, and the next start's sweep of `internal/lyxtemp` removes its directory once the OS has released its lock.
- After the run `Main` sweeps the servers and, on Linux, scans `/proc` for any process whose cwd, executable or an argv element references that directory, waiting up to two seconds for it to exit on its own.
  A leftover is killed and fails the package with exit 1, naming its pid, cwd and argv on stderr; a `lyx reed watchdog` daemon is killed without failing the package, because it idles out on its own schedule.
  Off Linux the scan is skipped.
- The probes behind the scan are kit exports, `Pids`, `ProcArgv`, `ProcCwd`, `ProcExe` and `IsWatchdog`; they read files only and start nothing.
- Before the sweep, `Main` checks the sockets under its directory's per-user directory: in a test binary built without the `tier2` and `tier3` tags every socket is a finding, and in one built with either only a socket whose key no test registered through `Socket`, `KillOnCleanup` or `PackageServer` is.
  A finding fails the package with exit 1, naming the key and the way forward: tag the file `tier2`, or register the key.
  The check runs before the sweep because the sweep removes the socket files it reads.
- The per-user directory `tmux-<uid>` is no finding by itself: tmux creates it on any invocation that names a `-L` key, a client probe of a server that never started included, and a client probe starts no server.
- Bound: the check misses a test that points `TMUX_TMPDIR` elsewhere and a server started by a binary other than tmux.
  It also misses a server that reed tore down itself within the run, since reed removes its server's socket file, in tier 2 and under any tag alike.
- Bound: a test binary killed by a panic or timeout skips the post-run check.

## Hermetic servers

- `Socket` and `KillOnCleanup` pre-start a server on the key before returning, from `tmux -f <config> -L <key> start-server`, so no pre-started server reads `~/.tmux.conf`.
  The kit writes the config once per test binary, with four options: `default-shell` and an equal `default-command` (a non-empty `default-command` makes tmux start a pane's shell without the login flag, so the operator's rc files still run and the login profile does not), `exit-empty off` (the server outlives having no session) and `@lyx_test_server on`.
- A server reed starts sources the file `tmux_conf` names, which `tmuxkit.Main` empties by unsetting `LYX_REED_TMUX_CONF`, so a test that exercises it sets the key to a file it wrote and registers its key after reed's first boot.
- The pre-start applies the environment hygiene reed's own server spawn applies: the server's environment is the test process's less `CLAUDECODE`, `CLAUDE_CODE_*`, `LYX_TRACE_ID`, `LYX_STRAND_NAME` and `LYX_PARENT`, because every session and pane on the key inherits from that server.
  The Claude filter is a copy of the scrub the engines declare, which the kit cannot import because reed's own tests import the kit; `shuttleengines`' `TestEnvScrub_MatchesTmuxkitServerEnviron` pins `tmuxkit.ServerEnviron` to that scrub.
- Reed's `sessionlessSocketHolderPersists` is the one production accommodation: before its grace loop it asks the holder `show-options -gqv @lyx_test_server`, and a holder answering `on` is never stale.
  Without it, reed would reap the session-less pre-started server after `staleSocketGrace` and replace it with a config-less one.
  Bound: only the kit's config sets the option, and reed's own boots never do; an operator config that sets it disables the reap for its own servers, and a server that sets `exit-empty off` without it is reaped as before.
- A server reed starts itself reads no `~/.tmux.conf` either: its spawn and its capability probe carry `-f /dev/null` on every platform but Windows, and reed pins `default-shell` and an equal `default-command` on each session, so strand panes start non-login shells.
  The kit's server differs from reed's own only by `exit-empty off` and the `@lyx_test_server` marker, which reed's boots never set.
  Reed's server is started by a test that registers its key after reed's boot, and by every boot after a test's own `down` or `kill-server` on its key, as those tests pin reed's own server start, teardown or recovery, which a pre-started server would bypass.
  The pre-started server runs without reed's `-v` debug flags and with the kit's directory as its cwd, so it writes no `tmux-server-<pid>.log` into the hub logs dir; a test that asserts on those artefacts registers its key after reed's boot.

## Testkit

Shared test support — fakes, builders, fixtures and the scan harness — used by two or more packages is shared through one kit package under `internal/testkit/<kit>/`, never duplicated per package and never placed under the package it fakes.

- No non-test file outside `internal/testkit/` imports a path under it, so every kit is reachable only from tests.
  Files under `internal/testkit/` are exempt, so a kit may build on another kit.
- No non-test file under `internal/testkit/` imports an `internal/*cli` package.
- No non-test file under `internal/testkit/` imports `os/exec`, `internal/gitexec`, `internal/gitkit`, `internal/hubforge`, `internal/testkit/lyxbin`, `internal/testkit/tmuxkit` or `internal/testkit/llmkit`.
  - `internal/testkit/lyxbin` is exempt from the `os/exec` ban alone, bounded to `go build` of `./cmd/lyx` and running that binary under group kill.
    It builds at most once per test binary and per ldflags, or returns the binary `gateslot.PrebuiltLyxEnv` names, which a gate run builds once for every package of a tagged run.
    Banning its import keeps the kit-on-kit exemption from handing another kit a transitive `go build`.
  - `internal/testkit/tmuxkit` is the second exemption from the `os/exec` ban alone, bounded to running the `tmux` binary against sockets under its own directory or its own fixture keys, to starting servers under the kit's own config, and to the post-run `/proc` scan that kills a leftover process.
    No other kit imports it, for the same reason.
  - `internal/testkit/llmkit` is the third exemption from the `os/exec` ban alone, bounded to `exec.LookPath` for the `claude` binary; it starts nothing.
    Only `tier3`-tagged test files import it, and no other kit does, for the same reason.
- A kit imports only the lowest packages defining the types it fakes.
  A package an import cycle bars from a kit keeps exactly one local copy, and a fixture used by one package stays in that package's `_test.go` files.
- `internal/testkit/scankit` imports the standard library only, so every package's tests can import it.

### Enforcement

`internal/testkit/enforcement_test.go` enforces the import rules.
Review discipline covers the rest: a kit starts no process, tmux server or agent beyond what its imports allow.
A kit asserts nothing beyond `t.Fatalf` on its own setup, the `shedfake` `Call` and `RequireOutcome` outcome check, and the `envelope` `RequireOK` and `RequireErr` shape check.
`scankit` additionally asserts scan results, while `plankit` and `stencilkit` assert nothing beyond `t.Fatalf` on their own setup.

## Hub fixtures

Every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
No hub is hand-assembled.

- No package in `fabriccli`'s dependency set may import `hubforge`.
- A copy from `hubforge.CopyHub` is a relocation of a hub `fabriccli.CloneAndWire` built into a per-binary template, never a hub assembled another way; `SharedHub` hands out that template itself to a test that only reads.
- No test package wraps `hubforge` in its own fixture type; a test takes a `*hubforge.Hub` and reads `h.Topology`, `AddPairWith` and `OpenFabric` directly.
  Enforcement of this clause is review discipline, not a scan.
