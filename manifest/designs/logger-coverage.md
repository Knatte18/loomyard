# logger-coverage — audit of internal/logger coverage across spawn/hard-error paths

> **Status: a durable, re-runnable survey — not a module-design draft, and not deleted when its fixes land.**
> Classified explicitly by the 2026-08-29 designs audit, which found this file was the one doc under `manifest/designs/` with no Status line of its own.
> The [documentation lifecycle](../../docs/overview.md#documentation-lifecycle)'s delete-on-landing rule applies to per-module design drafts;
> this is instead the enumerated evidence behind `CONSTRAINTS.md`'s Live-Substrate Spawn Observability invariant, and `cmd/lyx/spawnobservability_test.go` cites it by name for the argument the guard test deliberately does not encode.
> Every verdict the `logger-coverage-audit` task acted on has since landed and the tables below are re-verified against the tree as of 2026-08-29;
> re-run both selectors rather than trusting them after any further spawn-site work.

## What this is, and why it exists

This document is the enumerated coverage survey behind the **Live-Substrate Spawn Observability** invariant (`CONSTRAINTS.md`).
`internal/logger` fans every `Info`+ record out to a durable, per-process trace file,
and that trail is the raw material for the bug-reporting and watchdog work being built on top of loom.
It only works if the events that matter actually reach it,
so this survey enumerates two bounded universes — every production process-spawn site, and every orchestration-terminating hard-error-return site — and records a per-site verdict for each.

## Selectors

Both universes are enumerated mechanically, by AST walk, not by grep — a doc-comment mention is not a call, and a grep would count it as one.
A later reader can re-run either selector directly rather than trust the tables below,
which are a snapshot of the walk's output at one point in time.

**Spawn selector.**
Every `*ast.CallExpr` in a production (non-`_test.go`) `.go` file under `internal/` and `cmd/` whose `Fun` is an `*ast.SelectorExpr` whose `X` matches the file's own `os/exec` import name and whose `Sel.Name` is `Command` or `CommandContext`.

**Hard-error selector.**
Every `*ast.SwitchStmt` any of whose `case` expressions is a selector whose `X` matches the file's `shuttleengine` import name and whose `Sel.Name` begins `Outcome`,
plus every `*ast.BinaryExpr` with `==`/`!=` where either operand is such a selector.

Both are AST rather than grep because a doc-comment mention is not a call.
Three files hit the `exec.Command` substring with zero real calls, all of them prose in a doc comment: `internal/githubclient/doc.go`, `internal/reedengine/doc.go`, `internal/reedengine/attach.go`.

## Spawn sites

Counts are call expressions;
comment mentions are excluded, per the spawn selector above.

| Site | Calls | Waits? | Verdict |
| --- | --- | --- | --- |
| `internal/reedengine/lifecycle.go` | 1 | `Run` | covered |
| `internal/reedengine/overlay.go` | 2 | `Run`/`Output` | covered |
| `internal/reedcli/attach.go` | 1 | waits via `Run` | covered (`Info` spawn + teardown) |
| `internal/loomcli/run.go` | 2 | one waits via `Run` | covered (both; `Info` spawn + teardown on the attach site) |
| `internal/fabricengine/spawn.go` | 1 | detached | covered (`Info` spawn, `Warn` on `Start` failure) |
| `internal/websterengine/integration.go` | 1 | `Run` | covered (`Info` spawn + teardown) |
| `internal/treadleengine/gate.go` | 1 | `CombinedOutput` | covered (`Info` spawn + teardown) |
| `internal/configengine/edit.go` | 1 | `Run` | covered (`Info` spawn + teardown) |
| `internal/boardengine/spawn.go` | 1 | detached | covered (`Info` spawn, `Warn` on `Start` failure) |
| `internal/vscode/launch_linux.go` | 1 | detached | covered (`Info` spawn, `Warn` on `Start` failure) |
| `internal/vscode/launch_windows.go` | 1 | detached | covered (same shape) |
| `internal/reedengine/proctree_windows.go` | 2 | `Output` | covered (`Debug` only) |
| `internal/gitexec/gitexec.go` | 1 | `Run` | blocked (import cycle) |
| `internal/gitkit/gitkit.go` | 3 | `Run` | blocked (gitkit Leaf Invariant) |
| `internal/githubclient/token.go` | 1 | `Output` | blocked (GitHub Auth leaf allowlist) |
| `internal/hubforge/hub.go` | 1 | `Run` | excluded (test-fixture builder) |
| `cmd/testtiming/main.go` | 1 | `Run` | excluded (test-timing harness) |
| `tools/deploy/main.go`, `tools/sandbox/*` | 7 | — | excluded (dev tooling, outside the walk) |

`internal/loomcli/run.go`'s two sites were the survey's one split verdict: the `loom drive` spawn was already covered (`Info` at spawn), while the tmux-attach spawn waited via `Run` unlogged.
Both are logged now.
`internal/loomcli/run.go`, `internal/reedcli/attach.go`, and `internal/fabricengine/spawn.go` were re-verdicted from an earlier, coarser `covered` reading before being fixed — see "What 'covered' means here" below.

## Detached spawns are spawn-only

`internal/boardengine/spawn.go`, `internal/fabricengine/spawn.go`, and both `internal/vscode` launchers call `Start` and never `Wait`,
so there is no teardown event to observe.
Demanding a teardown log at those sites would be unsatisfiable — the `Info` line records the spawn,
and a `Warn` records a `Start` failure.

## What "covered" means here

A verdict of `covered` is a per-call-site claim — the spawn call itself is logged — not merely a claim that the enclosing file imports `internal/logger` somewhere.
That distinction is why the survey is worth re-running rather than replacing with a file-level grep: three sites originally read as `covered` on the coarser file-level measure were re-verdicted `add` on a per-call read, and only then fixed.
As found, before the fixes landed:

- `internal/fabricengine/spawn.go` logged only a `Warn` on `Start` failure and announced no spawn.
- `internal/reedcli/attach.go`'s only `logger` line was an unrelated terminal-size warning, leaving its tmux-attach spawn unlogged.
- `internal/loomcli/run.go` logged its `loom drive` spawn but not its tmux-attach spawn.

`internal/reedengine/overlay.go`'s two sites are `covered` at `Debug` — `TmuxCmd.run` and `TmuxCmd.output` each log the argv immediately before spawning —
and `Debug` is correct there for the same reason it is correct for `internal/reedengine/proctree_windows.go`: both are high-frequency probe wrappers whose `Info` volume would flood the durable sink.

Batch 5's guard cannot distinguish either of these cases: it checks file-level import presence only,
which is the blind spot its own header comment documents.

## Hard-error-return sites

Sites are cited by file and enclosing function, never by line number.

| Site | Non-Done handling | Verdict |
| --- | --- | --- |
| `internal/shedadapters/singlellm.go` `mapOutcome` | `Asking`→`Stuck`, `Died`/`Timeout`→error, `default`→error | covered (`Warn` on `Asking`, on `Died`/`Timeout`, and on `default`) |
| `internal/websterengine/runlevel.go` `Run`'s Master outcome switch | `Asking`/`Died`/`Timeout`→typed errors, `default`→error | covered (`Warn` on all four non-`Done` branches) |
| `internal/mergeresolve/mergeresolve.go` `Resolve`'s `!= OutcomeDone` branch | → `abortAndStuck` | covered (`Warn` before `abortAndStuck`) |
| `internal/shedadapters/burler.go` two switches (`Call`, `probeLiveRound`) | retry then respawn | covered |
| `internal/shedadapters/bouncer.go` two comparisons (`runSeedSpawn`, `judgeCall`) | seed run and judge run did not complete | covered |
| `internal/treadleengine/run.go` two comparisons (`runRound`) | retry on `Died`/`Timeout` | covered |
| `internal/treadleengine/judge.go` two comparisons (`runJudgeCall`, `runTriage`) | degrade to default verdict | covered |
| `internal/treadleengine/targeting.go` one comparison (`runTargeting`) | degrade to no seed | covered |
| `internal/burlerengine/engine.go` one comparison (`Run`) | returns `result, nil` | excluded — a normal loop event, not a hard error; the caller branches |

## Enforcement asymmetry

The spawn table is enforced by a tree-wide guard test under `cmd/lyx/`,
while the hard-error table is document-only.
The reason: a new unlogged `exec.Command` is nearly always a real miss,
so a file-level check has a high signal rate.
A new outcome-switch branch, by contrast, may legitimately return normally for the caller to branch on — `internal/burlerengine/engine.go`'s `Run` does exactly that —
so the same file-level check would fire on correct code.

The accepted cost is stated plainly: the hard-error table will rot,
and the next author adding an outcome switch will not be told to log it.
A branch-return-behaviour guard, distinguishing a legitimate normal-return branch from a swallowed hard error, is the obvious follow-up.

## Untested log lines

The four `Warn` lines added to `internal/websterengine/runlevel.go`'s Master outcome switch land without a direct test.
The switch is inline in `Run`, downstream of `SaveState`, the mutation-lease release, and `handle.Wait()`,
and the only existing driver is a tagged external fixture that offers no seam for forcing a chosen shuttle outcome.
Building one would be new production structure in an additive logging change,
so the four branches are recorded here as untested rather than left as an implicit gap.

## Structural blocks

### `internal/gitexec` — import cycle

`internal/logger/sink.go` imports `internal/lyxcwd`, and `internal/lyxcwd` imports `internal/gitexec` — mandated by CONSTRAINTS.md's Cwd Resolution Invariant,
which pins `lyxcwd`'s imports to stdlib plus `internal/gitexec` only.
Adding `logger` to `gitexec` would close the loop: `gitexec → logger → lyxcwd → gitexec`.
The diagnostic information is not actually lost by this exclusion: `gitexec.Run` already returns a `*GitError` carrying args, dir, exit code, and stderr,
so the diagnostic is fully reconstructable at whichever caller decides a failure mattered.

### `internal/gitkit` — gitkit Leaf Invariant

CONSTRAINTS.md's gitkit Leaf Invariant pins `gitkit`'s imports to a short, pinned list: stdlib, `lyxcwd`, `weftname`, `configengine`, `lyxdirs`.
Admitting `logger` would widen a deliberately narrow leaf,
so the three `exec.Command` sites in `internal/gitkit/gitkit.go` are recorded as blocked.

### `internal/githubclient` — GitHub Auth Invariant leaf allowlist

`internal/githubclient/leaf_enforcement_test.go` enforces the GitHub Auth Invariant's leaf half as a strict allowlist;
production code there may import only go-github, `golang.org/x/sys/windows`, and `internal/proc`.
GitHub failures are logged one layer up instead, in both of `internal/githubclient`'s production callers: `internal/selfreportengine/selfreport.go` and `internal/landingshed/publish.go`.
