// Package websterengine is the domain kernel behind webster, a fork-based implementer loop.
// One long-lived Master session reads the plan's overview and the repo's rules once, and never the code.
// It then forks one implementer per execution batch in-session (Claude Code's Agent tool, subagent_type "fork").
// Each fork reads the code its cards need itself.
// Forks run sequentially, one at a time in one worktree, in an order that follows the cards' declared dependencies.
// websterengine holds no loop itself;
// the loop is the Master session driving fat `lym webster` verbs (internal/webstercli).
// This package provides only those verbs' logic plus the distillation behind them.
//
// Every refusal this package returns ends in a trailing `way forward:` clause.
// contracts/specs/refusal-spec.md defines that clause.
//
// # Plan consumption
//
// webster reads the pinned flat card-list plan format (see contracts/specs/loom-plan-spec.md).
// internal/planparser is the sole parser of the on-disk `_lym/plan/` tree.
// RenderForkPrompt and RenderRecoveryPrompt take no *planparser.Plan;
// both render a card's content from its SourcePath pointer, per the fork-context-hygiene Shared Decision.
// internal/batcher groups the plan's flat, unordered cards into execution batches.
// internal/webstercli hands the resolved batcher to Run via RunDeps.Batcher.
// BatchState is keyed by execution-batch number, not card number:
// a grouping batchifier puts several cards in one batch.
//
// # Batch order and the recorded partition
//
// A batchifier returns its batches in plan card order, and webster runs them in that order and never reorders them.
// CheckBatchOrder only asserts it, refusing with ErrBatchOrder when a dependency edge runs to an earlier batch.
// The first init of a run records the partition in State.Partition;
// every other verb reads it through ExecutionBatches, so a changed estimate never regroups cards mid-run.
// A recorded id the plan lacks, or a plan card in no recorded batch, is refused with ErrPartitionMismatch.
// Only a first init or a rebaseline replaces the record.
// A state with no recorded partition runs on the identity batchifier.
// Merriam's start base, which prices each fork at partition time, is computed in merriambase.go.
//
// # Fork-return contract
//
// A fork returns the minimal report shape internal/runplan implements:
// `status: OK`, `FAILED` or `NEEDS_PARENT` with its `need`, plus the `head_sha` the fork left the worktree at.
// The plan's glyph names are the fork's contract.
// A fork renames a planned symbol only when it collides or breaks a language or lint rule.
// It lists each rename under `renames`, which drift detection repairs in later cards.
// The digest's `deviations` list holds files changed outside the batch's targets.
// Its `departures` list holds `Pattern-Departure:` trailers.
// Both are informational, never a failure condition.
// A fork returns FAILED on a failed card gate, card `**Verify:**` or batch gate, never on deviation alone.
// Each card lands as its own commit, and BatchState.CardSHAs records the trail.
//
// record-batch and recover-batch cross-check the report's `head_sha` against the worktree's HEAD.
// HEAD is accepted when it equals `head_sha` or sits above it by clean parent merges alone;
// so a parent merge-in cannot wedge the run.
// Any other movement, and any call during a git merge, is refused and leaves the batch retryable.
// A `head_sha` abbreviation naming no commit or several is refused with ErrHeadSHAUnresolved.
//
// # Every terminal batch runs the same mechanical pass
//
// record-batch after an in-session fork and recover-batch after a cold recovery strand both run runplan.PostBatchChecks:
// the card done-checks, the re-sign rebind, plan handle binding, the glyph scope guard and drift detection.
// A batch done through a recovery is as done as one done through a fork.
// Master's failure ladder therefore treats a terminal recovery digest as "move on".
// A recovery BatchState inherits the stuck fork's StartSHA, since the fork may have committed part of its work.
//
// A batch failed on a correctness finding records its suspect paths with the blob each held when it first failed.
// recover-batch records the batch done only once each was reverted or re-derived and committed.
//
// A create-not-done finding offers a card amendment and recover-batch as ways forward.
// It also offers a parent decision recorded with `--drops-create`, which makes the done-check skip the target.
// A rename-not-done finding offers the amendment and recover-batch alone, since a decision drops only a Create target.
//
// # One dispatch scope for begin-batch, run entry and validate
//
// A plan describes intended change.
// Re-resolving a card whose work may already have landed would report the plan working as designed as a defect.
// All three sites therefore take their scope from DispatchScope:
// cards of begun batches are not resolved, and the targets of non-terminal begun batches are forthcoming,
// so a later card that Uses one passes.
// begin-batch's blocking refusal is a PlanConflictError of the `plan_conflict` class.
// Its cause is `plan_order`, `plan_defect` or `tree_moved`.
// Drift in a begun, non-terminal card's own targets is reported by none of the three sites;
// its recorded CardHashes pin its bytes, and the done-checks still run at record time.
// loom's plan gate takes a narrower scope from runplan's DoneCards and EditedDoneCards.
//
// # The plan-staleness guard re-baselines at the rewrite, not at the return
//
// begin-batch refuses a plan whose directory fingerprint differs from the one state.json records.
// The run itself rewrites the plan (handle canonicalization, handle binding, exact-tier drift repair).
// The guard cannot tell that from a foreign edit.
// Every sanctioned rewrite is therefore re-baselined immediately after the call that performed it.
// That happens before any finding or error the call also reports is examined.
// The caller persists the re-baseline even when the call fails.
// A call can rewrite the plan and also block.
// Re-baselining only on success would make every later begin-batch fail ErrFingerprintMismatch on webster's own edit.
// An unchanged fingerprint writes nothing, so a genuine foreign edit still fails.
// The restamp also moves the CardHashes of begun cards the rewrite changed.
// validate, record-batch and recovery check the plan against the recorded fingerprint before their own rewrites.
// They refuse a foreign edit instead of adopting it.
// Every restamp stores the plan content under `<WebsterDir>/plan-baseline/`,
// and `lym webster restore-plan` (RestorePlan) restores differing plan files from it.
//
// # Rebaseline
//
// A foreign edit an operator means to keep is accepted with `lym webster rebaseline --card NN` (Rebaseline).
// It keeps every batch up to the last begun one as recorded.
// It regroups the cards after it with the active batchifier, and replaces State.Partition.
// The operator names every changed card; a changed card file not named is refused.
// A named card of a failed, dead or stuck batch has its CardHashes entry restamped.
// A named card of a dead batch or of an in-flight one is also recorded in AmendedCards with Rendered false.
// So is one of a failed or stuck batch that already holds an entry.
// This forces an in-flight batch's next report failed, so a recovery re-runs it on the edited cards.
// A dead batch's amendment earns it one uncounted recovery, so an edit to its cards clears its dead end.
// An edit to a done batch's card is refused, with a follow-up card as the way forward.
// Only the Card Index of 00-overview.md may change, held to the card-set rule:
// cards before the last begun batch keep their number and slug.
// Every refusal is keyed on RebaselineDeps.Step, so no text on the loom path names a verb to run by hand.
// Only the loom's Webster row sets RunOptions.AutoRebaseline.
// It runs Rebaseline on entry for every changed card file; its errors wrap ErrAutoRebaseline.
// Run renders every `Added after Discussion` decision entry into the master prompt's `parent_decisions` marker.
// The entries are numbered `Decision 1:` onward in record order,
// and a later entry that differs from an earlier one supersedes it for Master and every fork.
// Refusal and prompt text name a loom verb only as a step for the run's parent, never as a step for Master or a fork.
//
// # Audit findings: correctness fails the batch, policy warns once
//
// The fork and parent audits classify each finding (ClassifyViolation) as correctness or policy.
// A correctness finding means the delta or the run's own state may be wrong:
// a fork writing Master's contract files, the plan directory or webster's run directory beyond its report;
// a parent write under `_lym`, to tracked content, under `.lym`, or in another worktree;
// Master's write to a friction note any Master prompt of the run named (State.MasterNotePaths) is no finding at all;
// a fabric reference whose command the injected ReadOnly classifier does not accept.
// A policy finding breaks a steering rule without touching correctness, such as a named spawn or a nested agent call.
// The ledger in state.json dispositions each finding once per run.
// A policy finding becomes a batch warning once the cards' verify commands re-run clean;
// a failing re-run makes those findings correctness.
// A correctness finding fails the batch terminally.
// So does a secret file (RecordDeps.SecretPaths) added by any commit of the batch.
// In either case record-batch returns *BatchFailedError naming `lym webster recover-batch`.
// A secret file sits in history, so Master stops stuck and the parent resets the batch to its start first.
// An Uncheckable finding is one with no path, or a path outside the tracked tree and the plan directory.
// It makes recover-batch refuse with ErrRecoveryNeedsFresh;
// AcceptBatchFabricReference (`lym webster accept-audit --batch NN`) clears one narrow case:
// a pathless read-only fabric reference, on evidence the refusal names.
// A report that cannot be attributed to a begun batch or a fork transcript is archived.
// It is returned as *ReportArchivedError naming `lym webster begin-batch`.
// An OK report whose delta deleted a symbol a later card targets is handed back once as *LaterTargetRemovedError.
// That happens while the fork still holds the batch's context.
// A pending correctness finding stays in state.json until `lym webster accept-audit` clears it on evidence.
// That evidence needs HEAD at the last batch head, or above it only by two kinds of commit.
// Those are clean parent merges and the verify-gate fix commits from State.FixedBase to State.FixedHead.
// The fix commits count only while nothing but clean parent merges lies between the last batch head and FixedBase.
// A path on the run's records side, outside webster's own run state, has evidence of its own.
// State.MasterRecordsBase is the records side's HEAD from before Master spawned.
// Such a path clears when it is unchanged since that commit, absence included.
// A changed friction note (Geometry.FrictionDir) that exists clears as written:
// the way forward never restores one, since the restore could delete a note that commit lacks.
// A deleted note is restored like any other path.
// Without that commit such a path cannot be checked.
// Run entry refuses with ErrPendingAuditFindings meanwhile.
// The paths that do not clear that way clear only through `lym webster reset --to start` and then `lym webster run`.
// The contract files outcome.yaml and summary.md have no blob to compare;
// contractFileStatus clears one when it is absent or Master wrote it last.
// `run --fresh` drops pending findings with a warning each.
// It refuses when a pending path still differs from the run's start commit or HEAD is not the start commit.
//
// # Bracket verbs, not spawn/poll
//
// The fork runs inside Master's own session, so Go provides thin bracket verbs Master calls around each fork.
// begin-batch checks the fingerprint, records the batch's start SHA, and renders and writes the fork prompt.
// record-batch audits the fork, parses the batch report, distills the digest and updates state.
// The Agent-tool fork is backgrounded, so Master ends its turn after spawning it.
// Master then calls record-batch on the completion notification; shuttle reads that turn end as EventWaiting.
// await-batch is an operator-only stateless bounded wait that Master's loop does not use.
// Go cannot gate the fork itself, so enforcement is template discipline plus fail-loud detection after the fact:
// record-batch refuses a batch with no begin-batch record, and the audit cross-checks transcript and batch counts.
// This is a steering guard, not a security boundary.
// A fork-context PreToolUse(Bash) hook in the claudeengine seam refuses any `lym webster` command inside a fork.
// A fork inherits Master's prompt, and a fork driving the batch loop livelocks the run.
//
// # Roles and cold strands
//
// Forks inherit Master's model, so webster has no per-fork model override.
// RoleMaster and RoleRecovery are its only roles;
// cold strands, recovery and verify-gate fixer alike, run at RoleRecovery.
// recover-batch is a blocking, re-entrant verb that spawns a fresh implementer strand.
// It runs when a fork reports stuck or writes no report.
// Its prompt is the full cold-start RenderRecoveryPrompt and inherits no session context.
// Every terminal digest stops the strand through shuttle's stop verb.
// A batch's recoveries are counted in BatchState.Recoveries.
// A counted spawn is refused with ErrRecoveryExhausted once the batch has two recoveries.
// The envelope's `recovery_retry` tells Merriam whether another recovery is owed.
// Run refuses on entry, wrapping ErrBatchDeadEnd, while a recorded batch is at a dead end,
// so a halted run halts again with the same text and spends no Master session.
// A session's engine is recorded beside its session id, because a shuttle run record does not outlive its run;
// every recorded session is read through the engine its own entry records.
// A role whose engine is not held or cannot audit forks is refused naming its webster.yaml key.
//
// # A batch waiting on its parent
//
// A step only the run's parent can take, such as a hub setting or a board write, is no card failure.
// The implementer reports `status: NEEDS_PARENT` with a one-line `need`,
// and the batch is recorded as `waiting` (not `Terminal`), spending no recovery.
// A recovery strand with a question only the parent can answer reports the same way, with the question as its need.
// Merriam messages its parent and resumes through begin-batch's re-begin or recover-batch once the step is taken.
// Resuming a recovery takes the parent's reply in `--answer`, which the resumed strand's prompt holds beside the need.
// A recovery strand that ends its turn with no report stays dead/asking.
// Its final message is kept on the digest and quoted by the refusals that halt on it.
// A waiting batch and a decision with no batch left are the only times Merriam messages its parent;
// every other dead end is an `outcome: stuck` with no message.
//
// # Digest persistence
//
// The distilled Digest is persisted into BatchState.Digest at terminal classification.
// begin-batch(N+1) and a crash-resumed Master read it, and nothing re-Distills a report.
// That is because the report's originating HEAD may have moved.
//
// # Engine/cli split: webster is fabric-blind
//
// websterengine is _lym- and fabric-blind: every function takes an already-resolved directory string.
// No production file in this package imports internal/fabricengine or internal/lymcwd.
// The engine declares RefMatcher for the fork audit's fabric-reference class, supplied by the caller.
// Paths arrive through a Geometry value (geometry.go) built by internal/hubgeom or internal/standalonegeom.
// Those packages import websterengine and never the reverse.
// Every fabric commit of a webster artifact happens in internal/webstercli, never here.
// Neither Master nor its forks touch fabric or git for webster's own bookkeeping.
//
// # Crash/resume
//
// Forks die with Master, so only Master's and a recovery strand's sessions ever need reclaiming.
// Resuming is re-running `lym webster run`: entry-time reclaim stops any live recorded strand,
// and a fresh Master is hydrated from the reports directory plus state.json and re-drives the first unreported batch.
// A crash between a fork's report and record-batch leaves a report on disk for a recorded batch;
// begin-batch refuses to overwrite it, and the resumed Master consumes it with record-batch or recover-batch.
// That window resumes on the same machine only, because fork transcripts are machine-local.
// Elsewhere record-batch archives the report as forged and returns *ReportArchivedError.
// A plan edited between runs refuses with ErrFingerprintMismatch, and `--fresh` archives the record.
//
// # The plan-conflict record
//
// begin-batch leaves two never-tracked files in `.lym/implementer/`:
// `plan-conflict.md` (ConflictRecordPath), naming the batch, cause and blocking findings,
// and `plan-conflict-routed.md` (RoutedConflictPath), holding the findings last routed to plan review.
// The record exists only when the latest begin-batch of the call refused with `plan_conflict`.
// Under loom, the Webster producer claims it when Master ends stuck (ClaimConflictRoute):
// findings new as a set reroute to plan review, and a recurrence halts for a plan amendment.
//
// # The verify-gate fixers
//
// Run hands Merriam's spawn the hub's `webster` gate as one must-pass entry named `verify` (NewVerifyGate).
// A failure writes the verify-gate report (VerifyGateReportPath) and routes to one of two fixers, FixerFork or FixerCold.
// The fixer fork is spawned in-session by Merriam from a prompt Run rendered at entry (RenderVerifyFixPrompt).
// The cold fixer is `lym webster fix-verify` (FixVerifySpawnOrAttach, FixVerifyAwait).
// It is a strand through internal/gatefixer at RoleRecovery with role `verify-fixer`.
// Both share one body and run the hub's `webster-fix` gate call before reporting.
// A fixer fixes the cause in source and never deletes, skips or weakens a test.
// It never touches the plan directory or `_lym`.
// A fixer's commits skip the post-batch checks.
// The run-exit fork audit, the gate's commit check (fixCommitRejection) and the Code-Review perch therefore bound them.
// The gate, not the verb, judges a fix: a dead cold fixer leaves the failure for the next evaluation.
// The first failed evaluation records HEAD as `PreFixHead`, which `reset --to pre-fix` targets.
// A later run's gate keeps that head while no batch has landed above it and the commit check passes above it.
// A pass with HEAD above it records it as `FixedBase` and HEAD as `FixedHead`.
// accept-audit accepts the fix commits in that range.
// A failure is rerun once and only a failure that survives the rerun goes to a fixer;
// a hang (classifyHang) or a run that judged no code is never rerun, since neither is flakiness.
// Tests that fail once and pass on the rerun are reported as flaky, not failed.
// A run whose gate spent its budget ends stuck with a reason that names the command and what failed.
// A run-level entry's failure quotes the head of its findings and names the report, which holds them all.
// A drift entry's way forward also names the parent's `lym drift amend`, which accepts a drift the plan intends.
//
// # The plan fixer
//
// A `plan_conflict` refusal with cause `plan_defect` or `tree_moved` goes to a cold plan fixer:
// `lym webster fix-plan` (FixPlanSpawnOrAttach), run before Master ends stuck.
// A `plan_order` cause goes to no fixer, since moving a delete reshapes cards.
// The fixer edits only the target names, paths and glyphs the findings name.
// It runs `lym webster rebaseline --card NN`, and replies `verdict: fixed` or `verdict: plan_writer`.
// The verb refuses when an earlier fixer of the run saw the same findings, or past planFixersPerBatch.
// It overrules a `fixed` verdict when the fixer moved HEAD or dirtied a path that was clean at the spawn.
//
// # Background shells in Master's wait
//
// Master's spawn declares its awaited shell prefixes (masterAwaitedShellPrefixes):
// the recovery verb, the cold fixer verb and the plan fixer verb.
// Those are bounded by webster.recovery_timeout_min and never expire a turn end.
// A turn end waiting only on other shells is held until `shuttle.background_shell_wait_min` passes.
// lym does not stop a shell still running when the run ends;
// Run writes a `webster-background-shell` friction note and a `RunResult.Warnings` entry for each.
//
// # Planning a reset
//
// PlanReset decides, read-only, what a reset of the task branch may do, and refuses before fabric is ever called.
// The target is one of five commits the run recorded (ResetTargets):
// `start`, `pre-fix`, `last-batch-head`, `batch-start` and `report-head`; no raw SHA is accepted.
// `--batch NN` is required for the last two and refused for the others.
// `report-head` is the one target allowed while the run lock is held, and Master runs it between a fork and record-batch.
// PlanReset refuses on a merge in progress, a branch other than the task branch, a missing or non-ancestor target,
// and a dirty tracked path outside the run's own writes.
// `start` does not refuse where the start cannot be moved to:
// it returns a plan with ArchiveOnly set and a Reason.
//
// `lym webster reset --to <target> [--batch NN]` (internal/webstercli) performs the planned reset,
// so no recovery needs the denied `git reset --hard`.
// It resets the pair's code checkout through fabric and moves the task branch on the remote under a lease.
// A reset to start then calls ArchiveRunAfterReset, so a following plain `run` starts a new run.
// It can discard only commits above a run-recorded commit on the task's own branch.
// Beyond that it can discard only uncommitted tracked changes to paths the run itself wrote.
// It cannot move another branch, take a raw SHA, touch the parent branch or untracked files, and has no `--force`.
// In standalone mode the verb uses gitrepo's keep-reset and touches no remote.
//
// # The Git seam and the code-index seam
//
// Every repository question of the bracket verbs and the run-level checks goes through the Git interface (git.go).
// Geometry.Git carries it, and nil means the real repository.
// UncommittedPaths is the one read of the task's uncommitted work, minus the run's own state.
// Every plan gate and the batch delta go through Geometry.Index, a planindex.Index,
// so this package imports neither internal/planglyph nor quarry.
// The CLI layer sets the real index, and a call that reaches a nil Index returns an error naming the missing wiring.
//
// # What webster shares with other loops
//
// websterengine imports no other batch-implementation module's plan parser, report schema, or digest contract.
// It shares internal/runplan's run-plan machinery and the substrate every `lym` module built on shuttle shares:
// internal/reedengine, internal/shuttleengine and its claudeengine, and internal/gitrepo.
// State embeds runplan.State, and every shared refusal names webster's way forward through the text webster passes.
//
// # Why cards run one at a time
//
// Cards run strictly in sequence inside one working tree, and the plan carries no scheduling DAG.
// Git's index is a single shared file per working tree, so two forks committing at once race on the same lock,
// and a fork's code-index queries would see other forks' broken, uncommitted edits.
// Serializing only the commit step through a mutex would still need strict file-disjointness.
// The shape that avoids the shared index is a `fabric`-spawned worktree per independent group;
// the board holds it as `webster-parallel-execution`, and its case is weak:
// a funnel near the end of a card DAG collapses the wave width, and a small plan gains close to nothing.
package websterengine
