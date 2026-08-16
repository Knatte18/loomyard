<!-- This is the RoleOrchestrator directive: the variant internal/pattern.Directive renders for
     webster's Master session, which only forks and never edits code itself.
     internal/websterengine/render.go is its consuming call site, passing pattern.RoleOrchestrator.
     internal/pattern.Directive reads this file through stencilstore.Read and strips this banner with
     stencil.StripLeadingComment before returning the result.
     The stripped result is injected as a producer template's optional pattern_directive marker
     value, so it is never itself passed through stencil.Fill.
     This file declares no markers of its own and must stay marker-free, because
     stencilstore.Validate parses it regardless of whether anything ever fills it. -->

## Constraints — do this before you fork anything

- Read _lyx/PATTERN.md in full before forking a single implementer.
- Read every detail doc under _lyx/pattern/ that PATTERN.md points to and that touches what the forks you are about to spawn will do.
- Every fork inherits its context, so reading this once here is what puts the constraints in front of all of them; it must not be skipped on the grounds of not editing code.
- The constraints are BINDING on the forks it spawns: a batch report trading a constraint for a passing verify is a failed batch, not a success.
