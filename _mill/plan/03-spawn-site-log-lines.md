# Batch: spawn-site-log-lines

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
batch: "spawn-site-log-lines"
number: 3
cards: 7
verify: go test ./internal/websterengine/ ./internal/treadleengine/ ./internal/configengine/ ./internal/boardengine/ ./internal/vscode/ ./internal/reedengine/ ./internal/fabricengine/ ./internal/reedcli/ ./internal/loomcli/ && go test -tags integration -run TestRunVerifyCommand ./internal/websterengine/
depends-on: [1]
```

## Batch Scope

This batch implements every `add` verdict from the audit's spawn-site table: the seven production files that call `exec.Command`/`exec.CommandContext` and today import `internal/logger` nowhere in that file, plus (card 12) the three call sites in files that DO import `logger` but never log the spawn itself.
It is one batch because every card is the same two-or-three-line insertion around an existing spawn call, differing only in level (`Info` for a lifecycle spawn, `Debug` for the polling probe) and in whether the site waits (a waiting site logs teardown, a detached site logs the spawn alone and warns on `Start` failure).
Splitting it would put near-identical edits behind separate batch boundaries for no review benefit.

The external interface batch 5 consumes is exactly this batch's file-level effect: after it lands, every walked production file containing a spawn call either imports `internal/logger` or is one of the five sites batch 5 allowlists.
Batch 5's guard cannot go green before this batch lands, which is why it depends on it.
Card 12 is the exception to that framing and is here deliberately: its three sites already satisfy the guard's file-level check and would pass it unchanged, but they do not satisfy the sharpened invariant text card 2 lands, which is a per-call rule.
Without card 12 this task would ship an invariant three of its own `covered` rows silently violate.

Batch-local decision differing from `## Shared Decisions`: card 11's two lines are `Debug`, not `Info`. `Debug` never reaches the durable trace file, so those two probes are deliberately outside the bug-report trail — the alternative floods the durable sink, because both sit inside a polling probe run repeatedly against a live process tree.

## Cards

### Card 6: Log spawn and teardown around webster's verify-command spawn

- **Context:**
  - `internal/websterengine/gitwrap_test.go`
  - `internal/websterengine/testmain_test.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/websterengine/integration.go`
- **Creates:**
  - `internal/websterengine/runverify_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/integration.go`, add the import `github.com/Knatte18/loomyard/internal/logger` if it is not already present in that file, and instrument `runVerifyCommand`:

  - a `logger.Info` immediately before `cmd.Run()`, carrying `shell` (the resolved shell name), `verifyCmd`, and `worktree`, with a package-prefixed message naming the verify-command spawn;
  - inside the `if err := cmd.Run(); err != nil` block, distinguish the two existing outcomes rather than logging them alike. On the `*exec.ExitError` path — a failed verify, which is an expected answer — emit a `logger.Info` teardown line carrying `verifyCmd`, `worktree`, and `exitCode` (`exitErr.ExitCode()`) before the existing `return false, nil`. On the fall-through path — a genuine spawn failure — emit a `logger.Warn` carrying `verifyCmd`, `worktree`, and `cause` (the error) before the existing `return false, fmt.Errorf(...)`;
  - on success, a `logger.Info` teardown line carrying `verifyCmd`, `worktree`, and `exitCode` of `0`, before the existing `return true, nil`.

  Do not change `runVerifyCommand`'s signature, its `sh -c` / `cmd /C` selection, its `cmd.Dir` assignment, or either return value.

  Create `internal/websterengine/runverify_test.go` as an internal-package, `integration`-tagged test file: it must open with the build constraint `//go:build integration` and declare `package websterengine`.
  Both halves are required — an external test package cannot call the unexported `runVerifyCommand`, and an untagged file may not spawn a process under the Test Tier Purity Invariant.
  `internal/websterengine/gitwrap_test.go` is the in-package model for exactly this combination and reuses the package's hermetic `TestMain`, which this file inherits for free.
  Write a test named `TestRunVerifyCommand` covering two cases: a command that exits non-zero (expect `false, nil`, and a captured `INFO` teardown line carrying the `exitCode` key), and a command naming a binary that does not exist (expect a non-nil error, and a captured `WARN` line carrying the `cause` key).
  Assert that the two paths log differently — the non-zero-exit case must not produce a `WARN` line.
  Use the inline capture pattern from the `test-log-capture-pattern` shared decision, and call `logger.SetVerbosity(1)` with a `t.Cleanup` restoring `logger.SetVerbosity(0)`, per the `info-assertions-need-verbosity` shared decision — without it the `Info` lines never reach the buffer.
- **Commit:** `feat(websterengine): log spawn and teardown around the verify command`

### Card 7: Log spawn and teardown around the treadle gate command

- **Context:**
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/treadleengine/gate.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/treadleengine/gate.go`, add the import `github.com/Knatte18/loomyard/internal/logger` and instrument `execGateCommand`:

  - a `logger.Info` immediately before `cmd.CombinedOutput()`, carrying `argv`, `dir`, and `timeout`, with a package-prefixed message naming the gate-command spawn;
  - a `logger.Info` teardown line on each of the four post-`CombinedOutput` outcomes that return without an error, before that outcome's existing return. Naming all four explicitly, in source order: the clean-exit path (`err == nil`); the `ctx.Err() == context.DeadlineExceeded` path (carrying `timedOut` true); the `errors.Is(err, exec.ErrWaitDelay)` path; and the ordinary non-zero-exit path (`errors.As(err, &exitErr)`, which returns `output, false, nil`). That fourth branch is the "gate command ran and failed" case `converged` checks every round — it is the most-hit path of the four and must not be left unlogged. Each line carries `argv`, `dir`, `exitZero` (the bool that return already computes), and `durationMs`, measured from a `time.Now()` captured immediately before `cmd.CombinedOutput()`;
  - a `logger.Warn` on the final failed-to-start path, carrying `argv`, `dir`, `durationMs`, and `cause`, before the existing `fmt.Errorf` return.

  Do not change `execGateCommand`'s signature, its `context.WithTimeout` construction, its `cmd.WaitDelay` assignment, or any of its return values.
  The package's import allowlist in `internal/treadleengine/seam_enforcement_test.go` already admits `logger`, so no allowlist edit is needed here — confirm that by reading it rather than assuming.
- **Commit:** `feat(treadleengine): log spawn and teardown around the gate command`

### Card 8: Log spawn and teardown around the configengine editor spawn

- **Context:**
  - `internal/configengine/config.go`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/configengine/edit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/configengine/edit.go`, add the import `github.com/Knatte18/loomyard/internal/logger` — the package already imports it in `config.go`, but the guard in batch 5 checks file-level import presence, and this is the file with the spawn — and instrument `DefaultEditor`:

  - a `logger.Info` immediately before `cmd.Run()`, carrying `editor` (the resolved `editorCmd`) and `path`, with a package-prefixed message naming the editor spawn;
  - a `logger.Info` teardown line on the success path and a `logger.Warn` on the failure path, both carrying `editor` and `path`, the failure line additionally carrying `cause`.

  `DefaultEditor` currently ends in a bare `return cmd.Run()`; restructure that single statement into an `err := cmd.Run()` assignment plus the two branches, changing nothing about what is returned in either case.
  Do not change `DefaultEditor`'s signature, its `$VISUAL`/`$EDITOR`/platform-fallback resolution, or its std-stream wiring.
- **Commit:** `feat(configengine): log spawn and teardown around the editor spawn`

### Card 9: Log the detached board sync spawn

- **Context:**
  - `internal/proc/`
  - `internal/logger/logger.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/boardengine/spawn.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/boardengine/spawn.go`, add the import `github.com/Knatte18/loomyard/internal/logger` and instrument `spawnSync`:

  - a `logger.Info` immediately before `cmd.Start()`, carrying `exe`, `boardPath` (the resolved `abs`), and a package-prefixed message naming the detached board-sync spawn;
  - a `logger.Warn` when `cmd.Start()` returns a non-nil error, carrying `exe`, `boardPath`, and `cause`.

  Log no teardown: the spawn is detached (`cmd.Start()`, never `Wait`ed), so there is no teardown event to observe.
  `spawnSync` currently ends in a bare `return cmd.Start() // intentionally not Wait()ed`; restructure that single statement into an `err := cmd.Start()` assignment plus the failure branch, preserving the existing trailing comment's meaning in place.
  Do not change `spawnSync`'s signature, its `os.Executable`/`filepath.Abs` resolution, its `proc.Detach(cmd)` call, or the deliberate absence of std-stream wiring.
- **Commit:** `feat(boardengine): log the detached board sync spawn`

### Card 10: Log the detached VS Code launcher spawns on both platforms

- **Context:**
  - `internal/proc/`
  - `internal/logger/logger.go`
  - `cmd/lyx/crosscompile_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/vscode/launch_linux.go`
  - `internal/vscode/launch_windows.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In each of `internal/vscode/launch_linux.go` and `internal/vscode/launch_windows.go`, add the import `github.com/Knatte18/loomyard/internal/logger` and instrument that file's `Launch`:

  - a `logger.Info` immediately before `cmd.Start()`, carrying `worktreeDir` and a package-prefixed message naming the VS Code launch spawn;
  - a `logger.Warn` inside the existing `if err := cmd.Start(); err != nil` block, carrying `worktreeDir` and `cause`, placed before the existing `fmt.Errorf` return.

  Log no teardown in either file: both launchers are detached (`cmd.Start()`, never `Wait`ed).
  Keep the two files' existing platform differences intact — the Linux file invokes `code` directly, the Windows file goes through `cmd /c` and calls `proc.HideWindow(cmd)`.
  Do not change either `Launch`'s signature or its returned error text.
  `internal/vscode/launch_windows.go` carries `//go:build windows` and is never compiled by a native `go test` on Linux; the module-wide `GOOS=windows go build ./...` in the overview's `verify:` is what proves it still compiles.
- **Commit:** `feat(vscode): log the detached VS Code launcher spawns`

### Card 11: Debug-log the two Windows process-tree probes

- **Context:**
  - `internal/reedengine/proctree.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/logger/logger.go`
  - `cmd/lyx/crosscompile_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/reedengine/proctree_windows.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/reedengine/proctree_windows.go`, add the import `github.com/Knatte18/loomyard/internal/logger` — the package imports it elsewhere, but batch 5's guard checks file-level import presence, and this is the file with the spawns — and add one `logger.Debug` call before each of the file's two `exec.Command(...).Output()` probe calls:

  - in `(*Engine).descendantClosurePIDs`, carrying `shell` (`e.cfg.Shell`) and `roots` (the root pid slice);
  - in `(*Engine).serverProcessesOnSocket`, carrying `shell` (`e.cfg.Shell`) and `socket` (`e.Socket()`).

  Both lines are `Debug`, not `Info`, and this is deliberate: each sits inside a polling probe run repeatedly against a live process tree, and `Info` reaches the durable trace file unconditionally, which would flood it.
  `Debug` therefore never reaches the durable sink — these two probes are outside the bug-report trail by design, which card 1's audit document records.
  Do not add a teardown line at either site and do not log the probe's failure path: both probes already degrade silently by contract (`descendantClosurePIDs` falls back to bare roots, `serverProcessesOnSocket` returns nil), and turning either into a `Warn` would change a documented degrade into a reported failure.
  Do not change either method's signature, its PowerShell script text, or its fallback behaviour.
  The file carries an implicit `windows` build constraint via its `_windows.go` suffix and is never compiled by a native `go test` on Linux; the module-wide `GOOS=windows go build ./...` in the overview's `verify:` is what proves it still compiles.
- **Commit:** `feat(reedengine): debug-log the windows process-tree probes`

### Card 12: Log the three spawn sites in files that import logger but never log the spawn

- **Context:**
  - `internal/reedengine/lifecycle.go`
  - `internal/proc/`
  - `internal/logger/logger.go`
  - `manifest/designs/logger-coverage.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/fabricengine/spawn.go`
  - `internal/reedcli/attach.go`
  - `internal/loomcli/run.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Each of these three files already imports `github.com/Knatte18/loomyard/internal/logger`, so no import changes anywhere in this card — the gap is that the spawn call itself is unlogged, which the sharpened invariant card 2 lands makes a violation and which batch 5's file-level guard structurally cannot see.
  `internal/reedengine/lifecycle.go`'s `spawnSession` closure is the in-repo model for the shape wanted at every site: a `logger.Info` naming the spawn after a successful `Start`, and a `logger.Warn` on the failure path.

  - In `internal/fabricengine/spawn.go`, in `SpawnDetachedPush`, add a `logger.Info` immediately after a successful `cmd.Start()` — that is, on the path that reaches the final `return nil // intentionally not Wait()ed` — carrying `exe`, `args`, and `pid` (`cmd.Process.Pid`), with a package-prefixed message naming the detached both-sides push spawn. Log no teardown: the child is started and never `Wait`ed. Leave the existing `logger.Warn("fabricengine: spawn detached push failed", …)` on the `Start`-failure path exactly as it is — do not reword it, do not change its `error` key to `cause`, and do not renumber its fields; the `additive-only` shared decision forbids touching an existing line even to bring it to the majority spelling.
  - In `internal/reedcli/attach.go`, around the `attach := exec.Command(c.eng.TmuxPath(), …)` tmux-attach handover, add a `logger.Info` immediately before `attach.Run()` carrying `tmux` (`c.eng.TmuxPath()`), `cols`, and `rows`, and a `logger.Info` teardown line after `attach.Run()` returns carrying `tmux` and `exitCode`. The teardown line must be emitted on both the success and the non-zero-exit paths, and must not change the existing `clihelp.SetExit` behaviour or the documented no-JSON-envelope terminal-handover exception. Do not touch the existing terminal-size `logger.Warn` above it.
  - In `internal/loomcli/run.go`, apply the identical treatment to the `attach := exec.Command(c.reed.TmuxPath(), …)` tmux-attach handover only. Do not touch the `driveCmd := exec.Command(exe, "loom", "drive")` spawn or its existing `logger.Info("loom: spawned detached driver", …)` line — that site is already covered and is what the audit's `covered` half of this file's row refers to.

  Change no control flow, no error text, and no return value at any of the three sites.
- **Commit:** `feat(spawn): log the three unlogged spawn call sites`

## Batch Tests

`verify:` has two halves.

The first, `go test ./internal/websterengine/ ./internal/treadleengine/ ./internal/configengine/ ./internal/boardengine/ ./internal/vscode/ ./internal/reedengine/ ./internal/fabricengine/ ./internal/reedcli/ ./internal/loomcli/`, runs the untagged suite of every package this batch edits.
Its job is regression, not assertion of the new lines: six of the seven cards add log calls around existing, already-tested spawn calls and get no dedicated test of their own, per `_mill/discussion.md`'s Testing section — a per-site log-content test at each would be test-for-test's-sake.
What this half genuinely catches is the risk that actually exists here: a new `logger` import tripping a package's own leaf or seam-enforcement test.
`internal/treadleengine/seam_enforcement_test.go` is the one such test in scope, and it runs in this half.
Card 12 adds no import at all — all three of its files already import `logger` — so it carries none of that risk; the three packages are in the list to catch a behavioural regression from restructuring a spawn's surrounding statements, particularly `internal/reedcli`'s and `internal/loomcli`'s exit-code propagation through `clihelp.SetExit`.

The second, `go test -tags integration -run TestRunVerifyCommand ./internal/websterengine/`, is the one dedicated test this batch adds: card 6's `internal/websterengine/runverify_test.go`.
It needs the `integration` tag because it spawns real processes, and it is `-run`-scoped to `TestRunVerifyCommand` so the batch's verify does not drag in the rest of webster's tagged suite (real scratch git repos, whole-run fixtures) on every implementer and fixer round.

Two things this batch changes are invisible to both halves on a Linux host: `internal/vscode/launch_windows.go` and `internal/reedengine/proctree_windows.go` are Windows-tagged and are never compiled by a native `go test`.
The module-wide `verify:` in the overview (`go build ./... && GOOS=windows go build ./...`) runs at the batch boundary and is what proves both still compile; `cmd/lyx/crosscompile_test.go` is the durable in-repo guard for the same property and runs in the repo-wide `done_gate`.

Card 6 is the batch's TDD candidate — `runVerifyCommand` is directly callable from an in-package tagged test, so its two cases can be written and observed to fail before the log calls exist.
