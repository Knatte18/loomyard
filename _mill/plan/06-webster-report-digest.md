# Batch: webster-report-digest

```yaml
task: 'webster: rewrite for flat card list'
batch: webster-report-digest
number: 6
cards: 4
verify: go test -tags integration ./internal/websterengine/...
depends-on: [5]
```

## Batch Scope

Second half of the builder decouple: webster's own fork-return report contract (replacing
`builderengine.ParseReport`/`Report`), its own state Digest (replacing `builderengine.Digest`),
and the recovery classification + long-poll helpers (`Classify`/`ClassifyInputs`/
`PollUntilTerminal`) that consume them. These are rewrite-anyway contracts — webster defines its
own minimal shapes, not copies of builder's YAML grammar. Like batch 5 this only ADDS files;
call-site retargets are batch 7. The report/digest types are the external surface batches
7, 8, 9 consume.

## Cards

### Card 21: fork-return report contract (parser + writer)

- **Context:**
  - `internal/builderengine/report.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/planparser/plan.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/report.go`
  - `internal/websterengine/report_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `report.go` define webster's minimal fork-return contract, replacing `builderengine.ParseReport`/`Report` and its `done/stuck/green/red/skipped`/`out_of_scope` grammar entirely. Type `Report struct { Status string; HeadSHA string; Deviations []string }` with yaml tags. Status vocabulary consts `ReportStatusOK = "OK"`, `ReportStatusFailed = "FAILED"`. `ParseReport(path string) (*Report, error)`: strict `yaml.Decoder.KnownFields(true)` decode, validate `Status ∈ {OK, FAILED}`, `HeadSHA` non-empty, `Deviations` a (possibly empty) list of worktree-relative paths; wrapped `websterengine:`-prefixed errors on violation, including malformed/empty input. `WriteReport(path string, r *Report) error`: serialize the three fields to the per-batch report file the fork writes under `hubgeometry.WebsterReportsDir(...)` (caller supplies the path; do not build the `_lyx` tokens here). Add `ReportFileName(number int, slug string) string` returning `fmt.Sprintf("%02d-%s.yaml", number, slug)` (webster's own naming, replacing `builderengine.BatchReportFileName`). Per the deviation-is-informational Shared Decision, the parser NEVER treats a non-empty `Deviations` as an error — only `Status`/`HeadSHA` shape violations fail the parse. In `report_test.go` add Tier-1 tests (no git; `t.TempDir()`) for OK/FAILED round-trips through WriteReport→ParseReport, empty and populated deviation lists, and strict-decode rejection of unknown keys / missing head SHA / bad status.
- **Commit:** `feat(websterengine): fork-return report contract (OK/FAILED, head SHA, deviations)`

### Card 22: webster state Digest + status vocabulary

- **Context:**
  - `internal/builderengine/digest.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/gitwrap.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/digest.go`
  - `internal/websterengine/digest_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `digest.go` define webster's own EXPORTED state digest (`internal/webstercli` consumes it via a `digestFields` adapter — batch 9): `type Digest struct { Batch string; Status string; HeadSHA string; Deviations []string; DeadReason string; ElapsedS int }` with json tags — a batch-outcome snapshot for `state.json` and the resume trail, carrying the fork-return facts (status, head SHA, informational deviation list) plus recovery metadata. Status consts `DigestStatusRunning/Done/Stuck/Dead` and dead-reason consts `DeadReasonAsking/Timeout/Died` (webster-local; do not import builder's). Add `distill(r *Report, changed []string) Digest` computing a `Done`/`Stuck` digest from a fork Report: map `Report.Status` OK→`DigestStatusDone` / FAILED→`DigestStatusStuck`, carry `HeadSHA`, and record the informational deviation list (fork-reported `r.Deviations`; the `changed` argument from `changedFiles` is an optional Master cross-check per the deviation-is-informational Shared Decision — record but never fail on it). DROP builder's `out_of_scope`/`DriftUnreported`/scope-drift model entirely (no `## Scope` in the flat format). In `digest_test.go` add Tier-1 tests (no git) asserting OK→Done and FAILED→Stuck mapping, head-SHA/deviation carry-through, and that a large deviation list never changes Done/Stuck status.
- **Commit:** `feat(websterengine): webster-local state Digest and distill`

### Card 23: recovery classification (Classify / ClassifyInputs)

- **Context:**
  - `internal/builderengine/poll.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/strand.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/classify.go`
  - `internal/websterengine/classify_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `classify.go` add webster-local copies of the recovery-classification decision logic (`builderengine.Classify`/`ClassifyInputs`) retargeted to webster's `Report`/`Digest`: `type ClassifyInputs struct { BatchNumber int; BatchSlug string; ReportPath string; Report *Report; TurnEnded bool; StrandLive bool; Elapsed time.Duration; BatchTimeout time.Duration; Changed []string; Dirty bool }` (drop the v2 `Scope` field — no scope model). `Classify(in ClassifyInputs) (Digest, bool)` preserving the frozen pinned decision order: `Report` present → `distill` (terminal, bool true); else `TurnEnded` → dead/asking; else `Elapsed > BatchTimeout` → dead/timeout; else `!StrandLive` → dead/died; else a running snapshot (non-terminal, bool false). In `classify_test.go` add Tier-1 tests (no git; construct inputs directly) covering each branch in order, especially that a present Report short-circuits to terminal and that the dead-reason precedence matches the pinned order.
- **Commit:** `feat(websterengine): webster-local recovery classification`

### Card 24: long-poll to terminal (PollUntilTerminal + clock seam)

- **Context:**
  - `internal/builderengine/poll.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/classify.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/poll.go`
  - `internal/websterengine/poll_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `poll.go` add a webster-local copy of `builderengine.PollUntilTerminal` retargeted to webster's `Digest`: `PollUntilTerminal(gather func() (Digest, bool, error), wait time.Duration, clk clock) (Digest, error)` — a blocking long-poll loop calling `gather` on a 1s tick until it reports terminal (bool true) or the `wait` deadline elapses; a `gather` error propagates; on deadline return the last running digest with nil error. Bring the unexported `clock` interface, `realClock`, and the `pollTick` seam over as local copies so tests can inject a fake clock (mirror the frozen seam). In `poll_test.go` add Tier-1 tests (no git; fake clock) for terminal-before-deadline, deadline-with-running-digest, and gather-error propagation.
- **Commit:** `feat(websterengine): webster-local PollUntilTerminal with injectable clock`

## Batch Tests

`verify: go test -tags integration ./internal/websterengine/...` runs the Tier-1 report/digest/
classify/poll tests (all `t.TempDir()`/fake-based, no git) plus the batch-5 git tests. New types
are additive and not yet wired into the verb files; the retargets land in batch 7.
