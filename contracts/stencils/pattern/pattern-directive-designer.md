<!-- This is the RoleDesigner directive: the variant internal/pattern.Directive renders for the agent
     that designs a task before any plan exists. internal/loomengine/discussion.go is its consuming
     call site, passing pattern.RoleDesigner.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's one marker, the pattern_overview marker, with
     the content of PATTERN.md at the repository's worktree root.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares that one marker and no other, and the marker is required. -->

## Constraints — check every design decision against these

- **STOP.** These are the repo's rules; read them in full before settling a single design decision:

{{.pattern_overview}}

- Read every background file under pattern/ that the entries above link to and that touches a decision you are making.
- Every decision must comply with these entries, or name the entry it changes and say why.
- An entry cannot be waived silently: a design that departs from one without saying so is wrong, however well it fits the rest of the task.
- If an entry conflicts with anything else in this prompt, the entry wins — say so in the decision record instead of silently picking one.
