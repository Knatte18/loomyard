// Package shedadapters holds the shedengine.ShedProducer adapters that let a Shed-built
// product drive shuttle, Webster, the two halves of a burlerengine round, a seatengine table of seats, and the generic review-gate
// Bouncer as ordinary producers in its own flat producer list.
// WebsterProducer wraps one websterengine
// multi-spawn run, BurlerReviewProducer and BurlerFixProducer each wrap one half of a burlerengine round, a reviewer strand or a fixer strand, and MultiLLMProducer wraps one seatengine table, a chair strand and its advisors' strands, as a single Shed
// row: each of these is a thin translation layer over an already-shipped engine, never a
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
//   - WebsterProducer: Webster's own "done" outcome maps to Done, reporting Webster's summary path
//     (summaryparser.Path) as the pointer's path.
//     Its run is handed two options: Fresh is always false, and AutoRebaseline is always true, so a plan changed between steps is rebaselined on entry.
//     Webster's own "stuck" outcome, a websterengine.ErrPendingAuditFindings error and a websterengine.ErrAutoRebaseline error all map to Stuck with an empty Path and a Reason: Master's own stuck_reason or the run-entry refusal's text, whose way forward already ends in this row's re-entry step (NewWebsterProducer sets RunDeps.ReentryStep to "re-step the <row> row" when it is empty).
//     A Stuck from Webster's own "stuck" outcome consults the plan-conflict record begin-batch left (websterengine.ClaimConflictRoute) after the cancellation check:
//     a conflict whose findings are new asks for a reroute (OutputPointer.Reroute) to the row's on_reroute target, a record repeating the last routed findings stays a plain Stuck that names the hand edit, and no record, an unreadable one or a failed write stays a plain Stuck with Master's reason.
//     The error outcomes never consult the record and never reroute.
//     Any other outcome value reaching Call is an engine-level error.
//     It is a gate carrier: its done and its stuck pointers carry the gate count and spans of Merriam's run, the verify gate's entry included.
//   - BurlerReviewProducer and BurlerFixProducer: a finished half -- shuttleengine.OutcomeDone reached within the bounded
//     retry -- maps to Stuck, never Done, reporting the round's own review path as the pointer.
//     That Stuck is a routine hand-off, the review row's to the fix row and the fix row's to the segment's Bouncer via OnStuck, never a real stuck
//     condition: a half producer has no independent notion of "finished," only the judge does.
//     Every hand-on, a finished half and an unjudged highest round, is BudgetExempt, so the Bouncer row's own budget check is the segment's one budget on judged rounds.
//     The fix row's gate-failed exit is counted against its own MaxBounces.
//     Both rows are gate carriers: the review row's hand-on pointer carries the gate count and spans of the reviewer's review-parse gate, for a spawned and a resumed review alike, and the fix row's gate-failed and finished-fix pointers carry the fixer's.
//     A round names two strands, the reviewer's and the fixer's, one per row, and each attempt picks its half's model from BurlerHalfDeps.Models for the round.
//     A runner error wrapping burlerengine.ErrHalfNotStopped returns without archiving and without the retry, since a live half may still write the round's files.
//     Before archiving or spawning, each row probes its half and resumes or respawns by what is live (see "Every spawning adapter probes for a live agent first").
//     Every non-done shuttle outcome that survives the bounded retry -- two
//     consecutive OutcomeDied/OutcomeTimeout results, or an unrecognized outcome -- is an
//     engine-level error, not Stuck, because the Bouncer tells its seed call from its judge call by
//     the round artifacts on disk, and a failed half returning Stuck with no review written would
//     be misread as a seed call.
//   - MultiLLMProducer: the chair's result maps as follows:
//     shuttleengine.OutcomeDone maps to Done with the chair's first output as the pointer's path,
//     a done chair whose gate failed maps to Stuck with that pointer and the gate's attempts and reason,
//     and OutcomeDied and OutcomeTimeout are engine-level errors, wrapping shuttleengine.ErrNotStarted when the chair's provider never came up and carrying the start error's text when shuttle reported one.
//     The advisors' results are logged and never judged, since an advisor's death weakens nothing the chair decided.
//     Before archiving anything Call probes the table's seats through its SeatRunner: a live chair is resumed and its result mapped,
//     and otherwise the probe has stopped every live advisor, so Call archives every seat's stale outputs and runs the table fresh.
//     A probe, resume or run error returns the context error when the context is cancelled and otherwise the error wrapped with the producer's name and engine label;
//     an error wrapping seatengine.ErrSeatNotStopped keeps its way forward even under cancellation, a probe's halting Call before the archive and a run's after it.
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
//     A fanned segment's facts file (BouncerConfig.ClusterExcludes) also carries a `## Lenses` section:
//     the fan and lenses the latest round ran, read from its usage record, the lenses already excluded, and the latest review's finding counts per `origin` by severity and class.
//     A missing usage record or an unparseable review renders as a line saying so, and a solo segment's file carries no such section.
//     A Go guard backs the prompt: a CIRCLING verdict with no decision file recorded for its round is read as CONTINUE, with a warning,
//     when the round is below the checkpoint or when no gating finding is open in this round's ledger and an earlier one (circlingEvidence).
//     The guard only narrows CIRCLING to CONTINUE, reads only on-disk state, and leaves a round that already has a decision file as recorded.
//     Roles: the judge and the seed spawn each load the `scribe:prose` skill (bouncerJudgeSkills, bouncerSeedSkills),
//     and both prompts carry the parent directive that parentdirective.Directive renders from BouncerConfig.ParentName, in its non-interactive form;
//     an empty ParentName renders the no-parent variant.
//     The seed and judge sessions carry the roles bouncer-seed and bouncer-judge, each led by BouncerConfig.RolePrefix and a hyphen when one is configured, in the spawn and in the probe that attaches to it.
//     The escalation brief and the parent notice are read by the parent, not by a spawned role, and carry no directive.
//     Escalation: a CONTINUE or CIRCLING round escalates to the run's parent instead of halting for a human, for one of three causes.
//     Soft-cap rule: the soft cap is reached when the BouncerConfig.Bounces seam reports count >= budget, the comparison Shed applies.
//     The rule's window opens on the soft cap by that count or on the hard cap by the round number, whichever comes first, so a round past a goto that reset the count is still settled by the rule.
//     A round in the window with no decision file is settled in five steps:
//     a review that does not parse, or a ledger that cannot be read, escalates with cause budget;
//     a ledger with no design-wrong entry (open, BLOCKING or MEDIUM, and not tagged `mechanical` by the judge, so a missing or unknown tag counts) records an automatic accept;
//     a round at or past the hard cap escalates with cause hard-cap;
//     a CIRCLING verdict, or a design-wrong key open in this round and in the previous round when the previous round's decision is a recorded continue, escalates with cause circling, and an unreadable previous decision or ledger escalates with cause budget;
//     otherwise an automatic continue is recorded (cause budget, `automatic: true`) and the round returns a budget-exempt Stuck.
//     A CIRCLING verdict below the window escalates with cause circling.
//     The rule reads only files in the run directory, so a harvest and a replay settle alike.
//     The Bouncer renders the bouncer-template-escalation brief and the bouncer-template-parent-notice line,
//     writes them with the Go-owned cause frontmatter as round-<N>-escalation.md and round-<N>-parent-notice.md,
//     and returns Awaiting with the ledger path, a Reason naming the segment, round, cause, brief and the `lyx loom circling` verbs with the `lyx loom resume` resume,
//     and the notice as OutputPointer.ParentNotice, which Shed carries as parent_notice.
//     A failed render warns and degrades to the plain Reason with no notice, the record frontmatter still written.
//     A re-call over an existing record rewrites nothing and returns the same Awaiting.
//     A CONTINUE with no spent budget, and one whose Bounces seam is unwired, unknown or errored, returns Stuck and takes Shed's generic bounce-budget block.
//     A recorded decision is acted on first, whatever the budget:
//     a continue below the hard cap maps to Stuck, marked BudgetExempt when the decision's cause is budget or the budget is reached, so a continue past the soft cap spends no budget whoever recorded it;
//     a pending accept settles its record and maps to Done after Approve and Commit, for any cause.
//     Hard cap: BouncerConfig.HardCap is the perch's hard cap as a round number in the segment's current generation, 0 meaning none.
//     A continue recorded at or past it is moved aside (the escalation record, its parent notice and then the decision file, each to a timestamped sibling no reader opens)
//     and the round re-escalates with cause `hard-cap`, the third cause beside budget and circling.
//     An escalation at or past the hard cap, whatever its cause, carries the accept-only Reason and the bouncer-fragment-decide-capped decide step; below it the Reason and the bouncer-fragment-decide-open step offer accept and continue.
//     PendingCirclingDecision, PendingEscalation and PendingEscalationCause read a run directory's latest round: the first reports a decision file not yet settled, the second a Bouncer-written escalation, with its round and brief path, that no decision file answers, and the third the same escalation with its cause and brief path.
//     Auto-accept: a Bouncer told BouncerConfig.AutoAccept settles a spent budget as an accept instead of running the soft-cap rule.
//     It fires at the soft cap alone, also over a CIRCLING verdict, and only on a harvest, the judge having written the verdict in this call, over a round with no decision and no escalation record.
//     A replay and a round BlockingEscalates sends to the rule (an open BLOCKING entry, or an unreadable ledger) go to the rule instead.
//     It records the round's decision file as an accept with cause budget and `automatic: true`, then takes the path a recorded parent accept takes (carry-over, settled record, Approve, Commit, Done),
//     writing no escalation record and no parent notice; settling the record keeps its `automatic` value, which a parent's accept never sets.
//     A CIRCLING below the budget, an already escalated round, a recorded decision and an unknown or errored Bounces seam behave as with the flag false, which is the default.
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
//     The legacy words APPROVED and BLOCKING are read as CONVERGED and CONTINUE only for a verdict already on disk at Call entry (the clear, the replay and the review row's may-advance check).
//     A legacy word never settles a harvest:
//     a judge this Call spawned or attached that writes one has its three outputs archived and the round re-judged,
//     through the round producer's unjudged-round hand-back, at one bounce unit on each row.
//     A judgment the convergence guard refuses (see the end of this document) takes the same route beside the retired-word one, with a Reason naming the cause and ending in `the round is re-judged`;
//     a replayed round that carries an escalation record or a circling decision file stands as recorded and is never refused.
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
// MultiLLMProducer additionally takes an injected clock, a nil now defaulting to the real
// time.Now; the injected clock resolves only the archive filename's same-second collision suffix,
// never Shed's own history[].at field.
// BurlerReviewProducer and BurlerFixProducer are told an absolute run directory and an already-constructed half runner, and take the same injected clock, resolving only the archive filename's same-second collision suffix the same way.
// MultiLLMProducer is told a seatengine.Table and an already-constructed SeatRunner.
// NewMultiLLMProducerSourced is told a TableSource instead of a table: the source builds the table once per Call, so stencils and directives are read at call time.
// Bouncer's own told inputs are RunDir, StencilsDir, the resolved seed and judge pairs, and the report-name convention as a function.
// NewBouncer is the package's one validating, error-returning constructor, in contrast with the constructors that return a bare pointer: the MultiLLMProducer constructors, whose table is validated downstream by seatengine, and NewWebsterProducer.
// NewBouncer takes the error-returning shape because BouncerConfig carries eleven inputs with real invariants, two of which must be absolute paths, and validating lazily at first Call would turn a wiring typo into a mid-run failure in an unattended segment.
//
// # The round-artifact convention
//
// This is the binding two-sided contract between the Burler rows and their segment's Bouncer, pinned
// here durably rather than only in a board entry, so it survives independently of the entry
// that is deleted when the Bouncer item completes.
// Artifact paths are flat inside the told run directory, one canonical set per round with no
// attempt suffix: round-<N>-review.md, round-<N>-fixer-report.md and the acceptance record round-<N>-acceptance.md, with N a positive decimal
// integer carrying no leading zeros.
// A retry writes to the same paths, because a retry is a second try at the one artifact the
// round owes rather than a second artifact.
// The presence of all three files means, and only means, that round N completed and produced a usable
// review that was fixed -- never that anyone has judged it, which is why the review row pairs that predicate
// with a second one before advancing: round N's own bouncer verdict and ledger must both exist and
// parse too. Completion alone would let a Bouncer's degraded Stuck -- a judge spawn that died, a
// verdict that did not parse -- buy a whole extra fixer round over a review nobody judged, and leave
// round N+1's judge with no round-N ledger to carry findings forward from.
// The Bouncer's own round resolution is deliberately narrower and is stated here rather than
// implied: ResolveRound stats the REVIEW file alone, so the two sides do not run the same test.
// The asymmetry is safe only because of where an orphaned review can appear -- a process killed
// between the review row and the fix row leaves the run's current_producer naming one of them, which
// re-resolves the same round and archives the orphan before the Bouncer is routed to at all. A
// change that lets the Bouncer be entered with an orphaned review present would have it judge a
// review that no fixer round stands behind.
// The next-round directive is round-<N>-focus.md beside them -- YAML frontmatter carrying round,
// exclude_lenses, and focus, over optional prose -- whose token names the round the directives are
// for, not the round that produced them: a Bouncer rejecting round N writes the file for round N+1,
// and the seed call writes the file for round 1.
// One filename, one format, one parser: the Bouncer renders it and the Burler review row reads it back
// through that same pair. The two sides once disagreed -- the writer emitted this .md file while the
// reader opened a round-<N>-focus.json and strictly decoded JSON -- which silently emptied the
// directive on every production read, so the agreement is pinned here rather than left implicit.
// Only the judge call asks for exclude_lenses, and only when told ClusterExcludes, meaning its round runs a cluster
// fan the excludes can trim; the seed call and every other judge prompt request focus alone, so the key may be absent
// from a written file.
// The fanned judge prompt states three rules.
// An exclusion is permanent for the segment generation.
// A lens is excluded only when the latest round ran it and its area is settled, meaning nothing at MEDIUM or worse from it and not in the next round's focus.
// Earlier exclusions are never restated.
// Its exclude_lenses reach the round's ClusterExclude;
// the file itself reaches the round profile's focus-directive field whenever it carries a directive at all,
// and the explore step reads it there, which is how the judge's targeting reaches the fixer.
// Reading that file is fail-safe end to end, degrading to "no directive" with a warning rather than
// erroring, including at application time when a well-formed directive cannot be honoured.
// A segment that has already approved and is entered again does not replay that approval: its
// Bouncer archives the whole generation aside and re-judges from a fresh round 1 instead. Both rows'
// artifacts move together in that archive, because the review row would otherwise resume at round
// N+1, hydrating from a generation the Bouncer had already discarded.
// A fanned round's exclusions accumulate across the generation, held in Go:
// the round's ClusterExclude is the union of the eligible exclude_lenses of focus files 2 through the round, and ClusterExcludeHeld the union through the round before.
// An entry in round k's focus file is eligible only when round k-1's usage record names the same fan and lists the lens among those it ran,
// so a missing record, a switch from solo or from another fan, or a lens already excluded drops it, with a warning for the round's own file.
// Round 1 runs the whole fan and drops a focus file's excludes, and an excluded lens returns only with a new generation, since an archived generation moves the whole run directory aside.
// Every completed round, solo and fanned, also leaves round-<N>-usage.yaml beside them:
// the fan and lenses the round ran, and each half's model, run times, wall time and tokens, forks included, with an unknown reading left unknown rather than zero.
// The record also carries the Bouncer's `seed` and `judge` lists, one entry per pass in the same half shape.
// The Bouncer appends an entry for every pass whose result reached the Call, a judgment the guard refuses included, so a re-judged round lists every pass it took;
// a pass whose run errored, and a judgment a later Call settles from disk, record nothing, and a failed write only logs a warning.
// Every writer merges into the record on write, and a record that does not decode is archived aside and replaced by one holding only the writer's part.
// A record without a review half reads as missing for the lens facts and the lens exclusions, since only the Bouncer has written to it.
// A gate-failed round is archived and writes none, and a failed write is a warning that never changes the round's outcome.
// The told run directory's whole content and every timestamped archive the adapters write -- per-file siblings inside the run directory and whole-generation siblings of the run directory beside it (archiveRunDir) alike -- are durable record the wiring layer commits, so the adapters write nothing there that is not meant for git.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict -- shuttle's
// OutcomeDone, Webster's "done", the Bouncer's own harvested verdict, or
// a Burler row's finished half -- which is returned as its mapped shedengine.Done or
// shedengine.Stuck with its pointer regardless of cancellation.
// This exception exists because converting a finished success into the context error would make Shed
// record no history entry for it, so the next Call would archive a valid artifact and pay for the
// same LLM session twice; a finished artifact and a paid-for session are never discarded.
// For MultiLLMProducer and WebsterProducer, this principle applies as-is.
// For the Bouncer, the genuine-success exception covers a *harvested* verdict, not only a
// shuttleengine completion: a verdict and ledger that both exist and parse are returned as their
// mapped outcome and pointer even when the run that produced them reported an error, a non-done
// outcome, or arrived after the context was cancelled.
// For the Burler rows, the completion exception is narrower: a finished half's artifacts survive
// cancellation, but the hand-on itself is not returned as Stuck (see the note below).
// No adapter installs a mid-run cancellation bridge: none of the engines they wrap exposes a
// pause seam shaped as a caller-supplied callback.
//
// The Burler rows' cancellation behavior is governed by the seam obligation in internal/shedengine/producer.go that every implementation surface cancellation as a non-nil error and never as Stuck:
// these producers always error under cancellation, including on a finished half.
// The exception's purpose -- never discarding a paid-for artifact -- is served instead by an archive carve-out:
// a finished-then-cancelled half keeps its files, so from-disk round resolution advances past it on the next call, and only the re-derivable in-memory hand-on is dropped.
//
// # Every spawning adapter probes for a live agent first
//
// Every adapter answers the same question before it starts anything: is an agent for this exact
// step still alive? They answer it in different ways, and the difference is the engine's, not a
// policy choice here.
// Bouncer calls shuttleengine's Attach seam with the step's own OutputFiles and waits on a match.
// The Bouncer does so on its seed pass, on its judge pass, on the re-bounce, and once more at Call entry (see below).
// The review row and the fix row each probe their half through their runner (see below).
// MultiLLMProducer probes its table's seats through its SeatRunner: with the chair live it resumes the table, and otherwise the probe has already stopped every live advisor, so the archive that follows runs beside no live seat.
// A MultiLLMProducer built with a fresh-spawn preparation runs it on the fresh path only, after the probe and before the archive; its error fails the call with nothing archived and no seat started.
// The preparation returns a prompt amendment beside its error, which the producer sets on a copy of the table as its Amendment so the seat engine appends it to the lead's prompt; the resume path neither runs the preparation nor amends.
// The preparation cannot be a decorator wrapping the producer: a decorator runs before Call and therefore before the probe, so it would archive files a live agent is about to write.
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
// The probe always runs BEFORE the archive, in every probing adapter. Archiving renames the
// very files a live agent is about to write, and shuttle's Wait polls for bare existence at those
// paths, so archiving first would make an attached run unable to ever classify done -- in exactly
// the case the probe exists to protect.
//
// A Burler row's probe is one-stranded, because a row drives one half, a reviewer or a fixer, each declaring its own single output file.
// BurlerHalfRunner.ProbeReview and ProbeFix report the half as live, done or gone, and Call acts on it.
// A live half is resumed through ResumeReview or ResumeFix, and the result is mapped through the outcome switch a spawned attempt uses.
// A died or timeout result there falls through to a fresh attempt 1, since the resumed half is not counted as an attempt.
// A half that is done or gone is nothing live, so nothing is removed and it falls through to attempt 1.
// A fall-through spawn archives the half's outputs first, so a half is never spawned a second time beside a live one.
// A probe error and a resume error wrapping burlerengine.ErrHalfNotStopped are returned without archiving and without spawning, since a half that may still be live may be writing the round's files.
// A failed stop wraps burlerengine.ErrHalfNotStopped and ends with the way forward, run "lyx reed remove <guid>" and re-step the row.
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
// The Bouncer and the Burler rows once lacked this deliberately, recorded here as a scope call.
// It was not survivable: a driver crash inside any review segment left the round's agent alive, and
// the next Call spawned a second one over it -- two sessions writing one review and one fixer
// report, and on a fix-scope: source row, two sessions committing to one branch. Both were
// reproduced live before the probe was added, and neither run could continue without an operator
// deleting a pane by hand.
//
// # The review row and the fix row
//
// BurlerReviewProducer and BurlerFixProducer split a review segment's round into two Shed rows, each driving one half of a burlerengine round through BurlerHalfRunner.
// Neither returns Done: a finished half is a routine Stuck hand-off, the review row's to the fix row and the fix row's to the segment's Bouncer.
// The round's acceptance record, round-<n>-acceptance.md in the run directory, joins the round's review and fixer report as one of its paths.
// It holds the round and the hex SHA-256 of the accepted review in YAML frontmatter, and only the review row writes it, after the review passed the strict parse.
// A record that is absent or does not parse reads as absent.
//
// The review row resolves the round from the highest complete round, and hands on BudgetExempt without spawning in two cases:
// the highest complete round has no recorded verdict, and the next round's review already carries an acceptance record, so a restart with the fixer pending resumes at the fix row.
// Otherwise it probes for a live reviewer before archiving anything, resumes one it finds, and spawns at most two attempts, the second only after a died or timed-out first.
// An accepted review writes its record, merges the half's usage into the round's usage record and hands on with the review as the pointer's path.
// Every other end, a gate or strict-parse failure included, is a hard error that archives the round's review, fixer report and acceptance record;
// a reviewer that could not be stopped returns bare, with nothing archived.
// A review with no record beside it is an orphan and is reviewed again.
//
// The fix row hands on an unjudged highest round the same way, and otherwise requires the next round's acceptance record and review.
// Without them it is a hard error whose way forward is a goto to the review row and a resume, since the review row's hand-on never produces that state:
// its inputs are a goto into the fix row with no accepted review, and a resume after the fix row archived a review that changed since acceptance.
// It probes for a live fixer first and resumes it against the recorded digest, then spawns at most two attempts under that digest.
// A finished fix, a skipped empty review included, merges the fix half's usage and hands on BudgetExempt with the review as the pointer's path.
// A finished fix whose gate failed archives the review, fixer report and acceptance record and returns a counted Stuck with an empty path, so the Bouncer's next round reviews afresh.
// A review that changed after acceptance archives the same three and ends with the same way forward.
// Any other failure archives the fixer report alone, keeping the accepted review and its record so a resume re-spawns the fixer without re-running the reviewer.
//
// # The switched producer
//
// SwitchedProducer wraps no engine: it is one told action behind one told flag, built under the OpenWindow, SeedIDE and OpenIDE registry engines, for operator conveniences a run must never halt over.
// A flag that is off is Done without acting, a flag that is on acts once and is Done even when the action fails, with a warning, and a flag that cannot be read is an error.
// The recipe's owner supplies both seams, so the flag lives in that owner's config and the producer knows no key name.
// Bound: a failed action is skipped silently but for its warning, and the next row runs as if it had succeeded.
//
// # Limitations
//
// Neither MultiLLMProducer nor WebsterProducer installs a mid-run cancellation bridge, so a cancel
// is observed only once the run reaches a terminal outcome or its own configured deadline elapses --
// bounded by the table's own timeout for MultiLLMProducer, and by Webster's own whole-run
// timeout for WebsterProducer.
// The Burler rows and Bouncer likewise install no mid-run cancellation bridge: the Burler rows because
// burlerengine exposes no pause seam (so a cancel is observed only once the half reaches a terminal
// outcome or its own RunOpts.Timeout elapses), and Bouncer for similar reasons at the shuttleengine
// layer (see the Shared cancellation rule above for both).
//
// The Bouncer does not trust the judge alone with the rules it states:
// a CONVERGED with an open BLOCKING ledger entry or a BLOCKING or unparseable review is archived and re-judged within the bounce budget.
// So is any verdict whose ledger drops a key of the previous round's ledger.
// Only keys are compared across ledgers, never status, class or severity.
package shedadapters
