# Batch: anomaly-detector

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
batch: 'anomaly-detector'
number: 2
cards: 2
verify: go test ./internal/loomengine/
depends-on: []
```

## Batch Scope

This batch delivers the whole judgment half of Tier 1 as pure, no-I/O Go in `internal/loomengine`: the told-input value types, the five-trigger detector, the deterministic title rendering each detected anomaly carries, and the sibling body renderer.
It is independent of batch 1 — the ledger content it consumes crosses the package boundary as a `loomengine`-declared value type the caller populates, so this package gains no `internal/shedadapters` import and nothing here reads a file or derives a path.

The external interface batch 3 consumes is: `loomengine.EntryObservation`, `loomengine.LedgerObservation`, `loomengine.Anomaly` with its `AnomalyKind` constants, `loomengine.DetectAnomalies`, `loomengine.DetectCrashResume`, and `loomengine.RenderAnomalyBody`.

Batch-local decision differing from the overview's Shared Decisions: none.

## Cards

### Card 5: the anomaly value types, the five-trigger detector, and title rendering

- **Context:**
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/status.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/anomaly.go`
  - `internal/loomengine/anomaly_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomengine/anomaly.go` with a file doc comment declaring it the pure, no-I/O, spawn-free detector over told inputs, exhaustively table-tested in Tier 1, and naming `coherence.go` as the in-package shape it follows.

  Declare `type AnomalyKind string` with five exported constants and these exact wire spellings, which are the `<kind>` token every title embeds:
  `AnomalyCrashResume AnomalyKind = "crash-resume"`,
  `AnomalyEscalation AnomalyKind = "escalation-to-human"`,
  `AnomalyBudgetExhausted AnomalyKind = "bounce-budget-exhausted"`,
  `AnomalyProducerFailure AnomalyKind = "producer-hard-failure"`,
  `AnomalyRecurringFinding AnomalyKind = "recurring-finding"`.

  Declare `type EntryObservation struct` carrying the whole entry-time observation as data so the cancelled-context path needs no read of its own: `Observed bool` (false when the entry step was skipped or its read failed), `RunLockHeld bool` (the non-blocking run-lock probe's result, taken before the read), `State shedengine.State`, `CurrentProducer string`, `HistoryLength int`, `Slug string`, `Parent string`.

  Declare `type LedgerObservation struct { Producer string; Round int; Key string; Rounds []int; Status string }` — one ledger entry plus the round its file claimed and the `producer` field of the history entry that published that file's path.
  `Producer` is the Bouncer row name, read from history data by the caller, never derived from a recipe row list here.

  Declare `type Anomaly struct` carrying everything the title and body need with no further lookup: `Kind AnomalyKind`, `Title string` (rendered by the detector, never by the caller), `Slug string`, `Parent string`, `State shedengine.State`, `CurrentProducer string`, `Error string`, `History []shedengine.HistoryEntry`, `Round int`, `BouncerRow string`, `LedgerKey string`, `LedgerRounds []int`, `LedgerStatus string`.
  The five trigger-5-only fields are zero for the other four kinds, and `Round` is the ledger file's own round, used by the caller's collapse step to keep the most complete occurrence.

  Declare `func DetectCrashResume(entry EntryObservation, slug string) (Anomaly, bool)` as its own exported function, not merely an internal branch, because the caller must be able to reach trigger 1 alone on a cancelled context without a final status in hand.
  It reports an anomaly when, and only when, `entry.Observed` is true, `entry.RunLockHeld` is false, `entry.State` is `shedengine.StateRunning`, and `entry.HistoryLength` is greater than zero.
  A held run lock means a live driver, never a crash.
  An empty history is a fresh seed at `Preflight`, byte-identical on disk to a crash at `Preflight`, so it is deliberately not reported — the alternative would file a crash-resume for every ordinary first drive of every task.
  A `paused`, `blocked`, or `failed` entry state is an ordinary human resume, not a crash.

  Declare `func DetectAnomalies(entry EntryObservation, final shedengine.Status, product Status, ledgers []LedgerObservation) []Anomaly` returning every detected anomaly in one deterministic order: the crash-resume first when present, then the one halt-kind anomaly derived from `final` when present, then the trigger-5 anomalies in the order their `ledgers` elements were told.
  Pin that order in the doc comment — an unordered result cannot be table-asserted.
  It takes `product` because every title embeds the slug and `shedengine.Status` does not carry one;
  `Slug` and `Parent` on each returned `Anomaly` come from `product`, except on the crash-resume, which takes them from `entry` so the cancelled-context path stays read-free.
  `DetectAnomalies` calls `DetectCrashResume` internally rather than reimplementing it.

  The four remaining triggers, all read straight off `final`:
  `final.State == shedengine.StateBlocked` with `final.Error` exactly `"stuck with no OnStuck target"` is `AnomalyEscalation`.
  `final.State == shedengine.StateBlocked` with `final.Error` exactly `"bounce budget exhausted"` is `AnomalyBudgetExhausted`.
  Both literals are set verbatim by the `outcome == Stuck` arm the Context lists;
  a blocked state carrying any other error text matches neither, and must not over-match on state alone.
  `final.State == shedengine.StateFailed` is `AnomalyProducerFailure`, matching on state alone with `final.Error` carried verbatim, since that text is whatever the producer returned and matches no literal.
  A `LedgerObservation` whose `Status` is `"open"` and whose `Rounds` has three or more elements is `AnomalyRecurringFinding`;
  a `"resolved"` status is never reported however long its `Rounds` list, and a short or missing list means no anomaly rather than corruption, because carry-forward is a claim the judge made and not something the parser guarantees.
  Threshold three, not five, because every review segment carries `max_bounces: 5` and the Bouncer's own seed call permanently consumes one unit — firing at three reports the recurrence while the segment is still alive rather than only after it has degenerated into the bounce-budget trigger.

  Render each anomaly's `Title` inside the detector, deterministically, with the discriminator differing per trigger because the triggers differ in whether they are re-observed at all.
  For the three halt kinds: `loom anomaly: <kind> — <slug> — <producer>#<success-count>`, where `<producer>` is `final.CurrentProducer` and `<success-count>` is the number of entries in `final.History` whose `Producer` equals that name and whose `Outcome` is `shedengine.Done` — that is, how many times this row had previously succeeded.
  `Outcome` and its `Done` constant are declared in `internal/shedengine/producer.go`, listed in this card's Context;
  `run.go` uses them unqualified in-package, so the identifier is visible there but not declared there.
  A count is required rather than a history length because a blocked run's history grows on every resume: a resume of an unfixed escalation re-calls the halting producer and appends another stuck entry, so a length-keyed title would mint a fresh title — and therefore a fresh issue — on every single resume, which is the exact failure the marker exists to prevent.
  Counting that producer's own done entries is invariant under precisely that append and moves only when the row genuinely succeeds.
  For the crash-resume: `loom anomaly: crash-resume — <slug> — <producer>@<history-length>`, using `entry.CurrentProducer` and `entry.HistoryLength`, because a crash-resume is observed at most once and needs no stability under re-observation, only the finer distinctness that separates a crash at one point in the run from a crash at another.
  For the recurring finding: `loom anomaly: recurring-finding — <slug> — <bouncer-row> — <ledger-key>`.
  The row name is required because a ledger key is an LLM-authored short finding identity scoped to its own segment's run directory, with nothing making it unique across the discussion, plan, and webster segments — two unrelated findings that happened to pick the same key would otherwise collapse into one issue and permanently suppress the second.
  Use an em dash with a single space on each side as the separator in all three shapes.

  Create `internal/loomengine/anomaly_test.go` as an untagged Tier-1 table suite following `coherence_test.go`'s shape: hand-built `shedengine.Status` values and hand-built observations, asserting the exact set of detected anomalies.
  Write the table before the detector.
  Mandatory cases: a clean done run with non-empty history and no ledgers yields an empty result;
  entry `running` with non-empty history yields a crash-resume;
  entry `running` with empty history yields none;
  entry `paused`, `blocked`, and `failed` each yield none;
  entry `running` with non-empty history but the probe reporting the lock held yields none, asserted directly because a live driver's run must never be reported as a crash;
  blocked with each of the two exact error literals yields its own kind;
  blocked with some other error text yields neither;
  failed with arbitrary error text yields the producer-hard-failure kind and neither blocked kind.
  Title stability, asserted directly rather than implied: take a blocked status, build the resumed shape of the same unresolved halt by appending one more stuck entry for the same producer, and assert the two yield byte-identical titles — a length-based discriminator passes every other case in this list and fails only this one.
  Title distinctness: two escalations at different producers yield different titles, and an escalation at a producer with one prior done entry differs from one at a producer with none.
  Trigger-5 titles: two ledger observations carrying the same key but different `Producer` values yield two distinct titles, not one;
  and the same producer-and-key pair recurring after a resolved round yields the same title, asserted as intended identity-dedupe behaviour so it is not later read as a bug.
  Ledger thresholds: `status: open` with three rounds yields one anomaly, with two rounds yields none, with five rounds yields one and not three;
  `status: resolved` with a long rounds list yields none.
  Multiple independent anomalies in one status are all reported in the pinned order.
  The same input detected twice yields byte-identical titles.
- **Commit:** `feat(loomengine): add the pure Tier-1 structural-anomaly detector`

### Card 6: render the issue body

- **Context:**
  - `internal/loomengine/anomaly.go`
  - `internal/shedengine/status.go`
- **Edits:** none
- **Creates:**
  - `internal/loomengine/anomalybody.go`
  - `internal/loomengine/anomalybody_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomengine/anomalybody.go` declaring `func RenderAnomalyBody(a Anomaly) string`, a second pure function beside the detector, and give the file a doc comment stating that rendering lives here — not in the I/O layer — so both the title and the body are table-testable in Tier 1.

  The rendered markdown must be actionable by someone reading it cold with no access to the worktree, because the status file lives under `_lyx` and the ledgers under `.lyx`, in a worktree that may already be torn down.
  It carries, in this order: the anomaly kind and a one-line statement of what was detected;
  the task slug and parent branch;
  the final state, current producer, and error verbatim;
  the relevant history entries rendered as producer/outcome/at rows;
  and, for the recurring-finding kind only, the Bouncer row name plus the ledger entry's key, its rounds list, and its status.

  Every field it renders is already carried on the `Anomaly` value, so the function performs no lookup, no read, and no derivation.
  Render the error text inside a fenced block so a producer error containing markdown cannot corrupt the issue body.
  Omit the recurring-finding section entirely for the other four kinds rather than emitting empty headings.
  Render an empty history as an explicit "no history entries" line rather than an empty section.

  Create `internal/loomengine/anomalybody_test.go` as an untagged Tier-1 suite asserting: the body of each of the five kinds contains the slug and the parent;
  a halt-kind body contains the final state, the current producer, and the error text verbatim;
  a recurring-finding body contains the Bouncer row name, the ledger key, every element of the rounds list, and the status, and the other four kinds' bodies contain none of those;
  a history slice renders one row per entry naming its producer, outcome, and timestamp;
  an empty history renders the explicit no-entries line;
  and the same `Anomaly` rendered twice yields a byte-identical string.
- **Commit:** `feat(loomengine): render the anomaly issue body as a pure function`

## Batch Tests

`verify:` runs `internal/loomengine` alone — the only package this batch touches.
Files covered: `internal/loomengine/anomaly_test.go` (card 5's detection, ordering, and title table) and `internal/loomengine/anomalybody_test.go` (card 6's rendering cases), plus the package's existing suite, which must keep passing since both cards add files rather than changing any.

Everything here is pure in-memory table work over hand-built values: no process spawn, no temp directory, no fixture tree.
