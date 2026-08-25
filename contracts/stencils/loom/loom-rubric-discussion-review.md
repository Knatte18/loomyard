<!-- This is the Discussion-Review rubric. It is read by both rows of the Discussion-Review perch:
     the Discussion-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's {{.rubric}} marker value, and the Discussion-Burler row interpolates
     it the same way into internal/burlerengine's own round prompt.
     It is a marker VALUE, never a template -- it carries no {{.X}} markers of its own, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Discussion-Review rubric

The subject under review is the discussion artifact pair, `decision-record.md` and `support-log.md`.
The mechanical section contract over that pair — which sections exist, in which file — is enforced upstream by `Discussion-Validate`, and is not this rubric's subject.

`Discussion-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.

## Do not flag

Do not flag any of the following as a finding:

- **A missing "Notes for the plan writer" subsection.**
  It is optional by contract; its absence is never a deficiency.
- **Missing rejected alternatives in `decision-record.md`.**
  Rejected alternatives belong in `support-log.md`'s Rejected alternatives section, not in `decision-record.md`;
  their absence from `decision-record.md` is by design, not an omission.
- **Incomplete call-site or cross-reference enumeration.**
  That enumeration belongs to the compiler and to `Plan-Sweep`'s mechanical inventory, not to `Discussion-Review`.

## Also flag

- **Relocation and exclusion findings are legitimate, on equal footing with gap-filling findings.**
  Accept "this belongs in `support-log.md`, not `decision-record.md`" and "this doesn't belong in Discussion at all" as legitimate findings, exactly as legitimate as a finding that fills a gap.
  A review loop that can only resolve a finding by adding content is the concrete mechanism behind discussion files that only grow across rounds.
- **The completeness-before-leanness test.**
  Before raising any relocation finding, check whether the content carries a requirement or constraint the plan writer needs.
  Extract that into `decision-record.md`'s own Decisions or Constraints first, and move only the surrounding deliberation narrative — because `Plan-Write` never reads `support-log.md`, making a careless move a silent loss rather than a relocation.
- **The writer/reviewer symmetry note.**
  Whatever the discussion writer's own stencil says not to gather, this rubric must not flag as missing, or the additive bias reappears even with the writer-side fix in place.
