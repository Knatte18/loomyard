# Batch: measurement-gate

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
batch: "measurement-gate"
number: 2
cards: 5
verify: go test -tags smoke -count=1 -timeout 40m -run 'TestSmokeDotFill|TestSmokeRepaintCandidate' ./internal/reedcli/
depends-on: [1]
```

## Batch Scope

This batch runs the measurement gate that authorises — or refuses — the repaint entry batch 3 would ship.
It measures each candidate by installing that candidate's body into the `window-resized` array **directly from a smoke scenario**, as a literal string, using batch 1's `rewriteWindowResizedArray`.
No production code is written here, and `internal/shell` is not touched: measuring by building the production code first would mean writing exactly the code the gate exists to authorise.

It is one batch because the two acceptance criteria (`no repeated hook fire`, `no resize storm`), the candidate body composition, and the scenario that exercises them are one experiment;
splitting them would leave an instrument in one batch and its readings in another.

The external interface batch 3 consumes is the `Measurement record (repaint candidates)` block this batch writes into `internal/reedengine/doc.go`'s package doc comment, beside the resize round-robin and hook bullets.
That block is the single source of truth for which branch batch 3 takes and for the four values its anti-drift pin needs.

Batch-local decision beyond `## Shared Decisions`: this batch's test scaffolding composes tmux hook bodies as literal strings in test code.
That is deliberate and is not a Shell Mechanics Seam violation — the seam binds production pane-shell command strings, and the whole point of the gate is to measure a body before any production builder for it exists.

## Cards

### Card 7: acceptance-criteria instrumentation

- **Context:**
  - `internal/reedcli/smoke_dotfill_test.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/windowsize.go`
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_dotfill_measure_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create the file with a `//go:build smoke` constraint on its first line and `package reedcli`, plus a file-level doc comment stating that this file is the measurement gate for the repaint candidates named in reed's `repaint-mechanism` decision, that it installs each candidate's body as a literal from the test rather than from production code, and that a candidate is accepted only when it clears the artifact **and** satisfies both criteria in the `repaint-must-not-self-retrigger` decision.

  Add:
  - `func fireCounterEntry(t *testing.T, countPath string) string` — returns a `window-resized` array entry body that appends one line to `countPath` when the hook fires.
    Compose it as `run-shell -b` plus a double-quoted shell fragment, escaping the value the same way `tmuxQuoteValue` does (backslash-escape `\`, `"`, and `$`).
    `-b` is mandatory: without it the tmux server blocks while the command runs.
  - `func fireCount(t *testing.T, countPath string) int` — reads `countPath` and returns its line count, treating an absent file as zero.
  - `func sampleWindowSize(t *testing.T, tmuxPath, socket, session string, window time.Duration) []string` — polls `display-message -p -t "=<session>:" "#{window_width} #{window_height}"` every 100 ms for the whole of `window` and returns every answer in order.
  - `func assertWindowSizeSettles(t *testing.T, samples []string)` — fails unless the final third of `samples` is a single repeated value.
    Document that this is the `no resize storm` criterion: the window's size must be observably stable after the trigger settles, rather than oscillating between two clients' sizes.
  - `func assertSingleHookFire(t *testing.T, got int)` — fails unless `got` is exactly 1.
    Document that this is the `no repeated hook fire` criterion: one settled resize must yield the documented single fire, not a growing series, and that a growing series is the resize-storm feedback path where a server-issued repaint would move the most-recently-used client pointer that `window-size latest` keys on.

  The counting entry is appended to the array for the duration of the measurement only.
  Document that it is the cheapest available instrument and that it is never part of any shipped array.
- **Commit:** `test(reed): add repaint measurement-gate instrumentation`

### Card 8: candidate body composers

- **Context:**
  - `internal/reedengine/watchdog.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/overlay.go`
  - `internal/shell/posix.go`
  - `internal/shell/shell.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_measure_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add two composers returning the literal array-entry body each candidate would install:
  - `func candidateOneBody(tmuxPath, socket, session string) string` — candidate 1 from the `repaint-mechanism` decision: a `run-shell -b` invocation that enumerates the session's clients and refreshes each one, so it reaches clients other than the one whose resize fired the hook.
    Its shell fragment must embed, in this shape:
    - the multiplexer binary path, POSIX-single-quoted, in **both** tmux invocations — the tmux server's `run-shell` inherits no reed context, so the path cannot be omitted;
    - reed's socket as the `-L` argument, POSIX-single-quoted, in **both** invocations — without it the fragment talks to the default socket, which is not reed's;
    - `list-clients -t '=<session>'` — the **bare** session target form, matching `exactSessionTarget`, never the `=<session>:` window form `exactSessionWindowTarget` produces.
      `list-clients -t` takes a session target;
      the trailing colon exists solely for tmux's window/pane parsers.
      The `=` prefix is what stops tmux prefix-matching a sibling worktree's session on the shared per-hub server and is non-negotiable;
    - `-F '#{client_name}'` as the enumeration format;
    - a POSIX `| while IFS= read -r line; do ... ; done` loop issuing one `refresh-client -t $line` per line.

    Wrap the whole fragment the way `tmuxQuoteValue` does — tmux double quotes with `\`, `"`, and `$` backslash-escaped — and prefix `run-shell -b `.
    The `$` escaping is load-bearing here: the loop's own shell variable must reach the shell as a literal `$`, which is exactly what escaping it away from tmux's double-quote expansion achieves.

    **Known hazard the measurement must settle empirically, stated here so it is not rediscovered as a mystery:** tmux performs format expansion on a `run-shell` argument, so a literal `#{client_name}` inside the fragment may be expanded by tmux before the shell ever sees it, collapsing the enumeration to the hook's own client or to an empty string.
    The documented escape is to double the `#` (`##{client_name}`), which tmux reduces to a literal `#{`.
    Whichever form the measurement proves working is the form that must be recorded in card 11's measurement record — the recorded string is the contract batch 3 reproduces, not this card's prose.
  - `func candidateTwoBody() string` — candidate 2 from the same decision: the literal tmux command `refresh-client`, with no target.
    Document that this is a tmux command and not a shell fragment, so it carries no `run-shell`, no `-b`, no `tmuxQuoteValue` wrapping, and no shell involvement at all — forcing it through candidate 1's machinery would be wrong by construction.
    It reaches only the hook's own client, which is why it is measured second.
- **Commit:** `test(reed): compose the two repaint candidate hook bodies for measurement`

### Card 9: the candidate measurement scenario

- **Context:**
  - `internal/reedcli/smoke_dotfill_test.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/windowsize.go`
- **Edits:**
  - `internal/reedcli/smoke_dotfill_measure_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func TestSmokeRepaintCandidateMeasurement(t *testing.T)` with one subtest per candidate, named `Candidate1` and `Candidate2`, driven by a shared body so both are measured identically.

  Each subtest, in order:
  1. `newDotFillHarness(t, 140, 42)`, `harnessOnlyPaneID`, `attachIn` — the same setup the resize control uses, so the two are directly comparable.
  2. `countPath := filepath.Join(t.TempDir(), "fires")`.
  3. **Last setup step, after the attach:** read reed's array with `windowResizedEntries`, take `pinOnlyEntries`, then rewrite it with `rewriteWindowResizedArray` as: every pin, then the candidate's body, then `fireCounterEntry`.
     The candidate body goes **after** the pins and **before** the counting entry, which is the position the `repaint-mechanism` decision specifies for the shipped entry relative to the pins.
  4. Read the array back and assert per entry that the candidate's body is present verbatim and that the pins are present — the mirror of the control's `assertOnlyPinEntries`.
     This also proves the body round-trips byte-identically through `show-options -v`, which `hookInstalledLocked`'s per-entry matching depends on;
     fail with a message naming both the installed and the read-back string when it does not.
  5. Fire the resize trigger exactly as `TestSmokeDotFillResizeControl` does: `resize-window` against the harness socket, shrink then grow.
  6. Record three readings rather than asserting a pass/fail verdict for the whole subtest:
     - artifact cleared: `paneStaysCleanOfDotRun` over a fixed 3 s window — a fixed window, never an early return on the first clean sample.
     - single fire: `fireCount` compared via `assertSingleHookFire`.
     - no storm: `sampleWindowSize` over 3 s passed to `assertWindowSizeSettles`.
  7. `t.Logf` all three readings in a single structured line prefixed `REPAINT-MEASUREMENT`, naming the candidate, the tmux version from `tmux -V`, each reading's outcome, and the exact body string installed together with the tmux binary path, socket name, and session name used to compose it.
     That log line is the raw material card 11 transcribes into the measurement record.

  The subtests must not `t.Skip` and must not fail merely because a candidate did not clear the artifact — a negative reading is a valid, recordable result.
  They fail only on harness faults: the setup failing, the readback not matching what was installed, or the trigger not firing at all.
- **Commit:** `test(reed): add the repaint candidate measurement scenario`

### Card 10: run the gate

- **Context:**
  - `internal/reedcli/smoke_dotfill_measure_test.go`
  - `internal/reedcli/smoke_dotfill_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Execute the gate for real and capture its output.
  Run, as one foreground process, waited to completion, never backgrounded and never in parallel with another live-substrate invocation:

  ```
  go test -tags smoke -count=1 -timeout 40m -v -run 'TestSmokeRepaintCandidateMeasurement' ./internal/reedcli/
  ```

  Also capture the measured tmux version with `tmux -V`.

  Read the `REPAINT-MEASUREMENT` log lines and decide the outcome per the `repaint-mechanism` and `repaint-must-not-self-retrigger` decisions:
  - Take candidates in order, 1 then 2.
    The first candidate that clears the artifact **and** satisfies both acceptance criteria is the accepted candidate.
  - A candidate that clears the artifact but trips either criterion is **rejected**, not shipped.
  - If neither candidate qualifies, the outcome is "no candidate accepted", which is a complete and acceptable result rather than a failure.

  This card writes no file.
  Its output is the readings card 11 transcribes.
  If the run fails as a harness fault rather than producing readings — the setup erroring, the readback mismatching, the trigger not firing — fix the harness and re-run;
  do not record a fault as a negative candidate result.
- **Commit:** none

### Card 11: record the measurement outcome in doc.go

- **Context:**
  - `internal/reedcli/smoke_dotfill_measure_test.go`
  - `internal/reedengine/windowsize.go`
  - `internal/reedengine/watchdog.go`
- **Edits:**
  - `internal/reedengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a `Measurement record (repaint candidates)` bullet to the package doc comment's `Load-bearing behavioral assumptions` list, placed beside the existing resize round-robin and hook bullets, written in the same voice and with the same "verified live on tmux 3.6" evidence discipline the surrounding bullets use.

  The record must state, in this order:
  - the tmux version the measurement ran on, taken from the captured `tmux -V`;
  - which candidates were tried, in the order they were tried;
  - for each rejected candidate, which of the three gates it failed — did not clear the artifact, repeated hook fire, or resize storm;
  - the accepted candidate, or the literal statement that no candidate was accepted.

  When a candidate was accepted, the record must additionally carry the four values batch 3's anti-drift pin needs, each labelled and reproduced verbatim from the `REPAINT-MEASUREMENT` log line:
  the exact measured hook-entry body string, the tmux binary path, the socket name, and the session name the measuring scenario used to compose it.
  For candidate 2 the body embeds none of the three, so record them as not applicable and record the body as the constant it is.

  Do not paraphrase the measured body string and do not re-derive it from card 8's prose;
  transcribe the string the scenario actually installed and read back.
  Batch 3 pins its builder against this recorded string, so a paraphrase here silently weakens that pin.

  This bullet is an unconditional deliverable: it is written in every branch, including the branch where no candidate was accepted.
- **Commit:** `docs(reed): record the repaint candidate measurement outcome`

## Batch Tests

`verify:` runs `go test -tags smoke -count=1 -timeout 40m -run 'TestSmokeDotFill|TestSmokeRepaintCandidate' ./internal/reedcli/`.
The pattern covers this batch's `TestSmokeRepaintCandidateMeasurement` plus batch 1's three `TestSmokeDotFill*` scenarios, which are re-run here as the regression guard that this batch's new file did not disturb the controls — the controls are what make every reading in this batch meaningful.
`-tags smoke` is required (both files are behind `//go:build smoke`);
the explicit `-run` pattern keeps the run off the package's `claude`-driving smoke suites;
`-count=1` defeats the test cache for a timing-dependent live-substrate test.
The 40 m timeout is wider than batch 1's because the measurement scenario runs two full candidate subtests, each with its own hub fixture, attach, trigger, and 3 s settling windows.

Card 11 edits `internal/reedengine/doc.go`, which is comment-only and carries no runnable surface of its own;
it is covered by the package still compiling under the same `go test` invocation batch 3 and batch 5 run.
