// Package treadleengine is a round-loop engine.
// Each iteration it spawns a round through a caller-supplied RoundRunner and gates convergence.
// The gate is an LLM verdict, a command, or both.
// It runs an ephemeral progress judge against a milestone-capped round ladder.
// It persists per-round state for crash and pause resume.
// A consumer supplies a RoundRunner without duplicating any of this machinery.
//
// The package has no consumer yet.
// It is kept for the future Tenter module (see the board's `hardener` note).
// Its behaviour-review rounds need this machinery with a different RoundRunner inside it.
// Nothing in the tree calls Engine.Run outside this package's own tests.
// Each contract below is behavior a future consumer inherits, not one a live caller depends on.
//
// # Text review and behaviour review
//
// A text-review round reads an artifact.
// A behaviour-review round runs a live-substrate module and reacts to what it observes.
// It builds adversarial scenarios to break the module.
// The two share the review-round discipline (A-review before B-fix, no self-grading, commit-per-fix, fix everything).
// They differ along every other axis.
// A text round runs in the worktree and is cheap.
// It is gated by an LLM verdict or a light command and sits between phases on the spine.
// A behaviour round needs a live sandbox repo with slow git and go operations.
// It is gated deterministically (the smoke suite run N times concurrently, zero stray state).
// It costs hours per iteration and runs on demand after loom.
// This engine serves both through the RoundRunner seam; only the text profile exists.
//
// # Why every round respawns
//
// Every round is a fresh one-shot spawn, so no context window carries state between rounds.
// After each blocking round a progress judge independently validates the findings and rewrites the handoff in place.
// It also decides whether another round is needed.
// The independent validation is required: a round's own "merge-ready" verdict is not trusted.
// A judge may also run before the round, reading the handoff and writing the round's seed prompt.
// Pre-round targeting below covers it.
// The handoff is the only accumulation vehicle, so what a live thread would know implicitly must be explicit in it.
// The prime case is the finding-recurrence ledger, which records which findings reappeared in which rounds.
// The handoff's prose is distilled, but the ledger stays lossless:
// an in-place edit that tidies a finding that looks resolved silently disables stuck detection.
//
// # The RoundRunner seam: attempt-level, not round-level
//
// The RoundRunner interface (runner.go) has one method, RunAttempt(AttemptInput) (AttemptResult, error).
// It runs one attempt of one round.
// It reports a shuttle-style outcome, a generic Verdict and the blocking-findings count.
// It also reports artifact paths and diagnostic identities.
// The seam is attempt-level because Engine owns the machinery every runner needs:
// the two-attempt retry policy on died or timeout;
// stale-artifact move-aside before a re-run;
// round and attempt token naming (roundToken, e.g. "3" then "3b" for a retry);
// artifact path derivation;
// and the prior-round hydration list assembly (collectPriorHydration) fed into every attempt's input.
// A RoundRunner implementation therefore only adapts "spawn one attempt, report its result" onto its own domain.
//
// # Judge-maintained handoff: a bounded judge read-set
//
// The progress judge runs on every blocking round, and the same call also maintains a handoff (handoff.go).
// The handoff is round-<token>-handoff.md, written alongside the verdict file in the same shuttle spawn.
// The next judge call reads that handoff and the current round's fresh review.
// It also reads the review of every completed round that the handoff's covers_rounds has not absorbed (judgeReadSet).
// The read-set stays bounded instead of growing with every prior round's review.
// The handoff cannot be a single free-form prose summary.
// The circling check depends on knowing whether a specific finding has recurred across rounds,
// and a distilled summary that silently drops a recurring finding would break circling detection quietly.
// The handoff therefore carries a lossless, structured Ledger of finding identities.
// Each Ledger entry has a stable key, the rounds it has been seen in, and an open/resolved Status.
// CoversRounds records which rounds' reviews the handoff has already absorbed.
// A distilled prose narrative holds everything else.
// Every entry from the previous handoff must reappear in the Ledger; the prompt enforces this and this package does not.
//
// The handoff is a required second entry in that spawn's OutputFiles.
// Shuttle reports OutcomeDone only once every output file exists.
// A judge call that renders a well-formed verdict and then fails to write its handoff comes back non-done.
// runJudgeCall then takes its fail-safe branch with the verdict file unread.
// The fallback is PROGRESSING/CONTINUE, never STUCK, and the hard cap still bounds the block.
//
// Parsing is two-layered, as in judgeverdict.go.
// ParseHandoff is fail-loud:
// a malformed handoff is an agent defect that must be visible as an error to any direct caller, including tests.
// The round loop that calls it (run.go, via judgeReadSet and latestValidHandoff) is fail-safe.
// A missing or unparseable handoff logs a name-prefixed logger.Warn and falls back.
// It neither errors nor forces STUCK.
// The fallback walks a block's rounds newest-to-oldest for the latest handoff that both reads and ParseHandoffs cleanly.
// One corrupted handoff therefore degrades to the next older valid one.
// It does not take every later judge call down with it.
// With no valid handoff at all, the read-set is every round's review (collectJudgeReviews).
// That is the case for a fresh block and for a block whose recorded handoffs all failed.
// A round with no judge call carries no HandoffPath and never appears in a later handoff's CoversRounds.
// Such a round is round 1, a round right after an APPROVED verdict, or a round whose judge call failed.
// Its review is always fed to some later judge call, which closes the judge-gap hole a bounded read-set would open.
//
// # Pre-round targeting: optional, profile-gated
//
// Profile.PreRoundTargeting (targeting.go) enables an optional ephemeral judge framing.
// It runs once per round before attempt 1, when a valid handoff already exists to target from.
// It reads that handoff and writes a short prose seed brief to that round's AttemptInput.SeedPath.
// The path is resolved once at attempt 1's token and reused unchanged by a same-round retry.
// It produces no verdict, only unconstrained prose a RoundRunner may read or ignore.
// A text-review profile has no use for it, since its rounds reuse a fixed rubric.
// It exists for a future consumer whose rounds benefit from retargeted focus.
// That consumer is Tenter (see the board's `hardener` note).
// Like every other ephemeral call in this package, it is fail-safe end to end.
// These failures degrade to "no seed" with a name-prefixed logger.Warn, never an error:
// a stencil-fill failure, a shuttle Run error, a non-done outcome, and an empty or unreadable seed file.
// A missed targeting call costs the round only the guidance it would have added.
//
// # Model choices
//
// Profile.Judge and Profile.Attempt are the two shuttleengine.ModelChoices, engine included.
// Judge goes on the circling, milestone and targeting spawns.
// Attempt reaches each attempt as AttemptInput.Choice.
// Treadle declares no requirement on either spawn.
//
// # Skills and the parent directive
//
// The spawned roles (judge, targeting) each load `scribe:prose` through the spec's Skills.
// The skills load before the prompt is delivered.
// Each opening stencil carries `{{.parent_directive}}` and `{{.edit_directive}}` near its top.
// `{{.parent_directive}}` is filled with `parentdirective.Directive` from the told Options.ParentName.
// `{{.edit_directive}}` is filled with `editdirective.Directive`.
// An empty ParentName renders the no-parent variant;
// a directive that cannot be rendered takes the same fail-safe path as an unreadable stencil.
//
// # Name-parameterized diagnostics
//
// Engine is constructed with a name that prefixes every error and Warn string reached through an Engine method.
// Each caller (e.g. "tenter") gets its own diagnostics, so an operator can tell which caller's block failed.
// The prefix covers every Warn this package emits, including the handoff-fallback Warns in latestValidHandoff.
// Those reach an operator's stderr at logger's default threshold during an ordinary run,
// so they must never carry a module name the operator has no CLI for.
//
// Two carve-outs apply.
// The name pins the prefix only: error bodies are runner-agnostic ("round N attempt run", "kept run dir").
// This package cannot name a specific runner's domain (burler, shuttle) without violating the runner-seam invariant.
// The exported fail-loud parsers are ParseJudgeVerdict and ParseHandoff.
// They and the splitFrontmatter they share are package-level pure functions with no Engine in scope.
// Their errors therefore keep a fixed "treadle: " prefix.
// Those strings never reach a caller as an engine error.
// They surface only as the cause= field of a name-prefixed Warn (judge.go, run.go) or to a direct caller such as a test.
//
// # No burlerengine import: the runner-seam invariant
//
// This package imports only the allowlist seam_enforcement_test.go holds.
// It never imports internal/burlerengine, internal/lymcwd or any internal/*cli package; the test enforces it.
// It defines its own vocabulary, Verdict and AttemptInput/AttemptResult.
// It does not reuse burlerengine.Verdict or burlerengine.Finding.
// A round-runner adapter maps its own domain's result type onto treadle's.
// A type needed by both treadle and a runner gets extracted out of burler into shared ground, never imported downward.
//
// # Geometry-blindness and fabric-blindness
//
// treadleengine never imports internal/lymcwd directly, and it never constructs a _lym path itself.
// The ban is a discipline against deriving geometry, not an isolation guarantee:
// the allowlist permits internal/logger and internal/shuttleengine, each of which imports lymcwd directly.
// Engine.Run operates on a caller-supplied absolute runDir.
// A block's Profile carries GateDir, the absolute cwd the gate command runs in.
// The caller resolves GateDir from its own geometry.
// treadleengine also never touches fabric git.
// Committing a block's run-dir artifacts to fabric is the loop owner's job.
// PATTERN-fabric-boundary requires this one layer up.
//
// # Round-loop machinery
//
// The milestone ladder (RoundCaps): every entry but the last is a judge-gated milestone rung.
// The last entry is the hard cap.
// The verdict-judge model: a per-round circling check plus a milestone continuation gate, both fail-safe.
// Any judge infrastructure failure degrades to progressing/CONTINUE with a Warn, never STUCK or an engine error.
// The pluggable GateModes and their convergence semantics.
// The pause seam, checked only at round boundaries.
// Run-dir mutual exclusion (run.lock, the ErrBlockBusy sentinel).
package treadleengine
