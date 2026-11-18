<!-- This is burler's diff-pieces part: the list of a round's diff, written out in pieces, in the explore step.
     `diffPiecesBlock` (internal/burlerengine/prompt.go) fills it, with piece_list as one bullet per piece.
     `composeReviewPrompt` injects the result as burler-step-1-explore's optional diff_pieces marker value.
     It does so only in a round whose profile carries a diff. -->
## The diff under review, in pieces

The task's full diff against its merge base is already written out below, in pieces of one package each, every piece small enough to read whole.
A package too large for one piece is split at file, hunk or line boundaries into numbered parts.
A file split across pieces repeats its header at the top of each piece after its first, so every piece names the file it changes.
Read these pieces in place of the review range's `git diff`: never print the whole diff, and never write it to a file of your own.
Where the rubric narrows or refuses the review range, as for a merge from the parent or an unreadable base,
the rubric's range governs, and the pieces serve only as reading aid for the commits that range names.
Read every piece the review needs together in one message, each piece whole.
A command the rubric names beside the diff, such as `git log` over the range or `git show` of a merge commit, you still run.

{{.piece_list}}
