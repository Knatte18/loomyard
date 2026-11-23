<!-- This is the PR-Review session's review-mode block for a run whose autonomy is on: the parent orch reviews, no operator is in the pane.
     The PR-Review stencil takes it through its review_mode marker, and landingshed fills this block with the slug first.
     Its one top-level marker is slug. -->

## Your parent reviews

Your parent orch reviews the pull request in the operator's place, and no operator is in this pane.
Settle what you can from the change and the run's material, since no one answers a question here.

After each set of fixes:

1. Commit your work so the worktree is clean.
2. Run `lym loom parent-review ask {{.slug}}`.
   It records your parent's ask at your current head and sends it.
3. Run `lym loom parent-review await {{.slug}}` and wait for your parent's answer.

Your parent answers that it is satisfied, or with findings.
On findings, run `lym loom pr-findings <file>` first, as the section above says, then fix them, commit, and ask again as above.
Run `lym loom pr-continue` only once your parent's answer is satisfied at your current head.
It ends the review, and the ordinary landing then merges the pull request on GitHub, after its gate.

When `await` reports an ask ended timeout, no-parent or not-sent, or reports that no ask stands at your head,
ask again with `lym loom parent-review ask {{.slug}}`.
After a no-parent end, end your turn first and ask again only on the next mail you receive,
so a parent that is down raises hold notices and not a loop of asks.
An undelivered end means creel refused or failed to deliver the ask, not that your parent is down: its `reason` names why.
Send your parent that reason and the ask's number with `lym creel post --parent`,
then end your turn.
On the next mail after a no-parent or undelivered end, run `await` first:
your parent may have answered that ask, and its answer then stands.
Ask again only when `await` still reports the ask ended no-parent or undelivered.

When lym mails a gate failure from outside the change, such as a file another process held open,
run `lym loom pr-continue` again once your parent's answer stands: it gates the same head again.
Fix a failure in the change itself in a new commit instead.
