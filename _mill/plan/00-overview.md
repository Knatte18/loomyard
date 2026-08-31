# Plan: Reed attach dot-fill render artifact on resize and cross-client mouse move

```yaml
task: "Reed attach dot-fill render artifact on resize and cross-client mouse move"
slug: "reed-attach-dotfill-artifact"
approved: false
started: "20260831-115653"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: dotfill-repro-harness
    file: 01-dotfill-repro-harness.md
    depends-on: []
    verify: go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/
  - number: 2
    name: measurement-gate
    file: 02-measurement-gate.md
    depends-on: [1]
    verify: go test -tags smoke -count=1 -timeout 40m -run 'TestSmokeDotFill|TestSmokeRepaintCandidate' ./internal/reedcli/
  - number: 3
    name: repaint-entry
    file: 03-repaint-entry.md
    depends-on: [2]
    verify: go test -count=1 ./internal/shell/ ./internal/reedengine/ && go test -tags smoke -count=1 -timeout 25m -run 'TestSmokeDotFill' ./internal/reedcli/
  - number: 4
    name: attach-multi-client-warning
    file: 04-attach-multi-client-warning.md
    depends-on: []
    verify: go test -count=1 ./internal/reedengine/
  - number: 5
    name: docs
    file: 05-docs.md
    depends-on: [2, 3, 4]
    verify: go test -count=1 ./internal/reedengine/ ./cmd/lyx/
```

## Shared Decisions

### Decision: root-cause-model-is-the-design-premise

- **Decision:** every batch designs against the model recorded in `_mill/discussion.md`'s `root-cause-model` decision — the dots are painted by **tmux itself**, in the region of an attached client's terminal that is either not covered by the current window geometry (the **uncovered subset**) or whose paint has gone stale relative to a just-changed window size (the **stale-paint subset**).
  reed never writes a dot;
  no code path under `internal/reedengine` or `internal/reedengine/render` produces one.
  The resize trigger is the stale-paint subset (repairable by a forced repaint);
  the cross-client trigger, as reported, is the uncovered subset (not repairable, documented instead).
- **Rationale:** the split is what makes the smoke suite's asymmetric shape correct — a control-and-treatment pair for resize, a control alone for cross-client — and what keeps a reviewer from asking for a cross-client treatment scenario that cannot exist.
- **Applies to:** all batches

### Decision: measurement-gate-authorises-the-production-code

- **Decision:** batch 3 writes no production repaint code until batch 2 has recorded a measurement outcome.
  Batch 2 measures each candidate by installing its body into the `window-resized` array **directly from a smoke scenario**, as a literal string, using the same `tmux set-hook` rewrite technique the batch-1 control already performs.
  A candidate is accepted only when it clears the artifact **and** satisfies both acceptance criteria from the `repaint-must-not-self-retrigger` decision (no repeated hook fire, no resize storm).
  Batch 3 is therefore explicitly three-way conditional on the recorded outcome — candidate 1 accepted, candidate 2 accepted, or no candidate accepted — and every branch is fully specified in that batch's cards.
- **Rationale:** candidate 1 needs a new `internal/shell` primitive and a new body builder, so "build it then decide" would mean writing exactly the production code the gate exists to authorise.
- **Applies to:** 02-measurement-gate, 03-repaint-entry

### Decision: the-measurement-record-lives-in-doc-go

- **Decision:** batch 2 records the gate's outcome as a `Measurement record (repaint candidates)` block in `internal/reedengine/doc.go`'s package doc comment, beside the resize round-robin and hook bullets in its `Load-bearing behavioral assumptions` list.
  The block records, verbatim and in this order: the tmux version measured on, which candidates were tried, which criterion each rejected candidate failed, the accepted candidate (or "none"), and — for the accepted candidate — the four values the batch-3 anti-drift pin needs: the exact measured hook-entry body string, and the tmux binary path, socket name, and session name the measuring scenario used to compose it.
- **Rationale:** batch 3's builder pin is stated as a *reproduction* property — "the builder, invoked with the same tmux path, socket, and session name the measuring scenario used, reproduces the measured string byte-identically" — which is unwritable unless those three inputs are recorded alongside the string.
  `doc.go` is reed's design record (the module has no `manifest/designs/reed.md`), so the record belongs there rather than in a new file.
- **Applies to:** 02-measurement-gate, 03-repaint-entry, 05-docs

### Decision: watchdog-off-in-every-smoke-scenario

- **Decision:** every smoke scenario added by this task boots its reed session with `LYX_REED_WATCHDOG=off` set via `t.Setenv` before the first in-process `RunCLI` call, so the reed config's `watchdog:` key resolves to `off` on every load.
- **Rationale:** with the watchdog on, the artifact self-heals in about a second via the watch loop's re-apply, so a treatment's "absent for the whole deadline" would be partly satisfied by the heal rather than by the repaint entry under test.
  The repaint entry is watchdog-independent, so turning the watchdog off costs the experiment nothing.
  A second, mechanical consequence the scenarios depend on: with `watchdog: off`, `resizeSignalHookCommand` answers `""`, so reed's `window-resized` array contains the resize-pane pins and (after batch 3) the repaint entry, and no signal entry — which is the array shape every rewrite and readback assertion in this plan is written against.
- **Applies to:** 01-dotfill-repro-harness, 02-measurement-gate, 03-repaint-entry

### Decision: smoke-runs-are-serial-and-name-exact-tests

- **Decision:** every `verify:` command in this plan that carries `-tags smoke` names its tests with an explicit `-run` pattern and runs as one foreground process.
  Never more than one live-substrate invocation at a time, never backgrounded, never a bare `Smoke` pattern.
- **Rationale:** this repo's crucible convention (`crucible/review-prompt-template.md`) already pins it: concurrent smoke runs contend for tmux sockets and hub fixtures, and a bare pattern sweeps unrelated suites that cost minutes each.
- **Applies to:** 01-dotfill-repro-harness, 02-measurement-gate, 03-repaint-entry

### Decision: geometry-tmux-failures-stay-non-fatal

- **Decision:** every tmux call this task adds — the attach-time `list-clients` query and the repaint hook entry — is non-fatal.
  A failure is answered with a `logger.Warn` and a safe degrade, never an error return, and neither `list-clients` nor `refresh-client` joins `requiredSubcommands` in `internal/reedengine/probe.go`.
- **Rationale:** this is the Shared Decision `geometry-tmux-failures-are-non-fatal-everywhere` already governing `windowsize.go` and `attach.go`, plus the `probe-verbs-not-extended` decision: `requiredSubcommands` is the set the engine cannot work without, and adding a cosmetic-feature verb would fail boot for a multiplexer that runs reed perfectly well today.
- **Applies to:** 03-repaint-entry, 04-attach-multi-client-warning

### Decision: windows-exclusion-is-never-inherited

- **Decision:** the repaint entry carries its own `runtime.GOOS == "windows"` check returning `""`, modelled on `resizeSignalHookCommand`.
  It inherits nothing from the existing Windows exclusion.
- **Rationale:** `installResizePinsLocked` has **no** `runtime.GOOS` gate — on Windows it issues the clear and every `resize-pane` pin argv exactly as elsewhere.
  The single mechanism keeping the signal entry off Windows is `resizeSignalHookCommand` returning `""` combined with `resizePinHookArgvs` emitting no entry for an empty body.
  `pinGeometryOptionsLocked`'s early Windows return covers only the *unset* half of the lifecycle.
- **Applies to:** 03-repaint-entry

### Decision: shell-mechanics-seam-binds-the-hook-body

- **Decision:** if candidate 1 is accepted, its `run-shell` body's shell fragment is composed **only** from `internal/shell` members — never by concatenating shell syntax inside `internal/reedengine`.
  The two new members (`ForEachLine`, `LineVarRef`) are added to the `Shell` interface and implemented in both the POSIX and pwsh dialects, even though only POSIX ever executes here.
- **Rationale:** CONSTRAINTS.md's **Shell Mechanics Seam** — pane-shell command strings are built ONLY via `internal/shell`.
  `LineVarRef` exists specifically so a caller never spells a dialect's loop-variable syntax itself, which is what keeps one composed body string correct in both dialects.
- **Applies to:** 03-repaint-entry

### Decision: install-site-stays-singular

- **Decision:** the repaint entry is installed by `installResizePinsLocked` as part of its existing whole-snapshot rebuild, ordered **after** every resize-pane pin and **before** the watchdog's signal entry.
  No second hook install site is created, and no new hook name is introduced.
- **Rationale:** the `window-resized` array is a whole-snapshot rebuild whose first entry uses the plain (replacing) `set-hook` form and every entry after it uses `-a`.
  A second writer would either clobber the pins this one just installed or accumulate a duplicate entry per attach.
  Ordering the repaint after the pins makes it paint the geometry the pins have already fixed up;
  ordering it before the signal entry preserves the signal entry's documented last position.
- **Applies to:** 03-repaint-entry

### Decision: plan-scope-excludes-window-size-policy-and-roadmap

- **Decision:** no batch changes the `window-size latest` pin, the `mouse` pin, the watchdog timings (`watchdogDebounceQuiet`, `watchdogSignalTick`, `watchdogPollCycle`), anything under `internal/reedengine/render`, or `manifest/roadmap.md`.
- **Rationale:** the `window-size latest` pin is load-bearing for the told-geometry attach chain (`AttachArgv` suppresses its chained `select-layout` on any other value) and every alternative policy turns a transient artifact into a standing one.
  The layout computation is correct — only its painting is at issue.
  Roadmap movement is reserved for completing or adding a planned item, and this is a bugfix/hardening pass.
- **Applies to:** all batches

## All Files Touched

- `internal/reedcli/smoke_dotfill_measure_test.go`
- `internal/reedcli/smoke_dotfill_test.go`
- `internal/reedengine/apply_test.go`
- `internal/reedengine/attach.go`
- `internal/reedengine/attach_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/reapply_test.go`
- `internal/reedengine/watchdog.go`
- `internal/reedengine/watchdog_test.go`
- `internal/reedengine/windowsize.go`
- `internal/reedengine/windowsize_test.go`
- `internal/shell/posix.go`
- `internal/shell/pwsh.go`
- `internal/shell/shell.go`
- `internal/shell/shell_test.go`
- `tools/sandbox/SANDBOX-REED-WATCH-SUITE.md`
