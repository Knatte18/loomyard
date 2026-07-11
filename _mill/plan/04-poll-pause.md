# Batch: poll-pause

```yaml
task: "Build builder - the batch-implementation loop"
batch: "poll-pause"
number: 4
cards: 3
verify: go test ./internal/builderengine/...
depends-on: [3]
```

## Batch Scope

The long-poll verb's engine core: pause flag mechanics (perch's discipline), the
cross-process terminal classification of an in-flight implementer (report / asking /
timeout / died), and the blocking wait loop with a clock seam. External interface
consumed later: `PauseFlagPath`/`PauseRequested`/`ClearPause`, `Classify`,
`PollUntilTerminal`.

## Cards

### Card 17: pause flag

- **Context:**
  - `internal/perchengine/state.go`
  - `internal/perchengine/doc.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/pause.go`
  - `internal/builderengine/pause_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror perchengine's pause-flag discipline against the builder
  dir: `PauseFlagPath(builderDir string) string` (a `pause` flag file inside
  builderDir), `RequestPause(builderDir string) error` (create the flag, MkdirAll
  first), `PauseRequested(builderDir string) bool` (Stat), `ClearPause(builderDir
  string) error` (remove, ignore not-exist). Godoc records the clearing rules the
  discussion pinned: cleared at `run` entry (never instantly re-pause on the flag that
  requested the pause being resumed from) and at terminal outcomes. Tests cover the
  request/observe/clear cycle and idempotent clear.
- **Commit:** `feat(builder): pause flag mechanics`

### Card 18: cross-process terminal classification

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/rundir.go`
  - `internal/shuttleengine/wait.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/mux.go`
  - `internal/muxengine/lifecycle.go`
- **Creates:**
  - `internal/builderengine/poll.go`
  - `internal/builderengine/poll_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Classify(in ClassifyInputs) (Digest, bool)` — pure decision
  function; the bool is "terminal". `ClassifyInputs` carries: `BatchNumber int`,
  `BatchSlug string`, `ReportPath string`, `Report *Report` (nil when absent),
  `TurnEnded bool`, `StrandLive bool`, `Elapsed time.Duration`,
  `BatchTimeout time.Duration`, plus the distillation inputs (`Changed []string`,
  `Scope []string`, `Dirty bool`). Decision order, exactly as the discussion pins:
  (1) report present → `done`/`stuck` digest via `Distill`; (2) no report, turn ended
  → `dead`/`dead_reason: asking`; (3) no report, elapsed > timeout → `dead`/
  `dead_reason: timeout`; (4) no report, turn in progress, strand pane gone → `dead`/
  `dead_reason: died`; else non-terminal `running` snapshot carrying only batch,
  status, `elapsed_s`. Alongside it, the impure gatherers, both riding
  provider-invariant seams (Shuttle Provider-Seam Invariant — builderengine never
  parses event grammar or pane state itself): `turnEnded(eventsPath string, engine
  shuttleengine.Engine) (bool, error)` reads the events file's bytes and delegates to
  `engine.ParseEvents(data)`, reporting whether any returned `Event` has
  `Kind == shuttleengine.EventStop` (a missing events file is `false, nil`; a
  ParseEvents error propagates); `strandLive(mux shuttleengine.MuxOps, guid string)
  (bool, error)` calls `mux.Status()` and scans the returned `Strands` for the guid's
  `Live` field (guid absent → `false, nil` — mux no longer tracks it). Liveness is
  NEVER read from persisted mux state (`muxengine.LoadState` carries no liveness;
  only a live `Status()` query does).
  Digest computation (diff, drift) runs ONLY on terminal classification — a `running`
  snapshot never touches git (discussion: drift on a half-done batch is noise).
  Concretely: `ClassifyInputs.Changed`/`Dirty` are filled ONLY when `Report != nil`
  — every gather implementation checks for the report FIRST and runs the gitquery
  helpers exclusively inside that report-present branch; a tick without a report
  passes zero values (they are unread on the non-report paths). A literal
  every-tick diff (hundreds of `git diff` runs per poll at the 1s tick) is a defect.
  Tests:
  a decision table over Classify covering all five outcomes; `turnEnded` against a
  fake `shuttleengine.Engine` whose `ParseEvents` returns a scripted Event sequence;
  `strandLive` against a fake `MuxOps`.
- **Commit:** `feat(builder): cross-process poll classification`

### Card 19: long-poll wait loop

- **Context:**
  - `internal/shuttleengine/wait.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/poll.go`
  - `internal/builderengine/poll_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `PollUntilTerminal(gather func() (Digest, bool, error), wait
  time.Duration, clk clock) (Digest, error)`: re-run gather on a fixed tick (1s)
  until it reports terminal or `wait` elapses; on deadline return the last
  non-terminal digest (the `running` snapshot the orchestrator re-polls on). Define a
  package-local `clock` seam (`Now()`, `Sleep(d)`) with a `realClock`, mirroring
  `shuttleengine`'s wait.go seam, so tests replay a whole poll sequence instantly.
  The long-poll IS the notification: the loop blocks inside Go, costing the
  orchestrator nothing (discussion Q3). Tests with a fake clock: terminal mid-wait
  returns early with the terminal digest; deadline returns running; gather error
  propagates.
- **Commit:** `feat(builder): long-poll wait loop with clock seam`

## Batch Tests

`verify:` runs the builderengine suite; this batch adds the Classify decision table,
the events.jsonl Stop detection fixture tests, and the fake-clock long-poll tests.
