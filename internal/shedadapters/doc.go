// Package shedadapters holds the four shedengine.ShedProducer adapters that let a Shed-built
// product drive shuttle, Webster, one burlerengine round, and the generic review-gate
// Bouncer as ordinary producers in its own flat producer list.
// SingleLLMProducer wraps one shuttleengine run, WebsterProducer wraps one websterengine
// multi-spawn run, and BurlerProducer wraps one burlerengine round, a reviewer strand and a fixer strand, as a single Shed
// row: each of these three is a thin translation layer over an already-shipped engine, never a
// second implementation of that engine's own loop.
// Bouncer is the one member of this package for which that is false: it is new logic over
// shuttleengine, composing its own prompt from stencils rather than translating an already-shipped
// engine's loop.
//
// # Outcome mapping
//
// Each adapter maps its own verdict onto Done or Stuck,
// and the Bouncer alone also onto Awaiting, the hand-off to the run's parent when its judge finds the review circling or its bounce budget is spent;
// the adapters report the output pointer differently because they report success differently:
//
//   - SingleLLMProducer: shuttleengine.OutcomeDone maps to Done, reporting the first entry of the
//     evaluated Spec's OutputFiles as the pointer's path.
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
//     between the probe and the archive.
//     The text it returns is appended verbatim to the prompt of the spec handed to the fresh run (and to no other spec),
//     so the new session can be told what the preparation did;
//     an empty amendment changes nothing.
//     It deliberately cannot be a decorator wrapping this producer: a decorator runs before Call and therefore before the probe, which is the same archive-before-probe hazard stated above, reintroduced one layer up.
//   - WebsterProducer: Webster's own "done" outcome maps to Done, reporting Webster's summary path
//     (summaryparser.Path) as the pointer's path.
//     Webster's own "stuck" outcome and a websterengine.ErrPendingAuditFindings error both map to Stuck with an empty Path and a Reason: Master's own stuck_reason or the run-entry refusal's text, whose way forward already ends in this row's re-entry step (NewWebsterProducer sets RunDeps.ReentryStep to "re-step the <row> row" when it is empty).
//     Webster's own "paused" outcome reaching Call out of band is an engine-level error.
//   - BurlerProducer: a completed round -- shuttleengine.OutcomeDone reached within the bounded
//     retry -- maps to Stuck, never Done, reporting the round's own review path as the pointer.
//     That Stuck is a routine hand-off to the segment's Bouncer via OnStuck, never a real stuck
//     condition: a round producer has no independent notion of "finished," only the judge does.
//     That Stuck is BudgetExempt when the previous round carries a recorded continue decision whose cause is budget,
//     so the one more round a budget continue grants spends no budget; each further round needs its own decision.
//     A round names two strands, the reviewer's and the fixer's, and each round's ready marker is derived from BurlerDeps.AnchorPath with burlermarker.Path.
//     Each attempt picks both halves' models from BurlerDeps.Models for the round.
//     A runner error wrapping burlerengine.ErrHalfNotStopped returns without archiving and without the retry, since a live half may still write the round's files.
//     Before archiving or spawning it probes the round's two halves and resumes, stops or respawns by what is live (see "Every spawning adapter probes for a live agent first").
//     Every non-done shuttle outcome that survives the bounded retry -- two
//     consecutive OutcomeDied/OutcomeTimeout results, or an unrecognized outcome -- is an
//     engine-level error, not Stuck, because the Bouncer tells its seed call from its judge call by
//     the round artifacts on disk, and a failed round returning Stuck with no review written would
//     be misread as a seed call.
//   - Bouncer: Call clears an already-approved round ahead of its own four-mode branch -- seed, re-bounce, judge, or replay --
//     and its harvest step acts on a judgment that provably happened (a verdict and ledger that both exist and parse) regardless of what the shuttle run itself reported.
//     The judge's verdict is exactly CONVERGED, CONTINUE or CIRCLING.
//     A parsed CONVERGED verdict maps to Done only on the harvest that earns it, within the same Call that produced it;
//     at Call entry, an already-CONVERGED verdict maps to the clear instead -- unless the entry-time probe finds the judge that wrote it still alive,
//     in which case waiting on that judge is itself the harvest that earns the Done (see "Every spawning adapter probes for a live agent first" below).
//     A parsed CONTINUE verdict maps to Stuck on harvest or on a CONTINUE replay,
//     and a parsed CIRCLING verdict maps to Awaiting on harvest or on a replay without spawning anything (see Escalation below),
//     all three reporting the round's ledger path as the pointer.
//     Checkpoint judging: the judge prompt's decision rule is held in Go (decisionRuleMarker), depends on the round, and offers CIRCLING only from BouncerConfig.CirclingCheckpoint on.
//     Round 1 rules CONTINUE over any BLOCKING finding or any gating-class finding at MEDIUM or worse.
//     From round 2 on only a BLOCKING finding, or a gating-class finding at MEDIUM or worse on a key the facts file lists as open in an earlier round, rules CONTINUE;
//     a gating finding on a key first raised in the latest round converges and is carried into the decision record's `## Open risks` through the CarryOver seam.
//     A parse-error line in the facts file's earlier-open list, from a missing or unparseable earlier ledger, sends the judge back to round 1's rule.
//     A Go guard backs the prompt: a CIRCLING verdict with no decision file recorded for its round is read as CONTINUE, with a warning,
//     when the round is below the checkpoint or when no gating finding is open in this round's ledger and an earlier one (circlingEvidence).
//     The guard only narrows CIRCLING to CONTINUE, reads only on-disk state, and leaves a round that already has a decision file as recorded.
//     Roles: the judge and the seed spawn each load the `scribe:prose` skill (bouncerJudgeSkills, bouncerSeedSkills),
//     and both prompts carry the parent directive that parentdirective.Directive renders from BouncerConfig.ParentName, in its non-interactive form;
//     an empty ParentName renders the no-parent variant.
//     The escalation brief and the parent notice are read by the parent, not by a spawned role, and carry no directive.
//     Escalation: a CONTINUE or CIRCLING round escalates to the run's parent instead of halting for a human, for one of two causes.
//     Cause budget holds whenever the BouncerConfig.Bounces seam reports count >= budget, the comparison Shed applies, even over a CIRCLING verdict;
//     cause circling holds for a guarded CIRCLING below the budget.
//     The Bouncer renders the bouncer-template-escalation brief and the bouncer-template-parent-notice line,
//     writes them with the Go-owned cause frontmatter as round-<N>-escalation.md and round-<N>-parent-notice.md,
//     and returns Awaiting with the ledger path, a Reason naming the segment, round, cause, brief and the `lyx loom circling` verbs with the `lyx loom resume` resume,
//     and the notice as OutputPointer.ParentNotice, which Shed carries as parent_notice.
//     A failed render warns and degrades to the plain Reason with no notice, the record frontmatter still written.
//     A re-call over an existing record rewrites nothing and returns the same Awaiting.
//     A CONTINUE with no spent budget, and one whose Bounces seam is unwired, unknown or errored, returns Stuck and takes Shed's generic bounce-budget block.
//     A recorded decision is acted on first, whatever the budget:
//     a continue maps to Stuck, marked BudgetExempt exactly when the escalation's cause is budget, so a continue after a budget escalation spends no budget;
//     a pending accept settles its record and maps to Done after Approve and Commit, for either cause.
//     Carry-over: a Bouncer told a CarryOver seam, with the Segment its entry is filed under and the AnchorPath its paths are relative to, calls it on every Done that settles a judged round.
//     The seam is called with discussionparser.CarryOverConverged before Approve on a CONVERGED settle,
//     and with CarryOverAccepted before the settle write of a pending accept, so a failed write leaves the accept pending and its re-call retries.
//     The entry lists the round ledger's open findings at MEDIUM or worse and the open ones missing a class or severity (labelled `unlabelled`),
//     sorted by key, each with the ledger rounds its key was open in and never the judge-written rounds list,
//     and the review and fixer-report paths relative to the anchor.
//     A round with no such finding calls the seam with an empty list, which removes the segment's entry.
//     The skip seam never calls it, since no round ran and the last reviewed generation's entry still describes the artifact.
//     The seam only writes the segment's `## Open risks` entry: it approves nothing and skips no seam,
//     and a failure is returned as the settle's own error with a way forward, so neither Approve nor Commit runs.
//     A nil seam leaves every path as before.
//     The exemption covers only the Bouncer's Stuck; each further round past the budget needs its own decision, and `lyx loom goto` grants a fresh budget.
//     Every other path -- the seed call, the re-bounce, the clear itself, every degraded path -- reports an empty Path,
//     with the re-bounce and degraded paths carrying their cause on Reason.
//     The legacy words APPROVED and BLOCKING are read as CONVERGED and CONTINUE only for a verdict already on disk at Call entry (the clear, the replay and BurlerProducer's may-advance check).
//     A legacy word never settles a harvest:
//     a judge this Call spawned or attached that writes one has its three outputs archived and the round re-judged,
//     through the round producer's unjudged-round hand-back, at one bounce unit on each row.
//     The ledger is reported rather than withheld because the Bouncer's ledger is a real cross-round artifact a human reads,
//     and hiding it on a CONTINUE Stuck would hide it exactly when an operator most needs it.
//     The exists-or-empty rule matters because Shed never stats a pointer,
//     so a pointer naming an unwritten file is caught nowhere and is simply persisted into the history for a human to read as though the artifact were there.
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
// here durably rather than only in a board entry, so it survives independently of the entry
// that is deleted when the Bouncer item completes.
// Artifact paths are flat inside the told run directory, one canonical pair per round with no
// attempt suffix: round-<N>-review.md and round-<N>-fixer-report.md, with N a positive decimal
// integer carrying no leading zeros.
// A retry writes to the same two paths, because a retry is a second try at the one artifact the
// round owes rather than a second artifact.
// The presence of both files means, and only means, that round N completed and produced a usable
// review -- never that anyone has judged it, which is why the round producer pairs that predicate
// with a second one before advancing: round N's own bouncer verdict and ledger must both exist and
// parse too. Completion alone would let a Bouncer's degraded Stuck -- a judge spawn that died, a
// verdict that did not parse -- buy a whole extra fixer round over a review nobody judged, and leave
// round N+1's judge with no round-N ledger to carry findings forward from.
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
// A Bouncer asks for exclude_lenses only when told ClusterExcludes, meaning its round runs a cluster
// fan the excludes can trim; otherwise its prompts request focus alone, so the key may be absent
// from a judge-written file.
// Its exclude_lenses reach the round's ClusterExclude;
// the file itself reaches the round profile's focus-directive field whenever it carries a directive at all,
// and the explore step reads it there, which is how the judge's targeting reaches the fixer.
// Reading that file is fail-safe end to end, degrading to "no directive" with a warning rather than
// erroring, including at application time when a well-formed directive cannot be honoured.
// A segment that has already approved and is entered again does not replay that approval: its
// Bouncer archives the whole generation aside and re-judges from a fresh round 1 instead. Both rows'
// artifacts move together in that archive, because BurlerProducer would otherwise resume at round
// N+1, hydrating from a generation the Bouncer had already discarded.
// The told run directory's whole content and every timestamped archive the adapters write -- per-file siblings inside the run directory and whole-generation siblings of the run directory beside it (archiveRunDir) alike -- are durable record the wiring layer commits, so the adapters write nothing there that is not meant for git.
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
// policy choice here.
// SingleLLMProducer and Bouncer call shuttleengine's Attach seam with the step's own OutputFiles and wait on a match.
// The Bouncer does so on its seed pass, on its judge pass, on the re-bounce, and once more at Call entry (see below).
// BurlerProducer probes its round's two halves through its runner (see below).
// WebsterProducer inherits websterengine's own entry-time reclaim, which stops a leftover Master rather than attaching to it.
//
// "Every mode" is meant literally, and was not always true. The re-bounce -- an already-seeded
// segment whose round producer handed back without a report -- spawns nothing, so it looked like a
// mode with nothing to probe for. It is not: a parsing round-1 focus file proves the seed agent
// wrote its one declared output, never that it exited, and shuttle's Wait polls for bare existence
// at that path, so a driver killed between the write and the exit lands on that branch with a live
// seed still holding the file. Returning Stuck without waiting abandoned it while the segment's
// round producer began reading the very file it might still be rewriting. Reproduced live, and
// closed by giving the branch the same probe the seed pass already had.
//
// The probe always runs BEFORE the archive, in all three probing adapters. Archiving renames the
// very files a live agent is about to write, and shuttle's Wait polls for bare existence at those
// paths, so archiving first would make an attached run unable to ever classify done -- in exactly
// the case the probe exists to protect.
//
// BurlerProducer's probe is two-stranded, because a round is a reviewer and a fixer, each declaring its own single output file.
// BurlerRunner.ProbeRound reports each half as live, done or gone, and Call acts on the pair.
// With both halves live it calls Resume and maps the result through the outcome switch a spawned attempt uses.
// A died or timeout result there falls through to a fresh attempt 1, since the resumed round is not counted as an attempt.
// With exactly one half live it stops that half through its handle's Stop, which records the stop, and falls through to attempt 1.
// So a live fixer beside a finished review is stopped and re-run rather than attached, and its target edits stay in the worktree for the next attempt's reviewer to review.
// With one half done and the other gone nothing is live, so nothing is removed and it falls through to attempt 1.
// With neither half live it falls through.
// A fall-through spawn archives the round's outputs and the engine removes the ready marker, so a round never ends with one half still live and a live half is never spawned a second time.
// A probe error, a failed stop and a Resume error wrapping burlerengine.ErrHalfNotStopped are returned without archiving and without spawning, since a half that may still be live may be writing the round's files.
// The failed-stop error wraps burlerengine.ErrHalfNotStopped and ends with the way forward, run "lyx reed remove <guid>" and re-step the row.
//
// The Bouncer's third probe covers the two modes that spawn nothing at all, and it exists because
// its own judgment record is narrower than the judge spawn that produces it: a recorded judgment is
// a verdict and a ledger, while the spawn declares those two plus the next round's focus file. A
// driver crash between the ledger write and the focus write therefore leaves a live judge behind an
// apparently-final verdict, landing the next Call in the clear branch or the replay branch -- one of
// which archives the run directory out from under that judge, and the other of which writes a
// synthetic focus file at a path it declared as an output. Both then hand back a Stuck whose next
// respawn could not attach to the survivor either, since the seed spec and the round producer's spec
// each name a different OutputFiles set than the judge's. So Call probes on the judge spec's own
// three paths before either branch acts: attaching makes that call the judgment's harvest, settling
// it rather than clearing it, and a not-found probe leaves both branches acting on exactly the state
// they always did.
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
