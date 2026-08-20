// Package shedadapters holds the four shedengine.ShedProducer adapters that let a Shed-built
// product drive shuttle, perch, Webster, and the generic review-gate Bouncer as ordinary producers
// in its own flat producer list.
// SingleLLMProducer wraps one shuttleengine run, PerchProducer wraps one perchengine block, and
// WebsterProducer wraps one websterengine multi-spawn run: each of these three is a thin
// translation layer over an already-shipped engine, never a second implementation of that engine's
// own loop. Bouncer is the one member of this package for which that is false: it is new logic
// over shuttleengine, composing its own prompt from stencils rather than translating an
// already-shipped engine's loop.
//
// # Outcome mapping
//
// Each adapter maps its own verdict onto shedengine's two-value Outcome contract, Done or Stuck,
// and reports the output pointer differently because the four adapters report success differently:
//
//   - SingleLLMProducer: shuttleengine.OutcomeDone maps to Done, reporting the first entry of the
//     evaluated Spec's OutputFiles as the pointer's path.
//     OutcomeAsking maps to Stuck with an empty pointer.
//     OutcomeDied and OutcomeTimeout are engine-level errors, not Stuck.
//   - PerchProducer: perchengine.OutcomeApproved maps to Done, reporting an empty output pointer
//     because a gate producer's verdict is always re-derived, never read back from a file.
//     OutcomeStuck maps to Stuck with an empty pointer.
//     OutcomePaused reaching Call out of band is an engine-level error.
//   - WebsterProducer: Webster's own "done" outcome maps to Done, reporting Webster's summary path
//     (websterengine.SummaryPath) as the pointer's path.
//     Webster's own "stuck" outcome, and a websterengine.ErrMasterAsking error, both map to Stuck
//     with an empty pointer.
//     Webster's own "paused" outcome reaching Call out of band is an engine-level error.
//   - Bouncer: Call resolves into one of four modes -- seed, re-bounce, judge, or replay -- and its
//     harvest step acts on a judgment that provably happened (a verdict and ledger that both exist
//     and parse) regardless of what the shuttle run itself reported. A parsed APPROVED verdict maps
//     to Done, and a parsed BLOCKING verdict maps to Stuck, both reporting the round's ledger path
//     as the pointer; every other path -- the seed call, the re-bounce, every degraded path --
//     reports an empty pointer. This is a deliberate delta versus PerchProducer, which reports an
//     empty pointer because a gate producer's verdict is re-derived rather than read back: the
//     Bouncer's ledger is a real cross-round artifact a human reads, and hiding it on a BLOCKING
//     Stuck would hide it exactly when an operator most needs it. The exists-or-empty rule matters
//     because Shed never stats a pointer, so a pointer naming an unwritten file is caught nowhere
//     and is simply persisted into the history for a human to read as though the artifact were
//     there.
//
// # Told, never derived
//
// Every constructor receives already-resolved absolute paths and already-constructed engines (or a
// factory over them); no adapter calls lyxcwd, os.Getwd, or git, and no adapter writes the literals
// _lyx or .lyx.
// Each New... constructor also takes a name string, used only as a log field and in error text --
// never compared, parsed, or used for control flow -- because Call(ctx) carries no identity of its
// own, and two instances of the same adapter type in one producer list is the expected shape.
// SingleLLMProducer additionally takes an injected clock, a nil now defaulting to the real
// time.Now; the injected clock resolves only the archive filename's same-second collision suffix,
// never Shed's own history[].at field.
// Bouncer's own told inputs are RunDir, StencilsDir, the resolved (Model, Effort, Version) triple,
// and the report-name convention as a function. NewBouncer is the second validating,
// error-returning constructor in the package, after NewPerchProducer, in contrast with the two
// that return a bare pointer: NewSingleLLMProducer, whose Spec is validated downstream by
// shuttleengine, and NewWebsterProducer. NewBouncer takes the error-returning shape because
// BouncerConfig carries eleven inputs with real invariants, two of which must be absolute paths,
// and validating lazily at first Call would turn a wiring typo into a mid-run failure in an
// unattended segment.
//
// # The perch run-id scheme
//
// PerchProducer resolves its own run identity from disk each Call rather than holding an attempt
// number in memory, so a process restart resolves the same attempt.
// The id has the shape <prefix>-<hash8>-<N>: hash8 is the first 8 hex characters of the profile's
// content hash, namespacing runs by profile content so an operator's mid-loop edit to the profile
// never collides with, or gets refused by, an older attempt's recorded hash -- an unnamespaced N
// would otherwise let a resumed block wedge permanently against a state.json recorded under a since-
// edited profile.
// N advances to N+1 only when the block currently at N is terminal; a non-terminal block (including
// one that has never started) is reused verbatim, so perch's own in-flight crash-resume survives
// unchanged.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict -- shuttle's
// OutcomeDone, perch's OutcomeApproved, Webster's "done", or the Bouncer's own harvested verdict --
// which is returned as its mapped shedengine.Done or shedengine.Stuck with its pointer regardless
// of cancellation.
// This exception exists because converting a finished success into the context error would make Shed
// record no history entry for it, so the next Call would archive a valid artifact and pay for the
// same LLM session twice; a finished artifact and a paid-for session are never discarded.
// For the Bouncer, the genuine-success exception covers a *harvested* verdict, not only a
// shuttleengine completion: a verdict and ledger that both exist and parse are returned as their
// mapped outcome and pointer even when the run that produced them reported an error, a non-done
// outcome, or arrived after the context was cancelled.
// Only PerchProducer installs a mid-run bridge (a pauseRequested callback built fresh over each
// Call's own context and passed into its PerchFactory), because perch's pause seam is the only one
// of the four shaped as a caller-supplied callback.
//
// # Limitations
//
// SingleLLMProducer never reattaches to a live shuttle session: on a stale output file it archives
// the file and respawns a fresh shuttle run, because shuttleengine exposes no reattach entry point
// for the adapter to call.
//
// Neither SingleLLMProducer nor WebsterProducer installs a mid-run cancellation bridge, so a cancel
// is observed only once the run reaches a terminal outcome or its own configured deadline elapses --
// bounded by the shuttle spec's own timeout for SingleLLMProducer, and by Webster's own whole-run
// timeout for WebsterProducer.
//
// The standalone perch pause verb (`lyx perch pause --run-id <id>`) is a silent no-op against a run
// dir PerchProducer is driving: the pause callback the adapter installs is the context-cancellation
// bridge, not the CLI's own flag-file closure the verb writes to, so the verb writes a flag nothing
// reads and reports success regardless.
// The remedy is never that verb against an adapter-driven run -- it is the product's own pause path,
// cancelling the context handed to Shed's run loop.
//
// The Bouncer accepts two further soft spots. Ledger carry-forward is enforced by the judge prompt
// alone, so a misbehaving judge can drop an entry with nothing at the Go layer catching it; closing
// that would require diffing the new ledger's key set against the previous one and deciding what a
// missing key means, which is a feature rather than a one-line addition. The Bouncer also installs
// no mid-run cancellation bridge, the same limitation SingleLLMProducer and WebsterProducer already
// record above.
package shedadapters
