<!-- This is the Plan-Review rubric. It is read by both rows of the Plan-Review perch:
     the Plan-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Plan-Burler row interpolates it the
     same way into internal/burlerengine's own round prompt.
     It is a marker VALUE, never a template -- it carries no top-level stencil markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Plan-Review rubric

The subject under review is the current plan: `_lyx/plan/00-overview.md` and the card files its Card Index names.
The plan directory may also hold `archive-*/` subdirectories, which are rotations of superseded plans;
they are out of scope, and a finding raised against one is never legitimate.

The format contract is `contracts/specs/loom-plan-spec.md`, and the Card model it implements is described in `manifest/designs/plan-card-format.md`.
This rubric points at both and restates neither.
The mechanical checks over that contract are already enforced — fifteen of them upstream by `Plan-Validate`, while `plan-unapproved` is enforced downstream by `Plan-Revalidate` instead.

`Plan-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
Sitting directly downstream of a sixteen-check mechanical validator makes this gate's over-flagging surface larger than that of a gate with no validator ahead of it, not smaller.

**`support-log.md` is outside this review entirely.**
It appears in neither the artifact list nor the answer key, and it must not be read or reasoned from.
`Plan-Write` provably never reads it, so a finding grounded in its content cannot be satisfied except by inventing the missing link.

## Do not flag

Do not flag any of the following as a finding:

- **Anything `Plan-Validate` or `Plan-Revalidate` already checks.**
  The sixteen check IDs `contracts/specs/loom-plan-spec.md`'s own validation-checks section lists, `format-unrecognized` through `commit-subject-mismatch`, are enforced deterministically — fifteen of the sixteen upstream by `Plan-Validate`, while `plan-unapproved` is enforced downstream by `Plan-Revalidate` instead.
  Re-deriving any of them here is duplicated work whose only possible outcome is disagreement with the parser.
- **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.**
  Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
  Plan-time completeness of that intersection is explicitly not provable;
  the real gate is the post-merge build and test.
- **A `Rename`, `Move`, `Prosa`, or `Custom` card carrying no `ImpactSummary`.**
  It is required for `Edit` and `Delete` only, per the per-type table in `manifest/designs/plan-card-format.md`.
  For `Rename` the reason is specific: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

## Also flag

- **Granularity.**
  One card per independently reviewable/testable unit, not one card per literal symbol.
  A private supporting type, or a constructor inseparable from its type, belongs in the other symbol's card;
  an independently testable symbol gets its own card even when one card is its only consumer.
- **`ImpactSummary` carries a real conclusion.**
  A one-line blast-radius conclusion — "3 callers, all local to the billing package, no cross-module effects" — never a restatement of `Intent`.
- **`Custom` is a last resort.**
  Used only where none of `Create`, `Edit`, `Delete`, `Rename`, `Move`, or `Prosa` genuinely fits, never as a shortcut around correct typing.
  A `Custom` card is exempt from `path-missing` on its own targets and from `prosa-symbol-target`, so a mistyped one silently escapes two checks the rest of the plan is held to.
- **Fidelity to the decision record.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope that file does not license.
  That path is anchor-relative: it resolves from this session's own working directory, and it is deliberately not the absolute form the artifact list uses.
  The decision record is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record.
