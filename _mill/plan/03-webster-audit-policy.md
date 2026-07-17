# Batch: webster-audit-policy

```yaml
task: 'Master Builder: new, parallel fork-based implementation module'
batch: 'webster-audit-policy'
number: 3
cards: 3
verify: go test ./internal/websterengine/...
depends-on: [2]
```

## Batch Scope

Webster's own fork-audit policy — the second consumer of shuttleengine's
policy-free `ForkAudit` facts, with rules opposite to burler's on writes:
implementer forks MUST write and commit to the host repo, so the policy allows
Write/Edit and host git while hard-banning nested Agent calls, named spawns,
and any weft reference; the Master (parent) session may write only its two
contract files. Plus the attribution engine: diffing new fork transcripts
against the seen set with a bounded settle-retry, and the pinned
transcript-count-before-report-presence check order. Pure functions throughout
(facts in, verdict out) — the TDD centre of the module. External interface:
`AuditBatch`, `AuditViolation`, `NewTranscripts`, `SettleRetry` consumed by
`RecordBatch` (batch 5) and the run-exit cross-check (batch 7).

## Cards

### Card 12: policy rules

- **Context:**
  - `internal/shuttleengine/forkaudit.go`
  - `internal/burlerengine/cluster.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/audit.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `audit.go`:
  (a) `type AuditViolation struct` (class, transcript path, detail) with an
  `Error()` — hard violations are errors, per the fail-loud posture;
  (b) `weftReferencePattern(layout *hubgeometry.Layout) *regexp.Regexp` (or an
  equivalent matcher constructor) matching a Bash command string that invokes
  `lyx weft` or `lyx warp`, or that references the weft worktree — built at
  runtime from `layout.WeftWorktree()` and the exported `hubgeometry.WeftSuffix`
  constant, NEVER from a `-weft` string literal in this package (the geometry
  token would trip `TestEnforcement_GeometryLiterals`);
  (c) `CheckFork(f shuttleengine.ForkReport, weftRef *regexp.Regexp) []AuditViolation`:
  hard — `AgentCalls > 0` (forks cannot nest, even denied attempts), any
  `BashCommands` entry matching `weftRef`. Write/Edit and host-repo git are
  explicitly allowed (per-card commits are the implementer contract) —
  document this contrast with burler's read-only policy;
  (d) `CheckParent(a shuttleengine.ForkAudit, outcomePath, summaryPath string, weftRef *regexp.Regexp) []AuditViolation`:
  hard — `NamedSpawns > 0` (silent context loss), any `ParentWrites` entry
  whose cleaned path is neither `outcomePath` nor `summaryPath` (a Master
  implementing batches itself or hand-writing a batch report), any
  `ParentBashCommands` entry matching `weftRef`;
  (e) `ForkWarnings(f shuttleengine.ForkReport) []string` — `!ReportReturned`
  is a warning, never round-failing (burler parity).
- **Commit:** `webster: fork-audit policy (writes allowed, weft and nesting banned)`

### Card 13: attribution and settle-retry

- **Context:**
  - `internal/shuttleengine/engine.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `internal/websterengine/audit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** (a) `NewTranscripts(audit shuttleengine.ForkAudit, seen []string) []shuttleengine.ForkReport`
  — the fork reports whose `TranscriptPath` is not in `seen` (defensive
  re-filter even when the engine already filtered);
  (b) `type Sleeper interface { Sleep(time.Duration) }` (or reuse a
  func-typed seam) so tests never sleep for real;
  (c) `SettleRetry(fetch func() (shuttleengine.ForkAudit, error), seen []string, window time.Duration, tick time.Duration, s Sleeper) (shuttleengine.ForkAudit, []shuttleengine.ForkReport, error)`
  — re-invokes `fetch` until at least one new transcript appears or `window`
  elapses; implements the flush-timing de-risk from discussion.md (`first
  miss is inconclusive`): the zero-transcript hard error is only issued by the
  caller AFTER the settle window is exhausted. Defaults documented:
  window a few seconds, 250ms tick;
  (d) `ClassifyAttribution(newReports []shuttleengine.ForkReport) (warning string, err error)`
  pinning the check order from discussion.md `fork-audit-policy`: zero new
  transcripts after settle = hard error REGARDLESS of report presence (an
  unforked batch's report means the Master wrote it); more than one = warning
  only (fork-error-then-re-fork without an intervening record-batch), never
  hard.
- **Commit:** `webster: transcript attribution with bounded settle-retry`

### Card 14: audit policy tests

- **Context:**
  - `internal/shuttleengine/forkaudit.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/audit_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Table-driven tests (the discussion's named TDD candidate)
  covering every violation class and every allowed case:
  fork nested-Agent → hard; fork Write/Edit → allowed; fork host `git commit`
  → allowed; fork `lyx weft sync` → hard; fork `git -C <weft-worktree> add`
  → hard; parent named spawn → hard; parent write to `outcome.yaml` /
  `summary.md` → allowed; parent write to any source file or to a reports-dir
  path → hard; parent weft bash → hard; `ReportReturned == false` → warning
  only. Attribution: zero-new after settle → error; one-new → clean;
  two-new → warning; `SettleRetry` returns early when a transcript appears on
  a later tick (fake sleeper records requested sleeps; no real sleeping); the
  weft matcher never contains a hardcoded geometry token (construct it from a
  fake layout in tests).
- **Commit:** `webster: table-driven audit policy tests`

## Batch Tests

`go test ./internal/websterengine/...`. The policy and attribution functions
are pure; tests are exhaustive tables with fake `ForkAudit` values and a
recording fake `Sleeper` — untagged, spawn-free, no real time. Weft-pattern
tests construct a `hubgeometry.Layout` value directly (no git spawns).
