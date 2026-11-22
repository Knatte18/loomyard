<!-- This is the PR-Review session's review-mode block for a run whose autonomy is off: the operator reviews in this pane.
     The PR-Review stencil takes it through its review_mode marker, and landingshed fills this block first.
     It declares no markers. -->

## The operator reviews with you

The operator discusses the pull request with you in this pane.
Answer the operator's questions about the change, and change it when the operator asks.
The run ends on the pull request's state, not on this session, so after each answer you wait for the operator's next message.

## Your parent's review

Your parent orch may review the pull request.
lym asks it on its own when the hub or the task turns `pr_review_ask_parent` on;
otherwise ask it only when the operator asks you to, with `lym creel post --parent --expect-reply`, naming the pull request.
Its answer arrives as creel mail: either that it is satisfied, or its findings.
Run `lym loom pr-findings <file>` on its findings first, as the section above says, then fix them as you would the operator's requests, commit, and tell the operator in this pane what you changed.
Then ask your parent to look again with `lym creel post --parent --expect-reply`, naming your commits, until it is satisfied.

## The operator's ending

When the operator says they are satisfied and asks you to continue:

1. Commit or discard your work so the worktree is clean.
2. Run `lym loom pr-continue`.
   It ends the review, and the ordinary landing then merges the pull request on GitHub, after its gate.
3. When lym then mails a gate failure from outside the change, such as a file another process held open,
   run `lym loom pr-continue` again: it gates the same head again.
   Fix a failure in the change itself in a new commit instead.
