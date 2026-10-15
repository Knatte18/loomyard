<!-- This is the Darn-Review rubric. It is read by the Bouncer row and the review row of the darn recipe's Darn-Review perch:
     the Darn-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Darn-Burler-Review row interpolates it
     the same way into internal/burlerengine's own review prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Darn-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the task's board entry, read with `lyx board get '{"slug":"<slug>"}'`, where `<slug>` is this worktree's own directory name, as amended by the `Added after Discussion` entries of `_lyx/discussion/decision-record.md`.
An entry by the parent, the operator or the arbiter wins over the board body where they differ.
The record's other sections are the writer's own claim, made before coding and reviewed by no one: check them against the board entry the way the change description is checked, never use them as the measuring stick.
The change description at `_lyx/landing/summary.md` is the writer's own account of the diff: read it as a claim to check against the diff, never as the measuring stick.

This perch is a safety net behind one writer who edited, tested and committed the change and passed a mechanical verify gate.
Over-flagging is a judgment failure mode: a finding the verify gate already decides is duplicated work rather than coverage.

## Determining the review range

This section is the single definition of the review range;
nothing else states it.

1. Read `_lyx/shed/<slug>/status.json` and take `product.parent`, the branch this run started from.
2. Review `git diff $(git merge-base <product.parent> HEAD)..HEAD` — every commit the current branch introduces over that merge base.

All steps are read-only.
If the status file cannot be read, or its `product.parent` is empty or absent, raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
Silently reviewing a guessed range is a worse failure than an honest block.

## Do not flag

Do not flag any of the following as a finding:

- **Anything the verify gate already decides.**
  A failing build or test is the verify gate's to report, and it ran on this tree before the review.
- **Findings raised against the board entry itself.**
  The board entry and the record's `Added after Discussion` entries are the measuring stick and never the subject.
  A finding about what the entry should have asked for cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **Anything that is not the diff.**
  This segment's own round artifacts under `_lyx/reviews/darn/` are never the subject of a finding.

## Also flag

- **Scope against the board entry.**
  A change the amended entry asks for that the diff does not make, and a change the diff makes that the amended entry does not license, are each a finding.
- **An unrecorded parent correction.**
  A deviation from the amended board entry that the change description names as an unrecorded parent correction is never fixed by revert.
  File it as class `decision` at severity MEDIUM.
  Its fix text tells the fixer to leave the code in place and defer the finding to the run's parent, since its only sound change, recording or refusing the correction with `lyx loom decision add`, is the parent's and lies outside the fixer's write surface.
- **A step that edits the task's own board entry.**
  A darn step that changes this task's own board entry's body or scope is a finding, because the run's board lock refuses such a change.
  A step writing another entry, the entry's status or its `run_settings`, or changing board-module code is not flagged by this bullet.
  Such a step goes through `lyx board` into the records repository, so the committed diff never shows it: the evidence is a claim in the change description that the run changed this task's own entry's body or scope, read as the writer's claim like the rest of that description.
  A decision-record step asking for such a change is evidence only of the request.
  A step left out through the darn writer's own-entry route is no scope finding once the record carries its `Added after Discussion` entry;
  before then the omission is an unrecorded parent correction, filed under that bullet as a MEDIUM `decision` finding deferred to the parent.
- **The repository's own rules.**
  The diff follows the target repository's constraints document if it has one, and the docs it names for the kind of change the diff makes land in the same diff.
  A rule written in that document outranks a convention inferred from the surrounding code.
- **Comment-convention compliance.**
  Any new or changed doc comment follows the target repository's own conventions.
  A comment's line breaks and wrapping are never a finding; the gate's `lyx loom lint-comments` alone checks them.
- **The change description.**
  A claim in `_lyx/landing/summary.md` that the diff does not bear out is a finding.
- **Stencil wording.**
  A diff that changes a stencil is read against the measuring stick, the board entry as the decision record's added entries amend it; a rule the entry did not ask to change that the diff drops, weakens or contradicts is a finding.
- **Test economy.**
  `PATTERN-test-economy` holds the rule; each of these is a finding, and each names what grounds it:
  - a new test that duplicates an existing test's coverage, naming the covering test;
  - a new assertion on a stencil's static prose, naming the assertion;
  - a behavior the diff adds with no coverage, naming the uncovered behavior; a stencil's static prose is exempt.

  A redundancy finding is raised only by naming the existing test that covers the behavior, so it cannot block on a vague suspicion.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the implementation does not make the change the board entry asks for, breaks the repository's invariants, or is incorrect.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
