<!-- This is the burler review-file format, the one format both halves of a round point at.
     It is shipped as an embedded default in the top-level stencils package (stencils/stencils.go), seeded to <hub>/_board/_lyx/stencils/burler/ and read
     from there at call time by composeReviewPrompt and composeFixPrompt (prompt.go), which render it unchanged into each half's instruction directory.
     The reviewer step names its path as the format it writes; the fixer seat names its path as the format it reads.
     It is marker-free on purpose: it states the format and carries no path of either half. -->

## Review-file format

Write the review file as `---`-delimited YAML frontmatter over unconstrained prose:

```
---
verdict: APPROVED
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: path/to/file.go:42
    summary: one-line description of the defect
    fix: the exact change to make, and where
---
```

Frontmatter rules, all strict:

- The frontmatter's top-level keys are `verdict` and `findings` only.
- `verdict` is exactly `APPROVED` or `BLOCKING` — no other spelling.
- `findings` is a list;
  every entry has a non-empty `id`, `severity`, `class`, `location`, `summary`, and `fix`.
- `fix` is the exact change to make and where, written so a fixer that has not seen your work can apply it from the review alone.
- `severity` is exactly one of `BLOCKING`, `MEDIUM`, `LOW`, `NIT`.
- `class` is exactly one of `design`, `scope`, `decision`, `consistency`, lowercase.
  - `design`: the design itself is wrong, or a decision is missing or rests on a false premise.
    The rubric says what `design` means for this target.
  - `scope`: the work missed a call site, file or case.
    A `scope` gap that recurs across rounds is raised once as a `design` finding about the method that keeps missing it, never as one finding per missed item.
  - `decision`: a choice is left open that the author must make.
  - `consistency`: two parts of the artifact disagree, or it departs from its own conventions.
  Class says who decides and when the loop stops, never whether a finding is fixed: every finding of every class is fixed.
- Every `id` is unique within the file.
- A `BLOCKING` verdict requires at least one `BLOCKING`-severity finding.
- Never write `APPROVED` while any finding is `BLOCKING` — a self-contradictory review file must never happen and must never look approved.
- `summary` must be valid YAML.
  If it needs to contain a `"` character (e.g. quoting a misspelled word or an error message), the ENTIRE value must be one double-quoted string covering the whole line — never a quoted fragment followed by unquoted trailing prose. `summary: "capital" is misspelled as "captial"` is INVALID YAML (the value ends at the first closing quote, and everything after it breaks the parse).
  Either quote the whole line (`summary: "\"capital\" is misspelled as \"captial\""`) or, simpler, just don't use literal quote characters in the summary (`summary: capital is misspelled as captial`).
- Omit `findings` entirely when you found nothing.
  Never invent a finding to pad the list.
- In a cluster round, each finding also carries an `origin:` key — `lens:<name>` for a finding kept from a named fork, or `handler` for one you found yourself.

Below the closing `---`, write prose: exactly one `### [SEVERITY] <title>` block per finding, each carrying `**Location:**` and `**Issue:**` lines.
The change lives in the finding's `fix` key; no prose `**Fix:**` line restates it.
A `## Rejected` section, if you write one, lists its items as bullets and never as `### [SEVERITY]` headings.

When you departed from a focus directive this round, add a `## Focus departures` section below the frontmatter, after the finding blocks.
Write one entry per departed directive, quoting the directive and giving your reason for departing.
Omit the section when you departed from none.
The section is prose and never a frontmatter key — the frontmatter schema above is unchanged.
