# Batch: shedadapters-approve-seam

```yaml
task: 'loom: Plan-Write/Plan-Validate approval deadlock (F7)'
batch: 'shedadapters-approve-seam'
number: 2
cards: 2
verify: go test ./internal/shedadapters/...
depends-on: []
```

## Batch Scope

This batch adds one optional injected closure to the generic review-gate producer: `BouncerConfig.Approve`, called on `settle`'s approved branch immediately before `Commit`.
It is deliberately independent of batch 1 — the seam is an opaque `func() error` and the `Bouncer` gains no plan-specific knowledge whatsoever, so nothing here imports `planparser` or knows an approval flag exists.

The external interface batch 4 consumes is the new `Approve` field on `shedadapters.BouncerConfig`.
Nil stays the absent value and means "approve nothing", which is what keeps every existing `Bouncer` construction — the `Discussion-Bouncer` and `Webster-Bouncer` rows and every existing test — valid unchanged.

Batch-local decision: `Approve` mirrors `Commit`'s shipped shape exactly, including its error disposition.
A non-nil error from `Approve` is returned as `settle`'s own error rather than routed through `degrade`, for the identical reason the existing `Commit` branch already carries in its own comment: `degrade` only ever returns `shedengine.Stuck`, so routing an I/O fault through it would silently convert an approval into a rejection.
`Approve` runs first and `Commit` is not attempted when it fails, so the only reachable inconsistency is a flag written in the working tree but not yet in git — the benign direction.

## Cards

### Card 5: Add the Approve seam to BouncerConfig and settle

- **Context:** none
- **Edits:**
  - `internal/shedadapters/bouncer.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an `Approve func() error` field to `BouncerConfig`, placed immediately before the existing `Commit` field so the two seams read as the pair they are.
  Its doc comment matches `Commit`'s phrasing shape: it is the injected closure the loop owner marks the reviewed artifacts approved through, called on the approved branch of `settle` before `Commit`, with nil the absent value meaning "approve nothing", which is what keeps a segment with no seam configured behaving exactly as before.
  Do not add a validation branch for it in `NewBouncer` — nil is permitted, so there is nothing to validate.
  In `settle`'s `case verdictApproved:` branch, call `b.cfg.Approve` when it is non-nil, before the existing `b.cfg.Commit` call, and on a non-nil error return it as `settle`'s own error in the same shape the `Commit` failure already uses — an empty `shedengine.Outcome`, an empty `shedengine.OutputPointer`, and a wrapped error naming the producer, the engine label, and the failing step.
  When `Approve` fails, do not call `Commit`.
  Extend the existing paragraph in `settle`'s own doc comment that explains why a `Commit` failure is not routed through `degrade` so that it covers `Approve` as well, rather than duplicating that paragraph — the reasoning is identical for both seams.
  State in that same doc comment that `Approve` runs before `Commit` and that a failing `Approve` skips `Commit`.
  The approved branch's existing contract that it performs its side effects even under an already-cancelled context is unchanged, and `Approve` inherits it: do not add a cancellation check of its own.
- **Commit:** `5: shedadapters: add the Bouncer Approve seam`

### Card 6: Cover the Approve seam, including its ordering against Commit

- **Context:**
  - `internal/shedadapters/bouncer.go`
  - `internal/shedadapters/bouncer_seed_test.go`
  - `internal/shedadapters/bouncer_judge_test.go`
- **Edits:**
  - `internal/shedadapters/bouncer_commit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing `Commit`-seam test file rather than inventing a new harness — every case below builds on the same `testBouncerConfig`, `judgeFakeShuttle`, `layoutBouncerRun`, `bouncerVerdictContent`, `bouncerLedgerContent`, and `bouncerReport` helpers the file's existing cases already use.
  Those six helpers are declared in two sibling files in the same package rather than in the file being edited: `testBouncerConfig` in `internal/shedadapters/bouncer_seed_test.go`, and the other five in `internal/shedadapters/bouncer_judge_test.go`.
  Both files are read-only context for this card — reuse them as they are and change neither.
  Widen the file-level comment so it names both seams rather than `Commit` alone.
  Add four cases.
  First: an APPROVED settle with a non-nil `Approve` calls it exactly once and calls it strictly before `Commit` — assert the ordering with one shared call-log slice both closures append a marker string to, never with two independent booleans, since two booleans cannot distinguish the two orderings.
  Second: a nil `Approve` on an APPROVED settle still commits normally, returns `shedengine.Done`, and is not an error.
  Third: an `Approve` returning a sentinel error makes `Call` return an error wrapping that sentinel, with `shedengine.Done` not returned and the `Commit` closure never invoked — assert the commit call count is zero.
  Fourth: a BLOCKING settle never calls `Approve`, mirroring the existing `TestBouncer_Commit_BlockingNeverCalls` case's shape.
- **Commit:** `6: shedadapters: test the Bouncer Approve seam`

## Batch Tests

`verify: go test ./internal/shedadapters/...` runs the whole package suite.
That scope is right rather than narrower: card 5 edits `bouncer.go`, which every `Bouncer` test in the package exercises, so the existing `bouncer_test.go` and `burler_test.go` cases are the regression surface for the nil-`Approve` default path and must run alongside the four new cases in `bouncer_commit_test.go`.
