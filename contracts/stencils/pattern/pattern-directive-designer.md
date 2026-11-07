<!-- This is the RoleDesigner directive: the variant internal/pattern.Directive renders for the agent
     that designs a task before any plan exists.
     internal/pattern.Directive reads this file through stencilstore.Read and strips this banner with stencil.StripLeadingComment.
     It fills the required pattern_overview marker with the content of PATTERN.md at the repository's worktree root.
     It fills the optional pattern_changes marker with the task's declared PATTERN changes.
     The rendered text is injected as a producer template's optional pattern_directive marker value,
     so it is never itself passed through stencil.Fill again.
     This file declares two markers, the required pattern_overview and the optional pattern_changes, and no other. -->

## Constraints — check every design decision against these

- **STOP.** These are the repo's rules; read them in full before settling a single design decision:

{{.pattern_overview}}
{{.pattern_changes}}

- Read every background file under pattern/ that the entries above link to and that touches a decision you are making.
- Every decision must comply with these entries, or name the entry it changes and say why.
- List in the decision record's `## PATTERN changes` every entry your design adds, changes or removes, in the shape `loom-discussion-spec.md` defines, and make no PATTERN edit yourself: the edit lands in a Webster card.
- An entry cannot be waived silently: a design that departs from one without saying so is wrong, however well it fits the rest of the task.
- If an entry conflicts with anything else in this prompt, the entry wins — say so in the decision record instead of silently picking one.
