# Batch: dotfill-repro-harness

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
batch: "dotfill-repro-harness"
number: 1
cards: 6
verify: go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/
depends-on: []
```

## Prior failure

- Round 1: `TestSmokeDotFillResizeControl cannot be made to reproduce the dot-fill artifact via harness resize-window in either direction in this environment, after fixing the dot-run predicate's glyph (tmux 3.6 emits U+00B7, not ASCII '.') got the other two new tests passing; extensive additional reproduction techniques (multi-step drag resize, concurrent-capture race hunting, 8-15 strand stacks) also failed to hit within the plan's 5s window`

## Batch Scope

This batch builds the measuring instrument and nothing else: a new build-tagged `smoke` test file that reproduces the dot-fill artifact headlessly and asserts on the *rendered client output*.
It adds no production code and changes no production behaviour.

It is one batch because every piece here is a single file's worth of mutually-dependent test scaffolding — the dot-run predicate, the harness-in-harness setup, the `window-resized` array rewrite/readback helpers, and the two control scenarios that prove the artifact reproduces.
Splitting them would put half a test file in one batch and half in another.

The external interface the next two batches consume is the exported-within-package helper set this file defines: `newDotFillHarness`, `pollPaneHasDotRun`, `paneStaysCleanOfDotRun`, `windowResizedEntries`, `rewriteWindowResizedArray`, `pinOnlyEntries`, and `assertOnlyPinEntries`.
Batch 2 composes candidate hook bodies on top of them;
batch 3 adds the treatment scenario on top of them.

Batch-local decision beyond `## Shared Decisions`: the control scenarios must **hit** — a control that does not reproduce the artifact fails the run, because that means the harness can no longer reproduce the bug and every companion assertion in batches 2 and 3 has become vacuous.

## Cards

### Card 1: dot-run predicate and its polling wrappers

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_attach_test.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the file with a `//go:build smoke` constraint on its first line and `package reedcli`, plus a file-level doc comment stating that this file reproduces the tmux client-side dot-fill artifact described by reed's `root-cause-model` decision, that the dots live in what tmux paints to a client's terminal and are in no pane's grid, and that capturing the *harness* pane hosting the attach client is therefore the only way to observe them.
  Add these identifiers:
  - `const dotRunFloor = 20` — the minimum number of consecutive `.` characters on one captured line that counts as the artifact.
    Document at the constant that 20 is fixed, not tunable: far above anything reed's own rendered content produces on one line, far below the width of any pane region tmux would pad.
    Card 6 validates it once against a clean capture, and that validation is a gate rather than a licence to retune.
  - `func lineHasDotRun(line string) bool` — reports whether `line` contains a run of at least `dotRunFloor` consecutive `.` characters.
    Implement it with `strings.Repeat(".", dotRunFloor)` and `strings.Contains`, so a longer run also matches.
  - `func captureHasDotRun(capture string) bool` — splits `capture` on `"\n"` and reports whether `lineHasDotRun` holds for any single line.
    Per-line is load-bearing: a whole-capture substring test would join dots across line boundaries.
  - `func pollPaneHasDotRun(t *testing.T, tmuxPath, socket, target string, timeout time.Duration) bool` — captures the pane via the existing `capturePane` helper every 100 ms until `captureHasDotRun` holds (returns `true`) or `timeout` elapses (returns `false`).
    It must be a `t.Helper()` and must not itself fail the test — the caller decides whether a hit or a miss is the expected outcome.
  - `func paneStaysCleanOfDotRun(t *testing.T, tmuxPath, socket, target string, window time.Duration) bool` — samples the pane every 100 ms for the whole of `window`, returning `false` on the first sample where `captureHasDotRun` holds and `true` only if every sample was clean.
    Document that it must never return early on a clean sample: an absence assertion that did so would pass before the artifact had a chance to appear.

  Both pollers sample at 100 ms rather than reusing `pollPaneContains`.
  Document why at each: `pollPaneContains` takes a plain substring, and legitimate harness-pane content contains dots (file paths, ellipses, the header template), so reusing it would ship a test that proves nothing;
  and its 500 ms cadence is a quarter of the window the artifact would occupy under `watchdog: on`, too coarse to characterise it.
- **Commit:** `test(reed): add the dot-run predicate for the client-side dot-fill artifact`

### Card 2: harness-in-harness scenario setup helper

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_attach_test.go`
  - `internal/hubforge/hub.go`
  - `internal/reedengine/template_posix.yaml`
  - `internal/reedengine/render/rules.go`
  - `internal/reedengine/attach.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `dotFillHarness` struct holding `tmuxPath`, `lyxExe`, `harnessSocket`, `reedSocket`, `reedSession` (all `string`), and a constructor `func newDotFillHarness(t *testing.T, cols, rows int) *dotFillHarness` that performs, in this order:
  1. `t.Setenv("LYX_REED_WATCHDOG", "off")` — **before** any `RunCLI` call, so every config load in this process resolves the key to `off`.
     Document that this is the `watchdog-off-in-every-smoke-scenario` shared decision, and that its mechanical consequence is that `resizeSignalHookCommand` answers `""`, so reed's `window-resized` array holds the resize-pane pins and no signal entry.
  2. `tmuxBinaryPath(t)`, `harnessShellBinaryPath(t)`, `buildLyxBinary(t)`.
  3. `hubforge.NewHub(t, ".")`, `deferHubRelease(t, h.PrimeWorktree())`, `t.Chdir(h.PrimeWorktree())`, and a `t.Cleanup` running `RunCLI` with `[]string{"down"}` into a discarded buffer — mirroring the opening of `TestSmokeAttachRendersInsideHarnessPane`.
  4. `RunCLI` with `[]string{"up"}`, failing the test on a non-zero code.
  5. Two `addStrand(t, smokeMarkerLaunchCmd(...), "--name", ...)` calls with distinct markers, so the reed session carries a header pane plus two strand panes.
     Two strands are a fidelity choice, not a pin-count requirement — state the reason accurately in the helper's comment rather than the pin-set one, which is wrong: `render.FixedHeightPins` emits the header pin whenever a header is placed and the layout is not the sole-header case, so a single strand already yields a non-empty pin set.
     What two strands buy is a taller stack for the resize round-robin to distribute rows across, so the mid-relayout region is larger and the artifact reproduces more reliably;
     and it keeps the scenario clear of `AttachArgv`'s `len(live) < 2` guard boundary rather than sitting exactly on it.
  6. `socketAndSession(t)` into `reedSocket`/`reedSession`.
  7. Boot a second tmux server on its own socket named `fmt.Sprintf("lyx-dotfill-harness-%d", os.Getpid())` with `new-session -d -s h -x <cols> -y <rows> <shellPath>`, register `t.Cleanup(func() { reapHarnessServer(t, tmuxPath, harnessSocket) })`, and poll `has-session -t h` to a 30 s deadline — the same shape `TestSmokeAttachRendersInsideHarnessPane` uses.
  8. Pin the harness window's geometry so a later resize is deterministic: `set-option -t h -w window-size manual` against the harness socket.

  Add `func (h *dotFillHarness) attachIn(t *testing.T, paneID, marker string)` that sends `smokeAttachInvokeLine(h.lyxExe)` into `paneID` via `sendKeysLine` and then waits for the attach to have rendered by calling `pollPaneContains(t, h.tmuxPath, h.harnessSocket, paneID, marker, 20*time.Second)` with one of the strand markers.
  Waiting on a marker rather than a fixed sleep is what makes the scenarios deterministic.
- **Commit:** `test(reed): add the harness-in-harness setup helper for dot-fill scenarios`

### Card 3: window-resized array rewrite and readback helpers

- **Context:**
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/reapply.go`
  - `internal/reedengine/attach.go`
  - `internal/reedcli/smoke_test.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add four helpers, all `t.Helper()`, all issuing tmux commands against reed's own socket:
  - `func windowResizedEntries(t *testing.T, tmuxPath, socket, session string) []string` — runs `show-options -v -t "=<session>:" window-resized` and splits the answer on `"\n"`, mirroring `hookArrayEntries` exactly.
    Document that `show-options -v` prints every entry of an array option one per line in index order with the `window-resized[N]` prefix suppressed, and that a trailing newline yields a trailing empty entry which callers must tolerate rather than special-case.
    The `=<session>:` window form is required here because `window-resized` is a window-scoped option;
    do not use the bare `=<session>` session form for this call.
  - `func pinOnlyEntries(entries []string) []string` — returns only the entries carrying the `"resize-pane "` prefix.
    Document that this is exactly "reed's own array minus the repaint entry" in both eras: before this task reed's array under `watchdog: off` is pins alone, and after batch 3 it is pins plus one repaint entry, so this filter needs no revision when the repaint entry ships.
  - `func rewriteWindowResizedArray(t *testing.T, tmuxPath, socket, session string, entries []string)` — issues `set-hook -u -w -t "=<session>:" window-resized` first, then one `set-hook` per entry: the plain replacing form for index 0 and the `-a` appending form for every entry after it.
    Skip empty entries.
    Document that this reproduces `resizePinHookArgvs`'s own plain-first/`-a`-after rebuild pattern, and that rewriting the array from the test rather than adding a production seam is deliberate — it needs no build-tagged env knob, no exported test hook, and no branch in shipping code.
  - `func assertOnlyPinEntries(t *testing.T, entries []string)` — fails the test unless every non-empty entry carries the `"resize-pane "` prefix, and unless at least one such entry is present.
    This is the control's proof that it fired against the array it wrote, converting "we think no attach intervened" into an assertion.
    It performs the same per-entry matching `hookInstalledLocked` performs, never a match against the whole answer.

  Document at `rewriteWindowResizedArray` the sequencing rule the control scenarios depend on: every `AttachArgv` pre-flight rebuilds the array from scratch, so the rewrite must be the **last** setup step, after every attach the scenario performs, and immediately before the trigger.
  An attach performed after the rewrite would re-install reed's own array and the control would silently assert against the wrong one.
- **Commit:** `test(reed): add window-resized array rewrite and readback helpers`

### Card 4: validate the 20-dot floor against a clean capture

- **Context:**
  - `internal/reedcli/smoke_test.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func TestSmokeDotFillFloorIsCleanOnASettledAttach(t *testing.T)`: build a harness via `newDotFillHarness` at a single known size, attach one client into the harness's only pane via `harnessOnlyPaneID` and `attachIn`, then assert `captureHasDotRun` is false for a capture taken once the attach has settled, and that `paneStaysCleanOfDotRun` holds for a 2 s window with no trigger fired.
  Fire no resize and deliver no input in this test.
  On failure, the test message must state that something in reed's own rendered output produces a run of at least `dotRunFloor` dots on one line, that this is itself news, and that the remedy is to report it — never to raise the floor until the test goes quiet.
- **Commit:** `test(reed): pin the dot-run floor against a clean settled attach`

### Card 5: resize-trigger control scenario

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_attach_test.go`
  - `internal/reedengine/windowsize.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func TestSmokeDotFillResizeControl(t *testing.T)`.
  Steps, in order:
  1. `newDotFillHarness(t, 140, 42)`.
  2. `harnessOnlyPaneID` for the harness pane, then `attachIn` on it.
  3. **Last setup step, after the attach:** read reed's array with `windowResizedEntries`, filter it with `pinOnlyEntries`, and rewrite it with `rewriteWindowResizedArray`.
     Then read it back again and pass it to `assertOnlyPinEntries`.
  4. Fire the trigger: resize **reed's own window directly**, with `resize-window -t <reedSession> -x <w> -y <h>` against **reed's own socket** (`h.reedSocket`/`h.reedSession`), in both directions — first a shrink to a distinctly smaller size, then a grow back past the original.
     Both directions are required;
     the shrink direction is the half a growth-only mechanism misses.
     (Corrected from an earlier cascaded-resize design that resized the outer harness window instead: a live diagnostic during batch 1 confirmed that resizing the harness window's pane, expecting the resize to cascade through the attached client's terminal into reed's own window-resize hook, does not reliably reproduce the artifact in this container's tmux 3.6 build within any timing tried — while resizing reed's own window directly on reed's own socket reproduces it reliably. Both paths exercise the same code under test (reed's `window-resized` hook array firing on a real window-dimension change to a window carrying the `resize-pane` pins); only the delivery mechanism changes.)
  5. Assert `pollPaneHasDotRun` returns `true` against the harness pane within a 5 s deadline, passing on the first hit.

  The failure message must state that a control that does not hit means the harness can no longer reproduce the bug and every companion absence assertion has become vacuous — so this is a run failure, not a skip.
  Do not assert the artifact appears on every size or on every run;
  that is exactly why this control exists as an executable assertion rather than as a note in a commit message.
- **Commit:** `test(reed): add the resize-trigger dot-fill control scenario`

### Card 6: cross-client-trigger control scenario

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_attach_test.go`
  - `internal/reedengine/windowsize.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func TestSmokeDotFillCrossClientControl(t *testing.T)`.
  This scenario is **control-only** — there is no cross-client treatment scenario, in any branch of the measurement gate.
  Its comment must cite the `uncovered-subset-is-documented-not-fixed` decision by name and state that it is a documentation-of-behaviour test, not a fix test.

  Steps, in order:
  1. `newDotFillHarness(t, 140, 42)`.
  2. Split the harness window with `split-window -v -t h` against the harness socket so it holds two panes, then size them deliberately unequally with `resize-pane`: the **observed** pane distinctly taller (about 30 rows) and the **toucher** pane distinctly shorter (about 8 rows).
     Record both pane ids from `list-panes -t h -F '#{pane_id}'`.
     Size them this way round because it is the configuration the field report describes — a VS Code integrated terminal is smaller than a standalone Konsole window — and it is the only configuration this trigger is known to reproduce in.
  3. `attachIn` on the observed pane, then `attachIn` on the toucher pane.
     Both attaches complete before anything else.
  4. **Last setup step, after both attaches:** `windowResizedEntries` -> `pinOnlyEntries` -> `rewriteWindowResizedArray`, then read back and `assertOnlyPinEntries`.
  5. Fire the trigger: deliver input to the toucher client with `send-keys -t <toucher pane> Escape` against the harness socket.
     Any client input suffices — a keystroke, a mouse report, a focus report — because what the trigger needs is only to make that client the most-recently-used one, which is what `window-size latest` keys on.
     No resize is fired and no reed code path runs.
  6. Assert `pollPaneHasDotRun` returns `true` against the **observed** pane within a 5 s deadline.
  7. Assert the artifact is standing rather than transient: `paneStaysCleanOfDotRun` against the observed pane over a 2 s window must return `false`.

  Document at the test that under `root-cause-model` these dots are the **uncovered** subset — the window shrank to the toucher client's size, so the taller observed client has real estate with nothing behind it and tmux is padding it correctly — and that no repaint mechanism can remove them.
- **Commit:** `test(reed): add the cross-client dot-fill control scenario`

## Batch Tests

`verify:` runs `go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/`, which covers exactly the three tests this batch adds: `TestSmokeDotFillFloorIsCleanOnASettledAttach`, `TestSmokeDotFillResizeControl`, and `TestSmokeDotFillCrossClientControl`.
The `-tags smoke` flag is required — the whole file is behind `//go:build smoke` — and the `-run` pattern keeps the run off every other smoke suite in the package, several of which drive a live `claude` session and cost minutes each.
`-count=1` defeats the test cache, which matters for a timing-dependent live-substrate test.
The 25 m timeout covers the hub fixture build plus the `lyx` binary build plus three live tmux scenarios.

There are no untagged tests in this batch: it adds no production code and no pure function that a Tier 1 test could reach.
