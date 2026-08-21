// Package treadleengine is the generalized round-loop engine: it spawns a
// round via a caller-supplied RoundRunner each iteration, gates convergence
// (llm-verdict / command / both), runs an ephemeral progress judge against a
// milestone-capped round ladder, and persists per-round state for
// crash/pause resume — all of it behind a seam, so a consumer supplies a
// round-runner without duplicating any of this machinery.
//
// The package has no consumer today. It was extracted out of a shipped
// review-gate loop that has since been retired, and is kept for the future
// Tenter module (see manifest/designs/hardener.md), whose behavior-review
// rounds need exactly this machinery with a different round-runner inside
// it. Nothing in the tree calls Engine.Run outside this package's own tests
// — treat every contract below as the shipped behavior a future consumer
// inherits, not as something a live caller depends on today.
//
// # The RoundRunner seam — attempt-level, not round-level
//
// What used to be a hardwired "spawn a fresh burlerengine round" is now
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
// report its result" onto its own domain.
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
// Riding the same call has one consequence worth naming, since it is a
// behavior change from the pre-handoff design rather than a pure addition:
// the handoff is a REQUIRED second entry in that spawn's OutputFiles, and
// shuttle reports OutcomeDone only once every output file exists. A judge
// call that renders a well-formed verdict and then fails to write its
// handoff therefore comes back non-done, and runJudgeCall takes its
// fail-safe branch with the verdict file unread — where pre-handoff, with
// the verdict as sole output file, that same call would have been honoured.
// The direction is safe (the fallback is PROGRESSING/CONTINUE, never STUCK,
// and the hard cap still bounds the block) and the alternative would mean
// trusting a file after shuttle's own done contract said the agent was not
// finished, so the coupling is deliberate — see runJudgeCall's doc.
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
// prose a RoundRunner MAY read or ignore entirely. A text-review profile
// has no use for it (its rounds keep re-using a fixed rubric); the
// capability exists for a future consumer (Tenter, see
// manifest/designs/hardener.md) whose rounds benefit from dynamically
// retargeted focus. Like every other
// ephemeral call in this package, it is fail-safe end to end: a stencil-fill
// failure, a shuttle Run error, a non-done outcome, or an empty/unreadable
// seed file all degrade to "no seed" with a name-prefixed logger.Warn,
// never an error — a missed targeting call only costs the round the
// guidance it would have added, never correctness.
//
// # Name-parameterized diagnostics
//
// Engine is constructed with a name that every error and Warn string
// reached through an Engine method is prefixed with, so each caller (e.g.
// "tenter") gets its own consistently-prefixed diagnostics for free rather
// than a generic "treadle: " label that would erase which caller's block
// failed. That
// covers every Warn this package emits without exception — including the
// fail-safe handoff-fallback Warns in latestValidHandoff, which reach an
// operator's stderr at logger's default threshold during an ordinary run and
// so must never wear a module name the operator has no CLI for.
//
// Two carve-outs, both deliberate. First, the name parameterization pins the
// PREFIX only: error BODIES are runner-agnostic by design ("round N attempt
// run", "kept run dir"), since this package cannot name a specific runner's
// domain (burler, shuttle) without violating the Runner-Seam Invariant.
// Second, the package's EXPORTED fail-loud parsers — ParseJudgeVerdict,
// ParseTriageVerdict, ParseHandoff, and the splitFrontmatter they share —
// are package-level pure functions with no Engine in scope, so their errors
// keep a fixed "treadle: " prefix. Those strings are never returned to a
// caller as an engine error; they surface only as the cause= field of a
// name-prefixed Warn (judge.go, run.go) or to a direct caller such as a
// test, where "treadle: " is the accurate attribution.
//
// # No burlerengine import — the Treadle Runner-Seam Invariant
//
// This package never imports internal/burlerengine or any internal/*cli
// package (see CONSTRAINTS.md's Treadle Runner-Seam Invariant, enforced by
// seam_enforcement_test.go). It defines its own vocabulary — Verdict,
// AttemptInput/AttemptResult — rather than reusing burlerengine.Verdict/
// Finding; a round-runner adapter maps its own domain's
// result type onto treadle's. If a type is ever genuinely needed by both
// treadle and a runner, it gets extracted out of burler into shared ground —
// never imported downward.
//
// # Geometry-blindness and fabric-blindness
//
// treadleengine never imports internal/lyxcwd directly — the ban is a
// discipline against deriving geometry, not an isolation guarantee, since
// the package's own allowlist permits internal/logger and
// internal/shuttleengine, each of which imports lyxcwd directly — and
// never constructs a _lyx path itself: Engine.Run operates on a
// caller-supplied absolute runDir, and a block's Profile carries GateDir —
// the absolute cwd the gate command runs in — supplied by the caller
// (which resolves it from its own geometry) rather than resolved by this
// package. Likewise treadleengine never touches fabric git; committing a
// block's run-dir artifacts to fabric remains the loop OWNER's job, exactly
// as CONSTRAINTS.md's Fabric Git Invariant already requires one layer up.
//
// # Everything else carried over unchanged from the original loop
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
// verbatim from the original shipped round loop; this package's job is to
// carry that behavior forward under the RoundRunner seam, not to change it.
package treadleengine
