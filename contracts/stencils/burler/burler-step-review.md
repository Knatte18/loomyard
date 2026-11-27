<!-- This is the reviewer's instruction 2 of 2:
     the cluster rules, the review-file format pointer, source-grounding, and prior-round hydration.
     It is an embedded default in contracts/stencils/stencils.go,
     seeded to the Bolt checkout's `_lym/stencils/burler/`,
     and composeReviewPrompt in internal/burlerengine/prompt.go reads it from there at call time.
     The reviewer reads it only when burler-template-reviewer.md directs it here, after instruction 1.
     The review-file format lives in the file review_format_path names (burler-review-format.md), never here.
     Every marker below is a top-level {{.X}} substitution.
     The fill requires cluster_rules, review_path, review_format_path and prior_rounds non-empty.
     assist_rules is optional:
     an assisted round fills it with the burler-assist-consolidation part, led by its blank line, and any other round renders it as nothing.
     parent_rules is optional:
     a round holding a parent-review ask fills it with the burler-parent-consolidation part, led by its blank line,
     and any other round renders it as nothing.
     There are no {{if}}/{{range}} conditionals in this file:
     a required marker inside a conditional branch would render silently blank when present-but-empty
     (see internal/stencil/stencil.go). -->

## Cluster rules

{{.cluster_rules}}{{.assist_rules}}{{.parent_rules}}

## The review file

Write the review to `{{.review_path}}`, in the format `{{.review_format_path}}` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

{{.prior_rounds}}
