# Plan: Audit internal/logger coverage across spawn/hard-error paths

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
slug: "logger-coverage-audit"
approved: false
started: "20260828-061252"
parent: "main"
root: ""
verify: go build ./... && GOOS=windows go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: audit-doc-and-constraints
    file: 01-audit-doc-and-constraints.md
    depends-on: []
    verify: go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
  - number: 2
    name: hard-error-warn-lines
    file: 02-hard-error-warn-lines.md
    depends-on: [1]
    verify: go test ./internal/shedadapters/ ./internal/mergeresolve/ ./internal/websterengine/
  - number: 3
    name: spawn-site-log-lines
    file: 03-spawn-site-log-lines.md
    depends-on: [1]
    verify: go test ./internal/websterengine/ ./internal/treadleengine/ ./internal/configengine/ ./internal/boardengine/ ./internal/vscode/ ./internal/reedengine/ && go test -tags integration -run TestRunVerifyCommand ./internal/websterengine/
  - number: 4
    name: github-caller-warn-lines
    file: 04-github-caller-warn-lines.md
    depends-on: [1]
    verify: go test ./internal/selfreportengine/ ./internal/landingshed/
  - number: 5
    name: spawn-observability-guard
    file: 05-spawn-observability-guard.md
    depends-on: [1, 3]
    verify: go test ./cmd/lyx/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: log-line-style

- **Decision:** Every new log line's message string is prefixed with the emitting package's name (`"shedadapters: …"`, `"websterengine: …"`, `"treadleengine: …"`, `"boardengine: …"`, `"vscode: …"`, `"configengine: …"`, `"reedengine: …"`, `"mergeresolve: …"`, `"selfreportengine: …"`, `"landingshed: …"`), and every field key is lowerCamelCase.
  The established key for a wrapped error is `cause`, not `err` or `error`.
- **Rationale:** `_mill/discussion.md`'s "Field-naming convention observed across existing call sites" section — the trace file is grepped by package prefix and by field key, which is its whole purpose. `burler.go:307` uses `error` and is the minority spelling; `cause` is what new lines use.
- **Applies to:** all batches

### Decision: additive-only

- **Decision:** No existing log line's level, message, or field set changes anywhere in this task.
  Every code change is the insertion of a new `logger.Debug`/`logger.Info`/`logger.Warn` call plus, where the file does not already have it, the `github.com/Knatte18/loomyard/internal/logger` import.
  No control flow, no error text, no return value changes.
- **Rationale:** `_mill/discussion.md`'s Scope Out — "Changing any existing log line's level, message, or field set. Additive only." A behaviour change smuggled into an observability task is unreviewable against this plan.
- **Applies to:** all batches

### Decision: level-policy

- **Decision:** `Warn` for a notable-but-recoverable failure (a non-`Done` shuttle outcome, a spawn that failed to start, a GitHub call that failed). `Info` for a real OS-process spawn or teardown lifecycle event. `Debug` for a spawn inside a polling probe.
  Nothing logs at `Warn` inside a loop body that can iterate more than roughly ten times without an intervening state change.
- **Rationale:** the `internal/logger` package doc's own level policy, quoted in `_mill/discussion.md`'s "Technical context". `Debug` never reaches the durable trace file, so a `Debug` verdict is a deliberate choice not to be in the bug-report trail.
- **Applies to:** all batches

### Decision: test-log-capture-pattern

- **Decision:** A test asserting on an emitted log line uses the in-repo inline pattern, not a new shared helper file: declare a local `var buf bytes.Buffer`, call `logger.SetOutput(&buf)`, and register `t.Cleanup(func() { logger.SetOutput(os.Stderr) })`.
  `internal/loomshed/gatefindings_test.go` (lines 26-30) is the model; `internal/treadleengine/engine_test.go` and `internal/fabricengine/add_rollback_adopt_test.go` use the identical shape.
  `internal/logger`'s own `withCapturedOutput` helper is unreachable from any other package — it lives in `package logger` — so no package tries to reuse it.
- **Rationale:** `_mill/discussion.md`'s Testing section requires each package to have its own capture seam; the repo already has one established spelling for it, and a new shared helper file would be a fourth variant of a three-line idiom.
- **Applies to:** hard-error-warn-lines, spawn-site-log-lines, github-caller-warn-lines

### Decision: info-assertions-need-verbosity

- **Decision:** A test asserting an `Info` line must call `logger.SetVerbosity(1)` and register `t.Cleanup(func() { logger.SetVerbosity(0) })`; a test asserting only `Warn` lines needs neither, because `Warn` is the default threshold.
- **Rationale:** `internal/logger/logger.go`'s `SetVerbosity` maps count<=0 to `slog.LevelWarn`, so `Info` records are dropped by the stderr half at the default level and never reach the captured buffer.
- **Applies to:** spawn-site-log-lines

### Decision: assert-keys-not-rendered-strings

- **Decision:** Log assertions check for the presence of the field keys and the level token in the captured output, never for an exact rendered line.
- **Rationale:** `_mill/discussion.md`'s Testing section states this directly. The stderr half is a `slog.NewTextHandler`, whose exact rendering (attribute ordering, quoting, the `time=` prefix) is not this task's contract.
- **Applies to:** hard-error-warn-lines, spawn-site-log-lines, github-caller-warn-lines

### Decision: audit-doc-uses-backticked-paths

- **Decision:** `manifest/designs/logger-coverage.md` refers to every source file by a backticked path, never by an inline markdown link.
- **Rationale:** the Markdown Link Integrity invariant requires every inline link under `manifest/` to resolve, file part and anchor. The audit document is written in batch 1 and names files that batches 3 and 5 have not created yet (`cmd/lyx/spawnobservability_test.go`), so an inline link would break `TestEnforcement_MarkdownLinks` at batch 1's own verify. A backticked path is not a link and is not scanned.
- **Applies to:** audit-doc-and-constraints

### Decision: constraints-prose-only

- **Decision:** The CONSTRAINTS.md edit replaces the body of the existing `## Live-Substrate Spawn Observability` section with the verbatim text recorded in `_mill/discussion.md`'s `constraints-md-prose-only` decision, and changes nothing else in the file.
  It names no test file, describes no allowlist, and adds no link to `manifest/designs/logger-coverage.md`.
  The heading itself is unchanged.
- **Rationale:** CONSTRAINTS.md's own line 4 declares it is "not a test-coverage index", and commit `d66cefe5` stripped exactly these references out of the file. Naming the guard there would reintroduce the pattern that commit removed.
- **Applies to:** audit-doc-and-constraints

### Decision: publish-warn-overlaps-reportstuck

- **Decision:** The `logger.Warn` lines added to `internal/landingshed/publish.go` are added even though the `stuckOrCancelled` path they precede already reaches `reportStuck`, which emits `logger.Warn("landingshed: producer stuck", …)` in `stuck.go`.
  The new lines are not folded into `reportStuck` and do not replace it.
- **Rationale:** `reportStuck`'s line carries the classified reason as one prose string and no operation-level structure. The added lines carry `action`, `owner`, `repo`, and `cause` as separate fields, which is what makes a GitHub failure greppable in the trace file — the exact compensation the `githubclient-leaf` decision buys by declining to log inside `internal/githubclient`. The duplication is deliberate and is recorded here so a reviewer does not read it as an oversight.
- **Applies to:** github-caller-warn-lines

### Decision: selector-reruns-are-the-authority

- **Decision:** The site tables in `_mill/discussion.md` are a snapshot; the AST selectors described in its `error-universe` decision were re-run against this worktree's HEAD at plan time and their output is what the batches below encode.
  Two line numbers drifted from the discussion's tables (`treadleengine/judge.go` 131/193 → 130/192, `treadleengine/targeting.go` 62 → 61) and one site's file-level state differed from the discussion's package-level claim (`internal/reedengine/proctree_windows.go` does not import `logger` in that file, only elsewhere in the package). The site *set* is otherwise identical.
- **Rationale:** `_mill/discussion.md`'s `error-universe` decision instructs the plan to re-run both selectors rather than trust the transcribed rows. Cards below therefore name functions and branches, not line numbers, so a further drift before implementation cannot mislead the implementer.
- **Applies to:** all batches

### Decision: covered-is-a-per-call-claim

- **Decision:** A spawn site's `covered` verdict means the spawn call itself is logged, not merely that the enclosing file imports `internal/logger` somewhere.
  Read against that stricter measure, three sites `_mill/discussion.md`'s `spawn-site-verdicts` table records as `covered` are not: `internal/fabricengine/spawn.go` (a `Warn` on `Start` failure, no spawn announcement), `internal/reedcli/attach.go` (its only `logger` line is an unrelated terminal-size warning), and one of `internal/loomcli/run.go`'s two sites (the `loom drive` spawn is logged, the tmux-attach spawn is not).
  All three are re-verdicted `add` and implemented by card 12; the audit document records both the correction and the measure.
- **Rationale:** card 2 lands a sharpened invariant reading "Every code path reachable from a `lyx` command that starts a real OS process logs its spawn via `internal/logger`" — a per-call rule. Left as `covered`, these three sites would violate the invariant this task itself sharpens, from the moment it landed, and batch 5's guard could never catch them because it checks file-level import presence only. `_mill/discussion.md`'s `error-universe` decision instructs the plan to re-run the selectors and regenerate the tables rather than trust the transcribed rows; this is that regeneration finding a third defect, alongside the two line-number drifts already recorded in `selector-reruns-are-the-authority`.
- **Applies to:** audit-doc-and-constraints, spawn-site-log-lines

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `cmd/lyx/spawnobservability_test.go`
- `cmd/lyx/tierpurity_test.go`
- `internal/boardengine/spawn.go`
- `internal/configengine/edit.go`
- `internal/fabricengine/spawn.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/publish_test.go`
- `internal/mergeresolve/mergeresolve.go`
- `internal/mergeresolve/mergeresolve_test.go`
- `internal/loomcli/run.go`
- `internal/mergeresolve/seam_enforcement_test.go`
- `internal/reedcli/attach.go`
- `internal/reedengine/proctree_windows.go`
- `internal/selfreportengine/selfreport.go`
- `internal/selfreportengine/selfreport_test.go`
- `internal/shedadapters/singlellm.go`
- `internal/shedadapters/singlellm_test.go`
- `internal/treadleengine/gate.go`
- `internal/vscode/launch_linux.go`
- `internal/vscode/launch_windows.go`
- `internal/websterengine/integration.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runverify_test.go`
- `manifest/designs/logger-coverage.md`
