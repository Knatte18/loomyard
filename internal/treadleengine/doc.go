// Package treadleengine is the generalized round-loop engine perch's
// existing, shipped orchestration loop was extracted out of: it spawns a
// round via a caller-supplied RoundRunner each iteration, gates convergence
// (llm-verdict / command / both), runs an ephemeral progress judge against a
// milestone-capped round ladder, and persists per-round state for
// crash/pause resume — everything internal/perchengine's own round loop did,
// generalized behind a seam so a second consumer (the future Tenter module,
// see manifest/designs/hardener.md) can supply a different round-runner
// without duplicating any of this machinery.
//
// internal/perchengine is treadle's first, reference consumer: a thin
// configuration layer that resolves perch.yaml/profile data, adapts
// burlerengine into the RoundRunner seam below, and delegates to this
// package's Engine.Run — perch's own exported Go API and behavior are
// unchanged from the outside. See internal/perchengine's package doc for the
// full shipped contract every invariant below must continue to satisfy.
//
// # The RoundRunner seam — attempt-level, not round-level
//
// What used to be perch hardwiring "spawn a fresh burlerengine round" is now
// the RoundRunner interface (runner.go): RunAttempt(AttemptInput)
// (AttemptResult, error) runs ONE attempt of one round and reports a
// shuttle-style outcome, a generic Verdict, blocking-findings count, artifact
// paths, and diagnostic identities. The seam is deliberately attempt-level,
// not round-level: Engine itself owns the generic machinery every future
// runner needs for free — the two-attempt retry policy on died/timeout,
// asking-triage (an ephemeral LLM utility call classifying whether a
// question-asking attempt can plausibly retry), stale-artifact move-aside
// before a re-run, round/attempt token naming (roundToken, e.g. "3" then
// "3b" for a retry), artifact path derivation, and the prior-round hydration
// list assembly (collectPriorHydration) fed into every attempt's input. A
// RoundRunner implementation therefore only ever adapts "spawn one attempt,
// report its result" onto its own domain — see internal/perchengine/adapter.go
// for the burlerengine adapter.
//
// # Name-parameterized diagnostics
//
// Engine is constructed with a name (perch passes "perch") that every error
// and Warn string this package produces is prefixed with, so a caller's
// diagnostics read exactly like perch's own literal "perch: "-prefixed
// messages today, and a future caller (e.g. "tenter") gets its own
// consistently-prefixed diagnostics for free rather than a generic
// "treadle: " label that would erase which caller's block failed.
//
// # No burlerengine import — the Treadle Runner-Seam Invariant
//
// This package never imports internal/burlerengine or any internal/*cli
// package (see CONSTRAINTS.md's Treadle Runner-Seam Invariant, enforced by
// seam_enforcement_test.go). It defines its own vocabulary — Verdict,
// AttemptInput/AttemptResult — rather than reusing burlerengine.Verdict/
// Finding; a round-runner adapter (perchengine's) maps its own domain's
// result type onto treadle's. If a type is ever genuinely needed by both
// treadle and a runner, it gets extracted out of burler into shared ground —
// never imported downward.
//
// # Geometry-blindness and weft-blindness
//
// treadleengine never imports internal/hubgeometry and never constructs a
// _lyx path itself: Engine.Run operates on a caller-supplied absolute
// runDir, and a block's Profile carries GateDir — the absolute cwd the gate
// command runs in — supplied by the caller (perchengine resolves it from its
// own *hubgeometry.Layout) rather than resolved by this package. Likewise
// treadleengine never touches weft git; committing a block's run-dir
// artifacts to the weft remains the loop OWNER's job (perchcli today), exactly
// as CONSTRAINTS.md's Weft Git Invariant already requires one layer up.
//
// # Everything else carried over unchanged from perch
//
// The milestone ladder (RoundCaps: every entry but the last is a
// judge-gated milestone rung, the last is the unconditional hard cap), the
// holistic verdict-judge model (per-round circling check plus milestone
// continuation gate, both fail-safe — any judge infrastructure failure
// degrades to progressing/CONTINUE with a Warn, never STUCK, never an
// engine error), the three pluggable GateModes and their convergence
// semantics, the pause seam (checked only at round boundaries, with the
// same pause-flag clearing rules), and run-dir mutual exclusion (run.lock,
// the ErrBlockBusy sentinel) are all generalized machinery moved here
// verbatim from perch's shipped round loop — see internal/perchengine/doc.go
// for the full narrative description of each; this package's job is to
// carry that behavior forward under the RoundRunner seam, not to change it.
package treadleengine
