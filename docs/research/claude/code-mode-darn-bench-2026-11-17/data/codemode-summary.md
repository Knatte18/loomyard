# Guard every pinned or spawned lym against running a test binary

On 2026-11-08 a reed status job was built from the running binary's own path. Under `go test` that path is the test binary, so every tmux session a test created ran the suite again, and those runs created more sessions. The machine froze twice. `3aaa98cc` fixed the Slack lines only. This change closes the whole class with three independent layers, one commit each.

## 1. `internal/lymexe`: the only resolver of "this lym binary"

A new stdlib-only leaf:

- `Path()` returns the running binary's path for a command that runs it. Under `go test` it returns `ErrTestBinary`.
- `Resolve(lookup)` does the same over an injected lookup, the seam tests replace with a stand-in binary. It refuses only the running test binary, compared by cleaned path or `os.SameFile`, so stand-ins still pass.
- `Runnable(path)` checks a path that was resolved earlier, at the site that runs it.
- `Self()` is for identity reads only: stat, VCS stamp, commit label, process match, or a path recorded for a later spawn.

`enforcement_test.go` fails on any production reference to `os.Executable` outside `internal/lymexe` (`internal/testkit` is exempt, since only tests import it).

Every call site was audited and moved:

- **Pins and spawns that run lym now use `Path`/`Resolve`:**
  - reed's switch, follow, button and both Slack lines, plus the button prompt's re-entry line. This replaces `slackLineExecutable`, so the refusal now covers every binding, not just Slack.
  - The per-hub watchdog spawn, `reed remove`'s detached child and `reed window`.
  - loom's status strand and orch's runs strand. Both are panes that run lym. Under test, loom now adds no status strand and logs why instead of failing.
  - The loom and batten loop specs (under test the loop gets no executable), `spawnGoDriver`, drift capture and raddle children.
  - batten's `loom start` child and detached pass, the adopt suite and resume, creel's daemon, gate's detached runner, fabric's push and teardown children, board sync, `beam start`, and the IDE config's lym path (under test it falls back to the bare name).
- **Paths recorded for later use now come from `Self` and are checked where they run:**
  - The loom driver's on-disk path: `spawnDetachedDrive` calls `Runnable`.
  - batten's `{lym}` placeholder: `shedadapters` refuses a test-binary `{lym}` before starting a process.
  - The dobby and Slack geometries: their spawners already return early under `testing.Testing()`.
- **Identity-only reads use `Self`:** `BinaryLabel`, `statuspane.CurrentBuild`, adopt's running build, tender's process match, `reed watchdog`'s binary stat and `selfexec`. `selfexec`'s leaf allowlist and `PATTERN-leaf-packages` now admit `lymexe`.

reed's `executablePath` field now defaults to `lymexe.Self`. The pane and window PATH prelude only adds the binary's directory to `PATH` and never runs it, so it still reads that path directly. Everything that does run it goes through `Resolve`.

## 2. Every test binary refuses a non-test first argument

`tmuxkit.Main`, which every test package's `TestMain` already calls (enforced), now returns 2 with a refusal on stderr when the first argument is not a flag. It checks only the first argument, as gitkit's guard does, so `-test.run Foo` still works. reedengine's own `flag.NArg` guard folds into it.

`tmuxkit.RequireNonTestArgsRefused(t)` proves the refusal: it starts the running test binary with `slack bar`. Each package whose code pins or spawns lym has a tier1 `argguard_test.go` calling it: reedengine, reedcli, loomcli, battencli, adoptcli, orchcli, fabriccli, fabricengine, driftcli, creelcli, gatecli, boardengine, ideengine, shedadapters, dobbyengine, slackengine, hubgeom and fronttask.

The proof checks for exit 2, empty stdout and "refusing" on stderr. Packages that call `gitkit.HermeticGitEnv` before `tmuxkit.Main` are refused by gitkit's equivalent guard, with its own wording.

`PATTERN-test-isolation` records the guard, the third kit-assertion exception and tmuxkit's widened `os/exec` exemption.

## 3. `lym gate test` caps the processes a tier may hold

Each tier command gets its own environment entry, `LYM_GATE_TIER_MARK_<pid>_<n>=1`. Processes inherit it even after leaving the process tree: a tmux server and its panes keep the mark. A nested gate adds its own entry next to the outer one.

Once a second the verb counts marked processes through the new `proc.PidsWithEnv`. Above `maxTierProcesses` (1500) it:

- cancels the command,
- SIGKILLs every marked process, for up to five rounds,
- fails the tier with exit 1 and a cause line naming the count, the cap and the fix.

A capped run is never rerun on the resource-shortage marker its own runaway printed. Off Linux the cap logs that it does not apply.

## Docs

- `PATTERN-child-processes` has a new "This lym binary" rule, with `(test)` added to its `PATTERN.md` entry.
- Updated package docs: `reedengine`, `selfexec`, `shedadapters`, `proc`, `gatecli` ("Process cap"), `tmuxkit` and the new `lymexe`.
- `docs/overview.md` lists `lymexe` in the module table and describes the cap.

## Review notes

- **Untested.** Only `go build` and `go vet` ran on the touched packages; `proc` and `gatecli` also built for `GOOS=windows`. No test ran. The new and changed tests are:
  - `lymexe`'s unit and scan tests
  - `tmuxkit`'s argument tests and the 18 `argguard` tests
  - `gatecli`'s `spawncap_test.go`
  - the updated `runInvocations` call in `testtargets_test.go`
  - the `TestBinaryExecutable` row in reed's `bindings_test.go`, which now expects no button line either
- **Behaviour change under test:** a reed engine that falls back on its default executable lookup no longer pins switch, follow or button lines. Integration tests that inject a stand-in binary are unaffected.
- **The cap value (1500) is a judgment call.** It is a named constant, not config. Raising it to a `gate.yaml` limit would need a `configengine`/`hubreconcile` reinterpretation.
- **Bound on the cap:** a process started with an environment built from scratch carries no mark and is not counted.

## To check by hand

- Run `lym gate test` over the touched packages at tiers 0 and 1, including `./internal/lymexe`, `./internal/testkit/tmuxkit`, `./internal/reedengine` and `./internal/gatecli`.
- On a live reed session, confirm the switch, follow, button and Slack bindings still name the installed `lym`.
