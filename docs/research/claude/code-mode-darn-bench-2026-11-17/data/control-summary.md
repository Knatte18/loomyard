# Guard every spawned or pinned lym path against the test binary, and cap a gate run's processes

On 2026-11-08 a reed Slack status job was built from `os.Executable`. Under `go test` that path is the test binary, so every tmux session a test created ran the suite again, which created more sessions. The machine froze twice. `3aaa98cc` fixed the Slack lines alone. This change guards the whole class in four parts, each matching an item of the board entry.

## 1–2. One resolver for "this lym binary": `internal/lymexe`

A new stdlib-only leaf, `internal/lymexe`, provides two functions:

- `Path()` returns the running lym binary's path. Under `go test` it returns `ErrTestBinary` and no path.
- `Refuse(path)` rejects any `*.test` or `*.test.exe` binary. Under `go test` it also rejects the running binary's own path. It is for paths that arrive through a seam.

Every site that hands the running binary's path to another process now uses `lymexe.Path` instead of `os.Executable`:

- **reedengine**
  - The engine's and the button click's `executablePath` now default to `lymexe.Path`.
  - The new `pinnedExecutable` replaces `slackLineExecutable`. It runs `Refuse` before every line reed pins and every prelude it writes: switch, follow, button, both Slack lines, the button re-entry prompt, the pane `LYM_BIN` prelude and the window prelude.
  - The watchdog spawn now uses `lymexe.Path` too.
- **Unattended pins and loops**
  - loomcli: the status strand, the loop job's executable, the drift capture child, the raddle `beam create` child and the Go driver spawn.
  - battencli: the loop spec, the `loom start` child and the pass spawn.
  - orchcli: the runs strand and the watcher.
  - reedcli: `reed window`'s terminal, through a new `lymPath` seam, and the detached remove.
  - ideengine: the `.vscode` lym path, which falls back to the bare name.
  - fronttask: the front-task actions. `startAction` now takes the lookup as a parameter.
- **Detached children that were already guarded by `testing.Testing()`**: fabriccli, fabricengine (push and teardown hold), driftcli, gatecli's detached runner, creelcli, boardengine and adoptcli. They now go through the helper too, so the rule has no exceptions.

Behaviour changes a reviewer should check:

- **Unwired path under test:** `battenCLI.wire` and `goDriverDepsFor` must still wire under test, because tests call them. They treat `ErrTestBinary` as an empty path, so a spawn from it fails instead of re-running the suite. In production nothing changes.
- **Status strand:** `ensureStatusStrand` now only warns and leaves the status pane out when the path does not resolve, as a failed replace already did. Before, it failed `start`.
- **Test expectation changed:** in reedengine, a test-binary executable now also leaves the button line unpinned. The `TestBinaryExecutable` row of `TestPinBindingsLocked_PinsTheSlackLinesOnlyWhenEverythingResolves` was updated to match.

`lymexe/executable_enforcement_test.go` scans every production file under `internal/` and `cmd/` and fails on `os.Executable` outside an allowlist. Each allowlist entry names its reason. All of them only read the binary (stat, build stamp, commit label, process comparison), except two:

- `hubgeom`'s dobby geometry and slack geometry carry the path to engines that already refuse to spawn under `go test`.
- `selfexec` re-execs its own process behind its own test refusal.

`hubgeom` was left on `os.Executable` because its tests assert an absolute path.

PATTERN changes:

- `PATTERN-child-processes` gains a "This lym binary" rule, marked `(test)` in `PATTERN.md`.
- `PATTERN-leaf-packages` lists `internal/lymexe`.
- The overview's module table gains `internal/lymexe`.

## 3. The TestMain guard, now in `tmuxkit.Main`

reedengine's `TestMain` used to refuse positional arguments itself. That guard now lives at the top of `tmuxkit.Main`, which every test package calls (`cmd/lym/tmuxisolation_test.go` enforces this, and its allowlist is empty). The guard works like this:

- It parses the flags. If any non-flag argument is left over, it prints `<binary>: refusing the non-test arguments …` and returns 2 before it creates a run directory or runs a test.
- Helper roles that a `TestMain` dispatches by environment before it calls `Main` are not affected.
- `gitkit.HermeticGitEnv` still refuses a leading positional argument, as before.

`tmuxkit.RequireArgGuard(t)` proves the guard on the calling package's own binary:

- It starts the binary with `-test.run=^$ slack bar`. The filter comes first so the line gets past gitkit's leading-argument check.
- It asserts exit 2, empty stdout and the refusal text.
- A child environment variable makes the test skip in the child, so a missing guard cannot recurse.

A tier1 `testmain_guard_test.go` calling it is added to reedengine (replacing its hand-rolled test) and to every package whose tests boot reed or reach a converted spawn or pin site:

- cmd/lym
- reedcli, loomcli, battencli, adoptcli, beamcli
- burlerengine, creelcli, driftcli, loomshed, orchcli
- seatengine, shedverbs, shuttlecli, standalonegeom, treadleengine
- webstercli, websterengine, fronttask, ideengine, boardengine

tmuxkit itself also gets one. `PATTERN-test-isolation` documents the refusal. It also extends tmuxkit's `os/exec` exemption, and its list of asserting kit helpers, to cover `RequireArgGuard`.

## 4. `lym gate test` caps a run's processes

The fork bomb ran through tmux servers, and tmux servers leave the tier's process group, so counting the tier's process tree would miss them. The cap counts by environment instead:

- **The mark:** the outermost gate run sets `LYM_GATE_PROCESS_MARK=<its pid>` in every tier's environment. Every descendant inherits it, including a tmux server a test starts and that server's panes.
- **The count:** once a second, `processCap` counts the marked processes through the new `proc.PidsWithEnv`, which reads `/proc/*/environ`. On Windows it returns `ErrUnsupported`, and the cap is off.
- **The trip:** past the new gate.yaml key `max_processes`, the cap prints `gateslot.ProcessCapLine`, which names the likely cause and the way forward. It kills every marked process, again at each later count, and once more as the tier ends. The tier then fails, with exit 1 if the shell still exited zero.
- **The key:** `max_processes` is cross-pair, defaults to 1000, and 0 turns the cap off.
- **Nested runs:** a nested `lym gate test`, detected through the existing `LYM_GATE_TEST_PID` ancestor check, keeps its enclosing run's mark and does not run a cap of its own.

The gatecli, gateslot and proc package docs and the overview's gate entry describe the cap. The test for `TemplateConfig` now expects `MaxProcesses: 1000`.

## Not verified here

Every touched package was compiled and vetted (`go build` / `go vet`, and `GOOS=windows` for `proc`). No tests were run, so the reviewer should run the gate. These carry the most risk:

- the guard tests and the `os.Executable` scan;
- tests that relied on a real executable path in an unstubbed reed engine or status strand;
- the 1000 default, which is a judgment call above a normal tier's process count and below the incident's roughly 1600 processes.
