<!-- This is the Code-Review rubric. It is read by the Bouncer row and the review row of the Code-Review perch, in the loom and darn recipes alike:
     the Code-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Code-Burler-Review row interpolates it
     the same way into internal/burlerengine's own review prompt.
     Each recipe sets the perch's answer key through the rows' artifact_paths and fasit, never through this rubric.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Code-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the answer key this prompt names: the files it lists as the artifacts or the answer key, read as the task's settled scope.
The answer key takes one of two forms, and the sections below that apply to only one of them say so:

- **A plan.**
  The decision record, `_lym/discussion/decision-record.md`, settles what the task's change must do, and the plan, `_lym/plan/00-overview.md` with the card files its Card Index names, is how the cards carry it out.
  The Card model the plan implements, and its format contract, are in `{{.specs_dir}}/loom/loom-plan-spec.md`; this rubric points at it and restates neither.
- **A board entry snapshot.**
  `_lym/discussion/board-entry.md` is the task's board entry, written by lym, and the decision record's `Added after Discussion` entries amend it, an entry by the parent, the operator or the arbiter winning over the snapshot where they differ.
  The record's other sections are the writer's own claim, made before coding and reviewed by no one: check them against the snapshot, never use them as the measuring stick.

The change description at `_lym/landing/summary.md`, when present, is the writer's own account of the diff: read it as a claim to check against the diff, never as the measuring stick.

`Code-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
This perch sits downstream of mechanical gates, among them the verify gate that ran on this tree, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

## Determining the review range

This section is the single definition of the review range;
nothing else states it.

Review `git diff {{.review_base}}..HEAD` — every commit the current branch introduces over the merge base the run's last merged parent tip leaves.
Nothing else widens the range.

The step is read-only.
If the range above reads `git diff none..HEAD`, its base could not be read: raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
Silently reviewing a guessed range is a worse failure than an honest block.

## Do not flag

Do not flag any of the following as a finding:

- **Anything a mechanical gate already decides.**
  A failing build or test is the verify gate's to report.
  With a plan, the plan's *format* is enforced deterministically by `Plan-Write`'s and `Plan-Burler-Fix`'s own gates and is not this perch's subject.
- **Findings raised against the answer key itself.**
  The answer key is the measuring stick and never the subject.
  A finding about what the plan, the decision record or the board entry should have said cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
  Both belong to `Plan-Review`, which has already passed.
- **Anything that is not the diff.**
  The discussion files and the plan directory under `_lym`, and this segment's own round artifacts under `_lym/reviews/code/`, are never the subject of a finding.

## Also flag

- **Code in general.**
  Review the code for correctness beyond compliance with the answer key: logic errors, unhandled failure paths, broken invariants and behavior that contradicts the answer key, each judged against the code as a careful reviewer reads it.
- **Scope against the answer key.**
  A change the answer key asks for that the diff does not make, and a change the diff makes that the answer key does not license, are each a finding.
- **The repository's own rules.**
  The diff follows the target repository's constraints document if it has one, and the docs it names for the kind of change the diff makes land in the same diff.
  A rule written in that document outranks a convention inferred from the surrounding code.
- **Comment-convention compliance.**
  Any new or changed doc comment follows the target repository's own conventions — its own constraints document if it has one, and the conventions the surrounding code already follows.
  A line width is never inferred from the surrounding code.
  A comment's line breaks and wrapping are never a finding; the gate's `lym loom lint-comments` alone checks them.
  This rubric checks compliance with the target repository's own standard, not loomyard's.
- **Per-card mechanical check, with a plan.**
  Confirm every one of the card's own groups' type-specific mechanical checks actually ran and passed, each against that group's own targets, not just the first label's, per the per-type table in `{{.specs_dir}}/loom/loom-plan-spec.md` — not merely that the diff compiles and its tests pass.
  For a `Rename` group, check the outcome the diff shows: no reference to the old name is left and the symbol's doc-comment opening is updated; never check that a tool ran.
  For a `Delete` group, check `assert-no-callers`.
- **Commit messages and departures.**
  Read the commit messages of the review range (`git log` over the same range) as well as the diff.
  A difference from the answer key that a `Pattern-Departure:` trailer explains is judged on that reason, and holds when the named entry forbids the answer key's way, read in its declared new text where the directive's declared changes list it.
  A trailer naming an unknown entry, or an entry that does not forbid that way, is a finding.
  A trailer excuses only the difference it names.
- **Declared PATTERN changes landed.**
  Check that the branch's `PATTERN.md` and `pattern/` diff carries each entry the constraints section's declared changes list, its add, change or remove with the declared text, and raise a BLOCKING finding for each declared entry the diff does not land.
  Read the declarations rendered into your directive, never the plan's or the record's section, so a bullet appended after the gate is checked too.
  Raise nothing when every declared entry lands, or when your directive declares none.
- **The change description.**
  A claim in `_lym/landing/summary.md` that the diff does not bear out is a finding.
- **An unrecorded parent correction, with a board entry snapshot.**
  A deviation from the amended board entry that the change description names as an unrecorded parent correction is never fixed by revert.
  File it as class `decision` at severity MEDIUM.
  Its fix text tells the fixer to leave the code in place and defer the finding to the run's parent, since its only sound change, recording or refusing the correction with `lym loom decision add`, is the parent's and lies outside the fixer's write surface.
- **A step that edits the task's own board entry, with a board entry snapshot.**
  A step that changes this task's own board entry's body or scope is a finding, because the run's board lock refuses such a change.
  A step writing another entry, the entry's status or its `run_settings`, or changing board-module code is not flagged by this bullet.
  Such a step goes through `lym board` into the records repository, so the committed diff never shows it: the evidence is a claim in the change description that the run changed this task's own entry's body or scope, read as the writer's claim like the rest of that description.
  A decision-record step asking for such a change is evidence only of the request.
  A step left out through the writer's own-entry route is no scope finding once the record carries its `Added after Discussion` entry;
  before then the omission is an unrecorded parent correction, filed under that bullet as a MEDIUM `decision` finding deferred to the parent.
- **Stencil wording.**
  A diff that changes a stencil is read against the measuring stick, the plan card's `Intent` with a plan and the amended board entry with a snapshot; a rule the answer key did not ask to change that the diff drops, weakens or contradicts is a finding.
- **Test economy.**
  Each of these is a finding, and each names what grounds it:
  - a new test that duplicates an existing test's coverage, naming the covering test;
  - a per-helper test the public surface's test already covers, naming that public-surface test;
  - a review-fix test added for a finding that was not a coverage gap, naming that finding;
  - a new assertion on a stencil's static prose, naming the assertion;
  - a behavior the diff adds with no coverage, naming the uncovered behavior; a stencil's static prose is exempt.

  A redundancy finding is raised only by naming the existing test that covers the behavior, so it cannot block on a vague suspicion.

## Amendment

{{.amendment}}

When the note above is not `none`, this round reviews an amendment: review only the change it names, and report nothing about the settled remainder.

## Review input

When the amendment note names a review input, that file is a report written after this perch last settled: the merge fixer's report of the latest Parent-Align, or the overlap note of a Finalize reroute.
Read it as an input besides the diff: judge from it whether the alignment it reports, and the parent changes it names, keep the code consistent with the answer key.
A finding about the report itself is never raised; each finding is about the code.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the implementation does not make the change the answer key asks for, breaks the repository's invariants, or is incorrect.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
