<!-- This is the Webster-Review rubric. It is read by the Bouncer row and the review row of the Webster-Review perch:
     the Webster-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Webster-Burler-Review row interpolates it
     the same way into internal/burlerengine's own review prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Webster-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the plan — `_lyx/plan/00-overview.md` and the card files its Card Index names.
The Card model the plan implements is described in `{{.specs_dir}}/loom/loom-plan-spec.md`, and the format contract is `{{.specs_dir}}/loom/loom-plan-spec.md`.
This rubric points at both and restates neither.

`Webster-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
This gate sits downstream of three separate upstream gates, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

## Determining the review range

This section is the single definition of the review range;
nothing else states it.

1. Read `_lyx/shed/<slug>/status.json` and take `product.parent`, the branch this run started from.
   `<slug>` is this worktree's own directory name;
   a run started before the rename keeps its status at `_lyx/shed/self/status.json`, so read that path when `_lyx/shed/<slug>/` does not exist.
2. Find the live generation's round.
   Among `_lyx/loom/rework/round-<N>/` directories, it is the highest `N` whose `record.json` carries a `class` and whose `first_card` equals the `first_card` in `_lyx/plan/00-overview.md`'s frontmatter (absent means `1`).
3. With such a round, take its `record.json` `head_sha` (the rejected PR head) and review only the branch's own commits after it, excluding the parent branch's changes a mid-run merge brought in:

   ```
   git log --first-parent --no-merges -p <head_sha>..HEAD
   ```

   Earlier generations' code is context, never the subject.
4. With no such round, review `git diff $(git merge-base <product.parent> HEAD)..HEAD` — every commit the current branch introduces over that merge base.
5. When the `## Publish failure` section below renders a merge-in commit, the range also includes that merge's diff, `git diff <merge>^1 <merge>`, which is the parent branch's changes the merge brought in and steps 3 and 4 exclude.
   Nothing else widens the range.

All steps are read-only.
If neither status file can be read, or its `product.parent` is empty or absent, raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
The same finding is raised when the live round's `record.json` is unreadable or its `head_sha` is empty.
Silently reviewing a guessed range is a worse failure than an honest block.

## Publish failure

{{.publish_failure}}

`none` means no Publish failure is on record, and this section raises nothing.
Otherwise a Publish verify failed after the branch's last merge-in, and the failure is what this round exists to fix.
Raise each failing test named above as a BLOCKING finding, reading the merge's diff and the log for its cause, in addition to reviewing the branch's own range.

## Do not flag

Do not flag any of the following as a finding:

- **Anything the plan's own gates already check.**
  The plan's *format* is enforced deterministically by `Plan-Write`'s and `Plan-Burler-Fix`'s own gates and is not this gate's subject.
- **Findings raised against the plan itself.**
  The plan is the measuring stick and never the subject, exactly as the decision record is for `Plan-Review`.
  A plan-authoring finding cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
  Both belong to `Plan-Review`, which has already passed.
- **Anything that is not the diff.**
  The discussion pair and the plan directory under `_lyx`, and this segment's own round artifacts under `_lyx/reviews/webster/`, are never the subject of a finding.

## Also flag

- **Comment-convention compliance.**
  Any new or changed doc comment follows the target repository's own conventions — its own constraints document if it has one, and the conventions the surrounding code already follows.
  A rule written in the target repository's constraints document outranks a convention inferred from the surrounding code.
  A line width is never inferred from the surrounding code.
  A comment's line breaks and wrapping are never a finding; the gate's `lyx loom lint-comments` alone checks them.
  This rubric checks compliance with the target repository's own standard, not loomyard's.
- **Per-card mechanical check.**
  Confirm every one of the card's own groups' type-specific mechanical checks passed, each against that group's own targets, not just the first label's, per the per-type table in `{{.specs_dir}}/loom/loom-plan-spec.md` — not merely that the diff compiles and its tests pass.
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
  `PATTERN-test-economy` holds the rule; each of these is a finding, and each names what grounds it:
  - a new test that duplicates an existing test's coverage, naming the covering test;
  - a per-helper test the public surface's test already covers, naming that public-surface test;
  - a review-fix test added for a finding that was not a coverage gap, naming that finding;
  - a new assertion on a stencil's static prose, naming the assertion;
  - a behavior the diff adds with no coverage, naming the uncovered behavior; a stencil's static prose is exempt.

  A redundancy finding is raised only by naming the existing test that covers the behavior, so it cannot block on a vague suspicion.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the implementation does not match the plan or the invariants, or is incorrect.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
