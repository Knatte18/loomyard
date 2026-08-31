# Batch: repaint-entry

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
batch: "repaint-entry"
number: 3
cards: 3
verify: go test -count=1 ./internal/shell/ ./internal/reedengine/ && go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/
depends-on: [2]
```

## Batch Scope

This batch ships whatever the measurement gate authorised, and nothing more.
Every card below is **three-way conditional** on the `Measurement record (repaint candidates)` block batch 2 wrote into `internal/reedengine/doc.go`'s package doc comment: candidate 1 accepted, candidate 2 accepted, or no candidate accepted.
Read that record first, decide the branch once, and apply the same branch consistently across all three cards.

It is one batch because the branch decision is a single fact and every card depends on it;
splitting it would risk two cards taking different branches.

Each card produces a real diff in every branch — the no-candidate branch is a complete, acceptable outcome, not a skip, and it still lands a production doc note, a test that documents the deliberate absence, and an inverted treatment scenario that acts as a live tripwire.

Batch-local decisions beyond `## Shared Decisions`:

- The repaint entry is **not** gated on `watchdogOption`.
  The `watchdog` key gates the watch loop and its signal entry only.
  A forced redraw mutates nothing, so an operator who turns off self-healing must keep the repaint.
- No card in this batch touches `manifest/roadmap.md`, the `window-size latest` pin, the `mouse` pin, the watchdog timings, or anything under `internal/reedengine/render`.

## Cards

### Card 12: production wiring for the accepted repaint mechanism

- **Context:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/attach.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/probe.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/windowsize.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Read the `Measurement record (repaint candidates)` block in `internal/reedengine/doc.go` and take exactly one branch.

  **Branch A — candidate 1 accepted.**
  1. `internal/shell/shell.go`: add two members to the `Shell` interface, each with a doc comment in the style of the existing `Quote`/`Invoke`/`ReadFile`/`WithEnv`/`Touch` comments:
     - `ForEachLine(command, body string) string` — returns the shell syntax that runs `command` and executes `body` once per line of its output.
     - `LineVarRef() string` — returns the dialect's reference to `ForEachLine`'s current-line value, so a caller never spells a dialect's variable syntax itself.
  2. `internal/shell/posix.go`: implement both on `posixShell`.
     `ForEachLine` returns `command + " | while IFS= read -r line; do " + body + "; done"`.
     `LineVarRef` returns `"$line"`.
     The loop variable name and `LineVarRef`'s answer must agree;
     say so in the doc comment, because they are the same fact spelled in two places.
  3. `internal/shell/pwsh.go`: implement both on `pwshShell`.
     `ForEachLine` returns `command + " | ForEach-Object { " + body + " }"`.
     `LineVarRef` returns `"$_"`.
     Document, as `Touch` already does, that only the POSIX dialect is executed in practice here, and that the pwsh implementation exists because the members are on the interface.
  4. `internal/reedengine/watchdog.go`: add a pure builder beside `resizeHookCommand`, sharing its file for the same reason `resizeHookCommand` lives there — it is the string, with no I/O and no engine state:

     ```go
     func repaintHookCommand(sh shell.Shell, tmuxPath, socket, sessionTarget string) string
     ```

     It composes the enumeration `sh.Invoke(tmuxPath) + " -L " + sh.Quote(socket) + " list-clients -t " + sh.Quote(sessionTarget) + " -F " + sh.Quote(<the client-name format the measurement record names>)`, the per-line body `sh.Invoke(tmuxPath) + " -L " + sh.Quote(socket) + " refresh-client -t " + sh.LineVarRef()`, joins them with `sh.ForEachLine`, and returns `"run-shell -b " + tmuxQuoteValue(<the joined fragment>)`.
     Use the client-name format string the measurement record names verbatim — the record settles whether tmux's own format expansion on a `run-shell` argument required the doubled-`#` escape.
     Build no shell syntax here by string concatenation of shell operators;
     every dialect-specific token comes from a `Shell` member, per CONSTRAINTS.md's Shell Mechanics Seam.
     Document that `tmuxQuoteValue`'s `$` escaping is load-bearing for this body specifically: the loop's own shell variable must reach the shell as a literal `$`.
  5. `internal/reedengine/windowsize.go`: add the engine wrapper beside `resizeSignalHookCommand`, which it is the direct analogue of:

     ```go
     func (e *Engine) resizeRepaintHookCommand() string
     ```

     It returns `""` when `runtime.GOOS == "windows"` and otherwise `repaintHookCommand(shell.ForGOOS(), e.TmuxPath(), e.Socket(), exactSessionTarget(e.SessionName()))`.
     The target is `exactSessionTarget` — the bare `=<name>` form — because `list-clients -t` takes a session target;
     `exactSessionWindowTarget`'s trailing colon exists solely for tmux's window/pane parsers, and `set-hook` keeps using the window form unchanged.
     Its doc comment must say that the `""`-on-Windows check is its own and inherits nothing: `installResizePinsLocked` carries no `runtime.GOOS` gate, and the only mechanism keeping the signal entry off Windows is `resizeSignalHookCommand` returning `""` combined with `resizePinHookArgvs` emitting no entry for an empty body.
     It must also say the wrapper is deliberately **not** gated on `watchdogOption`, and why.

  **Branch B — candidate 2 accepted.**
  Do the `internal/reedengine/windowsize.go` step 5 above with one difference: `resizeRepaintHookCommand` returns the literal tmux command `"refresh-client"` off Windows, declared as a file-level `const` beside `windowResizedHookName`'s style.
  Make no change to `internal/shell/shell.go`, `internal/shell/posix.go`, `internal/shell/pwsh.go`, or `internal/reedengine/watchdog.go` in this branch.
  Document at the wrapper that candidate 2's body is a tmux command rather than a shell fragment, so it carries no `run-shell`, no `-b`, no `tmuxQuoteValue`, and no `internal/shell` involvement, and that it reaches only the hook's own client.

  **Branches A and B share this wiring**, in `internal/reedengine/windowsize.go`:
  - Change `resizePinHookArgvs`'s signature to `resizePinHookArgvs(session string, pins []render.Pin, repaintCommand, signalCommand string) [][]string`.
    Emit the repaint entry after every resize-pane pin and before the signal entry, and emit no entry when `repaintCommand` is empty — the same empty-body rule `signalCommand` already follows.
    Extend its doc comment to state the new entry order and to say the repaint entry sits after the pins so it paints geometry the pins have already fixed up, and before the signal entry so the signal entry keeps its documented last position.
  - Change `installResizePinsLocked` to pass `e.resizeRepaintHookCommand()` as the new argument.
    It stays the array's only install site;
    add no second install site and add no new hook name.
  - Leave `hookInstalledLocked` in `internal/reedengine/reapply.go` unchanged.
    Its per-entry matching against the signal command already tolerates an additional unrelated entry, and card 13 pins that.
  - Add nothing to `requiredSubcommands` in `internal/reedengine/probe.go`.
    A `refresh-client` an alternative multiplexer does not implement makes the entry a server-fired no-op, which is the same outcome as the mitigation not helping.

  **Branch C — no candidate accepted.**
  Make no functional change.
  Instead add one paragraph to `installResizePinsLocked`'s doc comment in `internal/reedengine/windowsize.go` recording that a repaint entry was measured and not shipped, pointing at the `Measurement record (repaint candidates)` block in `internal/reedengine/doc.go` for which candidates were tried and which criterion each failed, and stating that the artifact's remaining duration is the latency of the watchdog round trip.
  Leave `resizePinHookArgvs`'s signature unchanged in this branch.
  Make no change to `internal/shell/shell.go`, `internal/shell/posix.go`, `internal/shell/pwsh.go`, or `internal/reedengine/watchdog.go` in this branch.
- **Commit:** `fix(reed): install the measured repaint entry in the window-resized array`

### Card 13: unit tests for the accepted mechanism and existing-test impact

- **Context:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/overlay.go`
  - `internal/reedengine/render/rules.go`
  - `internal/shell/shell.go`
  - `internal/shell/posix.go`
  - `internal/shell/pwsh.go`
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/shell/shell_test.go`
  - `internal/reedengine/watchdog_test.go`
  - `internal/reedengine/windowsize_test.go`
  - `internal/reedengine/reapply_test.go`
  - `internal/reedengine/apply_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Take the same branch card 12 took.

  **Branch A only — `internal/shell/shell_test.go`.**
  Add table tests for both new members in both dialects, in the shape the file's existing per-member tests use (`TestPosixShell_Touch` / `TestPwshShell_Touch` are the closest models):
  the emitted syntax for a simple command and body;
  a command containing spaces and quotes, composed through `Quote`;
  and that a body composed with `LineVarRef()` produces the dialect-correct variable — `$line` for POSIX, `$_` for pwsh.
  Both implementations are tested even though only POSIX executes here, because both are on the interface.

  **Branch A only — `internal/reedengine/watchdog_test.go`.**
  Add `TestRepaintHookCommand_Posix` asserting the composed string carries the `run-shell -b ` prefix, is wrapped by `tmuxQuoteValue` (leading and trailing double quote), escapes `\`, `"`, and `$`, embeds the binary path and socket quoted via `Shell.Quote` in **both** tmux invocations, and takes its per-line reference from `LineVarRef()` rather than a literal.
  Add `TestRepaintHookCommand_Pwsh` asserting the pwsh dialect's `ForEach-Object` shape appears intact.

  Add the anti-drift pin, `TestRepaintHookCommand_ReproducesTheMeasuredBody`.
  Declare four `const`s transcribed verbatim from the measurement record: the measured body string, the tmux binary path, the socket name, and the session name.
  Assert `repaintHookCommand(shell.Posix(), <recorded tmux path>, <recorded socket>, exactSessionTarget(<recorded session>))` equals the recorded body byte for byte.
  Document at the test that the pin is stated as a *reproduction* property rather than a bare literal because the body embeds machine-specific values, and that its purpose is to stop the shipped builder drifting from the string the gate actually measured.

  **Branch B only — `internal/reedengine/windowsize_test.go`.**
  Add the simpler literal pin: `resizeRepaintHookCommand()` returns the exact constant the measurement record names off Windows, and `""` on Windows, following the GOOS-skipped shape `TestResizeSignalHookCommand` already uses.
  Skip the `internal/shell/shell_test.go` and `internal/reedengine/watchdog_test.go` additions entirely in this branch.

  **Branches A and B share these.**
  - `internal/reedengine/windowsize_test.go`: update every `resizePinHookArgvs` call site for the new four-argument signature.
    Add cases covering the repaint entry's position and count: zero pins with a repaint entry only;
    several pins plus a repaint entry;
    pins plus a repaint entry plus a signal entry, asserting the order is pins, then repaint, then signal;
    an empty repaint body emitting no entry;
    and the case where the repaint entry is the array's only entry.
    In every case assert the clear stays first and unconditional, that index 0 uses the plain replacing form, and that every entry after it uses `-a` — reuse `assertResizePinHookArgvsWellFormed`.
    State in a comment that `resizePinHookArgvs` takes command *strings* and holds no `runtime.GOOS` branch, so the Windows behaviour cannot be asserted at this seam;
    what belongs here is only "an empty body emits no entry".
  - `internal/reedengine/windowsize_test.go`: update `TestInstallResizePinsLocked_IssuesTheSignalEntryLast`'s exact call counts for the extra entry, and strengthen it to assert the signal entry is the **last** argv rather than a fixed index, and that the repaint entry sits immediately before it.
    Its `watchdog: off` subtest must now expect the repaint entry to still be installed — that is the `repaint-is-independent-of-watchdog` decision, and it is the assertion that would catch the entry being wrongly gated on `watchdogOption`.
  - `internal/reedengine/apply_test.go`: update the two exact-count subtests that currently pin the zero-pin array.
    `WatchdogOffIsTheClearAlone` must expect the clear plus the repaint entry on a host where `resizeRepaintHookCommand()` is non-empty, and the clear alone where it is empty;
    derive the expectation from the engine rather than hardcoding a number, so the test stays correct on Windows.
    `WatchdogOnAlsoInstallsTheSignalEntry` must expect three calls and must locate the signal entry as the **last** argv rather than at index 1.
  - `internal/reedengine/reapply_test.go`: add a case to `TestReapplyLayout_HookProbeExactMatchOnly` whose scripted `show-options -v` answer contains resize-pane pins, a repaint entry, and reed's own signal command, and assert the probe still reports `HookInstalled == true`.
    This is the regression that matters most: the probe matches per entry, and a new entry must never make a healthy session read as "no hook".
    Add the mirror case too — pins plus a repaint entry and no signal command — asserting `HookInstalled == false` with `HookKnown == true`.

  **Branch C only.**
  Add one test to `internal/reedengine/windowsize_test.go` asserting `resizePinHookArgvs` still emits exactly the clear, one entry per pin, and the signal entry last, with a comment citing the `Measurement record (repaint candidates)` block in `internal/reedengine/doc.go` by name and stating that no repaint entry ships because no candidate satisfied the gate.
  Make no change to `internal/shell/shell_test.go`, `internal/reedengine/watchdog_test.go`, `internal/reedengine/reapply_test.go`, or `internal/reedengine/apply_test.go` in this branch.
- **Commit:** `test(reed): pin the repaint entry's position, quoting, and probe compatibility`

### Card 14: treatment smoke scenario

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_dotfill_measure_test.go`
  - `internal/reedengine/doc.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/reapply.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func TestSmokeDotFillResizeTreatment(t *testing.T)`, the treatment half of the resize pair whose control card 5 already landed.
  It shares the control's setup exactly — `newDotFillHarness(t, 140, 42)`, `harnessOnlyPaneID`, `attachIn` — and fires the same shrink-then-grow `resize-window` trigger.
  It differs in one way: it leaves reed's own array untouched rather than rewriting it.

  Add the helper `func assertRepaintEntryPresent(t *testing.T, entries []string, want string)` beside `assertOnlyPinEntries`, failing unless some entry equals `want` exactly.
  It performs the same per-entry matching `hookInstalledLocked` performs.

  **Branches A and B — a candidate was accepted.**
  Immediately before the trigger, read reed's array with `windowResizedEntries` and pass it to `assertRepaintEntryPresent` with the body the measurement record names.
  This readback is mandatory, not defensive: with `watchdog: off` the array is empty from boot and any *degrading* attach re-empties it, so without this assertion the treatment could pass because no array was installed at all.
  Then fire the trigger and assert `paneStaysCleanOfDotRun` holds against the harness pane for a fixed 3 s window.
  The window is fixed and every sample must be clean;
  an absence assertion that returned early on its first clean sample would pass before the artifact had a chance to appear.

  **Branch C — no candidate was accepted.**
  **Invert** the scenario rather than skipping or deleting it: assert `pollPaneHasDotRun` returns `true` within a 5 s deadline, and omit the `assertRepaintEntryPresent` readback since no repaint entry exists.
  Its comment must cite the `Measurement record (repaint candidates)` block in `internal/reedengine/doc.go` by name and state that an inverted treatment is a live tripwire — if a future tmux release or a future reed change makes the artifact stop appearing, this scenario fails and someone finds out, which a `t.Skip` would never do.
  Do not use `t.Skip` in this branch and do not delete the scenario.

  In every branch, leave `TestSmokeDotFillCrossClientControl` exactly as batch 1 wrote it.
  It asserts presence in every branch because its subject is the uncovered residual rather than the fix, so it needs no disposition here.
- **Commit:** `test(reed): add the resize-trigger dot-fill treatment scenario`

## Batch Tests

`verify:` runs two invocations chained with `&&`.

The first, `go test -count=1 ./internal/shell/ ./internal/reedengine/`, covers every untagged test this batch touches: `internal/shell/shell_test.go`'s new dialect table tests, `internal/reedengine/watchdog_test.go`'s builder and anti-drift pins, `internal/reedengine/windowsize_test.go`'s array-shape and position cases, `internal/reedengine/reapply_test.go`'s probe-compatibility regression, and `internal/reedengine/apply_test.go`'s updated exact-count subtests.
Both packages are named explicitly rather than running `./...`, keeping the per-round cost to the two packages this batch edits.

The second, `go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/`, covers the new treatment scenario alongside batch 1's controls.
Running the controls again here is the point of the pair: a treatment that passes while its control has stopped hitting proves nothing, so both halves must run together.
`-tags smoke` is required because `internal/reedcli/smoke_dotfill_test.go` is behind `//go:build smoke`;
the explicit `-run` pattern keeps the run off the package's `claude`-driving smoke suites.
