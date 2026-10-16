<!-- This is the reviewer's instruction 2 of 2: the cluster rules, the review-file format pointer, source-grounding, and prior-round hydration.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composeReviewPrompt
     (prompt.go) via internal/stencil, then read by the reviewer only when the reviewer stencil
     (burler-template-reviewer.md) directs it here, after instruction 1.
     The review-file format itself lives in the file review_format_path names (burler-review-format.md), never here.
     Every marker below is a top-level {{.X}} substitution;
     stencil.Fill requires all four non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go).
lyx-stencil: sha256=74537805d5cb8c7cfa5e7e9a1f7d3f3a412acd032242a6acb36eb1e7c3dd009e build=b2e4cf913d2834503e0338810b1f952e426af78f time=2026-10-11T18:29:27Z -->

## Cluster rules

{{.cluster_rules}}

### Fork rules for this round

These rules replace the fork boilerplate, the spawn trigger and the holistic paragraph above wherever they differ; the consolidation paragraph above stands.

Spawn the forks as soon as step 1's reading manifest is written, in the same turn.
Each fork's prompt is the boilerplate below, then the line `Your lens: <name>` and that lens's text, and nothing else: never add your own direction, hint, suspicion or reading list to a fork's prompt.

Boilerplate:

> You are one fork reviewer of this round, and you own one lens; the other forks own the other lenses, and the handler reviews holistically. Your inherited context already holds the target, the rules and the code listed in the reading manifest; review from it. Work your lens's procedure, and start your final message with the list your lens asks for. Report every finding your lens's procedure produces; report a finding outside your lens only if it is BLOCKING, because the other lenses and the handler cover the rest. You may make at most 6 tool calls, each only for something your procedure needs that the manifest lacks, such as a caller or a spawned process one hop beyond it, and never to re-read what your context already shows; end your final message with one line per tool call: what you read and why your context lacked it. You are READ-ONLY: never Write, Edit or delete any file, never run any git command, never touch the two round output files, and never call the Agent tool. Return your findings ONLY as your final message, each with a severity, a class, a location, a one-line summary and the file and line of the evidence you read.

While the forks run, YOU (the handler) do your own HOLISTIC review: what no lens owns, such as the record against its board entry and the rubric's leanness and relocation rules, plus architecture, cross-file invariants and PATTERN-fit, reading further only where your review needs it. Then prepare the ground truths and the severity rubric you will judge every finding against.

## The review file

Write the review to `{{.review_path}}`, in the format `{{.review_format_path}}` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

{{.prior_rounds}}
