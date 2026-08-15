# Batch: seam-invariant

```yaml
task: 'Shed: outer phase-FSM skeleton'
batch: seam-invariant
number: 5
cards: 2
verify: go test ./internal/shedengine/...
depends-on: [2]
```

## Batch Scope

This batch adds the new **Shed Producer-Seam Invariant** — the machine check that keeps `internal/shedengine`'s told-never-derived property from eroding silently — and records it in `CONSTRAINTS.md`.
It is one batch because the test and the constraint entry are two halves of one thing: an invariant with no enforcement test rots, and an enforcement test with no written invariant is an unexplained allowlist.

It depends on batch 2 because the test walks the package's non-test source and must see every production file the task creates.
It exposes no interface to a later batch.

Batch-local decision, beyond `## Shared Decisions` in the overview: the allowlist is exactly stdlib plus `internal/state` and `internal/lock`, and `internal/logger` is deliberately **not** on it.
Nothing in this package logs — it starts no OS process, so the Live-Substrate Spawn Observability invariant does not engage, and the product CLI owns operator-facing output.

## Cards

### Card 23: the seam enforcement test

- **Context:**
  - `_mill/discussion.md`
  - `internal/treadleengine/seam_enforcement_test.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/validate.go`
  - `internal/shedengine/activity.go`
  - `internal/shedengine/errors.go`
  - `internal/shedengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedengine/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/shedengine/seam_enforcement_test.go` in `package shedengine`, following `internal/treadleengine/seam_enforcement_test.go`'s structure exactly: locate the package directory from the test file's own path, walk it, skip directories and any file that is not a non-test `.go` file, parse each remaining file for imports only, classify an import whose first path segment contains no dot as stdlib, and fail on anything that is neither stdlib nor an allowlist entry.

  Name the test function `TestProducerSeamInvariant_AllowlistOnly` and the allowlist map so it reads as this package's own rather than a copy.
  The allowlist has exactly two entries: the `internal/state` and `internal/lock` package import paths.
  The failure message names the invariant and lists every offending file and import, as treadle's does.

  Write the file-level comment as the invariant's own statement, in treadle's shape: production code in this package imports only the standard library, `internal/state`, and `internal/lock` — never `internal/loomengine`, never any adapter package, never `internal/lyxcwd`, and never `internal/logger`.
  State that this is an allowlist rather than a banned list, so a future stray dependency is caught with no list maintenance.
  Then state the one observation worth recording: with this particular allowlist the exclusion of `internal/lyxcwd` happens to hold **transitively**, not merely on direct imports, because `internal/lock` imports no internal package at all and `internal/state` imports only `internal/fsx` and `internal/lock`.
  Mark that as an observation about today's allowlist, not as something this test enforces — the test checks direct imports only.
  Do not copy treadle's own "excluding it buys no isolation" caveat: for this allowlist it would be false.
- **Commit:** `test(shedengine): enforce the Shed Producer-Seam Invariant`

### Card 24: record the invariant in CONSTRAINTS.md

- **Context:**
  - `_mill/discussion.md`
  - `internal/shedengine/seam_enforcement_test.go`
  - `internal/shedengine/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new `## Shed Producer-Seam Invariant` section to `CONSTRAINTS.md`, placed immediately after the existing `## Treadle Runner-Seam Invariant` section and immediately before `## Tokenvocab Leaf Invariant`, and written in the same shape as its neighbours: a lead sentence stating the rule, then bullets for the allowlist and the enforcement.

  The content:
  `internal/shedengine` production code imports only the standard library, `internal/state`, and `internal/lock`;
  producers adapt onto the package's own `ShedProducer` seam in their own packages.
  Name the excluded packages explicitly — `internal/loomengine`, any engine adapter package, `internal/lyxcwd`, and `internal/logger`.
  State what the exclusion of `internal/lyxcwd` actually enforces, in the same terms the Treadle entry uses for itself: that `Shed` is *told* its geometry and never derives it — `StatusPath`, `LockPath`, and `StatusLockPath` are all caller-supplied, and the only paths the package constructs are the two lock parents it creates so a told path is usable.
  State that policing is on direct imports only, matching what the test checks, and then note the stronger fact this particular allowlist happens to buy: `internal/lyxcwd` is excluded transitively too, because `internal/lock` imports no internal package and `internal/state` imports only `internal/fsx` and `internal/lock`.
  Explain why `internal/logger` is excluded rather than kept for future convenience: nothing in the package logs, the package starts no OS process so the Live-Substrate Spawn Observability invariant does not engage, and keeping it would forfeit the transitive property above for zero present benefit — `internal/logger` itself imports `internal/lyxcwd`.
  Close with the `- **Enforced by**` bullet naming `internal/shedengine/seam_enforcement_test.go` and its test function, in the same format every other entry uses.

  Follow the repo's semantic-line-break rule throughout: one sentence per line, with an additional break at internal independent-clause boundaries, and no fixed-column hard wrap.
  Do not renumber, reorder, or reword any other section.
- **Commit:** `docs(constraints): record the Shed Producer-Seam Invariant`

## Batch Tests

`verify: go test ./internal/shedengine/...` runs the new `internal/shedengine/seam_enforcement_test.go` alongside every test batches 1 through 4 established.
The scope is exactly right for this batch: the seam test is the only runnable surface it adds, it lives in that package, and the `CONSTRAINTS.md` edit has no runnable surface of its own — the test *is* the enforcement the doc entry points at, so running the package proves the pair is consistent.

Nothing outside `internal/shedengine` changes here, so nothing outside it is run.
The two scoped `cmd/lyx` tier guards are deliberately absent from this batch's verify: the file this batch adds is a test file, but it spawns nothing, sleeps not at all, and touches no fixture — and the task-wide done gate runs the whole suite regardless.
