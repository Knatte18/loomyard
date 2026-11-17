<!-- This is the Claude engine's copy of the absolute-paths engine block:
     the line that keeps the paths a session passes to its file tools absolute.
     A stencil pulls it in on a line of its own with {{template "engine-absolute-paths"}}.
     internal/stencilstore reads it for the launch's engine.
     It declares no marker. -->
Paths you pass to the Read, Write and Edit tools stay absolute.
