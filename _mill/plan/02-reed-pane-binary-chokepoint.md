# Batch: reed-pane-binary-chokepoint

```yaml
task: "Spawned agent panes resolve the spawning lyx binary"
batch: "reed-pane-binary-chokepoint"
number: 2
cards: 5
verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/
depends-on: [1]
```

## Batch Scope

This batch lands the mechanism: a new `internal/reedengine/panebin.go` owning the pane-environment prelude, wired into `launchStrandLocked` — the single chokepoint every strand-realizing path already funnels through — plus the `CONSTRAINTS.md` clause recording the invariant, the enforcement test backing it, and the hermetic tests for the composition.
It is one batch because the seam file, its single call site, its invariant and its guards are one structural change: splitting them would leave an intermediate commit where the property is claimed but unenforced, or enforced but unrecorded.
It consumes batch 1's `Shell.ExportEnv` / `Shell.PrependPathEntry` / `Shell.Chain` and exposes no new external interface of its own — `panebin.go`'s identifiers are all unexported.
Batch-local addition to the overview's Shared Decisions: this batch's tests inject the executable path through a package-local `var executablePath = os.Executable` seam rather than reading the live value, because under `go test` that value is the test binary's path and `CONSTRAINTS.md`'s Live-Substrate Spawn Observability clause bars re-exec'ing it.

## Cards

### Card 3: Compose the pane-binary prelude at the strand-launch chokepoint

- **Context:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
  - `internal/reedengine/spawnwatchdog.go`
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/lock.go`
  - `tools/sandbox/resolve.go`
- **Edits:**
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/doc.go`
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/reedengine/panebin.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/panebin.go` in `package reedengine`, owning the whole pane-binary seam, with a file header comment in this package's established style stating what the file owns and why the composition lives at one chokepoint.
  It declares exactly four things:
  - `const lyxBinEnvKey = "LYX_BIN"` — the env key the prelude exports. It carries the absolute path of the binary itself, never its directory, so a script or prompt can invoke it explicitly without re-appending a platform-specific `lyx` / `lyx.exe` name.
  - `var executablePath = os.Executable` — a package-local testability seam over `os.Executable`, modelled on `tools/sandbox/resolve.go`'s own `var devBinPath = devbin.BinPath`. Its doc comment states why it exists: under `go test` the live value is the test binary's path, so hermetic tests inject a path instead of asserting against the real one.
  - `func paneBinPrelude(sh shell.Shell, exe string) string` — the pure, injectable composition. Returns `sh.Chain` over exactly two statements in this order: `sh.PrependPathEntry(filepath.Dir(exe))` first, then `sh.ExportEnv(lyxBinEnvKey, exe)`. The prepend is unconditional — no guard against the directory already being on the pane's `PATH` — because every strand pane is a fresh pane with a fresh shell, so nothing accumulates across relaunches or resumes, and the one nesting case produces a duplicate entry that resolves identically.
  - `func composePaneLaunchLine(sh shell.Shell, launchCmd, strandGUID string) string` — reads `executablePath()`. On error it calls `logger.Warn` naming the strand and the cause and returns `launchCmd` unchanged, so the launch proceeds with no prelude. On success it returns `sh.Chain(paneBinPrelude(sh, exe), launchCmd)`. Because `Chain` drops empty parts, an empty `launchCmd` yields the prelude alone with no trailing separator and no empty command fragment.

  `panebin.go` imports `os`, `path/filepath`, `github.com/Knatte18/loomyard/internal/logger` and `github.com/Knatte18/loomyard/internal/shell`.
  `internal/reedengine` already imports `internal/shell` and `internal/logger`, so no new dependency edge is created.
  `panebin.go` spells no raw POSIX or pwsh syntax of its own — every shell token comes from the `internal/shell` seam, per the Shell Mechanics Seam invariant.

  In `internal/reedengine/spawn.go`, change `launchStrandLocked` at exactly one place: the payload handed to the first `send-keys`.
  Compute the composed line once, before the send, as `composePaneLaunchLine(shell.ForGOOS(), launchCmd, s.GUID)`, and pass that value through `sendKeysLiteralArg` where `launchCmd` is passed today.
  `shell.ForGOOS()` is the dialect source — the same selector that builds the launch command the prelude is joined onto.
  Add the `internal/shell` import to `spawn.go`.
  Do not change the `split-window` argv assembly, the `-c` flag, `validateSplitCreatedNewPane`, the `Enter` send, `sendKeysLiteralArg` itself, or any part of the reconcile-before-allocate ordering.
  Extend `launchStrandLocked`'s own doc comment with a short paragraph naming `panebin.go` as the prelude's owner and stating that the split still carries no trailing shell-command, so the pane remains tmux's own `default-shell` started as a login shell.

  In `internal/reedengine/doc.go`, insert a new paragraph immediately before the `// # Multiplexer contract surface` heading line, after the live-geometry paragraph that ends with `told-box-wins-live-query-is-the-fallback).`.
  The paragraph states: every strand pane reed creates resolves `lyx` to the binary that spawned it, because `launchStrandLocked` prepends that binary's directory to the pane's `PATH` and exports `LYX_BIN` to its absolute path as shell statements riding the same `send-keys` line the launch command already rode;
  the composition lives in `panebin.go` and is reached from the one chokepoint, so the property holds by construction for every present and future `AddStrand` caller including `Resume`'s replay;
  the mechanism is a typed shell statement rather than `split-window -e` or `set-environment`, so it prepends onto the pane's live `PATH` instead of replacing it with a value computed from the lyx process's own environment, it survives the pane shell's profile (which has already run by the time anything is typed), and it needs no multiplexer capability;
  Selvage's pane and the `new-session` first pane are out of scope by name.

  In `CONSTRAINTS.md`, add a new `## Pane Binary Resolution` section placed immediately after the `## Live-Substrate Spawn Observability` section and immediately before `## Sandbox Suite Coverage`.
  Match the file's house style: an `##` heading, one sentence stating the FORM, then a bullet list of sub-clauses.
  The clause states that a strand pane reed creates resolves `lyx` to the binary that spawned it, and its sub-clauses record: `panebin.go` owns the seam and `launchStrandLocked` is its only call site;
  every shell token is emitted through `internal/shell`, per the Shell Mechanics Seam;
  the dialect is `shell.ForGOOS()`, the same selector as the launch command the prelude is joined onto, and reed neither derives a dialect of its own nor changes how a pane's shell is started;
  scope is strand panes only, with Selvage's split and the `new-session` first pane exempt by name, and the detached `lyx loom run` and watchdog daemon spawns excluded because both are already spawned from the executable path and neither resolves `lyx` from `PATH`;
  an unresolvable executable path degrades to a pane with no prelude plus a named `logger.Warn`, never a failed launch;
  and the clause is backed by `internal/reedengine/panebin_enforcement_test.go`, which fails if a `split-window` pane-creation site appears outside the chokepoint and outside the named allowlist.
  Write this section with semantic line breaks, one sentence per line, per the `markdown-semantic-line-breaks` Shared Decision.
- **Commit:** `feat(reed): resolve lyx to the spawning binary inside every strand pane`

### Card 4: Hermetic tests for the prelude composition

- **Context:**
  - `internal/reedengine/panebin.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/logcapture_test.go`
  - `internal/reedengine/lock_test.go`
  - `internal/shell/shell.go`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/panebin_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/panebin_test.go` in `package reedengine`, untagged, spawning no process, with a file header comment in this package's style.
  Every case injects the executable path by overriding `executablePath` and restoring it with a `t.Cleanup` call — a stdlib `testing` helper, signature inlined, no file read needed;
  no case reads the live value.

  `TestPaneBinPrelude_ComposesPrependThenExport` drives `paneBinPrelude` directly with an injected absolute path, once per dialect via `shell.Posix()` and `shell.Pwsh()`.
  It asserts the result is a single line with no newline;
  that the `PATH` prepend names the executable's parent directory and appears before the `LYX_BIN` export;
  that the export carries the full binary path, not the directory;
  and that the two statements are joined by `"; "`.

  `TestComposePaneLaunchLine_PreludeThenCommand` drives `composePaneLaunchLine` with a non-empty launch command and an injected executable path.
  It asserts the composed line is one line, that it ends with the launch command unchanged, and that the `PATH` prepend, the `LYX_BIN` export and the command appear in that order.

  `TestComposePaneLaunchLine_EmptyCmdEmitsThePreludeAlone` drives the empty-command case — the shape the interactive operator strand produces, since that strand is added with no command at all.
  It asserts the result equals `paneBinPrelude`'s own output exactly: no trailing separator, no empty trailing fragment.

  `TestComposePaneLaunchLine_ExecutableErrorWarnsAndPassesTheCommandThrough` overrides `executablePath` with a function returning an error, captures logs via this package's existing `captureLogOutput` helper, and asserts two things: the returned line is the launch command byte-for-byte unchanged, and the captured output contains a warning naming the strand.
  Add a companion case asserting that an executable-path error with an empty launch command returns the empty string, so the send-keys payload is identical to today's.

  `TestComposePaneLaunchLine_UsesTheSameDialectAsTheLaunchCommand` asserts that `composePaneLaunchLine(shell.ForGOOS(), …)` produces the dialect `shell.ForGOOS()` itself produces on the running host, by comparing against `paneBinPrelude(shell.ForGOOS(), exe)`.
  This is the guard that keeps the prelude and the `ForGOOS()`-built launch command from ever diverging;
  it must not branch on `runtime.GOOS`.

  `TestComposePaneLaunchLine_DashLeadingLineStillRoundTripsThroughSendKeysLiteralArg` composes a line from a dialect and injected path whose result begins with `-`, or asserts the property on a synthetic dash-leading composed string, and checks `sendKeysLiteralArg` still prefixes the single space tmux needs.
  The point is that the composed string is opaque to the dash guard — nothing downstream may assume the payload starts with the strand's own command.
- **Commit:** `test(reed): cover the pane-binary prelude composition hermetically`

### Card 5: Regression-guard the launch payload and the untouched split argv

- **Context:**
  - `internal/reedengine/panebin.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/lock_test.go`
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/shell/shell.go`
- **Edits:**
  - `internal/reedengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add cases to `internal/reedengine/spawn_test.go` that pin the two properties the wiring in card 3 could silently break, using the existing `newTestEngine` fixture and its `e.tmux.execHook` fake recorder that the file's `TestLaunchStrandLocked_*` cases already use.
  Both cases override `executablePath` with an injected path and restore it with a `t.Cleanup` call — a stdlib `testing` helper, signature inlined, no file read needed.

  `TestLaunchStrandLocked_SendsThePreludeAheadOfTheStrandCommand` records the `send-keys` argv the fake receives and asserts the literal payload equals `composePaneLaunchLine(shell.ForGOOS(), <the strand's cmd>, <the strand's GUID>)` — not the bare command.
  It also asserts the payload is a single line and that the `Enter` submit still follows as a separate `send-keys` call, so the two-step send is unchanged.

  `TestLaunchStrandLocked_SplitWindowCarriesNoTrailingShellCommand` is the regression guard for the `pane-start-mode-is-untouched` Shared Decision, and is the single most load-bearing assertion in this batch.
  It records the `split-window` argv and asserts it ends with the `-F` flag and its `#{pane_id}` value — that is, that no trailing shell-command argument was appended after them.
  The assertion must be written so that appending any trailing argument fails it, not merely so that a specific known-bad value fails it.
  Its doc comment records why: a trailing shell-command makes tmux hand the pane to `/bin/sh -c`, which execs a non-login shell that skips `~/.profile` / `~/.bash_profile` and therefore changes the pane's inherited `PATH` — and that pane resolves `claude` by bare name, so the change would stop the agent binary resolving at all.

  Update `spawn_test.go`'s file header comment so it names the added coverage alongside the reap-before-allocate ordering and `loadOrInitStateLocked` bootstrap it already lists.
  Do not modify the existing `TestLaunchStrandLocked_ReapsUntrackedPanesBeforeChoosingASplitTarget`, `TestLaunchStrandLocked_SkipsTheRedundantReEnumerationWhenNothingIsReaped`, `TestSendKeysLiteralArg`, `TestValidateSplitCreatedNewPane`, `TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentity`, `TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity` or `TestStatus_NeverReportsAStrandLiveOnAPaneAnotherOwnerClaims` cases — none of them asserts the send-keys payload or the split argv, so all of them stay green unchanged.
- **Commit:** `test(reed): guard the prelude payload and the unchanged split-window argv`

### Card 6: AST enforcement test for the pane-creation chokepoint

- **Context:**
  - `internal/reedengine/selvagepane_enforcement_test.go`
  - `internal/reedengine/panebin.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/selvagepane.go`
  - `internal/reedengine/probe.go`
  - `cmd/lyx/constraintchokepoint_test.go`
  - `tools/sandbox/pathresolve_guard_test.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/reedengine/panebin_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/reedengine/panebin_enforcement_test.go` in `package reedengine`, untagged and spawning no process, modelled directly on `selvagepane_enforcement_test.go`: `runtime.Caller(0)` to resolve this file's directory, then the repository root from it, then `go/parser` over every non-`_test.go` `.go` file directly inside `internal/reedengine` — never `internal/reedengine/render/` and never a sibling package.

  `TestPaneCreationSitesRouteThroughThePreludeChokepoint` walks each parsed file for an `*ast.BasicLit` whose value is the string `split-window` and fails for any occurrence in a file outside a named allowlist.
  Scanning the AST rather than raw bytes is what keeps a doc comment mentioning `split-window` from tripping the check — `go/parser` turns a comment into neither an identifier nor a basic literal.
  The allowlist is a `map[string]bool` with exactly three entries, each carrying a comment naming why it is allowed:
  - `spawn.go` — the chokepoint itself, the one strand-pane split.
  - `selvagepane.go` — Selvage's own split, exempt by name per the Pane Binary Resolution clause: Selvage is a one-row status band with no operator input and no `lyx` invocation.
  - `probe.go` — the capability-name table, which lists `split-window` as a required subcommand rather than issuing one.

  `TestLaunchStrandLockedStillComposesThePrelude` parses `spawn.go`, locates the `launchStrandLocked` function declaration, and fails unless its body contains a call to `composePaneLaunchLine`.
  This is what keeps the allowlisted chokepoint from silently becoming a plain pass-through: the first test proves no second pane-creation site exists, and this one proves the single site still composes the prelude.

  Guard against a misconfigured scan the way `tools/sandbox/pathresolve_guard_test.go` already does with its own scanned-file floor: fail if fewer than a plausible minimum number of non-test `.go` files were parsed, so a broken directory read reports as a failure rather than as a vacuous pass.

  Record the residual honestly in the file's doc comment, as this package's other enforcement test does: the scan is a tripwire over string literals in this one package, so a pane created by a helper in another package, or by a `split-window` value assembled from fragments or held in a variable defined elsewhere, is not seen.
  It narrows the gap the invariant exists to close rather than closing it, which is the same honesty `selvagepane_enforcement_test.go` and `cmd/lyx/constraintchokepoint_test.go` already practise.
- **Commit:** `test(reed): enforce that every pane-creation site routes through the prelude chokepoint`

### Card 7: Repoint the empty-command integration test at the prelude-only payload

- **Context:**
  - `internal/reedengine/panebin.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/ensuresession_integration_test.go`
  - `internal/reedengine/contract_integration_test.go`
- **Edits:**
  - `internal/reedengine/emptycmd_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `internal/reedengine/emptycmd_integration_test.go` currently pins a premise card 3 makes false.
  Its file doc comment states that `launchStrandLocked` issues a `send-keys` literal of the empty string followed by `Enter` for an empty command, and that `sendKeysLiteralArg` returns the empty string for it.
  With the prelude in place, an empty-`Cmd` strand receives the prelude alone.
  Update the test rather than replacing it — keep `TestAddStrand_EmptyCmdLeavesALivePane`, its `newColdScratchEngine` fixture, its `AddSpec` shape and its `Status`-based liveness assertion exactly as they are, and change only what describes the payload.

  Rewrite the file doc comment so the premise it pins is the prelude-only payload: for an empty command, `launchStrandLocked` sends the composed prelude with no trailing separator and no empty command fragment, and this test confirms a real tmux accepts that payload and leaves the pane live.
  Repoint its citation of a prior task's `_mill/discussion.md` `empty-cmd-leaves-the-panes-own-shell` decision at this task's `prelude-is-session-scoped-in-both-dialects` decision, which is the reason the empty-`Cmd` operator pane now receives a session-scoped statement in both dialects rather than nothing on POSIX.

  Update `TestAddStrand_EmptyCmdLeavesALivePane`'s own doc comment the same way: the property under test is that an empty command still leaves a live pane running the pane's own shell, now with the prelude applied to it, rather than that an empty `send-keys` literal is accepted.
  Keep the `//go:build integration` tag, the package clause and the imports unchanged.
- **Commit:** `test(reed): repoint the empty-cmd integration test at the prelude-only payload`

## Batch Tests

`verify: go test ./internal/reedengine/ && go test -tags integration ./internal/reedengine/` runs the one package every card in this batch touches, in both tiers.

The untagged half covers cards 4, 5 and 6 — `panebin_test.go`, the added cases in `spawn_test.go`, and `panebin_enforcement_test.go` — plus the whole existing hermetic suite, which is the regression surface for card 3's edit to `launchStrandLocked`.
It also re-runs `selvagepane_enforcement_test.go`, whose case-insensitive Selvage scan must keep passing over the newly added `panebin.go`.
The untagged run is ~2s.

The `-tags integration` half is required rather than optional: card 7 edits `emptycmd_integration_test.go`, which is behind that tag and is therefore invisible to the untagged run.
It is written as a second `&&`-chained invocation carrying its own `-tags` flag rather than by folding the tag into the first invocation, so neither run's tag set is widened for the other.
The tagged run takes ~20s against a live tmux and skips cleanly when the multiplexer is absent.

`CONSTRAINTS.md` and `doc.go`, both edited in card 3, have no assertion in this package;
`CONSTRAINTS.md`'s prose is covered by the task-wide `pipeline.done_gate`, and `doc.go` is a comment-only file with no runnable surface.
No wider scope is used: nothing outside `internal/reedengine` changes in this batch, and `internal/shell`'s own coverage landed in batch 1.
