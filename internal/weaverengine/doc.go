// Package weaverengine is Weaver, the `weaver` implementer of tapestry's implementer row.
// Weaver builds a plan's batches as cold strands and climbs a model ladder when an attempt fails.
//
// Its `weaver.yaml` keys follow.
// `weaver.ladder` holds the model-specs a batch attempt climbs in order.
// `weaver.attempt_timeout_min` is the minutes one batch attempt's strand may run, its gate waits and fixers included.
// A strand that a crashed call left gets those minutes again from the attach.
// `weaver.fixer_timeout_min` is the minutes a plan-level verify fixer may run.
// `weaver.verify_gate` is the gate key of the plan-level verify its Done rests on.
// The local override layer cannot change `weaver.verify_gate`, since a narrower key would hollow that guard.
//
// Weaver's durable run directory is `_lym/weaver/`.
// It holds the state file, the per-attempt reports, `journal.md`, `summary.md` and `outcome.yaml`.
// Its prompts and locks sit under `.lym/weaver/`.
//
// An attempt's report path is fixed before its strand spawns, so a crash leaves the strand's identity on disk.
// Every attempt and every verify fixer has its own output file, so a probe finds a strand's run by that file alone.
// The journal appends one entry per attempt, and a heading already in it makes a second append a no-op.
//
// One batch attempt is one cold strand started through StrandRunner, with a gate entry per card and one for the batch.
// Each entry runs its `lym gate test` call through `verifytree.Verify`, so the agent fixes its own gate failures.
// A waited attempt is judged into one verdict.
// An OK report with every gate passed is recordable.
// A FAILED report, a gate that still fails, a timeout and a missing or unparseable report are failures.
// A dead strand, a provider API error and a wait error from reed are machinery losses.
// A NEEDS_PARENT report stops the run for its need.
//
// Every attempt that ends, whatever its verdict, is recorded the same way:
// its outcome and cause, the batch's end commit and the report's journal entry are saved and committed.
// A recordable attempt first passes the clean-tree check, the head check and the shared post-batch checks of `runplan`.
// A pass takes the batch's record terminal done.
// Any other finding fails the attempt, and the next attempt is told it.
//
// After the last batch, the plan-level verify runs the hub's verify gate call through `verifytree.Verify`.
// A failure is rerun once, and a pass on the rerun is flaky.
// A failure that survives goes to one cold verify fixer on the next ladder rung.
// Weaver runs the verify again after it.
// The fixer carries no gates, since the re-run of the verify judges it.
// Run returns one gate span per run of the verify and one rework span per fixer, which the row records as gate spans.
//
// Run is the row's producer body, holding `run.lock` in the scratch directory for its whole call.
// A failed attempt climbs a rung, and a lost one is retried once on its rung first.
// A failure on the last rung, a needs-parent report and a verify that no fixer clears each stop the row.
// Every stop is returned as a stuck result.
// Only a Done return writes `outcome.yaml` and `summary.md`, and Run removes both on entry.
// The summary names each batch's files changed outside its targets and its `Pattern-Departure:` trailers.
//
// A call of Run settles what an earlier call left before it begins any batch.
// An attempt recorded in flight is probed by its report path, even when its strand's guid was never recorded.
// A run not found carries no gate verdict and counts as a machinery loss.
// The open batch then continues from its last attempt, which a crash never resets.
//
// A call that finds a stop record re-enters it.
// The recorded partition is mapped onto the plan, which refuses a plan that lost or gained a card,
// and the plan baseline is restamped, so an edit to a card carries into the next attempt.
// A needs-parent stop starts again on the rung that asked, and a verify stop runs the verify again.
// A stopped batch whose checks and batch gates pass at HEAD is recorded done with no spawn.
// A row with gate entries of its own never takes that path.
// Any other stopped batch, or a verify that fails again, gets one attempt or fixer on the last rung.
// The stop record is cleared the moment a batch is recorded done or the verify passes.
//
// Reset starts a run over for `lym weaver reset`.
// It refuses while the run lock is held, over a state with no begun batch, and during a merge.
// It also refuses over live strands of the run unless told to stop them, and over tracked paths the run did not leave.
// A reset-pending marker in the scratch directory stands from before the move until the commit lands,
// so a reset whose commit failed is finished by running it again.
//
// Active and ActiveOrSetting are the one seam every loom reader of implementer state goes through.
// The implementer holding begun batches is the active one.
// The `tapestry.implementer` setting decides when neither does.
// A reader that must stop on a refusal takes Active, and a reader that only advises takes ActiveOrSetting.
// Webster's and Weaver's batches never mix in one run, so a run whose two state files both hold begun batches is refused.
package weaverengine
