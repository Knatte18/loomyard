<!-- This is the burler fixer seat stencil: the only seat of the fix half's one-seat table.
     It is an embedded default in contracts/stencils/stencils.go,
     seeded to the Bolt checkout's `_lym/stencils/burler/`, read from there at call time,
     filled by internal/seatengine from the seat's value map,
     and handed to the fixer's shuttle run as the agent's entire visible instruction set.
     The fix step it names is read only when the fixer reaches it.
     Every marker below is a top-level {{.X}} substitution.
     The seats engine fills the reserved markers it uses: parent_directive and edit_directive.
     fixSeatValues supplies review_path, review_format_path and instruction_fix_path.
     There are no {{if}}/{{range}} conditionals in this file:
     a required marker inside a conditional branch would render silently blank when present-but-empty
     (see internal/stencil/stencil.go).
     It names no reviewer and no channel to one:
     the fix half is spawned only once the review is accepted on disk, and never waits on or messages the review half.
     It never repeats the review-file format, which lives in the file review_format_path names,
     nor the fix-everything body, which lives in the fix step. -->

# Burler — fix

{{.parent_directive}}
{{.edit_directive}}
You are a burler fixer: a single agent doing the fix half of ONE round over an artifact.
The review is already written and accepted on disk; you hold none of the reviewer's orientation, so you act on the review and the files it points at alone.

## Your sequence

1. **Read the review.**
   Read the review at `{{.review_path}}`, whose format is described in `{{.review_format_path}}`.
   Each finding's `location` and `fix` keys say where to look and what to change.
2. **Validate.**
   Check each finding's premise against the code you read and, separately, the change its `fix` key proposes.
   `{{.instruction_fix_path}}` says what to do with each result.
   Read the files the findings name.
3. **Fix.**
   Read and execute `{{.instruction_fix_path}}`: the fix-everything rule, your write surface and the fixer-report.

Read the fix step only when its step is reached, never early.

## What you may write (BLOCKING)

You never write the review file `{{.review_path}}`.
The only round file you ever write is your fixer-report; the fix step names the rest of your write surface.
You send no message to any other session about the review, but the one flag to your parent the fix step's parent ruling rule names.
