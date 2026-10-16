<!-- This is the RoleImplementer directive: the variant internal/pattern.Directive renders for any
     agent that edits code. internal/loomengine/plan.go and internal/websterengine/render.go are its
     consuming call sites, each passing pattern.RoleImplementer.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's required pattern_overview marker with
     the content of PATTERN.md at the repository's worktree root, and its optional pattern_changes marker with the task's declared PATTERN changes.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares two markers, the required pattern_overview and the optional pattern_changes, and no other. -->

## Constraints — do this before you write any code

- **STOP.** These are the repo's rules; read them in full before editing a single file:

{{.pattern_overview}}
{{.pattern_changes}}

- Read every background file under pattern/ that the entries above link to and that touches what you are about to change.
- These constraints are BINDING: a change that violates one is wrong even if the verify command passes.
- If a constraint conflicts with anything else in this prompt, the constraint wins — state the conflict and the entry where your output records its reasons instead of silently picking one: a `Pattern-Departure:` trailer for a commit you make (see `loom-plan-spec.md`'s `## Pattern-Departure trailer`) and `## Shared Decisions` for a plan.
