// Package shedadapters holds the four shedengine.ShedProducer adapters that let a Shed-built
// product drive shuttle, perch, Webster, and one burlerengine round as ordinary producers in its
// own flat producer list.
// SingleLLMProducer wraps one shuttleengine run, PerchProducer wraps one perchengine block,
// WebsterProducer wraps one websterengine multi-spawn run, and BurlerProducer wraps one
// burlerengine A-review/B-fix round as a single Shed row; each is a thin translation layer over an
// already-shipped engine, never a second implementation of that engine's own loop.
//
// # Outcome mapping
//
// Each adapter maps its wrapped engine's own verdict onto shedengine's two-value Outcome contract,
// Done or Stuck, and reports the output pointer differently because the four engines report success
// differently:
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
//   - BurlerProducer: a completed round -- shuttleengine.OutcomeDone reached within the bounded
//     retry -- maps to Stuck, never Done, reporting the round's own review path as the pointer.
//     That Stuck is a routine hand-off to the segment's Bouncer via OnStuck, never a real stuck
//     condition: a round producer has no independent notion of "finished," only the judge does.
//     Every non-done shuttle outcome that survives the bounded retry -- OutcomeAsking, two
//     consecutive OutcomeDied/OutcomeTimeout results, or an unrecognized outcome -- is an
//     engine-level error, not Stuck, because the Bouncer tells its seed call from its judge call by
//     the round artifacts on disk, and a failed round returning Stuck with no review written would
//     be misread as a seed call.
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
// BurlerProducer is told an absolute run directory and an already-constructed runner, and takes the
// same injected clock SingleLLMProducer does, resolving only the archive filename's same-second
// collision suffix the same way.
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
// # The round-artifact convention
//
// This is the binding two-sided contract between BurlerProducer and its segment's Bouncer, pinned
// here durably rather than only in manifest/roadmap.md, so it survives independently of the roadmap
// entry that is deleted when the Bouncer item completes.
// Artifact paths are flat inside the told run directory, one canonical pair per round with no
// attempt suffix: round-<N>-review.md and round-<N>-fixer-report.md, with N a positive decimal
// integer carrying no leading zeros.
// A retry writes to the same two paths, because a retry is a second try at the one artifact the
// round owes rather than a second artifact.
// The presence of both files means, and only means, that round N completed and produced a usable
// review; the round producer uses exactly that pair predicate to decide whether to advance, and the
// Bouncer uses exactly that pair predicate to tell its seed call from its judge call.
// The structured next-round directive is JSON at round-<N>-focus.json beside them, whose token names
// the round the directives are for, not the round that produced them: a Bouncer rejecting round N
// writes the file for round N+1, and the seed call writes the file for round 1.
// Reading that file is fail-safe end to end, degrading to "no directive" with a warning rather than
// erroring, including at application time when a well-formed directive cannot be honoured.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict -- shuttle's
// OutcomeDone, perch's OutcomeApproved, or Webster's "done" -- which is returned as shedengine.Done
// with its pointer regardless of cancellation.
// This exception exists because converting a finished success into the context error would make Shed
// record no history entry for it, so the next Call would archive a valid artifact and pay for the
// same LLM session twice; a finished artifact and a paid-for session are never discarded.
// Only PerchProducer installs a mid-run bridge (a pauseRequested callback built fresh over each
// Call's own context and passed into its PerchFactory), because perch's pause seam is the only one
// of the three shaped as a caller-supplied callback.
//
// BurlerProducer does not need this section's success exception at all -- not that it reads the
// exception differently.
// internal/shedengine/producer.go binds every implementation to surface cancellation as a non-nil
// error and never as Stuck, so this producer always errors under cancellation, including on a
// completed round.
// The exception's purpose -- never discarding a paid-for artifact -- is served instead by an archive
// carve-out: a completed-then-cancelled round keeps its two files, so from-disk round resolution
// advances past it on the next call, and only the re-derivable in-memory verdict is dropped.
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
// BurlerProducer likewise installs no mid-run cancellation bridge, because burlerengine exposes no
// pause seam, so a cancel is observed only once the round reaches a terminal outcome or its own
// RunOpts.Timeout elapses.
//
// The standalone perch pause verb (`lyx perch pause --run-id <id>`) is a silent no-op against a run
// dir PerchProducer is driving: the pause callback the adapter installs is the context-cancellation
// bridge, not the CLI's own flag-file closure the verb writes to, so the verb writes a flag nothing
// reads and reports success regardless.
// The remedy is never that verb against an adapter-driven run -- it is the product's own pause path,
// cancelling the context handed to Shed's run loop.
package shedadapters
