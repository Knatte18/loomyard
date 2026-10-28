// Package landingshed owns landing's general producers, Parent-Align, the PR-Review stub and Finalize, which any producer list may name -- none is special-cased by the engine that drives them.
//
// PR-Review is the stub of the human pull-request review that is not built yet; PRReview names it for a recipe row.
// Its call is Done every time: with its flag off it logs nothing, and with its flag on it logs one Info line that the PR review is not built.
// It opens no pull request, calls no GitHub API, reads no file and takes no lock, and a cancelled context is an error.
//
// A merge or test failure at Parent-Align or Finalize is owned by the recipe: it ends in the row's fixer, a reroute or a halt to the orch.
//
// Finalize holds the parent lock, which it takes through the shared guard; the guard grants a run that already holds it, so a re-call after a resume takes back a record handed to batten.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// Finalize's constructor refuses a nil Deps.Lock.Store, a nil Deps.Lock.Owner and a nil Deps.ReadParentTips, and a row built without its guard, as a struct-literal test builds it, takes no lock.
// Finalize takes the lock first, after parsing the change description and committing the status file and before anything reads or moves the parent, and releases it on every way out of its Call: Done, a reroute, a Stuck and an error.
// A lock a dead Finalize left is taken over at the next acquire, since the lock store grants a record whose owner process no longer lives.
// Under that lock it runs the same run-scope origin reconcile and maps its ended and stopped verdicts as Parent-Align does, then the owner checks.
// It then opens the parent pair and syncs it before any merge: a pair that is missing, mid-merge or holding tracked dirt in its code checkout is Stuck with its way forward, whether the parent is behind, equal, without an upstream or under the push skip.
// With no push skip a parent behind its upstream is fast-forwarded in its own pair, which moves it to the upstream tip and touches no local commit, and one ahead of, diverged from or no longer a fast-forward of its upstream is Stuck, since Finalize never pushes or rewrites the parent here.
// Under the push skip it neither fetches nor fast-forwards, and a parent that already holds the task's work is past its parent-side merge and is left as it is.
// A run whose Finalize merged into the local parent here is never read as ended, so a Finalize re-entered after its done mark proceeds;
// a Finalize entering with no such merge, its entry done on origin, stops with Halt, since another machine landed the run.
//
// The owner checks let a run land only on the machine that owns it and its parent, so a loom run and a darn run, landing with no orch turn, both pass them.
// Finalize reads Deps.ParentHoldsTaskWork first: a parent already holding the task's work marks a Finalize past its parent-side merge, which skips both owner checks and the unreachable halt.
// Otherwise an unreachable origin halts, the run's owner is the owner of its owned_elsewhere entry, and the parent's owner comes from Deps.ParentOwner, read from the local board the reconcile left.
// A run whose pair has no batten run record reads as owned here, since the reconcile judges only recorded pairs, but its parent's owner is still read.
// A parent or run owned by another machine, an unreachable origin and a failed read of either seam each stop with Halt and a reason behind OwnerHaltPrefix, so the loom halt note is written;
// the reason names the owners and the `lym adopt` commands that move them.
// Parent-Align proceeds on an owned_elsewhere verdict, since the owner check belongs to the landing.
// A nil seam reads as not past the merge and as the parent owned here, so a Finalize built with no owner seams lands as before.
//
// The land check follows the owner checks under the same lock, and a Finalize built with Deps.LandBlockers, a pawl run's, runs it: it reads the open members and the local pairs forked from the beam, and a read error or any blocker halts behind LandHaltPrefix.
// It is a backstop for an open member the run's earlier re-check missed, such as one a stale board on another machine added.
// Each way forward is Finalize's own and never the land-it wording of the beam's landing refusal, which the standing landing hold refuses:
// a forked pair, and a member with a pair or a run in flight, name `lym batten cancel <slug> --remove` run in Deps.ParentWorktree, or on the machine holding the pair when the run is in flight and its pair is not here, which `lym batten status <slug>` there shows;
// a member then names moving its `project` off the beam with `lym board upsert`, since the cancel leaves it a `draft` member, which still blocks the landing, and a member with no run status names that move alone;
// every halt ends in `lym loom resume` in the beam's worktree.
// A Finalize already past its parent-side merge, an empty list and a nil seam land as before.
// Bound: a member that lands into the beam between this check and the parent-side merge is not caught here, and the Worktree-Create hold read keeps any member from forking once the landing hold stands.
// A parent that already holds the task's work, after a GitHub merge or an earlier landing, lands through the already-landed path.
// Finalize then merges the parent branch into the task worktree through its resolver for every recipe, which resolves a conflict with the conflict session under the lock, and gates the merged tree.
// It reads the merge record's merged tip, clears a parked merge-in, checks the tree clean and merges.
// An unreadable merge record is logged and read as absent, so the parent side is measured from the task's merge base, as a run with no record is.
// The merge is large when the parent's changes since the merged tip it read touch a file the task changed since its merge base; a parent already holding the task's work is never large.
// A large merge writes the merge record, with its overlap, and goes on to the gate like any other merge, and a conflict or a large merge reroutes in no recipe.
// Bound: what lands with no review is the conflict session's resolution against the parent's newer work and a textually clean merge of the parent's changes to files the task also changed; the `finalize` gate over the merged tree bounds both, and a spent fixer, or a change past the caps the fix review does not approve, reroutes when a review follows the reroute and halts for the orch otherwise.
// The conflict session runs while the parent lock is held, so sibling landings onto that parent wait, bounded by the lock's owner, pause and release rules.
// Finalize then writes the merge record with the new merged tip and the aligned tip as it was.
// Next it runs its own gate key, `finalize`, over the merged tree through verifytree, which skips a tree it already verified, so a run that went right holds the lock for one merge, a skipped gate and the landing.
// A darn Finalize over a diff wholly under `tree_scope` runs the tree-scoped command, since darn's verify source answers it.
// A passing gate removes the landing failure record, and a failing one writes it.
// A failing gate spawns the merge fixer in finalize mode, which fixes failing tests only, on the attempts `finalize_fix_attempts` leaves the entry, read from the attempt record under the row's entry.
// The fixer gets no parent tip to read from and the merged tip the merge record carries.
// The first spawn writes the HEAD before the fixer's first turn into the record's Baseline, every resume of the entry keeps it but the one after an over-cap halt, and the fixer's whole change is judged by `LineChange(Baseline, HEAD)` against `finalize_fix_max_lines` and the run's test-file matcher, so turns that each stay under the cap but sum past it are over it.
// An entry has no attempts left when its Spent reaches `finalize_fix_attempts`, whatever the record's Halt notes, and a resumed entry with none left spawns no fixer: over a failing gate it ends as a spent cap, and over a passing one its change since Baseline is judged.
// A change over the caps, when a review follows the reroute (Deps.RerouteReviewed), first goes to the fix review: one solo session on the fixer's model-spec and turn timeout, which reads only the fixer's commits, Baseline..HEAD, and writes the report `landing-fix-review-spec.md` pins.
// An approval lands the change and keeps the approved HEAD in the record's Reviewed, so a later pass of the entry at that HEAD lands with no second review;
// a rejection, or a session that ends without a verdict, reroutes naming the findings or why;
// a session that dirties the tree or moves the task head halts with LandingStopPrefix, and one that fails to start is a transient error.
// Bound: the fix review lets an over-cap fixer change land behind one reviewer's explicit approval of the fixer's own commits instead of the run's full Code review, under the parent lock, for at most one `fixer_timeout_min` session and one re-prompt over an unparseable report.
// A spent cap, and a change over the caps the fix review does not approve, keep the fixer's commits and reroute through the reroute reason when a review follows the reroute, and halt with LandingStopPrefix otherwise, naming the commits and noting `spent` or `over-cap` in the record's Halt.
// A resume after an over-cap halt writes Baseline afresh at the HEAD the gate starts from, clears the note in the same record write, reads the operator's review as acceptance of the commits and runs the gate: a passing gate lands without judging the change, and a failing one runs the fixer on the attempts left.
// An escalate report keeps the commits and halts naming the report, with EscalateStopPrefix on a loom or darn run; a turn that ended without a usable result halts, and a failed spawn is an error that spends no attempt.
// An attempt record that does not decode halts naming the file, with removing it, which grants a fresh cap, as the way forward.
// A Finalize reroute leaves the entry's attempt record for the next entry to clear, and a Done clears it.
// The size and kind caps bound what a Finalize fix lets through without a reviewer: a fix within them lands with no perch, behind only the `finalize` gate it passed and the finalize mode's instruction to fix failing tests only.
// After a merge-in-required error from the parent-side merge Finalize retries once, which covers a parent moved by run records alone;
// the retry merges the parent again and gates the merged tree under the same lock, so a code-side commit that arrives between the first merge and the parent-side merge is merged, gated and landed.
// A dirty parent is Stuck naming the parent's code checkout and its tracked dirty paths, capped at 20, with the way forward per reason.
// The run records Finalize commits at its parent-records step never count as that dirt, since the guard reads the code checkout alone, whose `_lym` link is excluded from its index.
// A parent push that fails for a reason other than a transient one carries a repair note when origin's parent already holds the task's work, which an adopter-first landing leaves:
// it names the task's squash by the parent head's SHA, a `git reset --keep` to origin's parent when the squash is the only commit origin lacks or a `git rebase --onto` dropping it when another is listed, and then `lym fabric remove` of the run, leaving the owner pause standing.
// An origin parent lacking the work, or a failed read, adds no note.
//
// Parent-Align and Finalize clear the merge-in their own earlier attempt left parked before the clean-tree check that precedes each merge of the parent, Finalize's retry pass included.
// A parked fabric `merge-in` of the parent branch is aborted, whoever started it, so a conflict session's resolution and a hand-started merge-in with its partial resolution are both discarded and the merge starts afresh.
// Every live conflict-session strand of the run is stopped through Deps.StopConflictSession first, and only after every stop succeeded is the merge aborted, so no session is left writing into the redone merge;
// a stop that fails is Stuck naming the strand, with the merge untouched.
// Any other parked merge and git merge state fabric did not start is Stuck with its way forward, never stopped over or touched.
// The resolver's own abort stays as crash recovery within one merge-in step.
// The seams are nil-is-absent: a nil Deps.MergeState skips the step.
//
// Parent-Align is built here and listed by a recipe's producer list like the others.
// It commits the status file and runs the run-scope origin reconcile through the told Deps.Reconcile, then picks the parent tip to merge.
// Parent-Align takes no parent lock across the merge and writes nothing in the parent pair: the fast-forward of the local parent branch and the parent-pair refusals belong to Finalize, under its lock.
// The reconcile judges and never moves the parent.
// A run origin ended is a Stuck with Halt naming the landing or the board removal, with `lym cleanup --apply` from the prime as the way forward;
// a run its branches stop is a Stuck naming the branch, its state and its way forward, then `lym loom resume` in the task worktree;
// an unreachable origin logs a Warn and proceeds, so the row fails on its own remote steps, and a reconcile error is an error.
// With an upstream and no push skip it fetches and merges the parent's upstream tip, read from the fetching tips read and never the local tip, so a landing another run made locally and has not pushed is never merged in.
// Under the told push skip, or with no upstream, it takes the parent lock through the shared guard only for the read of the local tip and releases it before the merge, which then runs unlocked.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// It then clears a parked merge-in, merges that tip into the task worktree through internal/mergeresolve, and runs its own gate key, `parent-align`, over the merged tree.
// The task worktree must be clean at three points: before the merge-in (after the status commit), after the merge-in, and after the verify.
// A dirty tree at any of them is Stuck naming the paths, so nothing uncommitted is merged over or reviewed;
// a verify that dirties the tree is a non-hermetic test and halts rather than ships.
// The row's gate key is the told Deps.GateKey, which each row's registry entry sets from its optional `gate_key` config key, else its row name in lower case.
// The told call, that key's `lym gate test` call rendered from the hub's gate config at each run, runs through internal/verifytree, unless its verified-tree record already names the tree and the command.
// The merge fixer a row spawns judges each turn through that row's gate, so it runs the same key.
// Its base command is the told Deps.BaseCommand, the fix rows' `code-burler-fix` call, so a landing pass keeps the entry the Code-Burler-Fix lint base reads.
// A refusal of the gate config or of the key is the producer's returned error, carrying the refusal's way forward.
// A verify that does not finish within the verify timeout is killed and halts the run Stuck with the log path.
// While the verify runs, the told Deps.VerifyWaitMark callback marks the run's driver strand on screen as `verify <producer>`, and clears the mark on every way out of the verify.
// The mark is display only: a failing or absent callback changes no Stuck reason, error or verify outcome.
// The record is keyed on the tree, so the verify runs after a resolved conflict, after new parent commits and after a crash between a merge and its verify, and a no-op merge onto a verified tree skips it.
// A missing command logs a warning and proceeds.
// A failed verify writes the landing failure record, and a passing one, or a skip over a tree recorded as passed, removes it.
// After the gate Parent-Align aligns: the parent brought in commits when its merged tip adds something over the aligned tip, or over the task's fork point from the parent when the merge record holds none.
// A merge that brought none over a passing gate is aligned with no fixer and advances the aligned tip to the merged tip.
// Parent commits brought in, or a failing gate, run the merge fixer in parent-align mode on the attempts `parent_align_fix_attempts` leaves the entry; a `fixed` report over a passing gate advances the aligned tip and is Done.
// The entry's fork point is kept in its attempt record before the merge, so a resumed entry whose merge is already up to date still reads what the first merge brought in.
// With the Code-Review perch's budget 0, or told nothing, a fixer change over `finalize_fix_max_lines` or touching a test, testdata or gate configuration file, or adding a test skip or a build constraint, halts over-cap; with a budget above 0 the change goes to the Code-Perch unjudged.
// A spent cap, a turn that ended without a usable result and an over-cap change each halt with a reason opening with LandingStopPrefix and a way forward; the resume after an over-cap halt reads the operator's review as the alignment.
// Every Done clears the entry's attempt record, and an entry reached again from another row starts at zero attempts.
// A run halted on the row under its name before the rename, Merge-In, resumes with its spent attempts: the row reads the old name's attempt record when its own is absent, and every Done removes both.
// Last it writes the merge record: the tip merged, the task head before the merge, the parent tip last merged into the task, the aligned tip, the merge commit and where the parent's changes since the aligned tip meet the task's changes since its last merge base.
// The aligned tip is the parent tip up to which the merge fixer has read what the parent brought in; it is empty before the first alignment, and the overlap then starts at the task's fork point from the parent.
// The record carries the earlier record's aligned tip forward unchanged, and a record an older build wrote with a reviewed tip reads with an empty aligned tip.
// An up-to-date merge records an empty overlap, and a failed record write is an error, never Stuck.
// Parent-Align holds no lock on any way out of the row, so no later row inherits one.
//
// HandsBack is the one predicate that names the halts the orch takes over: a blocked stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix, and a blocked stop at Finalize whose reason opens with the reroute-cap prefix.
// Parent-Align has no reroute edge, and a reroute-cap halt at another row, such as Webster's, stays routed as before.
// A merge or test failure at Parent-Align or Finalize is the recipe's own to settle:
// the row's fixer fixes it, a spent fix or an over-cap fix its fix review does not approve at Finalize routes back through the recipe's reroute edge, or halts where that edge reaches no review, and the halt goes to the orch.
// No driver, batten, arbiter or record target routes a Parent-Align or Finalize failure to another row.
// The recipe's owner tells it to shedverbs as Spec.HandBack, which marks the loop's stop envelope and the status as hand_back; the drivers read the predicate or that mark.
//
// An escalate report halts with a reason opening with EscalateStopPrefix on a loom or darn run, which Deps.EscalateJudgedFirst tells, and with LandingStopPrefix on a pawl run, whose arbiter has no decision verb.
// EscalateHalt is the prefix's only reader: a blocked stop at Parent-Align, Finalize or Merge-In whose reason opens with it.
// Such a halt is no hand-back, so the driver's arbiter judges it first and batten's binary re-step leaves it alone.
// Only a fixer's escalate changes route; a halt an older build parked under LandingStopPrefix stays a hand-back.
//
// A darn run whose diff lies wholly under its configured non-production trees runs only the tree-scoped command at Parent-Align and Finalize, the hub's whole statement of what such a diff needs.
// landing.yaml's retired `publish_verify` key loads and is dropped, and `lym config reconcile` prunes it.
//
// Landing.yaml's `deploy` is read only by batten's Land row, from the board dir like the rest of landing config and never from a pair.
// It runs in the prime only for a parent branch in `deploy_branches` when the row runs.
// An empty value runs nothing, and `lym config` refuses a write to it from a pair, as for every hub-wide key, as a guard against a mistaken write rather than a determined agent.
// Landing config is hub-wide: Parent-Align and Finalize read it from the board dir, never from a task worktree's `_lym/config/`, and `lym config` refuses a write to it from a pair, so a task cannot weaken the key, or set `deploy_branches` or `squash`, through its own tree.
// A landing change is an operator or hub-orch `lym config landing` run from the prime, committed in `_board` and effective for every pair at once.
//
// Parent-Align and Finalize persist their verify failure.
// When a row's gate call fails with a non-zero exit, Parent-Align or Finalize writes the verifytree landing failure record with the row's gate key as its kind, the failing tier, the failing tests the told Deps.FailingTests parses from the log, HEAD, the write time in whole-second UTC, and the merge-in commit.
// The tier is the first failing tier or build the log's `lym gate test` summary lines name; when that is a build line the record's `build` field is true, its tier is the first tier the build blocked, and it lists no tests, since the gate failed before any test ran.
// A log without a failing line, such as a refusal before the first tier, takes the highest tier of the row's key, read through the told Deps.GateTiers.
// The merge-in commit is HEAD after a merge-in that was not already up to date and empty otherwise; Finalize takes it from its own merge.
// A dirty tree, a timeout and a shell that could not start write no record, and a write failure is logged and never changes the Stuck verdict.
// When Deps.TraceFailure is set and both the log and HEAD are non-empty, the record also carries the trace the seam returns: `cards`, `later_range` and `untraced`, the plan cards and commit ranges the failing locations trace to.
// A nil seam, as for a darn run with no plan, writes none of them, and a trace error is logged at Warn and leaves the record without them, never changing the Stuck verdict or the record's other fields.
// The Stuck reason of a failed verify ends by asking to fix forward on the task branch, then resume.
// When the failed verify's log holds the CPU watchdog's line that `lym gate test` prints after it quits a busy test binary, the reason names the window and says the goroutine stacks are in the log, before that clause.
// A watchdog quit of a healthy package has no code fix: the way forward is raising `busy_quit_sec` through `lym config gate` from the prime, then resuming.
// An unreadable log or one without the line leaves the reason unchanged, and the checks run no flake rerun.
// Parent-Align and Finalize remove the record once their landing call passes.
//
// deploy_branches is read only by batten's Land row, to decide whether a landing deploys:
// a landing into a listed base branch runs the hub-wide `deploy` command, and an empty list deploys nothing.
// It is a list of base-branch names, not a bool, because tasks branch off other tasks and only a landing into a listed branch is a deploy.
//
// Finalize always merges the parent branch into the task worktree and then merges the task branch into the parent pair itself.
// Each merge of the parent, the retry after a merge-in-required error included, runs the same three clean-tree checks and the same post-merge verify gate as Parent-Align before the parent-side merge,
// and a dirty tree is Stuck with the parent branch untouched, while a failing gate runs the merge fixer.
// After each merge of the parent and before the parent-side merge, Finalize commits the parent pair's own run records through the told Deps.CommitParentRecords, so a sibling run's uncommitted status record does not trip the merge guard;
// a clean subtree is a no-op, an absent seam skips the step, and a commit failure is Stuck naming the parent pair and its run records, with nothing merged.
// The landing commit carries the change description, then a blank line and the trailer block.
// The block is landing.yaml's trailer_text, then the engine_trailers text of each engine the task's sessions ran under, in engine-name order, each copied verbatim with no prefix, and then a `Lym-Task: <slug>` line naming the task.
// Finalize reads those engines through the told Deps.EnginesRun before any parent-side mutation, and a read failure is an error with nothing merged.
// An empty trailer_text adds no fixed line, and an engine left out of engine_trailers or mapped to empty text adds no engine line, so the block is the task line alone.
// lym config reconcile moves a retired co_authored_by value into engine_trailers as the claude entry `Co-Authored-By: <value>`, and drops an empty one without an entry.
// Before the merge of the parent, Finalize reads the per-worktree config files the task changed away from both its fork point and the parent's current tip, so a parent-side change never reads as a task edit, and, at Done, queues one notice about them naming the base, task tip and parent tip for the hub's orchestrator;
// it never carries those files to the parent,
// and a read failure or a failed notice never stops the landing.
// Immediately before each landing attempt, the retry's included, Finalize re-reads the parent lock record through the guard's store.
// A record that names this run passes; one that no longer names it means something released it: an owner pause, another machine's ownership or an operator's `lym batten lock release`.
// Finalize then lands nothing, takes no lock and is Stuck with Halt naming the release and its three causes, with `lym loom resume` once no owner pause stands and the run is this machine's as the way forward.
// An unreadable record is a Stuck with Halt too, as at the acquire, and both halts open with LandingStopPrefix, so only the orch's resume lifts them.
// The resume re-runs Finalize from its first step under a lock taken afresh, so a resume made before the cause clears lands nothing.
// The re-read and the landing are not atomic: a release between them is caught only by the parent push's rejection.
// After the parent-side merge and before the push, Finalize marks the board task done, and it opens, looks up and closes no pull request, so the landing makes no GitHub call. A parent
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
// the seat engine's geometry for Describe, and Deps.ParentName passed through to the resolver for Parent-Align and Finalize.
// DescribeTable builds a loom run's session, which reads the decision record and the webster run records;
// DarnDescribeTable builds a darn run's from its own stencil, `landing-template-describe-darn`, which reads the board entry, the decision record beside it, the diff and the review perch's run directory, under the same describe keys, role and skills.
// NewBeamDescribe is the beam recipe's Describe row and starts no agent: it writes the description from the beam's board entry and the subjects of the commits the beam adds, validates it with the check the Describe gate runs, and commits it.
// A finding is an error naming the finding and the way out, which is to end the landing with `lym batten cancel <slug>`, fix the entry with `lym board upsert` and run `lym beam land <slug>` again, since the landing's claim holds the entry under the run lock.
//
// It takes told absolute paths and has no direct production import of internal/lymcwd, per the
// Told-Geometry Invariant: every path this package operates on is handed to it by its caller, and it
// derives none of its own.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set, so none of its identifiers, string literals, or comments may name either fabric-internal side, and that ban is machine-enforced by an AST walk over every identifier (internal/lymcwd's TestEnforcement_FabricVocabulary).
package landingshed
