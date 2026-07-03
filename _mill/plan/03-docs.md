# Batch: docs

```yaml
task: Dedicated sandbox suite for mux
batch: docs
number: 3
cards: 2
verify: go test ./...
depends-on: [1, 2]
```

## Batch Scope

This batch updates every prose touchpoint that describes the sandbox layer so the docs
match the shipped multi-suite reality: the operator runbook and hub design doc gain the
mux suite and its launcher, the mux module doc gains a pointer to its manual-test
surface, and the overview's Sandbox Hub paragraph names the new launcher. It depends on
batches 1 and 2 because it documents both. No batch-local decisions beyond Shared
Decisions.

## Cards

### Card 8: Update the sandbox runbook and hub doc

- **Context:**
  - `sandbox-suite.cmd`
  - `sandbox-fetch.cmd`
- **Edits:**
  - `docs/sandbox-howto.md`
  - `docs/sandbox-hub.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/sandbox-howto.md`:
  - Add `mux-sandbox-suite.cmd` to the launcher list in the intro paragraph (the
    "each sandbox launcher does exactly one thing" sentence gains the fourth
    launcher: build / suite / mux-suite / fetch).
  - Add a step-level subsection after "### 4. Run the suite" (e.g. "### 4b. Run the
    mux suite (optional, needs live psmux)") stating: `mux-sandbox-suite.cmd` copies a
    fingerprinted `MUX-SANDBOX-SUITE.md` into the Hub host repo and launches the
    interactive agent there; it needs a live psmux (`psmux.exe` on PATH) and
    PowerShell 7; the attach scenario (M7) pauses for the operator to run
    `lyx mux attach` in a second terminal and confirm visually; findings go to the
    same `sandbox-report.json`, so step 5 (`sandbox-fetch.cmd`) and step 6 (triage)
    apply unchanged — fetch between sessions, do not run both suites and fetch once.
    Same `-claude`/`-prompt` overrides as `sandbox-suite.cmd`.
  - Add `tools/sandbox/MUX-SANDBOX-SUITE.md` to the "See also" list.

  In `docs/sandbox-hub.md`:
  - Extend the "Running the suite" area with a short mux-suite paragraph mirroring the
    main-suite description (copies `MUX-SANDBOX-SUITE.md`, git-excludes it, clears the
    stale report, launches the agent; live-psmux precondition; findings to the shared
    `sandbox-report.json`).
  - Update the subcommand/launcher mapping listing (the block showing
    `sandbox-build.cmd` / `sandbox-suite.cmd` / `sandbox-fetch.cmd` with their
    `go run ./tools/sandbox ...` expansions) with the
    `mux-sandbox-suite.cmd  # go run ./tools/sandbox -parent C:\Code mux-suite` row,
    and reword the "three subcommands" sentence to four.
- **Commit:** `docs(sandbox): document the mux suite runbook and launcher`

### Card 9: Point the mux module doc and overview at the suite

- **Context:**
  - `docs/sandbox-howto.md`
- **Edits:**
  - `docs/modules/mux.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/modules/mux.md`:
  - Add a short subsection after `## What actually works (empirical guardrails)`
    titled `## Manual test surface: the mux sandbox suite`, stating: mux's
    live/visual testing runs through the dedicated black-box suite
    `tools/sandbox/MUX-SANDBOX-SUITE.md`, launched via `mux-sandbox-suite.cmd`
    against the sandbox Hub host repo (see `docs/sandbox-howto.md`); the hermetic
    unit/golden tests and the opt-in `-tags smoke` test remain the automated layer;
    the suite's attach scenario is operator-assisted because attach is an interactive
    terminal takeover.

  In `docs/overview.md`:
  - In the `## Sandbox Hub` paragraph, extend the parenthetical naming the launchers
    so it also names `mux-sandbox-suite.cmd` as running the mux-specific suite
    (`MUX-SANDBOX-SUITE.md`, needs live psmux) with the report collected by the same
    `sandbox-fetch.cmd`.
  - Leave the `## Other docs` list unchanged (it links `sandbox-howto.md` /
    `sandbox-hub.md`, not individual suite files).
- **Commit:** `docs(mux): point module doc and overview at the mux sandbox suite`

## Batch Tests

`verify: go test ./...` — a pure docs batch has no runnable surface of its own, so the
verify slot is used for the plan's terminal full-suite gate: the complete hermetic
test tree (including the generalized coverage guard and the `tools/sandbox` launcher
tests from batches 1–2) must be green before the task can hand off. The full run is
cheap because the suite is hermetic (`-tags smoke` excluded by default), which is the
stated justification for the unbounded scope.
