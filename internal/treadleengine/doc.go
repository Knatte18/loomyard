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
// # Judge-maintained handoff — a bounded judge read-set
//
// The progress judge's per-round read-set was originally unbounded: every
// prior round's review file, growing O(N) as rounds accumulate. Since the
// judge already runs on every blocking round, the SAME call also maintains
// a handoff (handoff.go) — round-<token>-handoff.md, written alongside the
// verdict file in the same shuttle spawn (the handoff-on-disk shared
// decision) — so the NEXT judge call reads {that handoff + the reviews of
// every completed round its covers_rounds does not already absorb} plus the
// current round's fresh review, instead of every prior round's review
// (judgeReadSet). The handoff cannot be a single free-form prose summary:
// the circling check depends on knowing whether a SPECIFIC finding has
// recurred across rounds, so a distilled summary that silently drops a
// recurring finding would break circling-detection quietly — worse than the
// O(N) cost it replaces. It therefore carries two parts: a structured,
// lossless finding-identity Ledger (a stable key, the rounds it has been
// seen in, and an open/resolved Status — every entry from the previous
// handoff must reappear here, never dropped, a rule enforced at prompt
// level, not by this package) plus CoversRounds (which rounds' reviews this
// handoff has already absorbed) over a distilled prose narrative for
// everything else — "distill the prose, but keep the key-ledger lossless."
//
// Parsing is a deliberate two-layer split, mirroring judgeverdict.go:
// ParseHandoff itself is fail-loud — a malformed handoff is an agent defect
// that must be visible as an error to any direct caller, including tests —
// but the round loop that calls it (run.go, via judgeReadSet and
// latestValidHandoff) is fail-safe: a missing or unparseable handoff logs a
// name-prefixed logger.Warn and falls back rather than erroring or forcing
// STUCK. The fallback walks a block's rounds newest-to-oldest for the
// latest handoff that both reads and ParseHandoffs cleanly, so one
// corrupted handoff deterministically degrades to the next older valid one
// instead of taking every future judge call down with it; with no valid
// handoff at all (a fresh block, or every recorded one has failed), the
// read-set degrades to exactly the pre-handoff all-reviews behavior
// (collectJudgeReviews). A round with no judge call at all (round 1, a
// round right after an APPROVED verdict, or a round whose judge call itself
// failed) carries no HandoffPath and so never appears in a later handoff's
// CoversRounds — its review is therefore always fed to some future judge
// call, which is what closes the "judge-gap" hole a bounded read-set would
// otherwise open.
//
// # Pre-round targeting — optional, profile-gated
//
// Profile.PreRoundTargeting (targeting.go) is a third, OPTIONAL ephemeral
// judge framing, run once per round before attempt 1 when a valid handoff
// already exists to target from: it reads that handoff and writes a short
// prose seed brief to that round's AttemptInput.SeedPath, resolved once at
// attempt 1's token and reused unchanged by a same-round retry. Unlike
// runCircling/runMilestone it produces no verdict — only unconstrained
// prose a RoundRunner MAY read or ignore entirely. Perch's own profile
// never sets it (its rounds keep re-using a fixed rubric); the capability
// exists for a future consumer (Tenter, see manifest/designs/hardener.md)
// whose rounds benefit from dynamically retargeted focus. Like every other
// ephemeral call in this package, it is fail-safe end to end: a stencil-fill
// failure, a shuttle Run error, a non-done outcome, or an empty/unreadable
// seed file all degrade to "no seed" with a name-prefixed logger.Warn,
// never an error — a missed targeting call only costs the round the
// guidance it would have added, never correctness.
//
// # Name-parameterized diagnostics
//
// Engine is constructed with a name (perch passes "perch") that every error
// and Warn string this package produces is prefixed with, so a caller's
// diagnostics read exactly like perch's own literal "perch: "-prefixed
// messages today, and a future caller (e.g. "tenter") gets its own
// consistently-prefixed diagnostics for free rather than a generic
// "treadle: " label that would erase which caller's block failed. The
// name parameterization pins the PREFIX only: error BODIES are
// runner-agnostic by design ("round N attempt run", "kept run dir"),
// since this package cannot name a specific runner's domain (burler,
// shuttle) without violating the Runner-Seam Invariant — see
// internal/perchengine's package doc for the perch-visible consequence.
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
