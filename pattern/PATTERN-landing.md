# PATTERN-landing

Landing on a local parent branch or deciding on a pull request: only in a run's Finalize under `internal/parentlock`'s lock, alone or in a queue batch; landingshed's one resolver decides it; a pull request's review ends only on the operator's word, whatever the run's autonomy.

## Parent lock

Rule: Moving, merging into or landing on a local parent branch, or merging a run's pull request: only in a run's Finalize, under `internal/parentlock`'s lock for that parent.

- The local merge stays in Finalize under that lock.
  Only its push leaves the run: Finalize records the landing, and the parent target's pusher pushes it.
- A GitHub landing waits while its parent owes a landing push, so GitHub never merges onto a stale base.

## Landing queue

A run at Finalize writes an entry in its parent branch's landing queue and waits for the parent lock.

- **Leader.** The run that takes the lock with no outcome, and whose work the parent does not hold, leads.
- **Batch.** The leader takes its own entry, then live entries with no outcome in join order, up to `landing.batch_max`.
  It skips a paused or moved member.
  A run whose current Finalize entry holds a fixer change neither leads a batch nor lands in one.
  An entry with no outcome gets `failed` before its run's own fixer or fix review runs.
  The batch is a chain of landing commits that fabric writes with no worktree.
- **Gate.** A batch that passes is gated once, in the leader's task worktree detached at the chain commit.
  The worktree is restored on every way out.
- **Bisection.** A failure is bisected over prefixes of the chain under the same lock.
  The culprit gets the outcome `failed` and is fixed by its own fixer.
- **Landing.** A prefix lands by one guarded fast-forward of the parent,
  after each member's own checks, which run as reads.
- **Ledger.** The leader writes one ledger landing per member, each with the member's own landing commit,
  right after the move and under the lock.
  When a leader dies, the next lock taker's backstop writes the landings its move still owes.
- **Alone.** A pull-request landing, a batch of one, a run with a fixer change,
  and a run with outcome `failed` or `conflict` land alone.

## Pull request mode

Rule: Deciding if a landing uses a pull request: only landingshed's one resolver, from the landing's `beam.pull_request` as the per-task lookup reads it from the landing run's worktree up, a beam's pull-request record, the parent's GitHub rules, origin and task commits; a yes merges on GitHub.
A task whose `beam.pull_request` is `merge-in` is a no under every rule.
Its parent-side merge never squashes.

Rule: Ending a pull request's review: only on the operator's word, whatever the run's autonomy: an approval or a merge on GitHub, or `lym loom pr-continue`, which the operator runs or the run's PR-Review session or an orch runs when the operator says so; the verb refuses every other lym agent session.
Under `beam.pull_request: review` PR-Review waits for that ending with one interactive PR-Review session in the task's window, which never approves, merges or pushes; `trail` merges the pull request at once with no review.
