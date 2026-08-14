<!-- This is burler round instruction 2 of 3: the cluster rules, the
     review-file format, source-grounding, and prior-round hydration.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go),
     seeded to <hub>/_board/_lyx/stencils/burler/ and read from there at call time by composePrompt
     (prompt.go) via internal/stencil, then read by the agent only when the round orchestrator
     (burler-template-round-orchestrator.md) directs it here, after instruction
     1. Every marker below is a top-level {{.X}} substitution;
        stencil.Fill requires all three non-empty and there are no {{if}}/{{range}} conditionals anywhere in this file (a required marker inside a conditional branch would render silently blank when present-but-empty — see internal/stencil/stencil.go). -->

## Cluster rules

{{.cluster_rules}}

## Review-file format (write this to `{{.review_path}}`)

Write the review file as `---`-delimited YAML frontmatter over unconstrained prose:

```
---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    location: path/to/file.go:42
    summary: one-line description of the defect
---
```

Frontmatter rules, all strict:

- `verdict` is exactly `APPROVED` or `BLOCKING` — no other spelling.
- `findings` is a list;
  every entry has a non-empty `id`, `severity`, `location`, and `summary`.
- `severity` is exactly one of `BLOCKING`, `MEDIUM`, `LOW`, `NIT`.
- Every `id` is unique within the file.
- A `BLOCKING` verdict requires at least one `BLOCKING`-severity finding.
- Never write `APPROVED` while any finding is `BLOCKING` — a self-contradictory review file must never happen and must never look approved.
- `summary` must be valid YAML.
  If it needs to contain a `"` character (e.g. quoting a misspelled word or an error message), the ENTIRE value must be one double-quoted string covering the whole line — never a quoted fragment followed by unquoted trailing prose. `summary: "capital" is misspelled as "captial"` is INVALID YAML (the value ends at the first closing quote, and everything after it breaks the parse).
  Either quote the whole line (`summary: "\"capital\" is misspelled as \"captial\""`) or, simpler, just don't use literal quote characters in the summary (`summary: capital is misspelled as captial`).
- Omit `findings` entirely when you found nothing.
  Never invent a finding to pad the list.
- In a cluster round, each finding also carries an `origin:` key — `lens:<name>` for a finding kept from a named fork, or `handler` for one you found yourself.

Below the closing `---`, write prose: one `### [SEVERITY] <title>` block per finding, each carrying `**Location:**`, `**Issue:**`, and `**Fix:**` lines.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

{{.prior_rounds}}
