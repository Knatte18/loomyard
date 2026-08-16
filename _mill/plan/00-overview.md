# Plan: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
slug: shed-adapters
approved: true
started: '20260816-151326'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: package-foundation-and-singlellm
    file: 01-package-foundation-and-singlellm.md
    depends-on: []
    verify: go test ./internal/shedadapters/...
  - number: 2
    name: perch-producer
    file: 02-perch-producer.md
    depends-on: [1]
    verify: go test ./internal/shedadapters/...
  - number: 3
    name: webster-producer
    file: 03-webster-producer.md
    depends-on: [1]
    verify: go test ./internal/shedadapters/...
  - number: 4
    name: docs
    file: 04-docs.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/shedadapters/... ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: one package, `internal/shedadapters`

- **Decision:** all three adapters, their seam interfaces, and the shared context/archive helpers live in one new package `internal/shedadapters`.
  Nothing under `internal/shedengine`, `internal/shuttleengine`, `internal/perchengine`, or `internal/websterengine` is edited by this task.
- **Rationale:** the adapter lives in the caller of the seam-owning engine, mirroring `burlerAdapter` in `internal/perchengine/adapter.go` one level down.
  `CONSTRAINTS.md`'s Shed Producer-Seam Invariant forbids `shedengine` from importing producers, and `perchcli`/`webstercli` ship as standalone CLIs that must not gain a `shedengine` dependency.
- **Applies to:** all batches

### Decision: told, never derived

- **Decision:** every constructor receives already-resolved absolute paths and already-constructed engines (or a factory over them).
  No adapter calls `lyxcwd`, `os.Getwd`, or git; no adapter writes the literals `_lyx` or `.lyx`.
  The only paths any adapter builds are a run-id leaf joined onto a told base and an archive sibling beside a told output file.
- **Rationale:** the Cwd Resolution Invariant and the Lyxdirs Single-Declarer Invariant in `CONSTRAINTS.md`, plus the same discipline `shedengine`, `treadleengine`, and `perchengine` already hold.
- **Applies to:** all batches

### Decision: cancellation — entry check, exit precedence, bridge only for perch

- **Decision:** every adapter checks `ctx.Err()` at `Call` entry and returns immediately without starting anything.
  On exit, a cancelled context replaces every result **except** a genuine success verdict (shuttle `OutcomeDone`, perch `OutcomeApproved`, Webster `"done"`), which is returned as `shedengine.Done` with its pointer.
  Only `PerchProducer` installs a mid-run bridge, because only perch's pause seam is a caller-supplied callback.
- **Rationale:** `ShedProducer`'s second obligation is that cancellation surfaces as a non-nil error, never as `Stuck`.
  Converting a finished success into the ctx error would make Shed record no history entry, so the next `Call` would archive a valid artifact and pay for the same LLM session twice.
- **Applies to:** all batches

### Decision: every adapter is told its producer name

- **Decision:** each `New...` constructor takes a `name string`, used only as a log field and in error text.
  It is never compared, parsed, or used for control flow.
- **Rationale:** `Call(ctx)` carries no identity, and two instances of `SingleLLMProducer` in one producer list is the expected shape, so an unattributed log line or `state: "failed"` error string is unusable.
- **Applies to:** all batches

### Decision: `StuckReason` and asking detail ride the log, never the seam

- **Decision:** every `Stuck` return carries an empty `shedengine.OutputPointer` and a nil error; the engine's `StuckReason` (or the asking agent's last message) is emitted via `logger.Warn` with the told producer name and the engine name.
- **Rationale:** `Call` returns exactly `(Outcome, OutputPointer, error)`; Shed persists `OutputPointer.Path` verbatim as an artifact path a human opens, and a non-nil error makes Shed discard the verdict entirely.
- **Applies to:** all batches

### Decision: narrow local seams with compile-time proofs

- **Decision:** each engine that needs one gets a narrow local interface in `shedadapters` with a `var _ Seam = (*concrete)(nil)` proof line; Webster's free-function `Run` gets a func-typed seam with the same style of proof.
  Each adapter type also carries `var _ shedengine.ShedProducer = (*T)(nil)`.
- **Rationale:** matches `burlerengine.Shuttle` and `perchengine.Burler`, and is what makes every adapter tier-1 testable with a fake.
- **Applies to:** batches 1, 2, 3

### Decision: `New...` constructors over unexported fields

- **Decision:** each adapter is built by a `New...` constructor and holds unexported fields.
  `NewPerchProducer` additionally returns an error, because it validates `runIDPrefix` via `perchengine.ValidRunID` before any directory is touched.
- **Rationale:** the adapters wrap live seams a caller already constructed, not a human-configured validated field set — so `shedengine.Shed`'s deliberate no-constructor shape does not apply.
- **Applies to:** batches 1, 2, 3

### Decision: tier-1 fake-driven tests only

- **Decision:** every test file is untagged, in-package (`package shedadapters`), and driven by fakes for the three seams plus `t.TempDir()` for filesystem rows.
  No test drives a real `shedengine.Shed`, real tmux, real git, or a live provider.
- **Rationale:** the Test Tier Purity Invariant, and the fact that `shedengine`'s own routing/pause/persistence tests already prove Shed's loop — re-driving it here would re-test Shed, not the adapters.
- **Applies to:** all batches

### Decision: no new engine surface

- **Decision:** `shuttleengine`, `perchengine`, and `websterengine` are consumed exactly as they ship.
  Webster's three unexported outcome values are matched as local string literals with a `default:` branch that errors on an unrecognised value.
- **Rationale:** needing to widen an engine is a signal the mapping is wrong.
  `parseOutcome` already rejects a fourth value, so only a rename is the live risk — which the `default:` branch turns into a failing test.
- **Applies to:** batches 1, 2, 3

### Decision: the doc set lands in this task

- **Decision:** batch 4 carries the package `doc.go`, five named corrections in `manifest/designs/shed.md`, three edits in `docs/overview.md`, and three in `manifest/roadmap.md`.
  `manifest/designs/shed.md` is **kept**, and its retention is justified explicitly against the Documentation Lifecycle's two-class taxonomy rather than reworded into a fresh rationale — the file is the shared narrative four still-unbuilt modules and one durable reference doc are written against, not a per-module design draft whose module has now landed.
  No new `CONSTRAINTS.md` invariant is expected; if a batch discovers one, it lands in the same commit.
- **Rationale:** `CLAUDE.md`'s same-commit rule, plus the fact that four of the five shed.md claims become false the moment this package ships.
  The explicit retention justification exists because this task deletes the Planned item shed.md's current survival clause rests on, so leaving a reworded clause behind would hand the next reader an unexplained exemption from a lifecycle rule every sibling engine's design doc did follow.
- **Applies to:** batch 4

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `docs/overview.md`
- `internal/shedadapters/archive.go`
- `internal/shedadapters/archive_test.go`
- `internal/shedadapters/ctx.go`
- `internal/shedadapters/ctx_test.go`
- `internal/shedadapters/doc.go`
- `internal/shedadapters/perch.go`
- `internal/shedadapters/perch_test.go`
- `internal/shedadapters/singlellm.go`
- `internal/shedadapters/singlellm_test.go`
- `internal/shedadapters/webster.go`
- `internal/shedadapters/webster_test.go`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
