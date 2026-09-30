<!-- This is the loom Plan producer's prior-plan block.
     It is read at call time by the Plan-Write rotator (via loomengine) and appended to the end of the Plan prompt on a respawn, rather than rendered through a marker in loom-template-plan.md.
     Both markers below are top-level {{.X}} substitutions and both are required: archive_dir is the archive directory's absolute path, and moved_files is the list of file names moved into it.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## Prior plan

An earlier session of this run wrote a plan.
That plan was moved out of the plan directory into this archive directory:

{{.archive_dir}}

The files moved there are:

{{.moved_files}}

Before you write anything in the plan directory:

- Read the archived plan before writing anything.
- Check it against the decision record and the current tree.
- Carry forward every card that still holds by copying it into the plan directory rather than re-deriving it.
- Re-verify every glyph you carry forward with `lyx quarry`.
- Revise only what is wrong, missing or stale.
- Never modify the archive directory.
- Never copy `00-overview.md` from the archive; write it fresh, last, with `approved: false`.
