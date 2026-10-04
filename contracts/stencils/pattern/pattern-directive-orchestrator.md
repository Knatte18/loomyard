<!-- This is the RoleOrchestrator directive: the variant internal/pattern.Directive renders for
     webster's Master session, which only forks and never edits code itself.
     internal/websterengine/render.go is its consuming call site, passing pattern.RoleOrchestrator.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's one marker, the pattern_overview marker, with
     the content of PATTERN.md at the repository's worktree root.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares that one marker and no other, and the marker is required. -->

## Constraints — do this before you fork anything

- These are the repo's rules; read them in full before forking a single implementer:

{{.pattern_overview}}

- Read every background file under pattern/ that the entries above link to and that touches what the forks you are about to spawn will do.
- Every fork inherits its context, so reading this once here is what puts the constraints in front of all of them; it must not be skipped on the grounds of not editing code.
- The constraints are BINDING on the forks it spawns: a batch report trading a constraint for a passing verify is a failed batch, not a success.
