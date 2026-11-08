# PATTERN-test-isolation

A test package never reaches the operator's global gitconfig or default tmux socket directory.

## Hermetic git

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or the package is allowlisted (`internal/proc`).
- A package is git-spawning when a test file references a gitkit spawn, by the same definition `PATTERN-test-speed` uses.
- Enforced by `cmd/lym/hermeticenv_test.go`.

## tmux isolation

- Every test package runs its tests through `tmuxkit.Main`,
  so no test touches the operator's default tmux socket directory;
  its `t.TempDir` trees live in the process run directory.
- On every platform that compiles any of the package's test files, a file declaring `TestMain` compiles,
  and every `TestMain` in the package calls `tmuxkit.Main`.
- The rule covers every test package, not only those that start tmux:
  starting tmux has no static shape, so every package carries the one call;
  it costs one run directory per test binary.
- In a `tier0` run the entry point spawns nothing.
- The entry point exits 2 without running a test when the binary's first argument is not a flag,
  so a command that names a test binary in place of `lym` never runs its suite.
  A `TestMain` that serves helper roles of its own dispatches them before it calls the entry point.
- A package whose `TestMain` cannot call the entry point is admitted by an allowlist entry.
  The entry's reason names that obstacle; "does not start tmux" is not a reason.
  That `TestMain` removes the process run directory after `m.Run` through `lymtemp.Remove`.
  The entry lets through two lapses:
  the package's tmux isolation, and its `t.TempDir` trees left in the bare system temp directory after a kill.
- Enforced by `cmd/lym/tmuxisolation_test.go`.

## tmux socket files

- A test that starts a tmux server takes its `-L` key from `tmuxkit`:
  `Socket` mints one,
  and `KillOnCleanup` covers a key the test did not mint, such as a `reedengine.ServerName` key of a fixture hub.
- `PackageServer` returns one key per test binary, with a hermetic server started on first use.
  It has no cleanup of its own, because `Main`'s sweep kills every server under the kit's directory.
  A test takes it only when it names its own sessions and kills no server.
  The test must also assert nothing about the server's whole session set.
  A test that drives reed through a hub keeps the hub's `reedengine.ServerName` key;
  a test that runs `Down` or `kill-server`, or compares `list-sessions` to an exact set, keeps `Socket`.
- A `tier2` or `tier3` test that boots reed registers its key through `KillOnCleanup`.
  It registers as soon as the key is known, before the first `Up`, `EnsureSession`, `Resume` or `lym` run that arms reed,
  so reed's boot finds the hermetic server.
  The registration sits in the `tier2` or `tier3` test or a helper only such tests call.
  It never sits in a helper a `tier1` test calls, since that would start a server in tier 1.
  A test that asserts on the log files of a server reed starts itself registers its key after its first boot instead.
- Both `Socket` and `KillOnCleanup` run `kill-server` at cleanup and then remove that key's socket file,
  so a run leaves none behind.
  Every kit kill of a server, at cleanup or in `Main`'s sweep,
  then kills each process still in a session one of its panes led.
  The hangup `kill-server` sends misses a pane child that ignores SIGHUP or sits outside the terminal's foreground.
  The removal touches one path only: the key's socket under the current `TMUX_TMPDIR`'s per-user directory.
  It removes the socket only when it refuses connections.
- A test that builds a subprocess environment from scratch passes `TMUX_TMPDIR` through,
  so the subprocess's tmux lands in the isolated directory.
- Bound: socket files already in `/tmp/tmux-$UID/` from earlier runs are not cleaned.

## Private temp directory and leftovers

- `tmuxkit.Main` runs in the process run directory of `internal/lymtemp`.
  It points `TMPDIR` (and `TMP` and `TEMP` on Windows) at that directory, as it does `TMUX_TMPDIR`,
  so every `t.TempDir`, `os.MkdirTemp("")`, `lymbin` build and child temp file of a test package lands there.
  `Main` also sets `GIT_CEILING_DIRECTORIES` to that directory on every platform,
  so neither a repository-root walk nor a spawned git from a temp fixture climbs above it.
  `Main` also points `XDG_STATE_HOME`, and on Windows `LOCALAPPDATA`, at that directory,
  so a test, or a `lym creel daemon` it spawns, never writes the operator's per-user state directory;
  a test that sets the variable itself wins.
  A test binary re-executed as a helper inherits that directory,
  and its own run directory is made beside it by the temp package's beside-the-parent rule rather than inside it,
  so nesting never pushes a socket path past the `sun_path` limit.
- After the run `Main` removes the run directory on every platform, Windows included.
  A failed removal is reported on stderr and leaves the exit code unchanged;
  a lingering child's cwd blocking it on Windows is one such failure.
  A binary killed by a panic or timeout skips the removal,
  and the next start's sweep of `internal/lymtemp` removes its directory once the OS has released its lock.
- After the run `Main` sweeps the servers.
  On Linux it then scans `/proc` for any process whose cwd, executable or an argv element references that directory.
  It waits up to two seconds for such a process to exit on its own.
  A leftover is killed and fails the package with exit 1, naming its pid, cwd and argv on stderr;
  a `lym reed watchdog` daemon is killed without failing the package, because it idles out on its own schedule.
  Off Linux the scan is skipped.
- The probes behind the scan are kit exports: `Pids`, `ProcArgv`, `ProcCwd`, `ProcExe` and `IsWatchdog`.
  They read files only and start nothing.
- Before the sweep, `Main` checks the sockets under its directory's per-user directory:
  in a run whose `LYM_TEST_TIER` selection holds neither `tier2` nor `tier3`, every socket is a finding;
  in a run holding either, only a socket with an unregistered key is a finding.
  A test registers a key through `Socket`, `KillOnCleanup` or `PackageServer`.
  A finding fails the package with exit 1, naming the key and the way forward:
  make the test a `tier2` test, or register the key.
  The check runs before the sweep because the sweep removes the socket files it reads.
- The per-user directory `tmux-<uid>` is no finding by itself:
  tmux creates it on any invocation that names a `-L` key, including a client probe of a server that never started,
  and a client probe starts no server.
- Bound: the check misses a test that points `TMUX_TMPDIR` elsewhere and a server started by a binary other than tmux.
  It also misses a server that reed tore down itself within the run, since reed removes its server's socket file.
  That holds in every tier.
- After the run `Main` fails the package with exit 1 for a `.git` that appeared during its run in one of two places:
  directly in the lym temp root, or directly in the system temp directory above it.
  Both are shared by every test binary on the machine,
  so such an entry makes go's VCS stamping and git's discovery read everything below it as one repository.
  A test writes only under its own `t.TempDir`; the finding names the path.
  `Main` removes the entry when it is an empty directory and asks for its removal by hand otherwise.
  Bound: a concurrently running package fails for the same entry, since the check cannot tell which binary wrote it.
  An entry present before the run is no finding.
- Bound: a test binary killed by a panic or timeout skips the post-run check.

## Hermetic servers

- `Socket` and `KillOnCleanup` pre-start a server on the key before returning.
  The server starts from `tmux -f <config> -L <key> start-server`, so no pre-started server reads `~/.tmux.conf`.
  The kit writes the config once per test binary, with these options:
  - `default-shell` and an equal `default-command`:
    a non-empty `default-command` makes tmux start a pane's shell without the login flag,
    so the operator's rc files still run and the login profile does not.
  - `exit-empty off`, so the server outlives having no session.
  - `@lym_test_server on`.
- A server reed starts sources the file `tmux_conf` names.
  `tmuxkit.Main` empties that setting by unsetting `LYM_REED_TMUX_CONF`,
  so a test that exercises it sets `LYM_REED_TMUX_CONF` to a file it wrote and registers its key after reed's first boot.
- The pre-start applies the environment hygiene reed's own server spawn applies:
  the server's environment is the test process's, less these variables:
  `CLAUDECODE`, `CLAUDE_CODE_*`, `LYM_TRACE_ID`, `LYM_STRAND_NAME` and `LYM_PARENT`.
  Every session and pane on the key inherits from that server.
  The Claude filter copies the scrub the engines declare.
  The kit cannot import that scrub, because reed's own tests import the kit.
  `shuttleengines`' `TestEnvScrub_MatchesTmuxkitServerEnviron` pins `tmuxkit.ServerEnviron` to that scrub.
- Reed's `sessionlessSocketHolderPersists` is the one production accommodation:
  before its grace loop it asks the holder `show-options -gqv @lym_test_server`,
  and a holder answering `on` is never stale.
  Without it, reed would reap the session-less pre-started server after `staleSocketGrace`.
  It would then replace that server with a config-less one.
  Bound: only the kit's config sets the option, and reed's own boots never do;
  an operator config that sets it disables the reap for its own servers,
  and a server that sets `exit-empty off` without it is still reaped.
- A server reed starts itself reads no `~/.tmux.conf` either:
  its spawn and its capability probe carry `-f /dev/null` on every platform but Windows,
  and reed pins `default-shell` and an equal `default-command` on each session,
  so strand panes start non-login shells.
  The kit's server differs from reed's own only by `exit-empty off` and the `@lym_test_server` marker.
  Reed's boots never set either.
  Reed's server is started by a test that registers its key after reed's boot,
  and by every boot after a test's own `down` or `kill-server` on its key.
  Those tests pin reed's own server start, teardown or recovery, which a pre-started server would bypass.
  The pre-started server runs without reed's `-v` debug flags and with the kit's directory as its cwd,
  so it writes no `tmux-server-<pid>.log` into the hub logs dir;
  a test that asserts on those artefacts registers its key after reed's boot.

## Testkit

Test support used by two or more packages lives in one kit package under `internal/testkit/<kit>/`.
Such support covers fakes, builders, fixtures and the scan harness.
It is never duplicated per package and never placed under the package it fakes.

- No non-test file outside `internal/testkit/` imports a path under it, so every kit is reachable only from tests.
  Files under `internal/testkit/` are exempt, so a kit may build on another kit.
- No non-test file under `internal/testkit/` imports an `internal/*cli` package.
- No non-test file under `internal/testkit/` imports `os/exec`, `internal/gitexec` or `internal/gitkit`.
  None imports `internal/hubforge`, `internal/testkit/lymbin`, `internal/testkit/tmuxkit` or `internal/testkit/llmkit`.
  - `internal/testkit/lymbin` is exempt from the `os/exec` ban alone.
    The exemption covers `go build` of `./cmd/lym` and running that binary under group kill.
    It builds at most once per test binary and per ldflags, or returns the binary `gateslot.PrebuiltLymEnv` names.
    A gate run builds that binary once for every package of a tagged run.
    Banning its import keeps the kit-on-kit exemption from handing another kit a transitive `go build`.
  - `internal/testkit/tmuxkit` is also exempt from the `os/exec` ban alone, bounded to these uses:
    running the `tmux` binary against sockets under its own directory or its own fixture keys;
    starting servers under the kit's own config;
    and the post-run `/proc` scan that kills a leftover process.
    No other kit imports it, for the same reason.
  - `internal/testkit/llmkit` is also exempt from the `os/exec` ban alone.
    The exemption covers `exec.LookPath` for the `claude` binary; it starts nothing.
    Only test files of `tier3` tests and support files import it, and no other kit does, for the same reason.
- A kit imports only the lowest packages defining the types it fakes.
  A package an import cycle bars from a kit keeps exactly one local copy,
  and a fixture used by one package stays in that package's `_test.go` files.
- `internal/testkit/scankit` imports the standard library only, so every package's tests can import it.

### Enforcement

`internal/testkit/enforcement_test.go` enforces the import rules.
Review discipline covers the rest: a kit starts no process, tmux server or agent beyond what its imports allow.
A kit asserts nothing beyond `t.Fatalf` on its own setup, with two exceptions:
the `shedfake` `Call` and `RequireOutcome` outcome check,
and the `envelope` `RequireOK` and `RequireErr` shape check.
`scankit` additionally asserts scan results.
`plankit` and `stencilkit` assert nothing beyond `t.Fatalf` on their own setup.

## Hub fixtures

Every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`.
No hub is hand-assembled.

- No package in `fabriccli`'s dependency set may import `hubforge`.
- A copy from `hubforge.CopyHub` relocates a hub that `fabriccli.CloneAndWire` built into a per-binary template.
  It is never a hub assembled another way.
  `SharedHub` hands out that template itself to a test that only reads.
- A start state the closed shape set cannot hold, such as commits, pushes and pairs under fixed slugs,
  is a registered template: a package-level `hubforge.Registration` of a key, a base shape and a build function.
  `hubforge.CopyRegistered` copies it the way `CopyHub` copies a shape's.
  Its template is the base shape's `fabriccli.CloneAndWire` hub plus the build function's steps, never a copy.
  The build's TB fails it on `Cleanup`, `Setenv`, `TempDir`, `Chdir`, `ArtifactDir` or a skip,
  and the build fails unless the board's HEAD is its branch's tip in the records bare.
- A build function makes no process-wide change its TB cannot see:
  no `os.Setenv`, `os.MkdirTemp`, global git config, package-global seam setter or once-only registration,
  and no code whose result such a registration changes, such as a Bolt replay.
  A test that installs a process-global seam takes its copies before installing it.
  Enforcement of this clause is review discipline, not a scan.
- No test package wraps `hubforge` in its own fixture type;
  a test takes a `*hubforge.Hub` and reads `h.Topology`, `AddPairWith` and `OpenFabric` directly.
  Enforcement of this clause is review discipline, not a scan.
