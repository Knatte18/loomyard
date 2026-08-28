# Plan: reed: header pane's boot sometimes leaves shell/log noise in its scrollback

```yaml
task: "reed: header pane's boot sometimes leaves shell/log noise in its scrollback"
slug: "reed-header-pane-boot-noise"
approved: true
started: "20260828-083416"
parent: "main"
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: header-pane-runs-its-own-command
    file: 01-header-pane-runs-its-own-command.md
    depends-on: []
    verify: go test ./internal/reedengine/
  - number: 2
    name: header-declines-the-stencil-seed-pass
    file: 02-header-declines-the-stencil-seed-pass.md
    depends-on: []
    verify: go test ./cmd/lyx/ ./internal/clihelp/ ./internal/reedcli/ && go test -tags smoke -run TestSmokeHeaderDeclinesStencilSeedPass ./internal/reedcli/
  - number: 3
    name: scrollback-backstop-and-composite-smoke
    file: 03-scrollback-backstop-and-composite-smoke.md
    depends-on: [1, 2]
    verify: go test ./internal/reedcli/ && go test -tags smoke -run 'TestSmokeHeaderPayloadClearsPaneScrollback|TestSmokeHeaderPaneScrollbackIsClean' ./internal/reedcli/
```

## Shared Decisions

### Decision: three noise classes, three independent pins, one backstop

- **Decision:** each source fix carries a regression pin that observes that fix's *own* mechanism, never the composite scrollback symptom.
  P1 (batch 1) pins "the pane runs the command, not a shell" on the recorded fake-tmux argv plus a zero-`send-keys` assertion.
  P2 (batch 2) pins "the header declines the stencil-seed pass" on the real binary's stderr, with no tmux, pane, or escape sequence anywhere in the picture.
  P3 (batch 3) pins the clear sequence on a pure payload helper's return value.
  B (batch 3) is the composite `capture-pane -p -S -` assertion and pins **no individual fix**.
- **Rationale:** the `ED 3` backstop runs after everything else, so a single end-to-end scrollback assertion stays green even when a source fix regresses.
  Splitting verification by mechanism restores the property that each test goes red when its own change is reverted.
- **Applies to:** all batches

### Decision: pre-fix red observations are recorded in commit message bodies

- **Decision:** the two required pre-fix failure observations are captured by running the pin before its fix card lands, and a condensed excerpt of the failure output is pasted into the **body** of the fix card's own commit message.
  P2 is observed red against unmodified `main` (batch 2, before card 9's gate exists) and green after card 10.
  P1 is observed red against the **seam-landed, launch-change-not-yet-applied** intermediate state (batch 1, after card 1 and card 2, before card 3) and green after card 3 — never claimed as red on unmodified `main`, which is impossible for it because the suppression override does not exist there.
- **Rationale:** git history is the only durable, reviewable place for this evidence in a task that leaves no other artifact behind;
  `.scratch/` is gitignored and `_mill/` is task state, not evidence.
  A green run at the end of the task is explicitly not this evidence.
- **Applies to:** batches 1 and 2

### Decision: `ED 3`'s efficacy is proved directly, not inferred from a landing order

- **Decision:** the discussion's `ordering-lands-source-fixes-before-the-backstop` landing order is kept exactly as decided (source fixes and their pins in batches 1-2, `ED 3` and B in batch 3), but the "run B before the source fixes to prove `ED 3` took effect" observation it sketches is replaced by a direct, always-available proof: `TestSmokeHeaderPayloadClearsPaneScrollback` (card 14) writes real junk lines into a real tmux pane, then emits `headerBlockingPayload`'s exact bytes into that same pane, then asserts the full scrollback holds the header line and nothing else.
- **Rationale:** the discussion offers that observation as "available for free given ordering already records the pre-fix pane content at the start of the task", but nothing in the discussion or this repo provides a mechanism for that live pre-fix capture that does not require booting a real reed session against the operator's own hub — a shared-state side effect with an unreliable result (an already-alive header makes `reed up` idempotent and the capture stale).
  The pre-fix pane content is in any case already captured verbatim, twice, in the discussion's own Problem section;
  it does not need re-capturing.
  A direct pane-level proof of `ED 3` is strictly stronger than the ordering trick: it is repeatable, it is not masked by anything, and it keeps working forever rather than being available only during one window of the task.
  The Windows/psmux side stays unverified for the same reason every Windows claim in this task does.
- **Applies to:** batch 3

### Decision: no new tmux verb, no new shell primitive, no exported API

- **Decision:** the launch change adds a trailing shell-command argument to the existing `split-window`, and nothing else:
  no `clear-history`, no `-e`, no `exec` prefix, no new `internal/shell` method, no change to `headerLaunchCmd`/`headerLaunchLine`, and no exported symbol added to `internal/reedengine`.
  The suppression seam is an unexported `Engine` field set by an in-package test helper.
- **Rationale:** `internal/reedengine/probe.go` and `internal/reedengine/doc.go` both enumerate reed's tmux verb surface, and every addition to it is a psmux-compatibility risk this task has no way to verify.
  The `internal/shell` Shell Mechanics Seam likewise stays as-is.
- **Applies to:** batches 1 and 3

### Decision: only `reed header` opts out of the seed pass

- **Decision:** the cobra annotation is carried by `reed header` alone.
  Strand panes host lyx processes that read stencils through the same root pre-run and are deliberately left un-annotated.
  Both header modes (plain and `--blocking`) decline the pass, because a cobra annotation is per-command rather than per-flag and neither mode reads a stencil.
- **Rationale:** the header is the one pane whose entire purpose is to display a fixed line;
  a strand pane runs real work, so the pass is doing its job there and its warnings are information the operator wants.
- **Applies to:** batch 2

### Decision: Go conventions, no `PYTHONPATH=` prefix on `verify:`

- **Decision:** this is a Go repo, so every `verify:` command is the native `go test` invocation with no `PYTHONPATH=` prefix.
  Smoke-tagged assertions are run with an explicit `-run` filter rather than the whole `-tags smoke` package, which also drives real `claude` sessions and real transcript persistence.
- **Rationale:** the unfiltered reedcli smoke suite is far too slow to run after every implementer and fixer round;
  the `-run` filter keeps each batch's own tagged assertions in its verify loop without dragging the rest in.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/main.go`
- `cmd/lyx/stencilseed.go`
- `cmd/lyx/stencilseedgate_test.go`
- `internal/clihelp/annotations.go`
- `internal/clihelp/annotations_test.go`
- `internal/reedcli/header.go`
- `internal/reedcli/header_test.go`
- `internal/reedcli/smoke_headerscrollback_test.go`
- `internal/reedcli/smoke_headerseed_test.go`
- `internal/reedcli/smoke_test.go`
- `internal/reedengine/doc.go`
- `internal/reedengine/lifecycle.go`
- `internal/reedengine/lifecycle_test.go`
- `internal/reedengine/lock.go`
