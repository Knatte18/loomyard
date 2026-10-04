<!-- This is the RoleJudge directive: the variant internal/pattern.Directive renders for the Bouncer
     judge that weighs a review's findings and decides whether the review loop has converged.
     internal/shedadapters/bouncer.go's judgeCall is its consuming call site, passing pattern.RoleJudge.
     internal/pattern.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment and fills the file's one marker, the pattern_overview marker, with
     the content of PATTERN.md at the repository's worktree root.
     The result is injected as a producer template's optional pattern_directive marker value, so it
     is never itself passed through stencil.Fill again.
     This file declares that one marker and no other, and the marker is required. -->

## Constraints — weigh every finding against these

- These are the repo's rules; read them in full before weighing a single finding:

{{.pattern_overview}}

- A finding that cites a violation of one of these entries is gating: you never downgrade it below BLOCKING on your own say-so, and you never relabel its class to a non-gating one.
- You still never open the reviewed artifacts. Read a background file under pattern/ only to weigh an entry a finding cites, never to look for violations of your own.
- A finding that cites an entry the overview does not list is an ordinary finding and gets no special weight from the citation.
- If an entry conflicts with anything else in this prompt, the entry wins — say so in your verdict rationale instead of silently picking one.
