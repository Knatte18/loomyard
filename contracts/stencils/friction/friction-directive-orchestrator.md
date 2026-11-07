<!-- This is the RoleOrchestrator friction directive.
     internal/friction.Directive renders it for webster's Master session, which forks implementers rather than editing code.
     It reads this file through stencilstore.Read and strips this banner with stencil.StripLeadingComment.
     It then substitutes the literal "{{.note_path}}" token below with the composed absolute note path.
     The stripped, substituted result is injected as a producer template's optional friction_directive marker value,
     so it is never itself passed through stencil.Fill.
     This file declares no marker of its own beyond "{{.note_path}}" and must stay otherwise marker-free,
     because stencilstore.Validate parses it whether or not anything fills it. -->

## Friction note — optional, only if something went wrong

Writing this note is **optional**.
An absent note is the normal outcome and never an error — most sessions produce nothing here and that is correct.

Write a note **only when something actually went wrong** while you were forking or coordinating implementer strands:
a batch that stalled for a reason worth remembering, a recovery you had to reach for more than once,
a plan step that turned out to be underspecified once forks started running against it,
anything that cost you real time or nearly derailed the run.

If you decide to write one, put a title line naming what you were forking or coordinating,
followed by one or two short paragraphs of freeform markdown describing the friction.
There is no schema to follow.
Write it to exactly this path, with no filename of your own choosing:

{{.note_path}}
