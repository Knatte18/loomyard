// Package shedadapters holds the shedengine.ShedProducer adapters for a Shed-built product's flat producer list.
// Each adapter lets Shed drive one engine as an ordinary producer, as a thin translation layer over that engine.
// WebsterProducer wraps one websterengine multi-spawn run.
// WeaverProducer wraps one weaverengine run.
// BurlerReviewProducer and BurlerFixProducer each wrap one half of a burlerengine round.
// MultiLLMProducer wraps one seatengine table as a single Shed row.
// Bouncer is the generic review-gate producer and the exception:
// it is new logic over shuttleengine and composes its own prompt from stencils.
// CommandProducer wraps no engine: it runs one shell command once, for side actions a run must never halt over.
//
// # Outcome mapping
//
// Each adapter maps its own verdict onto Done or Stuck.
// The Bouncer alone also maps onto Awaiting, the hand-off to the run's parent.
// It does so when its judge finds the review circling or its bounce budget is spent.
//
//   - WebsterProducer and WeaverProducer: the engine's "done" outcome is Done with the engine's summary path.
//     Its "stuck" outcome and its run-entry refusals are Stuck.
//     A Stuck from a "stuck" outcome consults the plan-conflict record and may ask for a reroute (OutputPointer.Reroute).
//     Any other outcome value is an engine-level error.
//     WebsterProducer records every Stuck's final reason in the conflict directory (runplan.WriteHaltReason),
//     and its Done removes the record.
//     The plan segment's review row reads it, so the reviewer sees why the run was sent back.
//     A failed write or removal is logged and never changes the outcome.
//   - BurlerReviewProducer and BurlerFixProducer: a finished half maps to Stuck, never Done.
//     That Stuck is a routine hand-off, not a stuck condition:
//     the review row hands to the fix row, and the fix row hands to the segment's Bouncer.
//     Only the judge decides that a round is finished.
//     Hand-ons are BudgetExempt, so the Bouncer's own budget check is the segment's one budget.
//     A non-done shuttle outcome that survives the bounded retry is an engine-level error.
//   - MultiLLMProducer: the chair's done outcome is Done with the chair's first output as the path.
//     A done chair whose gate failed is Stuck.
//     A died or timed-out chair is an engine-level error wrapping shuttleengine.ErrNotStarted or ErrAgentDied.
//     A writer seat that ended the step decides it instead of the chair.
//     Every such Stuck reason ends with the way forward of re-running the row with `lym loom resume`.
//   - Bouncer: the judge's verdict is CONVERGED, CONTINUE or CIRCLING.
//     CONVERGED maps to Done only on the harvest that earns it, within the Call that produced it.
//     CONTINUE maps to Stuck, and CIRCLING to Awaiting.
//     All three report the round's ledger path.
//     Every other path reports an empty Path.
//
// A pointer is either an existing file or empty, because Shed never stats a pointer.
// A gated producer is a gate carrier: its pointers carry the gate count and spans of the run.
// A gated producer told a findings path keeps an exhausted gate's findings in it, and its Stuck reason names the file.
// A later call whose gate passed leaves the file in place, so a halt never names a removed file.
//
// # Told, never derived
//
// Every constructor receives already-resolved absolute paths and already-constructed engines (or a factory over them).
// No adapter calls lymcwd, os.Getwd or git, and no adapter writes the literals _lym or .lym.
// Each constructor also takes a name string, used only as a log field and in error text.
// Call(ctx) carries no identity of its own, so two instances of one adapter type in a producer list are expected.
// An injected clock resolves only the archive filename's same-second collision suffix.
// It never feeds Shed's own history[].at field.
// NewBouncer is the one validating, error-returning constructor:
// validating lazily at first Call would turn a wiring typo into a mid-run failure in an unattended segment.
//
// # The round-artifact convention
//
// This is the binding two-sided contract between the Burler rows and their segment's Bouncer.
// Artifact paths are flat inside the told run directory, one canonical set per round with no attempt suffix:
// round-<N>-review.md, round-<N>-fixer-report.md and the acceptance record round-<N>-acceptance.md.
// N is a positive decimal integer with no leading zeros.
// A retry writes to the same paths.
// The presence of all three files means only that round N completed with a usable, fixed review.
// It never means that anyone has judged the round.
// The review row therefore also requires round N's bouncer verdict and ledger to exist and parse before it advances:
// otherwise a Bouncer's degraded Stuck would buy an extra fixer round over a review nobody judged.
// The Bouncer's own ResolveRound stats the review file alone.
// That asymmetry is safe: a row that finds an orphaned review archives it before the Bouncer is routed to.
// An orphaned review is one left by a process killed between the two rows.
// The Bouncer must never be entered over an orphaned review.
//
// The next-round directive is round-<N>-focus.md, YAML frontmatter carrying round, exclude_lenses and focus.
// Its token names the round the directives are for: a Bouncer rejecting round N writes the file for round N+1,
// and the seed call writes the file for round 1.
// Reading it is fail-safe: it degrades to "no directive" with a warning rather than erroring.
// A fanned round's lens exclusions accumulate across the segment generation and return only with a new generation.
// Every completed round also leaves round-<N>-usage.yaml and round-<N>-snapshot.json beside its other files.
// A failed write of either is a warning that never changes the round's outcome.
//
// The told run directory's whole content is durable record the wiring layer commits.
// So is every timestamped archive the adapters write beside or inside it.
// The adapters therefore write nothing there that is not meant for git.
//
// # Shared cancellation rule
//
// Every adapter checks ctx.Err() at Call entry and returns immediately without starting anything.
// On exit, a cancelled context replaces every result except a genuine success verdict:
// shuttle's OutcomeDone, Webster's "done", or the Bouncer's own harvested verdict.
// Such a verdict is returned as its mapped outcome with its pointer.
// Discarding it would leave Shed no history entry and make the next Call pay for the same LLM session twice.
// A harvested verdict is a verdict and ledger that both exist and parse.
// It is returned even when the run that produced them reported an error.
//
// The Burler rows follow the seam obligation in internal/shedengine/producer.go instead:
// they always surface cancellation as a non-nil error, never as Stuck, including on a finished half.
// A finished-then-cancelled half keeps its files, so from-disk round resolution advances past it on the next call.
//
// No adapter installs a mid-run cancellation bridge.
// A cancel is therefore observed only once the wrapped run reaches a terminal outcome or its own configured deadline.
//
// # Every spawning adapter probes for a live agent first
//
// Every adapter asks whether an agent for this exact step is still alive before it starts anything.
// A driver crash leaves the round's agent alive, and without a probe the next Call would spawn a second agent over it:
// two sessions would write one review, and on a fix-scope: source row two sessions would commit to one branch.
// The probe always runs before the archive.
// Archiving renames the files a live agent is about to write and would make an attached run unable to classify done.
// A live agent is resumed or waited on; a done or gone one falls through to a fresh spawn, archiving first.
// An error wrapping burlerengine.ErrHalfNotStopped or seatengine.ErrSeatNotStopped returns without archiving or spawning,
// and its message ends with the way forward.
// A MultiLLMProducer's fresh-spawn preparation runs only on the fresh path, after the probe and before the archive,
// so it cannot be a decorator, which would run before the probe.
// WebsterProducer inherits websterengine's own entry-time reclaim, which stops a leftover Master rather than attaching.
//
// The Bouncer probes on its seed pass, its judge pass, the re-bounce and once more at Call entry.
// The re-bounce spawns nothing, but a parsing round-1 focus file proves only that the seed wrote its output;
// it does not prove that the seed exited, so returning Stuck without waiting could abandon a live seed.
// The entry probe uses the judge spec's own three output paths:
// attaching makes that call the judgment's harvest, rather than clearing or replaying over a live judge.
//
// # The review row and the fix row
//
// BurlerReviewProducer and BurlerFixProducer split a review segment's round into two Shed rows,
// each driving one half of a burlerengine round through BurlerHalfRunner.
// The acceptance record holds the round and the SHA-256 of the accepted review in YAML frontmatter.
// Only the review row writes it, after the review passed the strict parse.
// A record that is absent or does not parse reads as absent.
//
// The review row spawns at most two attempts, the second only after a died, timed-out or fork-count-violating first.
// An accepted review writes its record and hands on BudgetExempt with the review as the pointer's path.
// Every other end is a hard error that archives the round's review, fixer report and acceptance record.
// The exception is a reviewer that could not be stopped, which returns bare with nothing archived.
// A review with no acceptance record beside it is an orphan and is reviewed again.
// The fix row requires the next round's accepted review.
// Without one it is a hard error whose way forward is a goto to the review row and a resume.
// A finished fix hands on BudgetExempt.
// A finished fix whose gate failed archives the round's files and returns a counted Stuck with an empty path,
// so the Bouncer's next round reviews afresh.
// Any other fix failure archives the fixer report alone, so a resume re-spawns the fixer without the reviewer.
//
// Reviewer and fixer fans are recorded in the round's usage record before the first attempt, via `fans_recorded`.
// A later edit of the per-round fan lists therefore changes only rounds spawned after the edit.
//
// # The Bouncer
//
// The Bouncer does not trust the judge alone with the rules it states.
// A CONVERGED with an open BLOCKING ledger entry is archived and re-judged within the bounce budget.
// So is a CONVERGED over a BLOCKING or unparseable review.
// So is any verdict whose ledger drops a key of the previous round's ledger.
// Only keys are compared across ledgers.
// A verdict file must carry exactly CONVERGED, CONTINUE or CIRCLING; any other word reads as no judgment.
// Call acts on a judgment that provably happened: a verdict and ledger that both exist and parse.
// It acts regardless of what the shuttle run itself reported.
//
// A CONTINUE or CIRCLING round escalates to the run's parent instead of halting for a human.
// The causes are budget, circling, hard-cap, decision and stalemate; BouncerConfig names the seams that enable each.
// An escalation writes round-<N>-escalation.md and round-<N>-parent-notice.md and returns Awaiting with the ledger path.
// A re-call over an existing record rewrites nothing and returns the same Awaiting.
// A recorded decision is acted on first, whatever the budget.
// A pending accept settles its record and maps to Done after Approve and Commit.
// A continue below the hard cap maps to Stuck, BudgetExempt when the cause is budget or the budget is reached.
// The escalation rules read only files in the run directory, so a harvest and a replay settle alike.
// A pass that settles a judged round calls the CarryOver seam, which only writes the segment's `## Open risks` entry:
// it approves nothing, and a failure is the settle's own error, so neither Approve nor Commit runs.
// A decision halt is a recorded accept held by a refusal at the budget and the hard cap.
// It archives the judge outputs and halts the run with a Reason opening with DecisionHaltPrefix.
// Drift-Verify opens the Reason of its decision halts with the same prefix.
// arbiterstop hands every halt that opens with it to the parent, ahead of the tackler and the arbiter.
//
// Re-entry over a settled verdict depends on the generation and settle records written through internal/state.
// A live settle record over an unchanged subject re-settles with no spawn.
// A changed subject starts a new generation, which archives the run directory with its records.
// A segment without BouncerConfig.FullGeneration starts an amendment under BouncerConfig.AmendAllowance instead.
// With no record at all, the run clears and re-seeds.
//
// # The round context
//
// BuildRoundContext returns what a round's review is judged against, from round N-1's files in the run directory.
// RoundContextSources names what it reads beyond the run directory; an empty path or a nil reader reads nothing.
// The context's AmendingTexts holds the full text of every added entry whose title the plan amendment record lists.
// That includes entries added before the round's start.
// The caller reads those titles through the record's own reader and passes the reader in.
// The context's HaltReason holds the plan executor's last stuck reason from the conflict directory.
//
// # The Command producer
//
// CommandProducer runs one shell command once, through CommandDeps, and derives no path.
// The command is read only through ExecutableSetting, over the template and board default layer,
// so it runs with the operator's rights and never from a layer a pair can write.
// An empty command, or a value cmd cannot quote, is Done with no process.
// A non-zero exit, a timeout and a runner error are each Done with a warning naming the log path.
// Only an unreadable setting or a missing executable-key lookup halts the run, and a cancelled context returns its error.
package shedadapters
