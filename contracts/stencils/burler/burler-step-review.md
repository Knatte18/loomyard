<!-- This is the reviewer's instruction 2 of 2: the cluster rules, the review-file format pointer, source-grounding, and prior-round hydration.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composeReviewPrompt
     (prompt.go) via internal/stencil, then read by the reviewer only when the reviewer stencil
     (burler-template-reviewer.md) directs it here, after instruction 1.
     The review-file format itself lives in the file review_format_path names (burler-review-format.md), never here.
     Every marker below is a top-level {{.X}} substitution;
     stencil.Fill requires all four non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). -->

## Cluster rules

{{.cluster_rules}}

## The review file

Write the review to `{{.review_path}}`, in the format `{{.review_format_path}}` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

{{.prior_rounds}}
