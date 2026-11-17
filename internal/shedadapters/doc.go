// Package shedadapters holds the shedengine.ShedProducer adapters for a Shed-built product's flat producer list.
// Each adapter lets Shed drive one engine as an ordinary producer.
// WebsterProducer wraps one websterengine multi-spawn run.
// BurlerReviewProducer and BurlerFixProducer each wrap one half of a burlerengine round.
// The halves are a reviewer strand and a fixer strand.
// MultiLLMProducer wraps one seatengine table, a chair strand and its advisors' strands, as a single Shed row.
// Each of these is a thin translation layer over its engine and never a second implementation of the engine's loop.
// Bouncer is the generic review-gate producer and the exception.
// It is new logic over shuttleengine and composes its own prompt from stencils.
//
// # Outcome mapping
//
// Each adapter maps its own verdict onto Done or Stuck.
// The Bouncer alone also maps onto Awaiting, the hand-off to the run's parent.
// It does so when its judge finds the review circling or its bounce budget is spent.
// The adapters report the output pointer differently because they report success differently:
//
//   - WebsterProducer: Webster's "done" outcome maps to Done.
//     The pointer's path is Webster's summary path (summaryparser.Path).
//     Its run is handed two options: Fresh is always false, and AutoRebaseline is always true,
//     so a plan changed between steps is rebaselined on entry.
//     Webster's "stuck" outcome maps to Stuck with an empty Path.
//     So do the websterengine errors ErrPendingAuditFindings, ErrAutoRebaseline and ErrBatchDeadEnd.
//     The Reason is Master's stuck_reason or the run-entry refusal's text.
//     A refusal's way forward already ends in this row's re-entry step:
//     NewWebsterProducer sets RunDeps.ReentryStep to "re-step the <row> row" when it is empty.
//     A Stuck from Webster's "stuck" outcome consults the plan-conflict record that begin-batch left.
//     The consult (websterengine.ClaimConflictRoute) runs after the cancellation check:
//     a conflict whose findings are new asks for a reroute (OutputPointer.Reroute) to the row's on_reroute target;
//     a record repeating the last routed findings stays a plain Stuck that names the hand edit;
//     and no record, an unreadable one or a failed write stays a plain Stuck with Master's reason.
//     The error outcomes never consult the record and never reroute.
//     Any other outcome value reaching Call is an engine-level error.
//     WebsterProducer is a gate carrier: its done and stuck pointers carry the gate count and spans of the master run.
//     The spans include the verify gate's entry.
//   - BurlerReviewProducer and BurlerFixProducer: a finished half maps to Stuck, never Done.
//     A half is finished when shuttleengine.OutcomeDone is reached within the bounded retry.
//     The pointer's path is the round's review path.
//     That Stuck is a routine hand-off, never a real stuck condition:
//     the review row hands to the fix row, and the fix row hands to the segment's Bouncer via OnStuck.
//     A half producer has no independent notion of "finished"; only the judge does.
//     Every hand-on, a finished half and an unjudged highest round, is BudgetExempt,
//     so the Bouncer row's own budget check is the segment's one budget on judged rounds.
//     The fix row's gate-failed exit is counted against its own MaxBounces.
//     A gated producer told a findings path keeps an exhausted gate's findings in that file.
//     The path is MultiLLMProducer.KeepGateFindings or BurlerHalfDeps.GateFindingsPath.
//     Such a producer names the file in the Stuck reason and removes it on a call whose gate passed.
//     Both rows are gate carriers.
//     The review row's hand-on pointer carries the gate count and spans of the reviewer's review-parse gate.
//     That holds for a spawned and a resumed review alike.
//     The fix row's gate-failed and finished-fix pointers carry the fixer's gate count and spans.
//     A round names two strands, the reviewer's and the fixer's, one per row.
//     Each attempt picks its half's model from BurlerHalfDeps.Models for the round.
//     A runner error wrapping burlerengine.ErrHalfNotStopped returns without archiving and without the retry;
//     a live half may still write the round's files.
//     Before archiving or spawning, each row probes its half and resumes or respawns by what is live.
//     See "Every spawning adapter probes for a live agent first".
//     Every non-done shuttle outcome that survives the bounded retry is an engine-level error, not Stuck.
//     Such an outcome is two consecutive OutcomeDied or OutcomeTimeout results, or an unrecognized outcome.
//     The Bouncer tells its seed call from its judge call by the round artifacts on disk;
//     a failed half returning Stuck with no review written would be misread as a seed call.
//   - MultiLLMProducer: the chair's result maps as follows:
//     shuttleengine.OutcomeDone maps to Done with the chair's first output as the pointer's path;
//     a done chair whose gate failed maps to Stuck with that pointer and the gate's attempts and reason;
//     OutcomeDied and OutcomeTimeout are engine-level errors.
//     A chair whose provider never came up gives an error wrapping shuttleengine.ErrNotStarted.
//     That error carries the start error's text when shuttle reported one.
//     A chair that started and died gives an error wrapping shuttleengine.ErrAgentDied.
//     An advisor's result is logged only; an advisor's death weakens nothing the chair decided.
//     A writer seat that ended the step decides it instead of the chair, whatever the chair's result was.
//     The first such seat in table order decides.
//     The mapping uses the seat's name as both seat and part:
//     a start failure is an error wrapping shuttleengine.ErrNotStarted;
//     a died run or a wait error is an error wrapping shuttleengine.ErrAgentDied that names the writer;
//     a timeout, a failed gate, an Accept error and a writer lost at resume with its outputs missing are Stuck.
//     The Stuck pointer's path is the seat's first output, so the commit decorator commits the part.
//     The Stuck reason names the seat and its part.
//     A failed gate's reason keeps the findings in the seat's GateFindingsPath.
//     Its pointer carries the gate's attempts and spans.
//     Every Stuck reason ends with the way forward of re-running the row with `lym loom resume`.
//     Any other writer end is an error naming the writer and its outcome.
//     A cancelled context returns the context error first.
//     Before archiving anything, Call probes the table's seats through its SeatRunner.
//     A live chair is resumed and its result mapped.
//     Otherwise the probe has stopped every live advisor.
//     Call then archives every seat's stale outputs and runs the table fresh.
//     A probe, resume or run error returns the context error when the context is cancelled.
//     Otherwise it returns the error wrapped with the producer's name and engine label.
//     An error wrapping seatengine.ErrSeatNotStopped keeps its way forward even under cancellation;
//     a probe's error halts Call before the archive, and a run's halts it after the archive.
//   - Bouncer: Call clears an already-approved round ahead of its own four-mode branch.
//     The modes are seed, re-bounce, judge and replay.
//     The harvest step acts on a judgment that provably happened: a verdict and ledger that both exist and parse.
//     It does so regardless of what the shuttle run itself reported.
//     The judge's verdict is CONVERGED, CONTINUE or CIRCLING.
//     A parsed CONVERGED verdict maps to Done only on the harvest that earns it, within the same Call that produced it.
//     At Call entry, an already-CONVERGED verdict maps to the clear instead.
//     The exception is an entry-time probe that finds the judge that wrote it still alive:
//     waiting on that judge is itself the harvest that earns the Done.
//     The probe is described under "Every spawning adapter probes for a live agent first".
//     A parsed CONTINUE verdict maps to Stuck on harvest or on a CONTINUE replay.
//     A parsed CIRCLING verdict maps to Awaiting on harvest or on a replay, which spawns nothing (see Escalation below).
//     All three report the round's ledger path as the pointer.
//     Checkpoint judging: the judge prompt's decision rule is held in Go (decisionRuleMarker) and depends on the round.
//     It offers CIRCLING only from BouncerConfig.CirclingCheckpoint on.
//     Round 1 rules CONTINUE over any BLOCKING finding or any gating-class finding at MEDIUM or worse.
//     From round 2 on, a BLOCKING finding rules CONTINUE.
//     So does a gating-class finding at MEDIUM or worse on a key the facts file lists as open in an earlier round.
//     A gating finding on a key first raised in the latest round converges.
//     It is carried into the decision record's `## Open risks` through the CarryOver seam.
//     A missing or unparseable earlier ledger puts a parse-error line in the facts file's earlier-open list.
//     Such a line sends the judge back to round 1's rule.
//     A fanned round's facts file (see ClusterFans below) also carries a `## Lenses` section.
//     The section lists the fan and lenses the latest round ran, read from its usage record.
//     It lists the lenses already excluded.
//     It gives the latest review's finding counts per `origin` by severity and class.
//     A missing usage record or an unparseable review renders as a line saying so.
//     A solo segment's file carries no such section.
//     A Go guard (circlingEvidence) backs the prompt: it reads a CIRCLING verdict as CONTINUE, with a warning.
//     The guard applies to a round with no decision file recorded.
//     It applies when the round is below the checkpoint.
//     It also applies when no gating finding is open in this round's ledger and an earlier one.
//     The guard only narrows CIRCLING to CONTINUE and reads only on-disk state.
//     A round that already has a decision file stays as recorded.
//     Roles: the judge and the seed spawn take their skills from agents.yaml (roles `bouncer-judge`, `bouncer-seed`).
//     Both prompts carry the parent directive that parentdirective.Directive renders from BouncerConfig.ParentName.
//     The directive is in its non-interactive form.
//     An empty ParentName renders the no-parent variant.
//     The seed and judge sessions carry the roles bouncer-seed and bouncer-judge.
//     Each role is led by BouncerConfig.RolePrefix and a hyphen when one is configured.
//     The probe that attaches to a session uses the same role.
//     The escalation brief and the parent notice are read by the parent, not by a spawned role, and carry no directive.
//     Escalation: a CONTINUE or CIRCLING round escalates to the run's parent instead of halting for a human.
//     It does so for one of three causes.
//     A fourth cause, decision, escalates a round of any verdict (see Decision escalation).
//     Soft-cap rule: the soft cap is reached when the BouncerConfig.Bounces seam reports count >= budget.
//     Shed applies the same comparison.
//     The rule's window opens on whichever comes first: the soft cap by that count, or the hard cap by the round number.
//     A round past a new generation that reset the count is therefore still settled by the rule.
//     A design-wrong entry is open, BLOCKING or MEDIUM, and not tagged `mechanical` by the judge.
//     A missing or unknown tag counts.
//     A round in the window with no decision file is settled in five steps:
//     a review that does not parse, or a ledger that cannot be read, escalates with cause budget;
//     a ledger with no design-wrong entry records an automatic accept;
//     a round at or past the hard cap escalates with cause hard-cap;
//     a CIRCLING verdict escalates with cause circling;
//     so does a design-wrong key open in this and the previous round, when that decision is a recorded continue;
//     an unreadable previous decision or ledger escalates with cause budget;
//     otherwise an automatic continue is recorded (cause budget, `automatic: true`).
//     That last step returns a budget-exempt Stuck.
//     A CIRCLING verdict below the window escalates with cause circling.
//     The rule reads only files in the run directory, so a harvest and a replay settle alike.
//     The Bouncer renders the bouncer-template-escalation brief and the bouncer-template-parent-notice line.
//     It writes them, with the Go-owned cause frontmatter, as round-<N>-escalation.md and round-<N>-parent-notice.md.
//     It returns Awaiting with the ledger path.
//     The Reason names the segment, round, cause and brief.
//     It also names the `lym loom circling` verbs and the `lym loom resume` resume.
//     The notice is OutputPointer.ParentNotice, which Shed carries as parent_notice.
//     A failed render warns and degrades to the plain Reason with no notice; the record frontmatter is still written.
//     A re-call over an existing record rewrites nothing and returns the same Awaiting.
//     A CONTINUE with no spent budget returns Stuck and takes Shed's generic bounce-budget block.
//     So does a CONTINUE whose Bounces seam is unwired, unknown or errored.
//     A recorded decision is acted on first, whatever the budget.
//     A continue below the hard cap maps to Stuck.
//     The Stuck is marked BudgetExempt when the decision's cause is budget or the budget is reached.
//     A continue past the soft cap therefore spends no budget, whoever recorded it.
//     A pending accept settles its record and maps to Done after Approve and Commit, for any cause.
//     Hard cap: BouncerConfig.HardCap is the perch's hard cap as a round number in the segment's current generation.
//     0 means none.
//     A continue recorded at or past it is moved aside.
//     The escalation record, its parent notice and the decision file each move to a timestamped sibling.
//     No reader opens those siblings.
//     The round re-escalates with cause `hard-cap`, the third cause beside budget and circling.
//     An escalation at or past the hard cap carries the accept-only Reason, whatever its cause.
//     It carries the bouncer-fragment-decide-capped decide step.
//     Below the hard cap, the Reason and the bouncer-fragment-decide-open step offer accept and continue.
//     PendingCirclingDecision, PendingEscalation and PendingEscalationCause read a run directory's latest round.
//     The first reports a decision file not yet settled.
//     The second reports a Bouncer-written escalation that no decision file answers, with its round and brief path.
//     The third reports the same escalation with its cause and brief path.
//     Decision escalation applies to a Bouncer told BouncerConfig.EscalateDecisions.
//     Such a Bouncer escalates a judged round whose review holds a decision-class finding.
//     The escalation has cause `decision`, whatever the judge ruled.
//     A decision-class finding is a question the review cannot settle itself.
//     The arbiter settles it where the board entry and the decision record answer it.
//     The run's parent settles it otherwise.
//     The routing runs first in the settle.
//     It runs ahead of the judgment refusal, the soft-cap rule, the circling escalation and auto-accept,
//     so neither a re-judge nor auto-accept passes it.
//     It takes the round only when the review parses and holds such a finding.
//     It also requires that the round carries no escalation record of another cause.
//     It requires no decision file of another cause and no settled accept.
//     Every other round takes the ordinary route, as does every round of a Bouncer with the switch off.
//     While the escalation is open, a replay returns the same Awaiting instead of clearing a CONVERGED verdict.
//     So does a re-entry over a changed subject, instead of starting a generation.
//     The first escalation writes round-<N>-decision-snapshot.yaml.
//     It holds the titles of the decision record's `Added after Discussion` entries.
//     A re-escalation keeps it.
//     The brief carries the bouncer-fragment-decision detail.
//     The detail lists the decision findings and every BLOCKING finding of the review.
//     It also gives the refusal the recorded answer would meet and the verbs that record it.
//     A recorded continue passes nothing, so only the dropped-key check applies.
//     A refusal re-judges the round.
//     A continue at the hard cap is moved aside and the round escalates again.
//     Below the hard cap the next round starts.
//     A recorded accept faces the judgment refusal.
//     The round's decision-class findings and ledger entries are left out of its BLOCKING checks.
//     The outcome then depends on the refusal and the budget:
//     an admitted accept passes the round;
//     a refused one is re-judged while budget is left;
//     at the budget below the hard cap, it is moved aside and escalated again;
//     at the budget at or past the hard cap, it archives the judge outputs and halts the run.
//     The halt's Reason opens with DecisionHaltPrefix.
//     `lym loom resume` then re-judges the round under the recorded accept.
//     Drift-Verify opens the Reason of its decision halts with the same prefix.
//     arbiterstop hands every halt that opens with it to the parent, ahead of the tackler and the arbiter.
//     An accept's carry-over holds every decision-class finding of the review at any severity.
//     It includes the titles recorded since the snapshot.
//     It also holds every other BLOCKING finding of the review that the ledger does not carry.
//     A finding the judge closed therefore still reaches `## Open risks`.
//     Auto-accept: a Bouncer told BouncerConfig.AutoAccept settles a spent budget as an accept.
//     It does not run the soft-cap rule.
//     It fires at the soft cap alone, also over a CIRCLING verdict.
//     It fires only on a harvest, where the judge wrote the verdict in this call.
//     It fires only over a round with no decision and no escalation record.
//     A replay goes to the soft-cap rule instead.
//     So does a round that BlockingEscalates routes to the rule, meaning an open BLOCKING entry or an unreadable ledger.
//     Auto-accept records the round's decision file as an accept with cause budget and `automatic: true`.
//     It then takes the path a recorded parent accept takes: carry-over, settled record, Approve, Commit, Done.
//     It writes no escalation record and no parent notice.
//     Settling the record keeps its `automatic` value, which a parent's accept never sets.
//     The flag is false by default.
//     A CIRCLING verdict below the budget behaves as with the flag false.
//     So do an already escalated round, a recorded decision and an unknown or errored Bounces seam.
//     Carry-over: a Bouncer told a CarryOver seam calls it on every Done that settles a judged round.
//     The Bouncer also tells it the Segment its entry is filed under and the AnchorPath its paths are relative to.
//     On a CONVERGED settle the seam is called with discussionparser.CarryOverConverged before Approve.
//     On a pending accept it is called with CarryOverAccepted before the settle write.
//     A failed write therefore leaves the accept pending, and its re-call retries.
//     The entry lists the round ledger's open findings at MEDIUM or worse.
//     It also lists the open findings missing a class or severity, labelled `unlabelled`.
//     The findings are sorted by key.
//     Each carries the ledger rounds its key was open in, never the judge-written rounds list.
//     The entry also holds the review and fixer-report paths relative to the anchor.
//     A round with no such finding calls the seam with an empty list, which removes the segment's entry.
//     The skip seam never calls it.
//     No round ran, so the last reviewed generation's entry still describes the artifact.
//     The seam only writes the segment's `## Open risks` entry: it approves nothing and skips no seam,
//     and a failure is returned as the settle's own error with a way forward, so neither Approve nor Commit runs.
//     A nil seam skips the carry-over.
//     Settle records: a Bouncer told a BouncerConfig.Subject seam keeps two records in its run directory.
//     Both are written through internal/state and carried across an archive of the directory.
//     The generation record is written at every seed, generation 1 included, when the directory holds none.
//     A judge spawn writes the live subject, its base and the spawn time into it.
//     The settle record is written on every approved settle, with Settled false.
//     The write comes before the carry-over, Approve and Commit.
//     It is marked settled once Commit returned.
//     The skip seam's approval writes a generation record when none exists and then a round-0 record.
//     A Subject seam that is nil or answers no subject writes no record.
//     The settle record also carries round_started_at, the start of the settled round's review half.
//     It is read from the round's usage record and floored to the second in RFC 3339 UTC.
//     SettleRoundStart reads it from the live record.
//     A settle with no known round start carries none and reads as unknown.
//     That covers a round-0 settle, an absent usage record and a record without the field.
//     Re-entry over a settled verdict depends on the records.
//     A live settle record over an unchanged subject re-settles with no spawn.
//     The re-settle re-runs the carry-over (except for a round-0 record), Approve and Commit, and keeps the settle time.
//     One exception: BouncerConfig.ReviewInput may report an input written after the settle time.
//     Then an amendment generation over the input starts instead.
//     A live record over a changed subject starts a new generation.
//     So does a generation record whose judge subject differs from the live one.
//     A generation record whose judge subject equals the live one settles the verdict as a harvest.
//     With no record at all, the run clears and re-seeds.
//     A record that is not live settles on the round-by-round path and never clears.
//     A new generation archives the run directory with the records travelling along.
//     It writes the generation record with the next number and then seeds.
//     A segment without BouncerConfig.FullGeneration starts an amendment under BouncerConfig.AmendAllowance.
//     The generation record carries the Amendment mark.
//     It carries the base: the settle record's, or the judge base when the judge subject differs from the live one.
//     It carries the review input's path.
//     The input counts when BouncerConfig.ReviewInput reports it written after the settle or judge time.
//     AmendmentNote renders from the record the note that fills the rubric's optional `amendment` marker.
//     The note scopes the seed, the reviewers and the judge to the change:
//     for a document it is the artifacts' change since the base commit;
//     for code it is the stored settle-record.patch against the working-tree diff at the current merge base.
//     An amendment spends the allowance, clamped to the Bounces seam's budget.
//     Its hard cap is the allowance plus one, clamped to HardCap.
//     An allowance of 0 settles it without a review.
//     A full generation carries the number alone and keeps the perch's own budget and hard cap.
//     The generation's seed Stuck is marked GenerationStart, which restarts the row's bounce count.
//     A seed that failed after the archive is repeated by the next Call's seed.
//     The exemption covers only the Bouncer's Stuck.
//     Each further round past the budget needs its own decision, and the count restarts only with a new generation.
//     Every other path reports an empty Path: the seed call, the re-bounce, the clear itself and every degraded path.
//     The re-bounce and degraded paths carry their cause on Reason.
//     The Bouncer reports the ledger path on a CONTINUE Stuck.
//     The ledger is a cross-round artifact a human reads.
//     Hiding it there would hide it when an operator most needs it.
//     A pointer is either an existing file or empty, because Shed never stats a pointer.
//     A pointer naming an unwritten file would be persisted into the history and read as though the artifact were there.
//     A verdict already on disk at Call entry may use the legacy words APPROVED and BLOCKING.
//     They are read as CONVERGED and CONTINUE.
//     That covers the clear, the replay and the review row's may-advance check.
//     A legacy word never settles a harvest.
//     A judge this Call spawned or attached that writes one has its three outputs archived and the round re-judged.
//     The re-judge goes through the round producer's unjudged-round hand-back, at one bounce unit on each row.
//     A judgment the convergence guard refuses (see Limitations) takes the same route.
//     Its Reason names the cause and ends in `the round is re-judged`.
//     A replayed round that carries an escalation record or a circling decision file stands as recorded.
//     Such a round is never refused.
//
// # Told, never derived
//
// Every constructor receives already-resolved absolute paths and already-constructed engines (or a factory over them).
// No adapter calls lymcwd, os.Getwd or git, and no adapter writes the literals _lym or .lym.
// Each constructor also takes a name string.
// The name is used only as a log field and in error text, never compared, parsed or used for control flow.
// Call(ctx) carries no identity of its own.
// Two instances of one adapter type in a producer list are the expected shape.
// MultiLLMProducer additionally takes an injected clock, a nil now defaulting to the real time.Now.
// The injected clock resolves only the archive filename's same-second collision suffix.
// It never feeds Shed's own history[].at field.
// BurlerReviewProducer and BurlerFixProducer are told an absolute run directory and an already-constructed half runner.
// They take the same injected clock, used only for the archive filename's same-second collision suffix.
// MultiLLMProducer is told a seatengine.Table and an already-constructed SeatRunner.
// NewMultiLLMProducerSourced is told a TableSource instead of a table:
// the source builds the table once per Call, so stencils and directives are read at call time.
// Bouncer is told RunDir, StencilsDir, the resolved seed and judge pairs, and the report-name convention as a function.
// NewBouncer is the package's one validating, error-returning constructor.
// The MultiLLMProducer constructors and NewWebsterProducer return a bare pointer;
// seatengine validates a MultiLLMProducer's table downstream.
// NewBouncer returns an error because BouncerConfig carries inputs with real invariants, such as absolute paths.
// Validating lazily at first Call would turn a wiring typo into a mid-run failure in an unattended segment.
//
// # The round-artifact convention
//
// This is the binding two-sided contract between the Burler rows and their segment's Bouncer.
// Artifact paths are flat inside the told run directory, one canonical set per round with no attempt suffix:
// round-<N>-review.md, round-<N>-fixer-report.md and the acceptance record round-<N>-acceptance.md.
// N is a positive decimal integer with no leading zeros.
// A retry writes to the same paths.
// It is a second try at the one artifact the round owes, not a second artifact.
// The presence of all three files means only that round N completed and produced a usable review that was fixed.
// It never means that anyone has judged the round.
// The review row therefore pairs that predicate with a second one before advancing:
// round N's own bouncer verdict and ledger must both exist and parse too.
// Completion alone would let a Bouncer's degraded Stuck buy a whole extra fixer round over a review nobody judged.
// A degraded Stuck is a judge spawn that died or a verdict that did not parse.
// It would also leave round N+1's judge with no round-N ledger to carry findings forward from.
// The Bouncer's own round resolution is narrower:
// ResolveRound stats the review file alone, so the two sides do not run the same test.
// The asymmetry is safe only because of where an orphaned review can appear.
// A process killed between the review row and the fix row leaves the run's current_producer naming one of them.
// That row re-resolves the same round and archives the orphan before the Bouncer is routed to at all.
// The Bouncer must never be entered over an orphaned review: it would judge a review that no fixer round stands behind.
// The next-round directive is round-<N>-focus.md beside the other round files.
// It is YAML frontmatter carrying round, exclude_lenses and focus, over optional prose.
// Its token names the round the directives are for, not the round that produced them:
// a Bouncer rejecting round N writes the file for round N+1, and the seed call writes the file for round 1.
// The file has one filename, one format and one parser.
// The Bouncer renders it, and the Burler review row reads it back through the same pair.
// Only the judge call asks for exclude_lenses, and only when told ClusterExcludes.
// That means its round runs a cluster fan the excludes can trim.
// The seed call and every other judge prompt request focus alone, so the key may be absent from a written file.
// A Bouncer told BouncerConfig.ClusterFans decides per round instead.
// ClusterFans is the partner review row's effective per-round reviewer fan list.
// The judge of round N asks for exclude_lenses only under a condition:
// round N's recorded fan is non-empty and equals the list's entry for round N+1.
// The facts file carries its lens section under the same condition.
// A round whose usage record is missing, does not decode or lacks `fans_recorded` counts as a round that ran no fan.
// The facts file says so in one line, and the judge never fails on it.
// The fanned judge prompt states three rules.
// An exclusion is permanent for the segment generation.
// A lens is excluded only when the latest round ran it and its area is settled.
// An area is settled when the lens raised nothing at MEDIUM or worse and the area is not in the next round's focus.
// Earlier exclusions are never restated.
// The judge's exclude_lenses reach the round's ClusterExclude;
// the file itself reaches the round profile's focus-directive field whenever it carries a directive at all,
// and the explore step reads it there, which is how the judge's targeting reaches the fixer.
// Reading that file is fail-safe end to end.
// It degrades to "no directive" with a warning rather than erroring.
// That includes application time, when a well-formed directive cannot be honoured.
// A segment that has already approved and is entered again does not replay that approval.
// Its Bouncer archives the whole generation aside and re-judges from a fresh round 1.
// Both rows' artifacts move together in that archive.
// Otherwise the review row would resume at round N+1, hydrating from a generation the Bouncer had discarded.
// A fanned round's exclusions accumulate across the generation, held in Go:
// the round's ClusterExclude is the union of the eligible exclude_lenses of focus files 2 through the round;
// ClusterExcludeHeld is the union through the round before.
// An entry in round k's focus file is eligible only when round k-1's usage record meets all of these conditions:
// it carries `fans_recorded`, names the same fan and lists the lens among those it ran.
// A missing or flagless record, a switch from solo or from another fan, or a lens already excluded drops the entry;
// the round's own file draws a warning.
// Round 1 runs the whole fan and drops a focus file's excludes.
// An excluded lens returns only with a new generation.
// A new generation moves the whole run directory aside.
// Every completed round, solo and fanned, also leaves round-<N>-usage.yaml beside the round's other files.
// The record holds the fan and lenses the round ran.
// It holds the fixer fan and, for a twill fixer, its `fix_lenses` and `fix_forks`.
// It holds each half's engine and model, run times, wall time and tokens, forks included.
// An unknown reading stays unknown rather than zero.
// An assisted round also records the assistant's `assist_fan` and the ending its end marker read as `assist_ending`.
// It records as `assist_lenses_missing` each lens a done fanned assistant returned no lens report for.
// A seat that went solo therefore reads as one.
// The record also carries the Bouncer's `seed` and `judge` lists, one entry per pass in the same half shape.
// The Bouncer appends an entry for every pass whose result reached the Call, a judgment the guard refuses included.
// A re-judged round therefore lists every pass it took.
// A pass whose run errored, and a judgment a later Call settles from disk, record nothing.
// Every writer merges into the record on write.
// A record that does not decode is archived aside and replaced by one holding only the writer's part.
// A record without a review half reads as missing for the lens facts and the lens exclusions.
// Only the Bouncer writes to such a record.
// A gate-failed round is archived and writes no usage record.
// A failed write is a warning that never changes the round's outcome.
// The told run directory's whole content is durable record the wiring layer commits.
// So is every timestamped archive the adapters write:
// per-file siblings inside the run directory, and whole-generation siblings beside it (archiveRunDir).
// The adapters therefore write nothing there that is not meant for git.
//
// # The session usage writer
//
// RecordSessionUsage is the one writer of a session's usage record under a run's usage directory.
// When its engine reads transcripts, it prices the session's messages from them;
// otherwise it takes the token counts shuttle read.
// It logs a failure at Warn and never returns an error or stops the run.
// UsageRecorder is the session-end hook a run's runners install.
// It records each ended session under the run's current row.
// The row is read from the status file of the run in the worktree it is told.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict:
// shuttle's OutcomeDone, Webster's "done", or the Bouncer's own harvested verdict.
// Such a verdict is returned as its mapped shedengine.Done or shedengine.Stuck with its pointer.
// Cancellation does not change that.
// The exception exists because a finished success converted into the context error leaves Shed no history entry.
// The next Call would then archive a valid artifact and pay for the same LLM session twice.
// A finished artifact and a paid-for session are never discarded.
// MultiLLMProducer and WebsterProducer apply this rule as stated.
// For the Bouncer, the exception covers a harvested verdict, not only a shuttleengine completion.
// A verdict and ledger that both exist and parse are returned as their mapped outcome and pointer.
// That holds even when the run that produced them reported an error or a non-done outcome.
// It also holds when the run arrived after the context was cancelled.
// The Burler rows follow the narrower rule below.
// No adapter installs a mid-run cancellation bridge.
// None of the engines they wrap exposes a pause seam shaped as a caller-supplied callback.
//
// The Burler rows' cancellation behavior is governed by the seam obligation in internal/shedengine/producer.go:
// every implementation surfaces cancellation as a non-nil error and never as Stuck.
// These producers always error under cancellation, including on a finished half.
// The exception's purpose, never discarding a paid-for artifact, is served instead by an archive carve-out:
// a finished-then-cancelled half keeps its files, so from-disk round resolution advances past it on the next call.
// Only the re-derivable in-memory hand-on is dropped.
//
// # Every spawning adapter probes for a live agent first
//
// Every adapter answers the same question before it starts anything: is an agent for this exact step still alive?
// They answer it in different ways, and the difference is the engine's, not a policy choice here.
// The Bouncer calls shuttleengine's Attach seam with the step's own OutputFiles and waits on a match.
// It does so on its seed pass, on its judge pass, on the re-bounce, and once more at Call entry (see below).
// The review row and the fix row each probe their half through their runner (see below).
// MultiLLMProducer probes its table's seats through its SeatRunner:
// with the chair live it resumes the table.
// Otherwise the probe has already stopped every live advisor, so the archive that follows runs beside no live seat.
// A MultiLLMProducer built with a fresh-spawn preparation runs it on the fresh path only:
// after the probe and before the archive.
// Its error fails the call with nothing archived and no seat started.
// The preparation returns a prompt amendment beside its error.
// The producer sets it on a copy of the table as its Amendment, so the seat engine appends it to the lead's prompt.
// The resume path neither runs the preparation nor amends.
// The preparation cannot be a decorator wrapping the producer.
// A decorator runs before Call, and therefore before the probe.
// It would archive files a live agent is about to write.
// WebsterProducer inherits websterengine's own entry-time reclaim.
// That reclaim stops a leftover Master rather than attaching to it.
//
// Every mode probes, the re-bounce included.
// The re-bounce, an already-seeded segment whose round producer handed back without a report, spawns nothing.
// A parsing round-1 focus file proves the seed agent wrote its one declared output, never that it exited.
// Shuttle's Wait polls for bare existence at that path;
// a driver killed between the write and the exit lands on that branch with a live seed still holding the file.
// Returning Stuck without waiting would abandon that seed.
// The segment's round producer would then read a file the seed might still be rewriting.
// The re-bounce therefore runs the same probe the seed pass runs.
//
// The probe always runs before the archive, in every probing adapter.
// Archiving renames the files a live agent is about to write.
// Shuttle's Wait polls for bare existence at those paths;
// archiving first would make an attached run unable to classify done, the case the probe exists to protect.
//
// A Burler row's probe is one-stranded:
// a row drives one half, a reviewer or a fixer, each declaring its own single output file.
// BurlerHalfRunner.ProbeReview and ProbeFix report the half as live, done or gone, and Call acts on it.
// A live half is resumed through ResumeReview or ResumeFix,
// and the result is mapped through the outcome switch a spawned attempt uses.
// A died or timeout result there falls through to a fresh attempt 1.
// A resumed review that fails with the fork-count sentinel falls through the same way.
// The resumed half is not counted as an attempt.
// A half that is done or gone is nothing live, so nothing is removed and it falls through to attempt 1.
// A fall-through spawn archives the half's outputs first, so a half is never spawned a second time beside a live one.
// A probe error and a resume error wrapping burlerengine.ErrHalfNotStopped are returned without archiving or spawning.
// A half that may still be live may be writing the round's files.
// A failed stop wraps burlerengine.ErrHalfNotStopped.
// Its message ends with the way forward: run "lym reed remove <guid>" and re-step the row.
//
// The Bouncer's third probe covers the two modes that spawn nothing at all.
// It exists because its judgment record is narrower than the judge spawn that produces it:
// a recorded judgment is a verdict and a ledger, while the spawn declares those two plus the next round's focus file.
// A driver crash between the ledger write and the focus write leaves a live judge behind an apparently-final verdict.
// The next Call then lands in the clear branch or the replay branch.
// One of them archives the run directory out from under that judge;
// the other writes a synthetic focus file at a path it declared as an output.
// Both then hand back a Stuck.
// The next respawn could not attach to the survivor either:
// the seed spec and the round producer's spec each name a different OutputFiles set than the judge's.
// So Call probes on the judge spec's own three paths before either branch acts:
// attaching makes that call the judgment's harvest, settling it rather than clearing it,
// and a not-found probe leaves both branches to act on the state on disk.
//
// The Bouncer and the Burler rows probe because a driver crash inside any review segment leaves the round's agent alive.
// Without a probe the next Call would spawn a second agent over it:
// two sessions would write one review and one fixer report;
// on a fix-scope: source row, two sessions would commit to one branch.
// Neither run could then continue without an operator deleting a pane by hand.
//
// # The review row and the fix row
//
// BurlerReviewProducer and BurlerFixProducer split a review segment's round into two Shed rows.
// Each drives one half of a burlerengine round through BurlerHalfRunner.
// Neither returns Done; a finished half is a routine Stuck hand-off:
// the review row's to the fix row, and the fix row's to the segment's Bouncer.
// The round's acceptance record is round-<n>-acceptance.md in the run directory.
// It joins the round's review and fixer report as one of the round's paths.
// It holds the round and the hex SHA-256 of the accepted review in YAML frontmatter.
// Only the review row writes it, after the review passed the strict parse.
// A record that is absent or does not parse reads as absent.
//
// The review row resolves the round from the highest complete round.
// It hands on BudgetExempt without spawning in these cases:
// the highest complete round has no recorded verdict;
// the next round's review already carries an acceptance record.
// A restart with the fixer pending therefore resumes at the fix row.
// Otherwise it probes for a live reviewer before archiving anything, and it resumes one it finds.
// It spawns at most two attempts:
// the second follows only a died or timed-out first;
// it also follows a first whose chair spawned a fork count other than requested (burlerengine.ErrClusterForksMissing).
// A fork-count mismatch on the second attempt is a hard failure.
// It names both attempts' errors, the chair's strand and its session states, so the halt note says how the chair ended.
// Any other fork violation is a hard failure at once.
// A row told BurlerHalfDeps.Fans picks the round's reviewer and fixer fans from the per-round lists:
// entry N serves round N, and the last entry serves every later round.
// A nil Fans runs the profile's own cluster fan and records no names.
// Without a live reviewer, `recordRoundFans` writes these keys to the round's usage record before the first attempt:
// `fan`, `fix_fan` and `fans_recorded: true`.
// An empty name is written for a solo or single round.
// It is the only writer of those keys, and a failed write is a failure exit that spawns nothing.
// The fix half, a resumed reviewer and the Bouncer read the names through `readRecordedFans`.
// The `fans_recorded` key, never the `fan` key, says the names are there.
// A live reviewer is probed and resumed under the fan already recorded for its round.
// A list edited since therefore changes only rounds spawned after the edit.
// A row told BurlerHalfDeps.Assist picks the round's assistant reviewer and its fan from the per-round lists.
// An entry with no model runs no assistant.
// The row passes the assistant's key, capped timeout and skip reason on.
// Before each attempt spawns, `recordRoundAttempt` writes to the round's usage record:
// the attempt token, which is the round number, or the round number and b on the retry;
// and the assistant choice and fan.
// A failed write is a failure exit that spawns nothing.
// The probe builds its whole table, chair included, from that record through `readRoundAttempt`.
// A probe after lym dies mid-retry therefore finds the retry's chair and assistant.
// A record without the token reads as the bare round with no assistant, whatever the config now says.
// The copy of a done assistant's report beside the review joins the files archived before a retry or a new round.
// An accepted review writes its record and merges the lenses and the half's usage into the round's usage record.
// The recorded names stay as they are.
// The row then hands on with the review as the pointer's path.
// Every other end is a hard error that archives the round's review, fixer report and acceptance record.
// That includes a gate or strict-parse failure.
// A reviewer that could not be stopped returns bare, with nothing archived.
// A review with no record beside it is an orphan and is reviewed again.
// A row told BurlerHalfDeps.Parent asks the run's parent orch to review the round before the probe and before any spawn.
// The ask lives in burlerengine's never-tracked ask record.
// The row finds the round's ask by the generation's seed, the write time of its round-1 focus file, and by the round.
// A round already holding an ask is posted again only while that ask is pending-post, whatever the key now reads.
// A round holding none sends a new ask only with `discussion_review_ask_parent` on, and only when the round asks.
// A round asks when the run has no ask of this seed.
// It also asks when the history records a Discussion-Write call after the latest ask.
// It also asks when the round before ended done with findings, or an earlier round's ask was not-sent.
// The ask's one line opens `[discussion-review request]`.
// It goes to the strand resolved once, signed as the loom program.
// A wait the chair's timeout leaves no room for records the ask not-sent;
// an unresolved parent, a parent with no orch strand and a failed post record it no-parent;
// in every case the round goes on.
// The round's table is told the ask's number.
// Its chair then consolidates the parent's report, and its gate covers each report id.
// An accepted review, and the hand-on of a review accepted before a restart, settle the ask:
// an ask still open is set timeout, and a done report is copied beside the review once.
// The copy joins the files archived before a retry or a new round.
// A fix row told BurlerHalfDeps.NotifyParent sends the accepted review's decision-class findings to the run's parent.
// The findings go in one line.
// Each finding carries its id, summary and fix.
// The row sends the line before a fresh fix starts, and it runs the fix with burlerengine's DecisionsRouted set.
// The line is sent once per call that starts a fresh fix and never on the resume of a live fixer;
// a review that does not parse and a failed post are logged at Warn and never fail the round.
// A row told no notifier sends nothing and keeps every finding as fixer work.
// A row told BurlerHalfDeps.DecisionsEscalated posts nothing even with a notifier.
// Its segment's Bouncer escalates the decisions itself.
// It still runs the fix with DecisionsRouted set, so the Bouncer's escalation is the one parent message per round.
//
// The fix row hands on an unjudged highest round the same way.
// Otherwise it requires the next round's acceptance record and review.
// Without them it is a hard error whose way forward is a goto to the review row and a resume.
// The review row's hand-on never produces that state.
// The state arises only from a goto into the fix row with no accepted review.
// It also arises from a resume after the fix row archived a review that changed since acceptance.
// It probes for a live fixer first and resumes it against the recorded digest.
// It then spawns at most two attempts under that digest.
// A row told BurlerHalfDeps.RequiresFans runs the fixer fan recorded for the round.
// A list edited between the halves cannot reach it.
// A round's record may hold no names, and no live fixer may be found.
// The row then archives its review, fixer report and acceptance record.
// It refuses the round with the goto to the review row, which spawns a fresh reviewer that records them.
// A live fixer over such a record is resumed as the single fixer it was spawned as,
// and a row not told RequiresFans reads no names and runs the single fixer.
// A finished fix, a skipped empty review included, merges the fix half's usage.
// It also merges a twill fixer's lenses and fork count.
// It hands on BudgetExempt with the review as the pointer's path.
// A finished fix whose gate failed archives the review, fixer report and acceptance record.
// It returns a counted Stuck with an empty path, so the Bouncer's next round reviews afresh.
// A review that changed after acceptance archives the same files and ends with the same way forward.
// Any other failure archives the fixer report alone.
// The accepted review and its record stay, so a resume re-spawns the fixer without re-running the reviewer.
//
// # The Command producer
//
// CommandProducer wraps no engine: it runs one shell command, once, for side actions a run must never halt over.
// A row that should run only when a setting is on carries that setting as its entry gate,
// so the producer reads no switch and runs its command whenever it is called.
// The recipe's owner tells every seam through CommandDeps, so the producer derives no path.
// Call follows this order:
//
//  1. A nil ExecutableSetting is an error naming its way forward, before anything is read or run.
//  2. The command is read through ExecutableSetting.
//     The owner builds it over the template and board default layer only.
//     An unreadable or non-string command is an error.
//     It names the key and the correction `lym config <module> --layer default --set <key>=<value>`.
//     When Setting answers a different non-empty command for the key, the row warns that value is ignored.
//  3. An empty command is Done with no process.
//  4. The placeholders {lym}, {slug}, {task_dir} and {title} are replaced.
//     Each value is quoted by verifyrun.QuoteArg for the shell the command runs under.
//     Any other brace text, ${HOME} included, passes through literally.
//     TaskDir and Title are each called only when their placeholder appears.
//     A value cmd cannot quote, one holding a percent sign, makes the row Done with no process.
//     The row warns, naming the placeholder and its way forward.
//  5. The command runs once through Run, in Dir with the row's log path and timeout.
//     An exit of 0 is Done.
//     A non-zero exit, a timeout and a runner error are each Done with a warning naming the log path.
//     A cancelled context returns the cancellation error.
//
// Bound: whatever command the template or the board default layer holds runs with the operator's rights.
// The command is read only through ExecutableSetting, never through a layer a pair can write.
// It runs once, and the row's timeout ends a hang.
// No command outcome halts the run; only an unreadable setting or a missing executable-key lookup stops it.
//
// # Limitations
//
// Neither MultiLLMProducer nor WebsterProducer installs a mid-run cancellation bridge,
// so a cancel is observed only once the run reaches a terminal outcome or its own configured deadline elapses:
// the table's own timeout for MultiLLMProducer, and Webster's own whole-run timeout for WebsterProducer.
// The Burler rows and Bouncer likewise install no mid-run cancellation bridge.
// The Burler rows install none because burlerengine exposes no pause seam,
// so a cancel is observed only once the half reaches a terminal outcome or its own RunOpts.Timeout elapses.
// The Bouncer installs none for similar reasons at the shuttleengine layer.
// The Shared cancellation rule above covers both.
//
// The Bouncer does not trust the judge alone with the rules it states:
// a CONVERGED with an open BLOCKING ledger entry or a BLOCKING or unparseable review is archived;
// it is re-judged within the bounce budget.
// So is any verdict whose ledger drops a key of the previous round's ledger.
// Only keys are compared across ledgers, never status, class or severity.
package shedadapters
