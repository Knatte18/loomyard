// Package shedadapters holds the four shedengine.ShedProducer adapters that let a Shed-built
// product drive shuttle, Webster, one burlerengine round, and the generic review-gate
// Bouncer as ordinary producers in its own flat producer list.
// SingleLLMProducer wraps one shuttleengine run, WebsterProducer wraps one websterengine
// multi-spawn run, and BurlerProducer wraps one burlerengine A-review/B-fix round as a single Shed
// row: each of these three is a thin translation layer over an already-shipped engine, never a
// second implementation of that engine's own loop.
// Bouncer is the one member of this package for which that is false: it is new logic over
// shuttleengine, composing its own prompt from stencils rather than translating an already-shipped
// engine's loop.
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
//     Before any of this, SingleLLMProducer.Call probes shuttleengine's Attach seam for a still-live,
//     never-terminated run matching the evaluated Spec -- on every call and regardless of mode, not
//     only when spec.Interactive is set. Archiving renames the very files a live agent may be about
//     to write, so the probe runs before archiving anything: a found run's Result is mapped through
//     this identical outcome switch, and a not-found probe falls through to the unchanged
//     archive-then-run path. The probe applies to the PlanWrite and generic SingleLLM rows too, not
//     only DiscussionWrite.
//     A caller's own destructive preparation for a fresh agent -- rotating a stale output directory
//     aside, say -- rides the constructor's prepareFreshSpawn seam and runs on that not-found branch,
//     between the probe and the archive. It deliberately cannot be a decorator wrapping this
//     producer: a decorator runs before Call and therefore before the probe, which is the same
//     archive-before-probe hazard stated above, reintroduced one layer up.
//   - WebsterProducer: Webster's own "done" outcome maps to Done, reporting Webster's summary path
//     (summaryparser.Path) as the pointer's path.
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
//   - Bouncer: Call clears an already-approved round ahead of its own four-mode branch -- seed,
//     re-bounce, judge, or replay -- and its harvest step acts on a judgment that provably happened
//     (a verdict and ledger that both exist and parse) regardless of what the shuttle run itself
//     reported. A parsed APPROVED verdict maps to Done only on the harvest that earns it, within the
//     same Call that produced it; at Call entry, an already-APPROVED verdict maps to the clear
//     instead. A parsed BLOCKING verdict maps to Stuck on harvest or on a BLOCKING replay, both
//     reporting the round's ledger path as the pointer; every other path -- the seed call, the
//     re-bounce, the clear itself, every degraded path -- reports an empty pointer. The ledger is
//     reported rather than withheld because the Bouncer's ledger is a real cross-round artifact a
//     human reads, and hiding it on a BLOCKING Stuck would hide it exactly when an operator most
//     needs it. The exists-or-empty rule matters because Shed never stats a pointer, so a pointer
//     naming an unwritten file is caught nowhere and is simply persisted into the history for a
//     human to read as though the artifact were there.
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
// Bouncer's own told inputs are RunDir, StencilsDir, the resolved (Model, Effort, Version) triple,
// and the report-name convention as a function. NewBouncer is the package's one validating,
// error-returning constructor, in contrast with the two
// that return a bare pointer: NewSingleLLMProducer, whose Spec is validated downstream by
// shuttleengine, and NewWebsterProducer. NewBouncer takes the error-returning shape because
// BouncerConfig carries eleven inputs with real invariants, two of which must be absolute paths,
// and validating lazily at first Call would turn a wiring typo into a mid-run failure in an
// unattended segment.
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
// review, and the round producer uses exactly that pair predicate to decide whether to advance.
// The Bouncer's own round resolution is deliberately narrower and is stated here rather than
// implied: ResolveRound stats the REVIEW file alone, so the two sides do not run the same test.
// The asymmetry is safe only because of where an orphaned review can appear -- a process killed
// between phase A and phase B leaves the run's current_producer naming the round producer, which
// re-resolves the same round and archives the orphan before the Bouncer is routed to at all. A
// change that lets the Bouncer be entered with an orphaned review present would have it judge a
// review that no fixer round stands behind.
// The next-round directive is round-<N>-focus.md beside them -- YAML frontmatter carrying round,
// exclude_lenses, and focus, over optional prose -- whose token names the round the directives are
// for, not the round that produced them: a Bouncer rejecting round N writes the file for round N+1,
// and the seed call writes the file for round 1.
// One filename, one format, one parser: the Bouncer renders it and the BurlerRound row reads it back
// through that same pair. The two sides once disagreed -- the writer emitted this .md file while the
// reader opened a round-<N>-focus.json and strictly decoded JSON -- which silently emptied the
// directive on every production read, so the agreement is pinned here rather than left implicit.
// Its exclude_lenses reach the round's ClusterExclude; the file itself is hydrated into the round's
// prior-review context whenever it carries a directive at all, which is how the judge's targeting
// reaches the fixer.
// Reading that file is fail-safe end to end, degrading to "no directive" with a warning rather than
// erroring, including at application time when a well-formed directive cannot be honoured.
// A segment that has already approved and is entered again does not replay that approval: its
// Bouncer archives the whole generation aside and re-judges from a fresh round 1 instead. Both rows'
// artifacts move together in that archive, because BurlerProducer would otherwise resume at round
// N+1, hydrating from a generation the Bouncer had already discarded.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict -- shuttle's
// OutcomeDone, Webster's "done", the Bouncer's own harvested verdict, or
// a BurlerProducer's completed round -- which is returned as its mapped shedengine.Done or
// shedengine.Stuck with its pointer regardless of cancellation.
// This exception exists because converting a finished success into the context error would make Shed
// record no history entry for it, so the next Call would archive a valid artifact and pay for the
// same LLM session twice; a finished artifact and a paid-for session are never discarded.
// For SingleLLMProducer and WebsterProducer, this principle applies as-is.
// For the Bouncer, the genuine-success exception covers a *harvested* verdict, not only a
// shuttleengine completion: a verdict and ledger that both exist and parse are returned as their
// mapped outcome and pointer even when the run that produced them reported an error, a non-done
// outcome, or arrived after the context was cancelled.
// For BurlerProducer, the completion exception is narrower: a completed round's artifacts survive
// cancellation, but the verdict itself is not returned as Stuck (see the note below).
// No adapter installs a mid-run cancellation bridge: none of the four engines they wrap exposes a
// pause seam shaped as a caller-supplied callback.
//
// BurlerProducer's cancellation behavior is governed by the seam obligation in
// internal/shedengine/producer.go that every implementation surface cancellation as a non-nil
// error and never as Stuck: this producer always errors under cancellation, including on a
// completed round. The exception's purpose -- never discarding a paid-for artifact -- is served
// instead by an archive carve-out: a completed-then-cancelled round keeps its two files, so
// from-disk round resolution advances past it on the next call, and only the re-derivable
// in-memory verdict is dropped.
//
// # Every spawning adapter probes for a live agent first
//
// All four adapters answer the same question before they start anything: is an agent for this exact
// step still alive? They answer it in two different ways, and the difference is the engine's, not a
// policy choice here. SingleLLMProducer, Bouncer (on both its seed and its judge pass), and
// BurlerProducer all call shuttleengine's Attach seam with the step's own OutputFiles and wait on a
// match; WebsterProducer inherits websterengine's own entry-time reclaim, which stops a leftover
// Master rather than attaching to it.
//
// The probe always runs BEFORE the archive, in all three attaching adapters. Archiving renames the
// very files a live agent is about to write, and shuttle's Wait polls for bare existence at those
// paths, so archiving first would make an attached run unable to ever classify done -- in exactly
// the case the probe exists to protect.
//
// The Bouncer and BurlerProducer rows once lacked this deliberately, recorded here as a scope call.
// It was not survivable: a driver crash inside any review segment left the round's agent alive, and
// the next Call spawned a second one over it -- two sessions writing one review and one fixer
// report, and on a fix-scope: source row, two sessions committing to one branch. Both were
// reproduced live before the probe was added, and neither run could continue without an operator
// deleting a pane by hand.
//
// # Limitations
//
// Neither SingleLLMProducer nor WebsterProducer installs a mid-run cancellation bridge, so a cancel
// is observed only once the run reaches a terminal outcome or its own configured deadline elapses --
// bounded by the shuttle spec's own timeout for SingleLLMProducer, and by Webster's own whole-run
// timeout for WebsterProducer.
// BurlerProducer and Bouncer likewise install no mid-run cancellation bridge: BurlerProducer because
// burlerengine exposes no pause seam (so a cancel is observed only once the round reaches a terminal
// outcome or its own RunOpts.Timeout elapses), and Bouncer for similar reasons at the shuttleengine
// layer (see the Shared cancellation rule above for both).
//
// The Bouncer accepts one further soft spot: ledger carry-forward is enforced by the judge prompt
// alone, so a misbehaving judge can drop an entry with nothing at the Go layer catching it; closing
// that would require diffing the new ledger's key set against the previous one and deciding what a
// missing key means, which is a feature rather than a one-line addition.
package shedadapters
