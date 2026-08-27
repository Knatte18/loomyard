<!-- This is the Webster-Review rubric. It is read by both rows of the Webster-Review perch:
     the Webster-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Webster-Burler row interpolates it
     the same way into internal/burlerengine's own round prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Webster-Review rubric

The subject under review is the **committed diff**, not a file and not a directory.
Ordinary diff review is the base: read the diff as code, with no checklist supplied, and judge it the way a careful reviewer judges any change.
The two dimensions under `## Also flag` are added on top of that base, never a replacement for it.

The measuring stick is the plan — `_lyx/plan/00-overview.md` and the card files its Card Index names.
The Card model the plan implements is described in `manifest/designs/plan-card-format.md`, and the format contract is `contracts/specs/loom-plan-spec.md`.
This rubric points at both and restates neither.

`Webster-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
This gate sits downstream of three separate upstream gates, so a finding re-derived from one of their subjects is duplicated work rather than coverage.

## Determining the review range

This section is the single definition of the review range;
nothing else states it.

1. Read `_lyx/loom/status.json` and take `product.parent`, the branch this run started from.
2. Review `git diff $(git merge-base <product.parent> HEAD)..HEAD` — every commit the current branch introduces over that merge base.

Both steps are read-only.
If `_lyx/loom/status.json` cannot be read, or its `product.parent` is empty or absent, raise a BLOCKING finding stating that the review range could not be determined, and review nothing.
Silently reviewing a guessed range is a worse failure than an honest block.

## Do not flag

Do not flag any of the following as a finding:

- **Anything `Plan-Validate` or `Plan-Revalidate` already checks.**
  The plan's *format* is enforced deterministically upstream and is not this gate's subject.
- **Findings raised against the plan itself.**
  The plan is the measuring stick and never the subject, exactly as the decision record is for `Plan-Review`.
  A plan-authoring finding cannot be satisfied by changing the diff, which is the only thing this segment can fix.
- **A missing `ImpactSummary` on any card, or an incomplete `DependsOn`/`Produces` list.**
  Both belong to `Plan-Review`, which has already passed.
- **Anything that is not the diff.**
  The discussion pair and the plan directory under `_lyx`, and this segment's own round artifacts under `.lyx/loom/reviews/webster/`, are never the subject of a finding.

## Also flag

- **Comment-convention compliance.**
  Any new or changed doc comment follows `manifest/designs/code-comment-conventions.md`.
  This rubric points at that file and restates none of it.
- **Per-card mechanical check.**
  Confirm every one of the card's own groups' type-specific mechanical checks actually ran and passed, each against that group's own targets, not just the first label's — the AST-script-plus-grep for a `Rename` group, `assert-no-callers` for a `Delete` group, per the per-type table in `manifest/designs/plan-card-format.md` — not merely that the diff compiles and its tests pass.
