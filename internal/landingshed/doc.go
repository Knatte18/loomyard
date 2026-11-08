// Package landingshed owns landing's general producers: Parent-Align, the PR-Review row and Finalize.
// Any producer list may name them, and the engine that drives them special-cases none.
//
// PR-Review lands a run through a GitHub pull request when ResolvePullRequest says so;
// PRReview names it for a recipe row.
// Through Deps.PullRequest it reads the receiving branch's beam.yaml `pull_request` and the landing entry's field.
// It also reads the origin and whether the task branch has commits beyond its parent.
// It takes no parent lock.
// A beam.yaml it cannot read is Stuck with Halt naming the worktree and `lym config beam`,
// and a resolver Halt is Stuck with Halt.
// A no logs the reason, drops the record and ends Done.
// A recorded pull request that GitHub reports open is closed first, with the comment:
// `lym closed this pull request: <reason>`.
// A yes the resolver marks MergeAtOnce logs it and ends Done with no push, GitHub call, session or wait;
// Finalize opens and merges that pull request with no review.
// Any other yes pushes the task branch and finds the pull request the record names, or the open one on the task branch;
// failing both, it opens one.
// It sets the title from the change description.
// It sets the body from the description's body, the regression check and the credit block.
// It records the pull request's number, URL, base and head, and then waits on it in process.
// The record's gated head is set to the task head on the first entry only, so a re-entry keeps it.
// A record carrying the operator's ending keeps the run's ending when the task head is still the gated one.
// When the task head is not the gated one, it starts a new review.
//
// The wait has no timeout and wakes at least a second apart.
// The interval doubles to a minute while nothing changes and returns to a second when the task head moves.
// Each wake re-reads the record, which the agent's `lym loom pr-continue` writes the continue flag into;
// a record write goes through the record's update so a flag written meanwhile survives.
// A change to the record file, seen through the PullRequest.RecordChanges watch, ends the sleep between wakes early.
// The early wake comes no sooner than a second after the last wake began.
// A continue flag is thus acted on at once and not after up to a minute.
// A clean tree whose head the record has not gated runs the `pr-review` gate key.
// A pass pushes the head and records it as gated.
// A failure pushes nothing.
// It mails the reason, naming the failing tier and log, to the PR-Review session once per head.
// A failure whose every failed run crashed its test runner mails nothing.
// It halts the row with LandingStopPrefix, naming the crashed runs.
// A head moved or a tree dirtied during the gate discards its verdict.
// A dirty tree is the agent at work and is not gated.
// The pull request's status is read at most once a minute, the entry's own read counting as the first.
// It is also read at once on the first wake that sees the continue flag.
// A read that finds the pull request open, with no continue flag or ending recorded, sets its title and body again.
// That happens only when the change description yields other text than they were last set to.
// The agent therefore changes the title and body by editing the description.
// A description that yields no text, as one mid-edit may, is logged once and left for a later read.
// Merged at the gated head on a clean tree is Done.
// Merged without task commits or at another head is Stuck with Halt naming the way forward.
// Closed after the operator's ending is Done on a clean tree, and closed otherwise is Stuck with Halt.
// The continue flag on an open or closed pull request at a clean gated head records the operator's ending and is Done.
// It leaves the pull request as it is for Finalize to merge on GitHub;
// ending the review never lands the run by a direct push.
// An open pull request approved at the gated and task head is Done.
// A merged pull request takes the merged verdicts whatever the flag says.
// An uncommitted file at any ending is Stuck with Halt.
// A pause that covers the run holds the row in process, before its push and at each wake.
// The row does no gate, push or GitHub call meanwhile.
//
// An entry that finds its recorded pull request a draft marks it ready for review before it sets the title and body.
// The pull request thus keeps its link, comments and history.
//
// The row keeps one interactive agent session per run, for the operator to discuss the pull request in.
// The session has a window of its own in the task worktree, and the row never waits on its output.
// After the row records the pull request and before it waits, it reuses the recorded strand when that is live.
// When the strand is not live it resumes the recorded session id, and otherwise it starts a new session.
// A review whose record carries the operator's ending gets no session.
// The session starts from the `landing-template-pr-review` stencil, filled with paths and the regression check.
// It runs under the `pr_reviewer` model, and its pane process is registered under the run.
// The publish shell guard is installed:
// it leaves the session gh's reads, comments and title and body edits,
// and it refuses git push and every gh call that merges, closes or opens a pull request.
// A start that fails transiently is an error the driver re-steps,
// and any other start failure is Stuck with Halt naming `lym loom resume`.
// The row stops the session before each read that may end the review.
// Such a read finds a merged pull request, an approval of the gated head or the continue flag.
// The row resumes the session when that read keeps waiting.
// The row stops the session at the first wake a pause covers and resumes it at the first uncovered wake.
// It also stops the session on every verdict, Stuck and error, and leaves it running when the context is cancelled.
// The wait's wakes start no session.
// An operator who closes the window ends the agent until the row is next entered.
// The driver strand's wait mark reads `PR #<n>` while the wait runs.
// A transient push or GitHub failure is an error the driver re-steps.
// An unresolvable or rejected token, a push the fabric refuses and any other GitHub failure are Stuck with Halt.
// Each Halt names its way forward.
// The way forward names the token only for a token failure or a 401 or 403 response;
// a 422 says GitHub accepted the token but refused the request's content.
// A nil seam, an empty record path and a cancelled context are errors.
//
// ResolvePullRequest is the one decision of whether a landing goes through a pull request.
// PR-Review and Finalize therefore cannot disagree.
// It takes the receiving branch's beam.yaml `pull_request` (`on`, `off`, `required` or `merge`).
// It takes the landing entry's `pull_request` field (empty, `on`, `off` or `merge-in`).
// It takes the origin facts and whether the task branch has commits beyond its parent.
// It reads no file:
// its callers read the parent's beam.yaml, the entry and the task branch through git and tell the values.
// A required or merge parent wants a pull request whatever the field says.
// An on parent wants one unless the field is off, and an off parent wants one only when the field is on.
// A merge parent's yes is marked MergeAtOnce:
// the remote takes that parent only through a pull request, with no approval or check to wait for,
// so the pull request is opened and squash-merged at once.
// A `merge-in` field, a merge task's, is a no under every parent and GitHub rule, marked TrueMerge.
// PR-Review passes at once and Finalize's parent-side merge does not squash,
// so the beam keeps the parent's commit as an ancestor, and the beam's own pull request reviews the merge.
// A push the remote refuses then is Stuck as for any landing with no pull request.
// beam.yaml's `pull_request` set at the default layer applies to every branch of the hub.
// Set at a worktree's layer, it applies to that worktree's branch.
// A wanted pull request needs an origin on github.com that the owner and repository parse from.
// It also needs an upstream for the parent and no push skip.
// A task branch with no commit beyond its parent is a no, judged before any origin fact;
// GitHub opens no pull request on an empty branch.
// Both rows then take the no path, and Finalize's parent-side merge commits nothing.
// For a beam it also takes what the beam's landing record holds, `BeamRecord`, and does not read the entry's field.
// A recorded pull request is wanted whatever the mode says.
// A required or merge parent and GitHub's rules want one as for any run.
// An on or off parent with none recorded wants none.
// Every beam yes is marked MergeAtOnce, since a beam always lands by squash on GitHub with no review.
// A missing fact is a Halt when the parent is required or the field is on.
// It is a no with the fact named when the parent is on alone.
// An origin on github.com that names no repository is a Halt in every mode that wants a pull request.
// It also takes whether GitHub's rules for the parent branch require a pull request.
// That counts as a required parent whatever beam.yaml and the field say.
// GitHub refuses the direct push a landing without a pull request ends in.
// Both rows read those rules, `GET repos/{owner}/{repo}/rules/branches/{branch}`, only in one case:
// beam.yaml and the field give a no for a task branch with commits and an origin a pull request could be opened on.
// They then ask the resolver again with the rules.
// A cancelled context and a transient failure of that read are errors the driver re-steps.
// Any other failure of the read is a Halt when the run holds a pull request record;
// an unusable token is the exception.
// Reading it as no rule would close that pull request and land by a direct push GitHub refuses.
// For a run with no record, any other failure, an unusable token included, logs a Warn and reads as no rule.
// Bound: a rule the read misses still ends in a direct push GitHub refuses.
// Branch protection set outside repository rules does too.
//
// A merge or test failure at Parent-Align or Finalize is owned by the recipe:
// it ends in the row's fixer or a halt to the orch, and Finalize never sends the run back to an earlier row.
//
// Finalize holds the parent lock, which it takes through the shared guard.
// The guard grants a run that already holds it, so a re-call after a resume takes back a record handed to batten.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// Finalize's constructor refuses a nil Deps.Lock.Store, a nil Deps.Lock.Owner and a nil Deps.ReadParentTips.
// A row built without its guard, as a struct-literal test builds it, takes no lock.
// Finalize takes the lock after parsing the change description and committing the status file.
// It takes it before anything reads or moves the parent.
// It releases the lock on every way out of its Call: Done, a Stuck and an error.
// A lock a dead Finalize left is taken over at the next acquire.
// The lock store grants a record whose owner process no longer lives.
// Finalize frees the lock before its merge fixer or a fix review starts, since neither touches the parent.
// Other runs land on the parent meanwhile; a Finalize with no lock store frees nothing.
// Once their change is let through, Finalize takes the lock again at the front of the queue.
// It goes ahead of runs that queued while it fixed.
// It then runs every step under the lock again from the pull-request step on:
// it merges the parent anew and verifies under the lock as on its first try.
// When nothing landed meanwhile, the merge is a no-op and the verify is skipped.
// A failed verify on such a pass spawns the fixer on the attempts the entry has left, never a fresh cap.
// A Finalize resumed with an attempt record that shows a fixer's change let through queues at the front as well.
// This holds whatever halted it, since the record keeps that change until a Done.
// The attempt record keeps the HEAD at which the fixer's change was last let through.
// A pass whose merge brings parent commits onto that HEAD moves the entry's Baseline to the merged head,
// so the parent's commits never count as the fixer's change.
// Bound: a fixer change let through before such a merge and one made after it are each judged against the caps alone.
// While the fixer works, `lym loom status` shows its session.
// While the run queues again, it shows the parent lock wait.
// A hub pause that covers the run holds the fixer's gate calls and the requeue wait like any other run's;
// the run holds no lock then.
// Under that lock it runs the same run-scope origin reconcile as Parent-Align.
// It maps the reconcile's ended and stopped verdicts as Parent-Align does, and then runs the owner checks.
// It then opens the parent pair and syncs it before any merge.
// A pair that is missing, mid-merge or holding tracked dirt in its code checkout is Stuck with its way forward.
// That holds whether the parent is behind, equal, without an upstream or under the push skip.
// With no push skip a parent behind its upstream is fast-forwarded to the upstream tip in its own pair.
// That touches no local commit.
// A parent ahead of its upstream, diverged from it or no longer a fast-forward of it is Stuck.
// Finalize never pushes or rewrites the parent here.
// Under the push skip it neither fetches nor fast-forwards.
// A parent that already holds the task's work is past its parent-side merge and is left as it is.
// A run whose Finalize merged into the local parent here is never read as ended,
// so a Finalize re-entered after its done mark proceeds;
// a Finalize entering with no such merge, its entry done on origin, stops with Halt.
// Another machine landed that run.
//
// The owner checks let a run land only on the machine that owns it and its parent.
// A tapestry run and a darn run, landing with no orch turn, pass them too.
// Finalize reads Deps.ParentHoldsTaskWork first.
// A parent already holding the task's work marks a Finalize past its parent-side merge.
// Such a Finalize skips both owner checks and the unreachable halt.
// Otherwise an unreachable origin halts.
// The run's owner is the owner of its owned_elsewhere entry.
// The parent's owner comes from Deps.ParentOwner, read from the local board the reconcile left.
// A run whose pair has no batten run record reads as owned here, since the reconcile judges only recorded pairs,
// but its parent's owner is still read.
// Each of these stops with Halt and a reason behind OwnerHaltPrefix, so the loom halt note is written:
// a parent or run owned by another machine, an unreachable origin and a failed read of either seam.
// The reason names the owners and the `lym adopt` commands that move them.
// Parent-Align proceeds on an owned_elsewhere verdict, since the owner check belongs to the landing.
// A nil seam reads as not past the merge and as the parent owned here.
//
// The land check follows the owner checks under the same lock.
// A Finalize built with Deps.LandBlockers, a pawl run's, runs it.
// The check reads the open members and the local pairs forked from the beam.
// A read error or any blocker halts behind LandHaltPrefix.
// It is a backstop for an open member the run's earlier re-check missed.
// A stale board on another machine can add such a member.
// Each way forward is Finalize's own.
// None uses the land-it wording of the beam's landing refusal, which the standing landing hold refuses:
// a forked pair, and a member with a pair or a run in flight, name `lym batten cancel <slug> --remove`;
// it runs in Deps.ParentWorktree, or on the machine holding the pair when the run is in flight and its pair is not here;
// `lym batten status <slug>` there shows it;
// a member then names moving its `project` off the beam with `lym board upsert`;
// the cancel leaves it a `draft` member, which still blocks the landing;
// a member with no run status names that move alone;
// every halt ends in `lym loom resume` in the beam's worktree.
// A Finalize already past its parent-side merge, an empty list and a nil seam never halt on the check.
// Bound: a member that lands into the beam between this check and the parent-side merge is not caught here.
// The Worktree-Create hold read keeps any member from forking once the landing hold stands.
// A parent that already holds the task's work lands through the already-landed path.
// That happens after a GitHub merge or an earlier landing.
// Finalize then merges the parent branch into the task worktree through its resolver for every recipe.
// The resolver resolves a conflict with the conflict session under the lock, and Finalize gates the merged tree.
// It reads the merge record's merged tip, clears a parked merge-in, checks the tree clean and merges.
// An unreadable merge record is logged and read as absent, so the parent side is measured from the task's merge base.
// A run with no record is measured the same way.
// The merge is large when the parent's changes since the merged tip it read touch a file the task changed.
// The task's changes are those since its merge base.
// A parent already holding the task's work is never large.
// A large merge writes the merge record, with its overlap, and goes on to the gate like any other merge.
// Bound: what lands with no review is the conflict session's resolution against the parent's newer work.
// It also includes a textually clean merge of the parent's changes to files the task also changed.
// The `finalize` gate over the merged tree bounds both.
// A spent fixer halts for the orch.
// A change past the caps lands only behind the fix review's approval or the operator's acceptance.
// The conflict session runs while the parent lock is held, so sibling landings onto that parent wait.
// The wait is bounded by the lock's owner, pause and release rules.
// Finalize then writes the merge record with the new merged tip and the aligned tip as it was.
// Next it runs its own gate key, `finalize`, over the merged tree through verifytree.
// Verifytree skips a tree it already verified,
// so a run that went right holds the lock for one merge, a skipped gate and the landing.
// A darn Finalize over a diff wholly under `tree_scope` runs the tree-scoped command.
// Darn's verify source answers it.
// A passing gate removes the landing failure record, and a failing one writes it.
// A failing gate spawns the merge fixer in finalize mode, which fixes failing tests only.
// The attempts are what `finalize_fix_attempts` leaves the entry, read from the attempt record under the row's entry.
// The fixer gets no parent tip to read from and the merged tip the merge record carries.
// The first spawn writes the HEAD before the fixer's first turn into the record's Baseline.
// Every resume of the entry keeps it.
// The fixer's whole change is judged by `LineChange(Baseline, HEAD)`.
// The judgment is against `finalize_fix_max_lines` and the run's test-file matcher.
// Turns that each stay under the cap but sum past it are over it.
// An entry has no attempts left when its Spent reaches `finalize_fix_attempts`, whatever the record's Halt notes.
// A resumed entry with none left spawns no fixer:
// over a failing gate it ends as a spent cap, and over a passing one its change since Baseline is judged.
// A change over the caps, in every recipe, goes to the fix review.
// The review is one solo session on the fixer's model-spec and turn timeout.
// It reads only the fixer's commits, Baseline..HEAD, and writes the report `landing-fix-review-spec.md` pins.
// An approval lands the change and keeps the approved HEAD in the record's Reviewed;
// a later pass of the entry at that HEAD lands with no second review;
// a rejection hands its findings to the fixer, which fixes them in place on a fresh `finalize_fix_attempts` turn cap;
// the fixed change is judged again;
// a session that ends without a verdict runs again;
// each review spends one of `finalize_fix_review_rounds`;
// a review that does not approve on the last round halts with LandingStopPrefix;
// the record notes `review-capped` and the open findings;
// the way forward is `lym loom circling accept` or a fix on the task branch;
// a session that dirties the tree or moves the task head halts with LandingStopPrefix;
// one that fails to start is a transient error.
// Bound: the fix review lets an over-cap fixer change land behind one reviewer's approval of the fixer's own commits.
// The run's full Code review does not apply, and the parent lock is freed.
// The fixer's turns run between at most `finalize_fix_review_rounds` review sessions of `fixer_timeout_min` each.
// AcceptFixReview is the operator's acceptance after that halt.
// It appends the open findings to the change description under `## Unresolved fix review findings`,
// and it records the task HEAD it accepted at.
// The resume lands the change as it stands at that HEAD.
// A resume with no acceptance clears the note and the rounds.
// It runs the fix reviews afresh over the change as it then stands.
// A spent cap keeps the fixer's commits and halts with LandingStopPrefix.
// The halt names the commits and notes `spent` in the record's Halt.
// An escalate report keeps the commits and halts naming the report.
// The halt opens with EscalateStopPrefix on a tapestry, darn or sampler run.
// A turn that ended without a usable result halts, and a failed spawn is an error that spends no attempt.
// An attempt record that does not decode halts naming the file.
// The way forward is removing it, which grants a fresh cap.
// A Done clears the entry's attempt record.
// The size and kind caps bound what a Finalize fix lets through without a reviewer:
// a fix within them lands with no perch.
// Only the `finalize` gate it passed and the finalize mode's instruction to fix failing tests only stand behind it.
// After a merge-in-required error from the parent-side merge, Finalize retries once.
// That covers a parent moved by run records alone:
// the retry merges the parent again and gates the merged tree under the same lock,
// so a code-side commit that arrives between the first merge and the parent-side merge is merged, gated and landed.
// A dirty parent is Stuck naming the parent's code checkout and its tracked dirty paths, capped at 20.
// The Stuck reason carries the way forward per reason.
// The run records Finalize commits at its parent-records step never count as that dirt.
// The guard reads the code checkout alone, whose `_lym` link is excluded from its index.
// A parent push that fails for a reason other than a transient one carries a repair note in one case:
// origin's parent already holds the task's work, which an adopter-first landing leaves.
// The note names the task's squash by the parent head's SHA.
// It gives a `git reset --keep` to origin's parent when the squash is the only commit origin lacks.
// When another commit is listed, it gives a `git rebase --onto` dropping the squash.
// It ends with `lym fabric remove` of the run, leaving the owner pause standing.
// An origin parent lacking the work, or a failed read, adds no note.
// A parent push the remote refuses as a protected branch is read from GitHub's rejection wording:
// `protected branch` or `repository rule violations`.
// Its way forward replaces that note and any push class's way forward.
// It names the setting that lands the parent through the remote:
// `pull_request=merge` in the parent's worktree, or at the default layer for every branch.
// It also names the hand landing of the work the local parent alone holds:
// through a pull request, `git reset --keep origin/<parent>` and `lym loom resume`.
// A landing that went through a parent already marked merge leaves the setting out and keeps the hand landing.
// Bound: such a run is marked done on the board while its work is on the local parent only, until that hand landing.
//
// Parent-Align and Finalize clear the merge-in their own earlier attempt left parked.
// They clear it before the clean-tree check that precedes each merge of the parent, Finalize's retry pass included.
// A parked fabric `merge-in` of the parent branch is aborted, whoever started it.
// A conflict session's resolution and a hand-started merge-in with its partial resolution are both discarded,
// and the merge starts afresh.
// Every live conflict-session strand of the run is stopped through Deps.StopConflictSession first.
// The merge is aborted only after every stop succeeded, so no session is left writing into the redone merge;
// a stop that fails is Stuck naming the strand, with the merge untouched.
// Any other parked merge and git merge state fabric did not start is Stuck with its way forward.
// Such a state is never stopped over or touched.
// The resolver's own abort stays as crash recovery within one merge-in step.
// The seams are nil-is-absent: a nil Deps.MergeState skips the step.
//
// Parent-Align is built here and listed by a recipe's producer list like the others.
// It commits the status file and runs the run-scope origin reconcile through the told Deps.Reconcile.
// It then picks the parent tip to merge.
// Parent-Align takes no parent lock across the merge and writes nothing in the parent pair:
// the fast-forward of the local parent branch and the parent-pair refusals belong to Finalize, under its lock.
// The reconcile judges and never moves the parent.
// A run origin ended is a Stuck with Halt naming the landing or the board removal;
// the way forward is `lym cleanup --apply` from the prime;
// a run its branches stop is a Stuck naming the branch, its state and its way forward;
// the way forward ends in `lym loom resume` in the task worktree;
// an unreachable origin logs a Warn and proceeds, so the row fails on its own remote steps;
// a reconcile error is an error.
// A stopped reason whose way forward names `lym fabric remove` ends in `Deps.RemoveFollowUp`'s text, not that resume;
// the removal deletes the task worktree the resume runs in;
// an empty answer and a nil seam keep the resume, and so does a Finalize whose parent already holds the task's work.
// With an upstream and no push skip it fetches and merges the parent's upstream tip.
// The tip is read from the fetching tips read and never the local tip,
// so a landing another run made locally and has not pushed is never merged in.
// Under the told push skip, or with no upstream, it takes the parent lock through the shared guard.
// It holds it only for the read of the local tip and releases it before the merge, which then runs unlocked.
// A paused lock wait and an unreadable lock record are Stuck with Halt, so the run stops at the row.
// It then clears a parked merge-in and merges that tip into the task worktree through internal/mergeresolve.
// It runs its own gate key, `parent-align`, over the merged tree.
// The task worktree must be clean at three points:
// before the merge-in (after the status commit), after the merge-in, and after the verify.
// A dirty tree at any of them is Stuck naming the paths, so nothing uncommitted is merged over or reviewed;
// a verify that dirties the tree is a non-hermetic test and halts rather than ships.
// The row's gate key is the told Deps.GateKey.
// Each row's registry entry sets it from its optional `gate_key` config key, else its row name in lower case.
// The told call is that key's `lym gate test` call, rendered from the hub's gate config at each run.
// It runs through internal/verifytree, unless its verified-tree record already names the tree and the command.
// The merge fixer a row spawns judges each turn through that row's gate, so it runs the same key.
// A gate whose every failed run crashed its test runner with no failing test judged no code.
// That holds for the run and its one rerun.
// The row halts with LandingStopPrefix at Parent-Align, at Finalize, after a fixer's turn and at the pr-review gate.
// The halt names the crashed runs and the way forward of `lym loom resume`, which runs the gate again.
// It spawns no fixer and writes no landing failure record.
// A fixer turn that leaves the tree dirty is re-prompted, on a charged turn, with the paths.
// The prompt asks it to commit what belongs to the fix and delete the rest.
// Only a dirty tree after the last turn ends the fixer.
// Its base command is the told Deps.BaseCommand, the fix rows' `code-burler-fix` call.
// A landing pass thus keeps the entry the Code-Burler-Fix lint base reads.
// A refusal of the gate config or of the key is the producer's returned error, carrying the refusal's way forward.
// A verify that does not finish within the verify timeout is killed and halts the run Stuck with the log path.
// While the verify runs, the told Deps.VerifyWaitMark callback marks the run's driver strand as `verify <producer>`.
// The callback clears the mark on every way out of the verify.
// The mark is display only: a failing or absent callback changes no Stuck reason, error or verify outcome.
// The record is keyed on the tree.
// The verify therefore runs after a resolved conflict and after new parent commits.
// It also runs after a crash between a merge and its verify.
// A no-op merge onto a verified tree skips it.
// A missing command logs a warning and proceeds.
// A failed verify writes the landing failure record.
// A passing verify, or a skip over a tree recorded as passed, removes it.
// After the gate Parent-Align aligns.
// The parent brought in commits when its merged tip adds something over the aligned tip.
// When the merge record holds no aligned tip, the task's fork point from the parent stands in for it.
// A merge that brought none over a passing gate is aligned with no fixer.
// It advances the aligned tip to the merged tip.
// Parent commits brought in, or a failing gate, run the merge fixer in parent-align mode.
// The attempts are what `parent_align_fix_attempts` leaves the entry.
// A `fixed` report over a passing gate advances the aligned tip and is Done.
// The entry's fork point is kept in its attempt record before the merge,
// so a resumed entry whose merge is already up to date still reads what the first merge brought in.
// A fixer change counts as over-cap when it exceeds `finalize_fix_max_lines`.
// It also counts when it touches a test, testdata or gate configuration file, or adds a test skip or a build constraint.
// With the Code-Review perch's budget 0, or told nothing, an over-cap change halts.
// With a budget above 0 the change goes to the Code-Perch unjudged.
// Each of these halts with a reason opening with LandingStopPrefix and a way forward:
// a spent cap, a turn that ended without a usable result, a tree left dirty after the last turn and an over-cap change.
// A dirty-tree reason names the paths.
// The resume after an over-cap halt reads the operator's review as the alignment.
// Every Done clears the entry's attempt record, and an entry reached again from another row starts at zero attempts.
// A run halted on the row under its earlier name, Merge-In, resumes with its spent attempts:
// the row reads the earlier name's attempt record when its own is absent, and every Done removes both.
// Last it writes the merge record.
// The record holds the tip merged, the task head before the merge and the parent tip last merged into the task.
// It also holds the aligned tip, the merge commit and the overlap.
// The overlap is where two change sets meet:
// the parent's changes since the aligned tip, and the task's changes since its last merge base.
// The aligned tip is the parent tip up to which the merge fixer has read what the parent brought in;
// it is empty before the first alignment, and the overlap then starts at the task's fork point from the parent.
// The record carries the earlier record's aligned tip forward unchanged,
// and a record written with a reviewed tip, as an older build did, reads with an empty aligned tip.
// An up-to-date merge records an empty overlap, and a failed record write is an error, never Stuck.
// Parent-Align holds no lock on any way out of the row, so no later row inherits one.
//
// HandsBack is the one predicate that names the halts the orch takes over:
// a blocked stop at Parent-Align or Finalize whose reason opens with LandingStopPrefix.
// Neither row has a reroute edge, and a reroute-cap halt at another row, such as Webster's, stays routed.
// A merge or test failure at Parent-Align or Finalize is the recipe's own to settle:
// the row's fixer fixes it, and Finalize's fix review and fixer settle an over-cap fix in place;
// a spent fix or a fix review out of rounds halts, and the halt goes to the orch.
// No driver, batten, arbiter or record target routes a Parent-Align or Finalize failure to another row.
// The recipe's owner tells the predicate to shedverbs as Spec.HandBack.
// It marks the loop's stop envelope and the status as hand_back.
// The drivers read the predicate or that mark.
//
// Deps.EscalateJudgedFirst tells which prefix an escalate report halts under.
// On a tapestry, darn or sampler run it halts with a reason opening with EscalateStopPrefix.
// On a pawl run, whose arbiter has no decision verb, it halts with a reason opening with LandingStopPrefix.
// EscalateHalt is the only reader of the prefix:
// a blocked stop at Parent-Align, Finalize or Merge-In whose reason opens with it.
// Such a halt is no hand-back, so the driver's arbiter judges it first and batten's binary re-step leaves it alone.
// Only a fixer's escalate changes route; a halt an older build parked under LandingStopPrefix stays a hand-back.
//
// A darn run's diff may lie wholly under its configured non-production trees.
// Parent-Align and Finalize then run only the tree-scoped command, the hub's whole statement of what such a diff needs.
// landing.yaml's `publish_verify` key loads and is dropped, and `lym config reconcile` prunes it.
// landing.yaml's `pr_reviewer` is the model-spec of the PR-Review row's interactive session.
//
// landing.yaml's `deploy` is read only by batten's Land row.
// It is read from the board dir like the rest of landing config, never from a pair.
// It runs in the prime only for a parent branch in `deploy_branches` when the row runs.
// An empty `deploy` value runs nothing.
// Landing config is hub-wide.
// Parent-Align and Finalize read it from the board dir, never from a task worktree's `_lym/config/`.
// `lym config` refuses a write to it from a pair, as a guard against a mistaken write rather than a determined agent.
// So a task cannot weaken `deploy`, or set `deploy_branches` or `squash`, through its own tree.
// A landing change is an operator or hub-orch `lym config landing` run from the prime.
// It is committed in the Bolt checkout and effective for every pair at once.
//
// Parent-Align and Finalize persist their verify failure.
// Parent-Align and Finalize write the verifytree landing failure record when a gate call fails with a non-zero exit.
// The record holds the row's gate key as its kind, the failing tier and HEAD.
// It also holds the write time in whole-second UTC and the merge-in commit.
// The record also holds the failing tests the told Deps.FailingTests parses from the log.
// The tier is the first failing tier or build the log's `lym gate test` summary lines name.
// A build line sets the record's `build` field true and the tier to the first tier the build blocked.
// Such a record lists no tests, since the gate failed before any test ran.
// A log without a failing line, such as a refusal before the first tier, takes the highest tier of the row's key.
// The told Deps.GateTiers reads that tier.
// The merge-in commit is HEAD after a merge-in that was not already up to date, and empty otherwise;
// Finalize takes it from its own merge.
// A dirty tree, a timeout and a shell that could not start write no record,
// and a write failure is logged and never changes the Stuck verdict.
// With Deps.TraceFailure set and a non-empty log and HEAD, the record also carries the trace the seam returns:
// `cards`, `later_range` and `untraced`, the plan cards and commit ranges the failing locations trace to.
// A nil seam, as for a darn run with no plan, writes none of them,
// and a trace error is logged at Warn and leaves the record without them.
// It never changes the Stuck verdict or the record's other fields.
// The Stuck reason of a failed verify ends by asking to fix forward on the task branch, then resume.
// The CPU watchdog prints a line in the `lym gate test` log after it quits a busy test binary.
// When the failed verify's log holds that line, the reason names the window and says the log holds the goroutine stacks.
// That text comes before the fix-forward clause.
// A watchdog quit of a healthy package has no code fix:
// the way forward is raising `busy_quit_sec` through `lym config gate` from the prime, then resuming.
// An unreadable log or one without the line leaves the reason unchanged, and the checks run no flake rerun.
// Parent-Align and Finalize remove the record once their landing call passes.
//
// deploy_branches is read only by batten's Land row, to decide whether a landing deploys:
// a landing into a listed base branch runs the hub-wide `deploy` command, and an empty list deploys nothing.
// It is a list of base-branch names, not a bool.
// Tasks branch off other tasks, and only a landing into a listed branch is a deploy.
//
// Finalize always merges the parent branch into the task worktree first.
// It then merges the task branch into the parent pair itself.
// Each merge of the parent runs the same clean-tree checks and post-merge verify gate as Parent-Align.
// They run before the parent-side merge, and the retry after a merge-in-required error is included.
// A dirty tree is Stuck with the parent branch untouched, while a failing gate runs the merge fixer.
// After each merge of the parent and before the parent-side merge, Finalize commits the parent pair's own run records.
// The commit goes through the told Deps.CommitParentRecords.
// It keeps a sibling run's uncommitted status record from tripping the merge guard.
// A clean subtree is a no-op and an absent seam skips the step.
// A commit failure is Stuck naming the parent pair and its run records, with nothing merged.
// The landing commit carries the change description, the regression check and the credit block.
// Blank lines separate them.
// The credit block is plain lines, never git trailers.
// The regression check is the section the told Deps.DriftVerdict renders from the task's Drift records,
// so a check the Drift rows ran reaches the pull request and the landing commit.
// It does so whether or not Describe rewrote the description after the check.
// An empty section and a nil seam add nothing.
// Finalize reads it before any parent-side mutation, and PR-Review each time it sets the pull request's body;
// a read failure is an error.
// With landing.yaml's credit on, the block is credit_heading, then these ` - <item>` lines:
// one for credit_loomyard and one for the credit_engines text of each engine the task's sessions ran under.
// The engines come in engine-name order.
// Finalize reads those engines through the told Deps.EnginesRun before any parent-side mutation,
// and a read failure is an error with nothing merged.
// An empty text adds no line.
// A block with no item line, or with credit off, is empty, so the message ends at the body.
// PR-Review sets the pull request's body to the same body, regression check and block,
// so the commit and its pull request carry one credit.
// The co_authored_by, trailer_text and engine_trailers keys load, and `lym config reconcile` prunes them.
// Pruning carries no value over.
// Before the merge of the parent, Finalize reads the per-worktree config files the task changed.
// A file counts as changed when it differs from both the task's fork point and the parent's current tip,
// so a parent-side change never reads as a task edit.
// At Done, Finalize queues one notice about them for the hub's orchestrator, naming the base, task tip and parent tip.
// Finalize never carries those files to the parent, and a read failure or a failed notice never stops the landing.
// Finalize re-reads the parent lock record through the guard's store immediately before each landing attempt.
// The retry's attempt is included.
// A record that names this run passes;
// one that does not name it means something released it:
// an owner pause, another machine's ownership or an operator's `lym batten lock release`.
// Finalize then lands nothing, takes no lock and is Stuck with Halt.
// The Halt names the release and its causes,
// and it gives `lym loom resume` as the way forward once no owner pause stands and the run is this machine's.
// An unreadable record is a Stuck with Halt too, as at the acquire.
// Both halts open with LandingStopPrefix, so only the orch's resume lifts them.
// The resume re-runs Finalize from its first step under a lock taken afresh,
// so a resume made before the cause clears lands nothing.
// The re-read and the landing are not atomic: a release between them is caught only by the parent push's rejection.
// After the parent-side merge and before the push, Finalize marks the board task done.
// It hands the parent pair's head, the landing commit, to the told Deps.RecordLanding.
// It then hands the parent head it merged into the task worktree and the landing commit to Deps.RecordLandedSize;
// a failure of either is only logged.
// A Finalize that read no such parent head in its process, as one resumed past the merge, records no landed size.
//
// Finalize lands through the run's pull request, which it merges on GitHub only here, under the parent lock.
// Right after it takes the lock, Finalize asks ResolvePullRequest the question PR-Review asked.
// The inputs are the same beam.yaml, entry field, parent-branch rules, origin and task-branch commits.
// A Finalize with no `ParentMode` seam skips the step and lands locally.
// A no closes a recorded pull request GitHub reports open, with the comment `lym closed this pull request: <reason>`.
// It then removes the record and lands locally.
// A yes never lands by a local merge pushed to the parent.
// A yes with no record is Stuck with Halt naming `lym loom goto PR-Review`, except a yes marked MergeAtOnce.
// A yes whose pull request GitHub reports closed while the record carries no operator's ending is Stuck with Halt.
// That Halt names a reopen on GitHub and `lym batten cancel`.
// A yes marked MergeAtOnce takes neither halt.
// PR-Review writes that mark on the operator's continue.
// Finalize writes it on the record it opens itself for such a yes.
// GitHub squash-merges only an open pull request,
// so a closed one is reopened on GitHub and then merged like an open one.
// A yes whose record carries the operator's ending and whose pull request is closed gets that reopen.
// So does any closed recorded pull request under a yes marked MergeAtOnce.
// A failed reopen is judged as any GitHub failure.
// The reopen under a yes marked MergeAtOnce comes after the owner checks and the lock re-check.
// It comes right after the task push, so a machine that may not land the run reopens nothing.
// A merged pull request makes no GitHub merge,
// and the parent sync fast-forwards to the merged upstream so the parent-side merge takes the already-landed path.
// An open pull request is merged on GitHub after the lock re-check and before the parent-side merge.
// Finalize pushes the task branch and merges with the pushed head as the expected head.
// `squash` chooses the method.
// The commit title is the change description's title followed by ` (#<number>)`.
// That is the form GitHub's own squash merge writes,
// so the landed commit names its pull request after the board entry is pruned.
// The commit message is the body and the credit block.
// Finalize then fetches and fast-forwards the parent pair.
// A yes marked MergeAtOnce with no record opens the pull request at that point, after the task push.
// Finalize finds the open pull request from the task branch into the parent and sets its title and body.
// Failing that, it opens one with that title and body.
// It records the pull request with the pushed head as gated and the review ended, and merges it.
// A yes marked MergeAtOnce always merges by squash, whatever `squash` says.
// That holds on GitHub and in the parent-side merge alike.
// A beam's landing is one whose PullRequest.BeamRecord is set.
// It never reads the entry's field.
// Under the lock it reads the beam's landing record beside the run's own pull request record.
// A recorded pull request GitHub reports open is merged on GitHub, squashed, with no edit of its title or body;
// a closed one is Stuck with Halt naming the operator reopening it on GitHub and `lym loom resume`.
// A recorded pull request merged at the task head lands through the already-landed path;
// one merged at another head does too when it lacks no task commit;
// otherwise the landing is decided as with none recorded.
// With none recorded, an open pull request in the run's own record is merged as it stands;
// otherwise the resolver decides:
// a yes finds or opens the pull request from the beam branch, leaves the text of one it finds as it is, and merges it;
// a no lands by the local squash.
// Once the parent-side landing completes, `DropBeamRecord` drops a spent recorded pull request from the landing record;
// a failed drop is logged only.
// A refused merge of a beam's pull request names GitHub's message, `lym beam pr status <slug>` and `lym loom resume`.
// It never names `lym batten cancel`.
// A merge GitHub refuses as pending is retried for at most a minute under the lock.
// A method refusal names `lym config landing --set squash=<true|false>`, or the repository's settings under MergeAtOnce.
// Any other refusal names a second person's approval or a settled check or conflict, and `lym batten cancel`.
// The retry after a merge-in-required error never merges on GitHub a second time.
// Every GitHub failure is judged as PR-Review judges one, with each halt opening with LandingStopPrefix.
//
// A parent that already holds the task's work makes Finalize idempotent:
// it takes the already-landed path rather than merging again.
// The parent branch is pushed to its upstream, so a landing reaches the remote.
// It does not stop at the hub's own parent worktree.
// The parent-side merge is the producer's own merge critical section.
// Finalize.Call documents which of its steps that is.
//
// Finalize's parent-side merge squashes by default, except a landing the resolver marks TrueMerge.
// The default comes from landing.yaml's `squash` key, which defaults to true.
// A squashed branch leaves one commit per task branch and no merge commit linking it to its target.
// Git history alone then cannot show whether a branch was merged.
//
// The Describe session's skills come from agents.yaml (role `describer`) and load before its prompt.
// Its stencil and the conflict session's carry a parent directive that internal/parentdirective renders.
// The told name is the seat engine's geometry for Describe;
// for Parent-Align and Finalize it is Deps.ParentName, passed through to the resolver.
// DescribeTable builds every loom recipe's session from one stencil.
// The stencil reads the decision record, the board entry, the diff and the run records that exist,
// and it renders an absent record as `(none)`.
// The stencil also shows the regression check the landing appends,
// so the session writes no section of its own for it and lists no check it settled as one to run by hand.
// A beam's change description is written by the beam's orch; this package only drafts and records it, starting no agent.
// DraftBeamDescription writes a starting point the orch edits:
// the beam's board entry and the commits on the beam branch's own first-parent line.
// It never draws on the project record's decisions.
// Its `## Changes` list is ordered oldest first.
// It names each pull request a member landed through by the ` (#<number>)` Finalize writes into the merge's subject.
// Each such entry gives the title, merge time and first sentence of the body.
// It names each other commit, a merge from the parent included, by its object name, subject and time.
// It takes its told seams as BeamDraftDeps, which `lym beam describe --draft` fills.
// RecordBeamDescription validates the orch's text with the check the Describe gate runs, and it commits it.
// A finding is an error naming the finding and the way out:
// fix the description file and run `lym beam describe <slug>` again.
// OriginMissing names the first fact an origin lacks for a pull request, in the order ResolvePullRequest judges them,
// so `lym beam pr open` refuses an origin the resolver would refuse.
//
// It takes told absolute paths and has no direct production import of internal/lymcwd, per the Told-Geometry Invariant:
// every path this package operates on is handed to it by its caller, and it derives none of its own.
//
// This package describes one repository throughout.
// It is not in the Fabric Vocabulary Invariant's owner set.
// None of its identifiers, string literals, or comments may name either fabric-internal side.
// An AST walk over every identifier enforces that ban (internal/lymcwd's TestEnforcement_FabricVocabulary).
package landingshed
