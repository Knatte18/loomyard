// Package landingshed owns landing's general producers, Merge-In, Publish, PR-Align, PR-Gate and Finalize, which any producer list may name -- none is special-cased by the engine that drives them.
//
// Publish checks the configured require_pr_to_base list (see LoadConfig) against the task's own
// parent branch.
// When the parent is absent from that list, Publish is a no-op: Done immediately, no push, no GitHub call.
// When the parent is present, Publish runs the hub's `landing` and `publish` gates, pushes the task branch, and opens the pull request, or refreshes an open one's title and body, from the change description (the file the Describe row writes, one source shared with the landing commit), then returns Done.
// Every landing row's pull-request lookup takes the newest pull request created at or after the run's start, `Deps.RunStart`, whatever its state, and a zero start keeps the unbounded lookup.
// When the lookup finds none of the run's own but an open pull request from before the run started, Publish creates nothing, since GitHub allows one open pull request per head and base, and is Stuck with a way forward: close that pull request on GitHub, then resume the run with `lyx loom start`.
// The merge-in is Merge-In's, which runs ahead of Publish in a recipe's order and ran the `landing` gate over the merged tree.
// The PR-Gate producer in this package then owns approval, rejection and the wait for the reviewer;
// PRGate.Call documents its decision table.
//
// PR-Align runs between Publish and PR-Gate in a recipe that lists it.
// For a parent in require_pr_to_base it spawns the alignment agent for the open pull request, once per pull request head, and applies the agent's verdict through RecordApproval and RecordRejection, the same write path `lyx loom approve` and `lyx loom reject` run.
// It ends Done on every path: alignment switched off, a pull request it cannot align, an escalation, a timeout, any failure and a verdict it must not apply each leave the review to the run's parent, with an AlignmentNote naming the agent's report and the reason.
// PR-Gate reads that note when it holds the pull request awaiting and leads its reason, and its parent notice, with the report path.
// An agent approve is applied only when alignment_pr_approve is set, the decision snapshot is unchanged and the pull request and task heads still match; an agent reject is applied once per run, and the second goes to the parent.
// PR-Align takes no parent lock.
// The producer reaches the agent through the AlignmentSession interface, which `*alignment.Session` satisfies, and all its seams are told through Deps.Alignment.
//
// Publish, PR-Gate and Finalize hold the parent lock too, and a moved parent sends them back to Merge-In.
// Each takes the lock through the shared guard, which grants a run that already holds it, so a re-call after a resume takes back a record handed to batten.
// Moved means a code-side tip, the local one or the upstream one after a fetch unless the push is skipped or the branch has no upstream, differs from the merge record's merged tip and does not hold the task's work;
// a missing merge record counts as moved, so the row reroutes to Merge-In, which writes one.
// A moved parent is a Stuck with Reroute set, which takes the row's on_reroute edge to Merge-In and spends no bounce budget.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// Each constructor refuses a nil Deps.Lock.Store, a nil Deps.Lock.Owner and a nil Deps.ReadParentTips, and a row built without its guard, as a struct-literal test builds it, takes no lock and reads no tips.
// Publish, for a parent in require_pr_to_base, takes the lock and tests the parent before its gates and the push; a parent outside the list stays a no-op that touches neither git nor GitHub.
// PR-Gate does the same before it queries the pull request, so the orch never decides on a pull request whose base moved, and its matching-rejection Stuck releases the lock before returning:
// the edge to PR-Rework (darn: Darn-Write) leaves the Merge-In-to-Finalize window, and the reworked run takes the lock again at Merge-In.
// Finalize takes the lock and tests the parent after parsing the change description and committing the status file, before its catch-up merge-in.
// Under that lock it runs the same run-scope origin reconcile and maps its ended and stopped verdicts as Merge-In does, then the owner checks.
// A run whose Finalize merged into the local parent here is never read as ended, so a Finalize re-entered after its done mark proceeds;
// a Finalize entering with no such merge, its entry done on origin, stops with Halt, since another machine landed the run.
//
// The owner checks let a run land only on the machine that owns it and its parent, so a loom run, a darn run and an alignment approve that lands with no orch turn all pass them.
// Finalize reads Deps.ParentHoldsTaskWork first: a parent already holding the task's work marks a Finalize past its parent-side merge, which skips both owner checks and the unreachable halt.
// Otherwise an unreachable origin halts, the run's owner is the owner of its owned_elsewhere entry, and the parent's owner comes from Deps.ParentOwner, read from the local board the reconcile left.
// A run whose pair has no batten run record reads as owned here, since the reconcile judges only recorded pairs, but its parent's owner is still read.
// A parent or run owned by another machine, an unreachable origin and a failed read of either seam each stop with Halt and a reason behind OwnerHaltPrefix, so the loom halt note is written;
// the reason names the owners and the `lyx adopt` commands that move them.
// Merge-In proceeds on an owned_elsewhere verdict, since the owner check belongs to the landing.
// A nil seam reads as not past the merge and as the parent owned here, so a Finalize built with no owner seams lands as before.
// A parent that already holds the task's work, after a GitHub merge or an earlier landing, reads as not moved and lands through the already-landed path.
// The catch-up merge-in stays: the local tip is checked equal to the merged tip under the lock, so it brings no parent code the merge review has not covered while still syncing the records side.
// After a merge-in-required error from the parent-side merge Finalize tests the parent again, and retries once only when it has not moved, which covers a parent moved by run records alone;
// otherwise it reroutes, so a code-side commit that arrives between the check and the parent-side merge never lands.
// Finalize's Done releases the lock before Friction-Reflect runs.
// A parent push that fails for a reason other than a transient one carries a repair note when origin's parent already holds the task's work, which an adopter-first landing leaves:
// it names the task's squash by the parent head's SHA, a `git reset --keep` to origin's parent when the squash is the only commit origin lacks or a `git rebase --onto` dropping it when another is listed, and then `lyx fabric remove` of the run, leaving the owner pause standing.
// An origin parent lacking the work, or a failed read, adds no note.
//
// Merge-In and Finalize clear the merge-in their own earlier attempt left parked before the clean-tree check that precedes each merge-in, Finalize's retry pass included.
// A parked fabric `merge-in` of the parent branch is aborted, whoever started it, so a conflict session's resolution and a hand-started merge-in with its partial resolution are both discarded and the merge starts afresh.
// Every live conflict-session strand of the run is stopped through Deps.StopConflictSession first, and only after every stop succeeded is the merge aborted, so no session is left writing into the redone merge;
// a stop that fails is Stuck naming the strand, with the merge untouched.
// Any other parked merge and git merge state fabric did not start is Stuck with its way forward, never stopped over or touched.
// The resolver's own abort stays as crash recovery within one merge-in step.
// The seams are nil-is-absent: a nil Deps.MergeState skips the step.
//
// Merge-In is built here and listed by a recipe's producer list like the others.
// It commits the status file, runs the run-scope origin reconcile through the told Deps.Reconcile, takes the parent lock through the shared guard, and only then brings the local parent branch level with its upstream.
// The reconcile judges and never moves the parent.
// A run origin ended is a Stuck with Halt naming the landing or the board removal, with `lyx cleanup --apply` from the prime as the way forward;
// a run its branches stop is a Stuck naming the branch, its state and its way forward, then `lyx loom resume` in the task worktree;
// an unreachable origin logs a Warn and proceeds, so the row fails on its own remote steps, and a reconcile error is an error.
// Under the told push skip it neither fetches nor fast-forwards, and merges the local tip.
// A parent branch behind its upstream is fast-forwarded in its own pair under the lock, which moves it to the upstream tip and touches no local commit;
// one that is ahead of or diverged from its upstream, and a parent pair that is missing, dirty, mid-merge or no longer a fast-forward of its upstream, are Stuck with a way forward, since Merge-In never pushes or rewrites the parent.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// It then clears a parked merge-in, merges the parent tip into the task worktree through internal/mergeresolve, and runs the `landing` gate over the merged tree.
// The task worktree must be clean at three points: before the merge-in (after the status commit), after the merge-in, and after the verify.
// A dirty tree at any of them is Stuck naming the paths, so nothing uncommitted is merged over or reviewed;
// a verify that dirties the tree is a non-hermetic test and halts rather than ships.
// The told landing call, the `landing` gate's `lyx gate test` call rendered from the hub's gate config at each run, runs through internal/verifytree, unless its verified-tree record already names the tree and the command.
// Its base command is the told Deps.BaseCommand, so a landing-list pass keeps the entry a `final`-list pass wrote, and a tree Webster's gate passed is skipped when the two lists render the same call.
// A refusal of the gate config or of the `landing` list is the producer's returned error, carrying the refusal's way forward.
// A verify that does not finish within the verify timeout is killed and halts the run Stuck with the log path.
// While the verify runs, the told Deps.VerifyWaitMark callback marks the run's driver strand on screen as `verify <producer>`, and clears the mark on every way out of the verify.
// The mark is display only: a failing or absent callback changes no Stuck reason, error or verify outcome.
// The record is keyed on the tree, so the verify runs after a resolved conflict, after new parent commits and after a crash between a merge and its verify, and a no-op merge onto a verified tree skips it.
// A failure is Stuck with the merge commit kept for the operator to fix forward, and a missing command logs a warning and proceeds.
// A failed verify writes the Publish failure record with row `Merge-In`, and a passing one removes it.
// Last it writes the merge record: the tip merged, the task head before the merge, the parent tip last merged into the task, the parent tip the last finished merge review reached, the merge commit and where the parent's changes since that reviewed tip meet the task's changes since its last merge base.
// An up-to-date merge records an empty overlap, and a failed record write is an error, never Stuck.
// The lock is kept on every way out of the row: a later landing row or the loop's exit frees it.
//
// Publish runs the `landing` call, then the `publish` call, in the task worktree through the same verify function, at site `Publish`, before it pushes, and the tree must be clean after them.
// A dirty tree is Stuck naming the paths, before anything is pushed.
// The `publish` gate carries the hub's extra test tiers, which the `landing` and `final` gates leave out so every round and every gate does not pay for them.
// Both calls name the told base command as the site's base command, so neither pass prunes the entry the next review round diffs from.
// The landing call skips on the tree Merge-In verified.
// A failure is Stuck with the exit code and the log path, naming the `landing` or `publish` gate, and a dirty tree is Stuck as for Merge-In's verify.
// An empty `publish` list renders an empty call, which runs nothing and logs nothing, and Finalize never runs the `publish` gate.
// It runs only where Publish does, so a landing into a parent outside require_pr_to_base runs no such tier and only Finalize's landing call gates it.
// A recipe may tell Publish a skip seam, Deps.PublishSkip, read at each Publish after the parent lock is held.
// A seam answering true skips the `publish` gate and takes the path of an empty `publish` list: the Publish failure record is removed before the push.
// A seam error is Publish's returned error, with nothing verified or pushed, and false runs the `publish` gate as usual.
// A darn run tells the seam so a diff wholly under its configured non-production trees lands with no `publish` tiers and with only the tree-scoped command at Merge-In, Finalize and Publish, the hub's whole statement of what such a diff needs; a loom run tells none, so its `publish` gate runs whenever its list is not empty.
// landing.yaml's retired `publish_verify` key loads and is dropped, and `lyx config reconcile` prunes it.
//
// Landing.yaml's `deploy` is read only by batten's Land row, from the board dir like the rest of landing config and never from a pair.
// It runs in the prime only for a parent branch in `require_pr_to_base` when the row runs, so it executes code PR-Gate reviewed.
// The one exception is an edit to that list between PR-Gate and Land, which only the orch's own hub config edit can make: it deploys a tip PR-Gate passed unreviewed.
// An empty value runs nothing, and `lyx config` refuses a write to it from a pair, as for every hub-wide key, as a guard against a mistaken write rather than a determined agent.
// Landing config is hub-wide: Publish and Finalize read it from the board dir, never from a task worktree's `_lyx/config/`, and `lyx config` refuses a write to it from a pair, so a task cannot weaken the key, or set `require_pr_to_base` or `squash`, through its own tree.
// A landing change is an operator or hub-orch `lyx config landing` run from the prime, committed in `_board` and effective for every pair at once.
//
// Merge-In and Publish persist their verify failure; Finalize neither writes nor removes the record.
// When a `landing` or `publish` call fails with a non-zero exit, Merge-In or Publish writes the verifytree Publish failure record with the failing gate as its kind, the failing tier, the row that ran the verify, the failing tests the told Deps.FailingTests parses from the log, HEAD, the told Deps.VerifyFailureTarget row, the write time in whole-second UTC, and the merge-in commit.
// The tier is the last failing tier the log's `lyx gate test` summary lines name; a log without one, such as a refusal before the first tier, takes the highest tier of the gate's list, read through the told Deps.GateTiers.
// The merge-in commit is HEAD after a merge-in that was not already up to date and empty otherwise; Publish reads it from Merge-In's merge record.
// A dirty tree, a timeout and a shell that could not start write no record, and a write failure is logged and never changes the Stuck verdict.
// The Stuck reason of a failed verify ends with the way-forward clause the recipe tells in Deps.VerifyFailedWayForward only when the record was written; otherwise, and when none is told, it asks to fix forward on the task branch.
// When the failed verify's log holds the CPU watchdog's line that `lyx gate test` prints after it quits a busy test binary, the reason names the window and says the goroutine stacks are in the log, before that clause.
// A watchdog quit of a healthy package has no code fix: the way forward is raising `busy_quit_sec` through `lyx config gate` from the prime, then resuming.
// An unreadable log or one without the line leaves the reason unchanged, and the checks run no flake rerun.
// Merge-In removes the record once its landing call passes, and Publish removes it once its landing and publish calls pass, or at once when the publish list is empty or the skip seam answers true.
//
// The push that follows is never retried when the remote rejects it, because the remote task branch moved and a repeat would be rejected again.
// Publish reads the remote task branch's tip and the commits on it that the local branch lacks through Deps.RemoteOnlyCommits, and stops Stuck with a reason stating that the merge-in already ran, the tip, the commit count, and the way forward:
// merge `origin/<task-branch>` in the task worktree, then resume the run with `lyx loom start`, which re-runs Publish over the merged branch.
// A failed read keeps the rejection and the way forward and names the cause.
// A remote with no task branch, or a remote task branch that holds no commit the local branch lacks, is not cleared by a merge, so the reason names a remote rule as the cause and the way forward is to clear it, then resume.
// Every other push failure keeps its own reason, and a transient one is returned as an error so the driver re-steps.
//
// require_pr_to_base is a list of base-branch names, not a bool, because whether a pull request is
// needed depends on which parent branch a task targets -- a per-task runtime fact no static profile
// setting can encode as one flag. A bool would force (or skip) a pull request on every task-to-task
// merge alike, which a stacked branch whose own parent is not the trunk branch already disproves.
//
// Finalize always syncs the task branch against the parent through internal/mergeresolve, regardless
// of which branch Publish took -- the only sync in the no-pull-request case, a second one catching
// whatever landed in the parent while a pull request sat out for review in the other case -- and then
// merges the task branch into the parent pair itself.
// Each catch-up merge-in, the retry after the parent moved included, runs the same three clean-tree checks and the same post-merge verify gate as Merge-In before the parent-side merge,
// and a dirty tree or a failure is Stuck with the parent branch untouched.
// After each catch-up merge-in and before the parent-side merge, Finalize commits the parent pair's own run records through the told Deps.CommitParentRecords, so a sibling run's uncommitted status record does not trip the merge guard;
// a clean subtree is a no-op, an absent seam skips the step, and a commit failure is Stuck naming the parent pair and its run records, with nothing merged.
// The landing commit carries the change description, then a blank line and a trailer block when the block is non-empty.
// The block is landing.yaml's trailer_text, then the engine_trailers text of each engine the task's sessions ran under, in engine-name order, each copied verbatim with no prefix.
// Finalize reads those engines through the told Deps.EnginesRun before any parent-side mutation, and a read failure is an error with nothing merged.
// An empty trailer_text adds no fixed line, an engine left out of engine_trailers or mapped to empty text adds no engine line, and both empty land the commit with no trailer block.
// lyx config reconcile moves a retired co_authored_by value into engine_trailers as the claude entry `Co-Authored-By: <value>`, and drops an empty one without an entry.
// Before the catch-up merge-in, Finalize reads the per-worktree config files the task changed away from both its fork point and the parent's current tip, so a parent-side change never reads as a task edit, and, at Done, queues one notice about them naming the base, task tip and parent tip for the hub's orchestrator;
// it never carries those files to the parent,
// and a read failure or a failed notice never stops the landing.
// After the parent-side merge and before the push, Finalize marks the board task done; after a
// successful push it closes the open pull request with a comment naming the landing commit. A parent
// that already holds the task's work makes Finalize idempotent: it takes the already-landed path
// rather than merging again. The parent branch is pushed to its upstream, so a landing reaches the
// remote rather than only the hub's own parent worktree. That parent-side merge is this producer's own
// merge critical section: the one span a future regeneration step folds into rather than running as a
// separate step before or after it, because splitting the two would let the parent advance in between
// the read and the write. Finalize.Call documents which of its own steps is that section; nothing here
// reserves a hook or a scaffolded span for the fold ahead of that future work landing, since an
// interface with a permanently-nil implementation is exactly the hypothetical-requirement design this
// codebase avoids.
//
// Finalize's parent-side merge squashes by default (landing.yaml's squash key, defaulting to true):
// one commit per merged task branch was the goal, weighed against the accepted cost that a squashed
// branch carries no merge commit linking it to its target, so "was this branch merged?" becomes
// unanswerable from git history alone afterward. The commit noise an ordinary merge would leave behind
// -- one surviving commit per card the task landed -- was judged the worse trade.
//
// The Describe session loads `scribe:prose` before its prompt.
// Its stencil, like the conflict session's, carries the parent directive that internal/parentdirective renders from the told name:
// the seat engine's geometry for Describe, and Deps.ParentName passed through to the resolver for Merge-In and Finalize.
// DescribeTable builds a loom run's session, which reads the decision record and the webster run records;
// DarnDescribeTable builds a darn run's from its own stencil, `landing-template-describe-darn`, which reads the board entry, the decision record beside it, the diff and the review perch's run directory, under the same describe keys, role and skills.
//
// It takes told absolute paths and has no direct production import of internal/lyxcwd, per the
// Told-Geometry Invariant: every path this package operates on is handed to it by its caller, and it
// derives none of its own.
//
// This package describes one repository throughout. It is not in the Fabric Vocabulary Invariant's
// owner set, so none of its identifiers, string literals, or comments may name either fabric-internal
// side, and that ban is machine-enforced by an AST walk over every identifier
// (internal/lyxcwd's TestEnforcement_FabricVocabulary).
package landingshed
