<!-- This is the Merge-Review rubric. It is read by the Bouncer row and the review row of the Merge-Review perch:
     the Merge-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Merge-Burler-Review row interpolates it
     the same way into internal/burlerengine's own review prompt.
     It is a marker VALUE, never a template -- it carries only the optional markers internal/shedadapters' ReadRubric fills at read time,
     and internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Merge-Review rubric

The subject under review is how the task's code interacts with what the parent branch gained in the latest merge-in, not the task's own change and not the parent's own change.
Ordinary diff review is the base: read the code the two sides touched as code, and judge how it works together the way a careful reviewer judges a merge.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The merge-in is already committed and the plan's verify passed on it.
Over-flagging is a judgment failure mode: a defect that lives in one side alone is duplicated work rather than coverage.

## The merge

{{.merge_review}}

The note above names the parent range, the task range, the merge commit and where the two sides' changes overlap.
`none`, or a note saying no merge record is available, means the range is not known: review the whole merge-in diff of the latest merge of the parent into the task.
The commands below are read-only.

- The parent's side is `git log -p <parent range>`, and the task's side is `git log -p <task range>`.
- The merge itself is `git diff <merge commit>^1 <merge commit>`; a merge that was up to date has no merge commit and nothing to review.

## The measuring stick

The fasit is `PATTERN.md`, the `pattern/` files the note lists as changed, and the decision record at `_lyx/discussion/decision-record.md`.
Invariants the parent's commits added or changed are judged against the task's code, and the task's code is judged against the parent's new code.
The format contract of any spec is under `{{.specs_dir}}`; this rubric points at it and restates nothing of it.

## Do not flag

Do not flag any of the following as a finding:

- **Anything the Webster review already judged.**
  The task's own range was reviewed by the Webster-Review perch before this merge, so a defect that lives in the task's own commits alone is not this perch's subject.
- **Anything the parent's own review judged.**
  A defect that lives in the parent's commits alone, whatever the task's code does, belongs to the run that landed them.
- **Anything the plan's verify already decides.**
  A failing build or test is the verify gate's to report, and it ran on the merged tree before this review.
- **Anything that is not the interaction.**
  The decision record, the plan directory under `_lyx` and this segment's own round artifacts are never the subject of a finding.

## Also flag

- **A broken invariant.**
  An invariant the parent added or changed that the task's code no longer upholds, or a task invariant the parent's new code breaks, naming both sides.
- **A silent semantic conflict.**
  Code both sides changed that merged without a conflict and no longer does what either side meant, such as a renamed symbol the other side still calls, a changed contract the other side relies on, or two edits that each assume the other did not happen.
- **A pattern the parent changed.**
  A change to `PATTERN.md` or a `pattern/` file that the task's code does not follow.
- **Test economy.**
  `PATTERN-test-economy` holds the rule; each of these is a finding, and each names what grounds it:
  - a test the merge made redundant with another, naming the covering test;
  - a behavior of the interaction with no coverage, naming the uncovered behavior.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the merged code is incorrect or breaks an invariant of either side.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
