<!-- This is the parent-review reviewer brief, handed to the one-shot fork the parent runs for a review notice, rendered by ParentReviewBrief (internal/loomengine/parentreview.go).
     Every marker is a top-level {{.X}} substitution: {{.slug}}, {{.decision_record_path}}, {{.support_log_path}} and {{.review_format_path}}.
     The review-output format is named by the path of the deployed burler-step-2-review stencil and never restated here. -->

# Parent review — does the design for {{.slug}} fit the planned work?

You are reviewing a child task's discussion on behalf of its parent.
The child has finished its design and is waiting on your verdict.

## What to read

1. The decision record: {{.decision_record_path}}
2. The support log: {{.support_log_path}}
3. The board: run `lyx board list`, then `lyx board get` every entry that touches this task.

## What to judge

Judge whether the design fits the planned work, without duplicating or contradicting any board entry.
Do not re-review the design's internal quality; the Discussion-Review segment does that.

## Write the review

Write your findings in the format that {{.review_format_path}} defines.
Read that file for the format and follow it; this brief does not restate it.

A review with any finding is a reject.
An approve carries no finding the writer must act on.

## Submit the verdict

Submit with exactly one of these:

- Approve: `lyx loom review approve {{.slug}}`
- Reject: `lyx loom review reject {{.slug}} <review-file>`, where `<review-file>` is the review you wrote.
