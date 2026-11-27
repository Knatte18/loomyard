// Package weaverengine is Weaver, the `weaver` implementer of tapestry's implementer row.
// Weaver builds a plan's batches as cold strands and climbs a model ladder when an attempt fails.
//
// Its `weaver.yaml` keys are `ladder`, `fixer_timeout_min` and `verify_gate`.
// `ladder` holds the model-specs a batch attempt climbs in order.
// `fixer_timeout_min` is the minutes a plan-level verify fixer may run.
// `verify_gate` is the gate key of the plan-level verify its Done rests on.
// The local override layer cannot change `verify_gate`, since a narrower key would hollow that guard.
//
// Weaver's durable run directory is `_lym/weaver/`.
// It holds the state file, the per-attempt reports, `journal.md`, `summary.md` and `outcome.yaml`.
// Its prompts and locks sit under `.lym/weaver/`.
//
// The state file, `state.json`, embeds the shared run-plan state.
// Beside it sit Weaver's attempts, fixers, batch end commits and stop record.
// An attempt's report path is fixed before its strand spawns, so a crash leaves the strand's identity on disk.
// Every attempt and every verify fixer has its own output file, so a probe finds a strand's run by that file alone.
// The journal appends one entry per attempt, and a heading already in it makes a second append a no-op.
//
// RenderBatchPrompt renders the prompt of one batch attempt from the `weaver-template-batch` stencil.
// The attempt is a cold strand, so the prompt carries its whole orientation:
// the card pointers with their card gates, the batch gate, the decision record, the run journal and the report path.
// It also carries the failure digest and uncommitted paths of a retry.
// The PATTERN, parent, edit and friction directives close it.
// The edit directive is the copy for the engine the rung's model choice launches on, as is the verify fixer's.
// The plan-level verify fixer's body is the `weaver-body-verify-fix` stencil.
// Gatefixer joins that body behind its cold frame.
//
// One batch attempt is one cold strand started through StrandRunner, with a gate entry per card and one for the batch.
// Each entry runs its `lym gate test` call through `verifytree.Verify`, so the agent fixes its own gate failures.
// When the geometry tells a `ReachPending` source, the batch entry runs under `verifytree.WithReachTurn`.
// A writer whose batch gate passes then gets one turn for each unreached function of its reach report.
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
// The state save keeps every heading the decision verb's process recorded as delivered.
// RecordDelivery is how that process records one:
// it adds the headings to the attempt or fixer whose strand name matches, under the state-mutation lease.
//
// After the last batch, the plan-level verify runs the hub's verify gate call through `verifytree.Verify`.
// A failure is rerun once, and a pass on the rerun is flaky.
// A failure that survives writes the verify-failure record and goes to one cold verify fixer on the next ladder rung.
// The record carries the triage lines the verify log's failure report holds under its failed tests.
// Weaver runs the verify again after it.
// The fixer carries no gates, since the re-run of the verify judges it.
// RunDeps is everything the row's producer is told.
// Run is the row's producer body, holding `run.lock` in the scratch directory for its whole call.
// It begins each batch under the state-mutation lease, runs the batch's attempts up the ladder, then runs the verify.
// A failed attempt climbs a rung, and a lost one is retried once on its rung first.
// A failure on the last rung, a needs-parent report and a verify that no fixer clears each stop the row.
// Every stop is returned as a stuck result.
// Its reason names the plan, the state file, the last report, the journal and the decision record.
// Only a Done return writes `outcome.yaml` and `summary.md`, and Run removes both on entry.
// `summary.md` lists the batches, the flaky identities and each fixer's findings and `fix:` commits.
//
// A call of Run settles what an earlier call left before it begins any batch.
// The strand of every attempt and fixer whose outcome is recorded is stopped first.
// An attempt recorded in flight is probed by its report path, even when its strand's guid was never recorded.
// A run found is sent the decisions it lacks and waited on, and its result is recorded as an attempt's end is.
// A run not found carries no gate verdict and counts as a machinery loss.
// A fixer whose record is live is attached to, and a fixer recorded without one is probed by its reply path.
// The open batch then continues from its last attempt, which a crash never resets:
// a failure starts the next attempt on the next rung, and a lost attempt retries once on its rung.
//
// A call that finds a stop record re-enters it.
// The recorded partition is mapped onto the plan, which refuses a plan that lost or gained a card,
// and the plan baseline is restamped, so an edit to a card carries into the next attempt.
// A needs-parent stop starts again on the rung that asked, and a verify stop runs the verify again.
// A stopped batch whose checks and batch gates pass at HEAD is recorded done with no spawn.
// The checks are the clean tree and the post-batch checks.
// A row with gate entries of its own never takes that path.
// Any other stopped batch, or a verify that fails again, gets one attempt or fixer on the last rung.
// The stop record is cleared the moment a batch is recorded done or the verify passes.
//
// Reset starts a run over for `lym weaver reset`.
// It refuses while the run lock is held, over a state with no begun batch, and during a merge.
// It also refuses over live strands of the run unless told to stop them, and over tracked paths the run did not leave.
// Once it has stopped a strand, every tracked path left uncommitted counts as the run's own.
// It moves the task branch to the run's start commit and restores the plan to its content at the recorded records commit.
// It archives the run record in place under a UTC stamp and clears the scratch directory but its locks.
// It then commits the restore and the record together.
// A reset-pending marker in the scratch directory stands from before the move until the commit lands,
// so a reset whose commit failed is finished by running it again.
// ArchiveRunRecord moves Weaver's whole run directory into a directory the caller names.
// RunInFlight reports a run in flight while its state file exists and its outcome file is absent or not done.
// ArbiterNote renders the sentence that names an implementer's run files for the arbiter.
//
// Active and ActiveOrSetting are the one seam every loom reader of implementer state goes through.
// The implementer holding begun batches is the active one.
// The `tapestry.implementer` setting decides when neither does.
// A reader that must stop on a refusal takes Active, and a reader that only advises takes ActiveOrSetting.
// RunDirs names an implementer's run and scratch directories, so a reader holds no switch of its own.
// Webster's and Weaver's batches never mix in one run, so a run whose two state files both hold begun batches is refused.
package weaverengine
