<!-- This is the burler review-file format, the one format both halves of a round point at.
     It ships as an embedded default in contracts/stencils/stencils.go.
     It is seeded to the Bolt checkout's `_lym/stencils/burler/`.
     `composeReviewPrompt` and `composeFixPrompt` (internal/burlerengine/prompt.go) read it from there at call time.
     They render it unchanged into each half's instruction directory.
     The reviewer step names its path as the format it writes; the fixer seat names its path as the format it reads.
     It carries no marker, so it names no path of either half. -->

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

- The frontmatter's top-level keys are `verdict`, `findings` and `prior` only.
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
- Every value must be valid YAML.
  A value that holds `: ` (a colon followed by a space) must be ONE double-quoted string covering the whole value;
  so must a value that opens with `"`, `'`, a backtick, `[`, `{`, `*`, `&`, `!`, `|`, `>`, `%`, `@` or `#`.
  Inside it, escape each `"` and `\` with a backslash: `fix: "Print the \"creel: warning:\" line"`.
  An unquoted `summary: the refusal prints a creel: warning: line` is INVALID YAML, and the round cannot read the review.
- Never write a quoted fragment followed by unquoted trailing prose.
  `summary: "capital" is misspelled as "captial"` is INVALID YAML (the value ends at the first closing quote, and everything after it breaks the parse).
  Either quote the whole value (`summary: "\"capital\" is misspelled as \"captial\""`) or leave the quote characters out (`summary: capital is misspelled as captial`).
- Omit `findings` entirely when you found nothing.
  Never invent a finding to pad the list.
- In a cluster round, each finding also carries an `origin:` key — `lens:<name>` for a finding kept from a named fork, or `handler` for one you found yourself.
- A finding kept from the parent orch's report carries `origin: parent` and an optional `parent_ids` list naming the report ids it covers, such as `parent_ids: [P1, P3]`.
  No other finding carries `parent_ids`.
- A finding that raises the plan executor's halt reason, when the review step quotes one, carries `origin: webster`.
- A finding that records a focus entry the round left undriven or marked WARN carries `origin: focus` and the class `decision`.
  The rubric says when a round raises one, and a cluster round's consolidation keeps that origin.
- A finding's `location` opens with the file's path, followed by `:<line>` or `#<symbol>`.
  It names the site the fix changes, which may lie apart from where the defect shows:
  a wrong value that one file prints and another sets is located where it is set.
  When you did not find that site, `location` names where the defect shows,
  and the `fix` key opens with `site not found —` and names what the fixer must find.
  A carried finding keeps the location path the carry rules allow and names a fix site outside it in its `summary` and `fix`.
- `prior` is a list, written only when the review step's prior-rounds section lists prior findings.
  It holds one entry per listed finding, each with its `id`, a `status` and an optional `answer`.
  `status` is exactly one of `resolved`, `open`, `outdated`, `withdrawn`.
  `withdrawn` is for a finding the fixer disputed and you accept.
  An `open` entry for a disputed finding carries an `answer` to the rebuttal.
  A held `decision`-class finding takes `resolved` or `open` only, and `resolved` needs an `answer` quoting the title of a listed added entry.
- `carries` is an optional key of a finding: the `id` of the `open` prior finding it carries forward.
  Every `open` entry is carried by exactly one finding, and a finding with no `carries` is new.
  A carried finding keeps its `class`, keeps or lowers its `severity`, and keeps its location path or moves to a file the changes file lists.
- In an incremental round a new finding is `BLOCKING` and sits on a changed hunk, in a file the changes file lists.
  A card a listed added entry marked as amending the plan names is exempt from the file rule, and so is a new `origin: webster` finding.
- A hardener round's findings are exempt from the incremental round's rule above and from a carried finding's file rule:
  a new finding takes the severity it has and sits where the fault lives, and a carried finding may move to any file.

Below the closing `---`, write prose: exactly one `### [SEVERITY] <title>` block per finding, each carrying `**Location:**` and `**Issue:**` lines.
The change lives in the finding's `fix` key; no prose `**Fix:**` line restates it.
A `## Rejected` section, if you write one, lists its items as bullets and never as `### [SEVERITY]` headings.

When you departed from a focus directive this round, add a `## Focus departures` section below the frontmatter, after the finding blocks.
Write one entry per departed directive, quoting the directive and giving your reason for departing.
Omit the section when you departed from none.
The section is prose and never a frontmatter key — the frontmatter schema above is unchanged.
