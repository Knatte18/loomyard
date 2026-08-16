# Review: `_mill/discussion.md`

## Method

Every code citation was checked directly against the worktree source: `internal/shedengine/producer.go` and `shed.go`, the full `manifest/designs/shed.md` (all cited line ranges: `:3`, `:29`, `:34-38`, `:73`, `:82-86`, `:231`, `:253-257`, `:259-265`, `:278`), `internal/shuttleengine/spec.go` (`:82-91`, `:115-162`) and `run.go`/`engine.go`, `internal/perchengine/engine.go` (`:26-32`, `:41-67`, `:82`), `identity.go:57-63`, `result.go:16-59`, `internal/perchcli/pause.go` and `run.go:290-300`, `internal/treadleengine/state.go:120-155`, `internal/lock/lock.go:40-52`, `internal/websterengine/outcome.go`, `summary.go`, `runlevel.go` (all cited ranges), `internal/burlerengine/engine.go` (`:20-25`, `:91-95`, `:133-138`), `internal/loomengine/discussion.go` (`:19`, `:43`), `docs/overview.md` (`:226-230`, `:290-296`), `manifest/roadmap.md` (`:10-18`, `:193-201`), and `CONSTRAINTS.md`'s Shed Producer-Seam Invariant.

**Result: 30+ citations verified, all but one exact.** One minor mismatch, noted below.

## Verdict: sound, nothing should block Plan

This is the most complex discussion of the two I've reviewed this week — three distinct engines, a cancellation-semantics design, and a genuinely gnarly run-identity scheme for perch — and it holds up. The Q&A log shows six review rounds (r1–r6) that progressively closed real gaps rather than padding the document, and the gaps they closed are legitimate:

- **r1–r2**: where `StuckReason` goes given `Call`'s three-value return has no detail channel (resolved: log only); how the perch pause bridge is installable given `PauseRequested` is a construction-time field on `perchengine.Options` (verified exact at `engine.go:41-67` — confirmed `New` is the only place it's consumed, so a bare `Run(...)` seam genuinely could not install it); producer identity given `Call(ctx)` carries none (resolved: told `name`); the archive timestamp source (resolved: injected clock, matching webster's own `archiveStaleOutcome`/`ArchiveStaleSummary` signatures, verified exact).
- **r3**: the perch run-dir problem — `treadleengine.loadOrInitState` refuses a terminal run dir outright (verified verbatim at `state.go:126-128`: `"this block already finished (%s)"`), so a fixed `runDir` breaks on the adapter's second `Call`, which is exactly the bounce-back case this adapter exists to serve.
- **r6**: two gaps found *after* r3's fix looked complete — treadle's **second** refusal branch on a profile-hash mismatch (verified verbatim at `state.go:130-132`), which the r3 scheme didn't handle and which would permanently wedge a producer the first time an operator edits `perch.yaml` mid-loop; and whether a completed `Done` should be discarded on a lingering cancellation check, which the doc correctly resolves by letting a genuine success verdict survive (grounded in `shed.md`'s own step-6 routing, verified exact at `:82-86`: an error-with-cancelled-ctx path appends no history entry and leaves `current_producer` unchanged — so converting a finished `Done` into that path would silently force a full re-spawn on the next `Call`).

The scratch-dir-before-probing requirement is correctly derived from `lock.AcquireReadLock` never creating its parent directory (verified exact at `lock.go:44-50` — `flock.New` + `RLock`, no `MkdirAll`) and matches `perchcli/pause.go`'s own comment almost verbatim ("needs the directory to exist before it can even acquire its read lock", verified verbatim at `pause.go:78-79`).

The outcome-mapping decisions across all three adapters apply one coherent principle consistently: a verdict the engine reached (asking / STUCK / stuck) bounces via `Stuck`, while an infrastructure failure the engine couldn't classify as a verdict (died/timeout, an unmapped error) becomes a hard error so Shed re-calls the *same* producer rather than bouncing to an unrelated upstream one. That's a real design invariant, not three independently-argued special cases, and it's stated plainly enough that a future fourth adapter would know which bucket a new outcome belongs in.

The pause-channel decision (ctx only, no adapter reads Shed's status file or takes a second `PauseRequested`) is the right call and — notably — the doc doesn't hide its cost: `lyx perch pause` becomes a silent no-op against an adapter-driven run dir, stated explicitly as an accepted consequence rather than discovered later by an operator.

## One citation mismatch (non-blocking)

Line 314's "Construction" Decision quotes `internal/shedengine/shed.go:6-9` as saying a constructor "would create a second, unvalidated way to build one." The actual text at `shed.go:6-9` reads "there is no New constructor, which would leave a bare struct literal as a second, unvalidated door" — same meaning, different wording. The quoted phrase is verbatim from `shed.md:168` (the design doc), not from the source file cited. Not a design problem — the substance of the rejection (Shed's no-constructor rule is about a human-validated field set, which doesn't apply to a live-seam wrapper) is correct either way — just a citation that points at the wrong of two near-identical sources.

## Non-findings

- The "no new engine surface" constraint is respected throughout — every mapping decision consumes `shuttleengine`/`perchengine`/`websterengine` exactly as shipped; nowhere does the doc quietly propose widening one of them to make an adapter's job easier.
- Webster's outcome values are unexported strings (`outcome.go:24-28`, confirmed exact), so the adapter must hardcode literals — the doc names this duplication explicitly rather than hiding it, and closes the only real risk (a rename, not a new value — `parseOutcome`'s own switch at `:62-66` already rejects a fourth value) with a `default:` branch and a test row.
- `SummaryPath` as the Webster `Done` pointer is correctly justified against `RunResult` carrying no path of its own — confirmed `SummaryPath` at `summary.go:27` and the archive calls it must survive at `runlevel.go:440-446`.

## Overall

Extremely high citation-accuracy rate sustained across a much larger and more architecturally dense document than the last one I reviewed, six real review rounds that found and closed genuine gaps rather than cosmetic ones, and a design (the perch run-identity scheme especially) that correctly anticipates a hazard — the profile-hash mismatch branch — that a less careful pass would have shipped broken. Nothing here should block Plan.
