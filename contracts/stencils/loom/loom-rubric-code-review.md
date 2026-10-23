<!-- This is the Code-Review rubric. It is read by the Bouncer row and the review row of the Code-Review perch:
     the Code-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Code-Burler-Review row interpolates it
     the same way into internal/burlerengine's own review prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Code-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the decision record, `_lyx/discussion/decision-record.md`, and the plan, `_lyx/plan/00-overview.md` with the card files its Card Index names.
The decision record settles what the task's change must do, and the plan is how the cards carry it out.
The Card model the plan implements is described in `{{.specs_dir}}/loom/loom-plan-spec.md`, and the format contract is `{{.specs_dir}}/loom/loom-plan-spec.md`.
This rubric points at both and restates neither.

`Code-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
This gate sits downstream of three separate upstream gates, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

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

- **Anything the plan's own gates already check.**
  The plan's *format* is enforced deterministically by `Plan-Write`'s and `Plan-Burler-Fix`'s own gates and is not this gate's subject.
- **Findings raised against the decision record or the plan themselves.**
  Both are the measuring stick and never the subject, exactly as the decision record is for `Plan-Review`.
  A plan-authoring or decision finding cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
  Both belong to `Plan-Review`, which has already passed.
- **Anything that is not the diff.**
  The discussion pair and the plan directory under `_lyx`, and this segment's own round artifacts under `_lyx/reviews/code/`, are never the subject of a finding.

## Also flag

- **Code in general.**
  Review the code for correctness beyond plan compliance: logic errors, unhandled failure paths, broken invariants and behavior that contradicts the decision record, each judged against the code as a careful reviewer reads it and against both the decision record and the plan.
- **Comment-convention compliance.**
  Any new or changed doc comment follows the target repository's own conventions — its own constraints document if it has one, and the conventions the surrounding code already follows.
  A rule written in the target repository's constraints document outranks a convention inferred from the surrounding code.
  A line width is never inferred from the surrounding code.
  A comment's line breaks and wrapping are never a finding; the gate's `lyx loom lint-comments` alone checks them.
  This rubric checks compliance with the target repository's own standard, not loomyard's.
- **Per-card mechanical check.**
  Confirm every one of the card's own groups' type-specific mechanical checks actually ran and passed, each against that group's own targets, not just the first label's, per the per-type table in `{{.specs_dir}}/loom/loom-plan-spec.md` — not merely that the diff compiles and its tests pass.
  For a `Rename` group, check the outcome the diff shows: no reference to the old name is left and the symbol's doc-comment opening is updated; never check that a tool ran.
  For a `Delete` group, check `assert-no-callers`.
- **Commit messages and departures.**
  Read the commit messages of the review range (`git log` over the same range) as well as the diff.
  A difference from the plan that a `Pattern-Departure:` trailer explains is judged on that reason, and holds when the named entry forbids the card's way, read in its declared new text where the directive's declared changes list it.
  A trailer naming an unknown entry, or an entry that does not forbid the card's way, is a finding.
  A trailer excuses only the difference it names.
- **Declared PATTERN changes landed.**
  Check that the branch's `PATTERN.md` and `pattern/` diff carries each entry the constraints section's declared changes list, its add, change or remove with the declared text, and raise a BLOCKING finding for each declared entry the diff does not land.
  Read the declarations rendered into your directive, never the plan's section, so a bullet appended after the plan gate is checked too.
  Raise nothing when every declared entry lands.
- **Stencil wording.**
  A diff that changes a stencil is read against the measuring stick, the plan card's `Intent`; a rule the card did not ask to change that the diff drops, weakens or contradicts is a finding.
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

When the amendment note names a review input, that file is the merge fixer's report of the latest Merge-In, written after this perch last settled.
Read it as an input besides the diff: judge from it whether the alignment the fixer made, and the parent changes it reports on, keep the code consistent with the decision record and the plan.
A finding about the report itself is never raised; each finding is about the code.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the implementation does not match the plan or the invariants, or is incorrect.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
