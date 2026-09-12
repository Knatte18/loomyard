<!-- This is the RoleImplementer friction directive: the variant internal/friction.Directive renders
     for any agent that edits code. Its consuming call sites are the webster fork prompt, the webster
     recovery-strand prompt, the webster integration fork prompt (internal/websterengine/render.go),
     and loom's Plan-Write (internal/loomengine/plan.go).
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
sessions produce nothing here and that is correct.

Write a note **only when something actually went wrong** while you were editing code: a confusing
instruction, a tool that misbehaved, a constraint that contradicted itself, a dead end you had to
back out of, anything that cost you real time or nearly derailed the work.

If you decide to write one, put a title line naming what you were doing, followed by one or two
short paragraphs of freeform markdown describing the friction. There is no schema to follow. Write
it to exactly this path, with no filename of your own choosing:

{{.note_path}}
