<!-- This is the RoleReviewFix friction directive: the variant internal/friction.Directive renders
     for the Burler round's combined review-then-fix agent.
     Its consuming call site is the burler round's instruction 1 (internal/burlerengine/prompt.go),
     filled through the same stencil.FillOptional call that also carries pattern_directive.
     internal/friction.Directive reads this file through stencilstore.Read, strips this banner with
     stencil.StripLeadingComment, and substitutes the literal "{{.note_path}}" token below with the
     composed absolute note path before returning the result.
     The stripped, substituted result is injected as a producer template's optional
     friction_directive marker value, so it is never itself passed through stencil.Fill.
     This file declares no marker of its own beyond "{{.note_path}}" and must stay otherwise
     marker-free, because stencilstore.Validate parses it regardless of whether anything ever fills
     it. -->

## Friction note — optional, only if something went wrong

Writing this note is **optional**. An absent note is the normal outcome and never an error — most
rounds produce nothing here and that is correct.

Write a note **only when something actually went wrong**, whether in the review phase or the fix
phase of this round: a rubric item that was ambiguous, a finding you couldn't reach agreement with
yourself on, a fix that fought the codebase harder than it should have, or anything else that cost
you real time or nearly derailed either phase.

If you decide to write one, put a title line naming what you were reviewing or fixing, followed by
one or two short paragraphs of freeform markdown describing the friction. There is no schema to
follow. Write it to exactly this path, with no filename of your own choosing:

{{.note_path}}
