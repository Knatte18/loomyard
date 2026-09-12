<!-- This is the RoleInterview friction directive: the variant internal/friction.Directive renders
     for the Discussion-Write interview agent, whose job is neither editing nor reviewing.
     Its consuming call site is internal/loomengine/prompt.go's Discussion-Write composer.
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
interviews produce nothing here and that is correct.

Write a note **only when something actually went wrong** while you were interviewing the user to
build the decision record: a question that kept circling without landing, an answer that
contradicted an earlier one and never got reconciled, a scope boundary you couldn't pin down,
anything that cost you real time or nearly derailed the interview.

If you decide to write one, put a title line naming what the interview was about, followed by one
or two short paragraphs of freeform markdown describing the friction. There is no schema to follow.
Write it to exactly this path, with no filename of your own choosing:

{{.note_path}}
