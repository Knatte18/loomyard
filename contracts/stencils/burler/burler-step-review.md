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

### Rules for the prior findings

These rules apply only when the section above holds a heading line reading `Prior findings`;
a section without that heading has none of them.

- Give every listed finding one `prior` entry in the review's frontmatter, with the status `resolved`, `open`, `outdated` or `withdrawn`.
  Withdraw only a finding whose fixer disposition is `disputed` and whose rebuttal you accept.
  Keep a disputed finding `open` only when you answer the rebuttal in the entry's `answer`.
- Resolve a finding marked as held by the parent only by quoting, in the entry's `answer`, the title of a listed added entry that settles it.
  Otherwise mark it `open` and carry it.
- Carry every `open` finding as one finding with `carries` set to its id.
  Keep its class.
  Keep or lower its severity.
  Keep its location path, or move it to a file the changes file lists.
  When there is no changes file, name the moved site in the `summary` and the `fix`.
- In an incremental round, raise a new finding only at `BLOCKING`, and only on a changed hunk.
  Locate a regression a fix caused in an untouched file at the changed line that causes it, and name the broken site in its `summary`.
  A card that an added entry marked as amending the plan names may be located whether or not the changes file lists it.
  A finding raising the plan executor's halt reason, when the section above quotes one, carries `origin: webster` and may sit outside the changed files.
  In a hardener round, a new `origin: focus` finding of class `decision` may sit outside the changed hunks and files.
