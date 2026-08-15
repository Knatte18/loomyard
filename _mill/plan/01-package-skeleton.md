# Batch: package-skeleton

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: package-skeleton
number: 1
cards: 6
verify: go test ./internal/shedengine/...
depends-on: []
```

## Batch Scope

This batch creates the `internal/shedengine` package and every type in it that is not the loop itself: the `ShedProducer` seam, the `Shed`/`Result` engine shapes, the persisted status-file types, the mechanical `activity` fill, and `Run`'s pre-loop validation.
It is one batch because these are all pure, dependency-free declarations plus two small pure functions — nothing here reads or writes a file, so the whole batch compiles and tests in isolation.
The external interface batch 2 consumes is the whole of it: `Shed`, `ProducerDef`, `ShedProducer`, `Status`, `HistoryEntry`, `Activity`, `State`, `Outcome`, `RunOutcome`, `Result`, `ErrShedBusy`, `defaultMaxBounces`, `composeActivity`, and `(*Shed).validate`.

Batch-local decision, beyond `## Shared Decisions` in the overview: cards 4, 5, and 6 are the discussion's three named TDD candidates.
Each of those cards writes its test file first and the implementation second, within the one card — they are the pure, table-friendly units where test-first genuinely pays, and keeping the pair in one card is what makes "test first" executable by a single implementer without leaving the batch red across a card boundary.

## Cards

### Card 1: package doc

- **Context:**
  - `_mill/discussion.md`
  - `manifest/designs/shed.md`
  - `internal/treadleengine/doc.go`
  - `docs/reference/status-schema.md`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the package doc for `package shedengine`, modelled on `internal/treadleengine/doc.go`'s shape — a lead paragraph naming what the package is, then `# `-prefixed headings for each durable rationale section.
  Use only `//` line comments, never a `/* */` block.
  Sections required, each one a `# ` heading:
  (a) **What Shed is** — a generic outer phase-FSM that walks one flat, ordered list of producers with no predefined slots, honoring resume, crash-recovery, and pause uniformly at producer granularity; what makes a product a product is which producers are in its list and in what order.
  (b) **Told, never derived** — `Shed` is told `StatusPath`, `LockPath`, and `StatusLockPath` and derives none of them; it resolves no cwd and names no durable/ephemeral directory convention.
  State that the caller is responsible for supplying paths that already obey the Durable-vs-Ephemeral State Invariant in `CONSTRAINTS.md` (the status file durable, both locks never-tracked transients), because `Shed` cannot and does not choose either location.
  (c) **The `ShedProducer` contract's two caller-side obligations** — a producer must return exactly `Done` or `Stuck` and nothing else, and it must surface context cancellation as a non-nil `error` from `Call`, never as `Stuck`.
  Explain why the second cannot be enforced mechanically: a `Stuck` return with a cancelled context is indistinguishable to `Shed` from a genuine producer verdict, so a producer that reports cancellation as `Stuck` would silently consume bounce budget or escalate to blocked for what was an operator stop.
  (d) **The external-writer lock contract** — `Shed` is not the status file's only writer; any other actor that writes it (a product's pause verb, its spawn-time seeder, anything touching `product`) must go through `internal/state` using the same `StatusLockPath` `Shed` was told.
  State plainly that `internal/state`'s lock is advisory and keyed on the caller-supplied lock path, so the read-modify-write merge is safe against a concurrent external writer *that takes the same lock*, and against no other — never state the merge-safety property unconditionally.
  (e) **Divergence from loom's status schema** — `internal/loomengine`'s status type and `docs/reference/status-schema.md` pin a different shape (`phase`/`stage`/`history` entries of `{phase, outcome, bounced_to, ts}`) from this package's (`current_producer`/`state`/`activity`/`history` entries of `{producer, outcome, output, at}`), and this package deliberately defines its own rather than reconciling them.
  State explicitly that the opaque `product` passthrough field carries **no** compatibility claim for loom's schema — a Shed-written file would still fail loom's coherence check, since `phase`, `stage`, and `narration` are top-level fields there.
  Reconciling the two shapes is loom's own later rewiring work.
  Do not name `internal/loomengine` in an import; this is prose only.
- **Commit:** `docs(shedengine): add package doc for the outer phase-FSM skeleton`

### Card 2: the ShedProducer seam

- **Context:**
  - `manifest/designs/shed.md`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/result.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/producer.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/producer.go` holding the producer seam and nothing else, with a file-level comment in the shape `internal/treadleengine/runner.go` uses.
  Declare exactly:

  ```go
  // Outcome is one producer call's verdict.
  type Outcome string

  // The two legal Outcome values. A ShedProducer must return exactly one of these;
  // any other value is an engine-level failure, not a third verdict.
  const (
  	Done  Outcome = "done"
  	Stuck Outcome = "stuck"
  )

  // OutputPointer names a producer's artifact for a human to read.
  // Shed never introspects Path's contents, never validates it, and never stats it to make a
  // control-flow decision -- step 4 is an unconditional re-call.
  type OutputPointer struct {
  	Path string // "" = no artifact (gate or terminal producer)
  }

  // ShedProducer is the seam Shed drives once per iteration: Call runs one producer to a verdict
  // and reports it, an optional output pointer, and a hard error.
  // Two obligations Shed cannot enforce mechanically bind every implementation: return exactly
  // Done or Stuck, and surface context cancellation as a non-nil error, never as Stuck.
  type ShedProducer interface {
  	Call(ctx context.Context) (Outcome, OutputPointer, error)
  }

  // ProducerDef is one entry in Shed's flat, ordered producer list: the seam plus the two things
  // the list needs around it.
  type ProducerDef struct {
  	Name     string
  	Producer ShedProducer
  	OnStuck  string // "" = escalate to human; else bounce back to this Name
  }
  ```

  Give `ProducerDef`'s three fields their own explanatory doc comments or a field-level comment covering each — `Name` is the identity `current_producer` names on disk and must be non-empty, `Producer` must be non-nil, and `OnStuck` is what makes a bounce-back a per-producer config value rather than a hardcoded branch in the loop.
  The only import is `context`.
- **Commit:** `feat(shedengine): add the ShedProducer seam and ProducerDef`

### Card 3: the Shed engine shape, Result, and the busy sentinel

- **Context:**
  - `manifest/designs/shed.md`
  - `internal/treadleengine/result.go`
  - `internal/treadleengine/run.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/errors.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/shed.go` holding the engine struct and the run-level result vocabulary:

  ```go
  // Shed walks one flat, ordered producer list, honoring resume, crash-recovery, and pause
  // uniformly at producer granularity. It is a plain exported-field struct on purpose: there is no
  // New constructor, which would leave a bare struct literal as a second, unvalidated door.
  // Run validates every field below before it touches anything.
  type Shed struct {
  	Producers      []ProducerDef
  	StatusPath     string
  	LockPath       string
  	StatusLockPath string
  	MaxBounces     int
  }

  // RunOutcome is the whole run's terminal classification.
  type RunOutcome string

  // The three legal RunOutcome values. Their string values are deliberately identical to State's
  // three clean-exit values, so mapping between the two is identity, never a lookup table.
  const (
  	RunDone    RunOutcome = "done"
  	RunBlocked RunOutcome = "blocked"
  	RunPaused  RunOutcome = "paused"
  )

  // Result is what Run reports on a clean exit.
  type Result struct {
  	Outcome        RunOutcome
  	HaltedProducer string
  	Reason         string
  	History        []HistoryEntry
  }

  // defaultMaxBounces is the bounce budget Run uses when Shed.MaxBounces is 0.
  const defaultMaxBounces = 10
  ```

  Document the four `Shed` fields with the rules `Run` enforces: `StatusPath` is the durable status file, told and never derived; `LockPath` is the run lock, held non-blocking for the whole of one `Run`; `StatusLockPath` is the lock `internal/state` itself takes, and it must name a different file from `LockPath` because `internal/state` acquires it with the *blocking* form, so one shared path would hang on the first persist rather than failing; `MaxBounces` is the total `Stuck`-routed bounce budget for one `Run` call, in-memory and never persisted, where `0` means "use the internal default of 10" and never "no bounces allowed".
  Document `Result` with two rules stated outright, following the doc-comment discipline `internal/treadleengine/result.go` uses:
  a caller must branch on `Outcome` before reading `Reason`, which is populated only alongside `RunBlocked`;
  and `Result` is meaningless unless the returned `error` is nil, because `RunOutcome`'s zero value is the empty string — not one of the three legal constants — and every hard-error path returns an unpopulated `Result` alongside its error.
  Document `HaltedProducer` as the producer `current_producer` named when `Run` returned, and `History` as the full persisted history as it stands when `Run` returns, not only the entries this invocation appended.
  Then create `internal/shedengine/errors.go` declaring the package's single exported sentinel, mirroring `treadleengine.ErrBlockBusy`'s role and doc-comment shape: `ErrShedBusy`, returned wrapped with `%w` when `LockPath` is already held so a caller can `errors.Is`-match it and retry later rather than treating it as a real failure.
- **Commit:** `feat(shedengine): add the Shed engine shape, Result, and ErrShedBusy`

### Card 4: the persisted status-file types

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/state/state.go`
  - `internal/shedengine/producer.go`
  - `internal/loomengine/coherence.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/status.go`
  - `internal/shedengine/status_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `internal/shedengine/status_test.go` first, then `internal/shedengine/status.go`.

  `internal/shedengine/status.go` declares the persisted shape with its JSON tags:

  ```go
  // State is the status file's own lifecycle field: a superset of RunOutcome, adding the two
  // values Run can never return (running, failed).
  type State string

  // The five legal State values. A persisted value outside this set -- including the empty
  // string -- is a hard error at the read gate.
  const (
  	StateRunning State = "running"
  	StatePaused  State = "paused"
  	StateDone    State = "done"
  	StateBlocked State = "blocked"
  	StateFailed  State = "failed"
  )

  // Activity is the human-facing summary Shed fills mechanically on every persist.
  type Activity struct {
  	Now  string `json:"now"`
  	Last string `json:"last"`
  	Wait string `json:"wait"`
  }

  // HistoryEntry is one producer call's durable record, and the element type of Result.History.
  type HistoryEntry struct {
  	Producer string  `json:"producer"`
  	Outcome  Outcome `json:"outcome"`
  	Output   string  `json:"output"`
  	At       string  `json:"at"`
  }

  // Status is the whole status file.
  type Status struct {
  	CurrentProducer string          `json:"current_producer"`
  	State           State           `json:"state"`
  	Error           string          `json:"error"`
  	PauseRequested  bool            `json:"pause_requested"`
  	Activity        Activity        `json:"activity"`
  	History         []HistoryEntry  `json:"history"`
  	Product         json.RawMessage `json:"product,omitempty"`
  }
  ```

  Add an unexported method `func (s State) valid() bool` reporting whether the receiver is one of the five constants.
  Document why the empty string is rejected rather than tolerated: `State` is a mandatory enum string read from a file an external actor seeds, so a typo or a partial seed would otherwise fall through to undefined behaviour — silently treated as running, or as done.
  Name the in-repo precedent for the split in that comment: `checkCoherence` in `internal/loomengine/coherence.go` treats an empty mandatory enum string as absent and therefore a violation, while reserving zero-value tolerance for the nullable, bool, and slice fields.
  `PauseRequested` (bool) and `History` (slice) keep that zero-value tolerance here for the same reason.
  Document `Product` as an opaque product-owned payload `Shed` round-trips verbatim and never inspects, validates, or interprets, and state that it carries no compatibility claim for loom's own schema.
  Document the field-ownership split on the `Status` type itself: `CurrentProducer`, `State`, `Error`, `Activity`, and `History` are Shed-owned and rewritten on every persist;
  `PauseRequested` is shared write-to-clear, set true only by an outside actor and written false only by `Shed`, exactly once, in the persist that records `StatePaused`;
  `Product` is external-writer-owned and only ever carried through.
  The `encoding/json` import is needed for `json.RawMessage`.
  The file-level comment must warn that the persistence package `internal/state` and this file's own `State` type are two different things, and that local identifiers must keep the two apart.

  `internal/shedengine/status_test.go` covers the JSON round-trip TDD candidate:
  a `Status` value with every field populated, including a non-empty `History` and a `Product` payload, marshals and unmarshals back to an equal value;
  and a `Product` payload survives a full write-then-read cycle through `state.WriteJSON` and `state.ReadJSONStrict` against a file under `t.TempDir()`.
  Compare `Product` by **semantic** equality — unmarshal both sides into `any` and `reflect.DeepEqual` those, or re-marshal both through `json.Marshal` and compare the normalised bytes — never a raw byte compare of the two `json.RawMessage` values.
  State the reason in a comment on that assertion: persistence goes through `json.MarshalIndent`, which re-indents an embedded `json.RawMessage`, so a payload written with different whitespace survives semantically but not byte-for-byte.
  Also assert that `valid` accepts each of the five constants and rejects both a typo value and the empty string.
- **Commit:** `feat(shedengine): add the persisted status-file types`

### Card 5: the mechanical activity fill

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/activity.go`
  - `internal/shedengine/activity_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `internal/shedengine/activity_test.go` first, then `internal/shedengine/activity.go`.

  `internal/shedengine/activity.go` declares one unexported function:

  ```go
  func composeActivity(currentProducer string, history []HistoryEntry, st State, errText string) Activity
  ```

  Its three rules, all mechanical, all from data `Shed` already holds:
  `Now` is `currentProducer` verbatim;
  `Last` is the empty string when `history` is empty, and otherwise the most recent entry composed as exactly the producer name, a space, U+2192 RIGHTWARDS ARROW, a space, then the outcome — so a `Plan-Write` entry with outcome `done` composes to `Plan-Write → done`;
  `Wait` is `errText` when `st` is `StateBlocked` or `StateFailed`, and the empty string for every other state.
  Document why the `Last` format is pinned to an exact string rather than left to judgment: a test asserts this field, and an unpinned "formatted for a human" cannot be asserted, only approximated.

  `internal/shedengine/activity_test.go` is a table test over the three rules: an empty history yields an empty `Last`; a multi-entry history composes from the last entry only; `Wait` is populated for `StateBlocked` and `StateFailed` and empty for each of `StateRunning`, `StatePaused`, and `StateDone` even when `errText` is non-empty;
  and `Now` echoes `currentProducer` including when it is empty.
  Assert the composed `Last` string literally, arrow included.
- **Commit:** `feat(shedengine): add the mechanical activity fill`

### Card 6: Run's pre-loop validation

- **Context:**
  - `manifest/designs/shed.md`
  - `_mill/discussion.md`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/validate.go`
  - `internal/shedengine/validate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `internal/shedengine/validate_test.go` first, then `internal/shedengine/validate.go`.

  `internal/shedengine/validate.go` declares one unexported method, `func (s *Shed) validate() error`, which batch 2's `Run` calls before it acquires any lock, reads anything, or calls any producer.
  Every rule below is a returned error carrying its own distinct, `shedengine: `-prefixed message; no rule shares wording with another, because the test asserts a distinct substring per rule.
  Check, in this order:
  each of `StatusPath`, `LockPath`, and `StatusLockPath` being empty (three separate messages naming the field);
  `LockPath` equal to `StatusLockPath`;
  `MaxBounces` being negative;
  `Producers` being empty;
  and then, walking `Producers` in order, an empty `Name`, a nil `Producer`, a `Name` already seen earlier in the list, and an `OnStuck` that is non-empty and names no `Name` present in the list.
  The `OnStuck` check must run after the whole name set is collected, so a forward reference to a later producer is legal.
  Every per-producer message names the offending index and, where it exists, the offending `Name`.

  Document in the method's own comment why two of these rules exist, because both look like defensive noise otherwise:
  an empty `Name` is rejected because the empty string is already load-bearing twice over — it is `OnStuck`'s escalate-to-human sentinel, so a producer literally named `""` would make an `OnStuck: ""` ambiguous, and it is the zero value a malformed or partial seed leaves in `current_producer`, which the loop's lookup would then resolve successfully and *run*, turning a corrupt status file into silent execution;
  a nil `Producer` is rejected because it panics at the call step rather than failing loud, and a panic inside a long unattended run is strictly worse than a validation error at second zero.
  Also note that the equal-lock-paths rule exists to convert a deadlock into an error: `internal/state` acquires its lock with the blocking form, so a `Shed` whose two lock paths name one file would hang on its first persist rather than failing.

  `internal/shedengine/validate_test.go` is one table with one case per rule above, each asserting a non-nil error whose message contains that rule's own distinct substring, plus one case asserting a fully-valid `Shed` returns nil.
  The nil-`Producer` case must assert a returned error, never a recovered panic — if the test needs a `recover`, the implementation is wrong.
  Include a case proving a forward `OnStuck` reference (producer 1 bouncing to producer 2) is accepted.
  Use a trivial in-test stub type satisfying `ShedProducer` for the non-nil `Producer` fields;
  the shared `funcProducer` fake arrives in batch 2 and must not be anticipated here.
- **Commit:** `feat(shedengine): add Run's pre-loop validation`

## Batch Tests

`verify: go test ./internal/shedengine/...` runs the three test files this batch creates — `internal/shedengine/status_test.go`, `internal/shedengine/activity_test.go`, and `internal/shedengine/validate_test.go` — which are the whole of the package's test surface at this point.
The scope is exactly this batch's own new package, not the wider suite;
nothing outside `internal/shedengine` changes here, so nothing outside it needs running.
The three files are the discussion's three named TDD candidates and cover every pure unit this batch introduces: the status-file JSON round-trip including the `product` passthrough's semantic-equality rule, the `State.valid` enum gate, the three `composeActivity` rules, and every one of `validate`'s nine rules.
The loop itself has no test here because it does not exist yet — batches 2, 3, and 4 own that.
