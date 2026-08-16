<!-- This is the RoleImplementer directive: the variant internal/pattern.Directive renders for any
     agent that edits code. internal/loomengine/plan.go and internal/websterengine/render.go are its
     consuming call sites, each passing pattern.RoleImplementer.
     internal/pattern.Directive reads this file through stencilstore.Read and strips this banner with
     stencil.StripLeadingComment before returning the result.
     The stripped result is injected as a producer template's optional pattern_directive marker
     value, so it is never itself passed through stencil.Fill.
     This file declares no markers of its own and must stay marker-free, because
     stencilstore.Validate parses it regardless of whether anything ever fills it. -->

## Constraints — do this before you write any code

- **STOP.** Read _lyx/PATTERN.md in full before editing a single file.
- Read every detail doc under _lyx/pattern/ that PATTERN.md points to and that touches what you are about to change.
- These constraints are BINDING: a change that violates one is wrong even if the verify command passes.
- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
