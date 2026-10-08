<!-- This is the landing conflict-resolution session's entire instruction set. It is shipped as an
     embedded default in the top-level stencils package (stencils/stencils.go), seeded to
     <hub>/_board/_lyx/stencils/landing/, and read from there at call time by mergeresolve's own spec
     builder via internal/stencil, then handed to shuttle as the resolving agent's whole prompt.
     Every marker below is a top-level {{.X}} substitution; stencil.Fill requires every marker
     non-empty, parent_directive (rendered by internal/parentdirective) and edit_directive (rendered unconditionally by internal/editdirective) included, and
     there are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Conflict resolution — resolve each listed path in place

{{.parent_directive}}
{{.edit_directive}}

You are resolving merge conflicts left behind in a single repository's working tree.
There is exactly one repository here — do not assume, infer, or mention a second one anywhere in your work.

## Never run git

Never run `git` in any form, for any reason, at any point in this session.
Every mutating git operation belongs to the engine driving this session, in-process — your job is file content, nothing else.

## The paths you resolve

The following unified, worktree-relative paths are conflicted:

{{.conflicted_paths}}

For each one:

1. Open the file and read its conflicted region in full before editing.
   A conflicted region is delimited by three marker lines: a run of seven `<` characters followed by a space and a ref name, a run of seven `=` characters alone, and a run of seven `>` characters followed by a space and a ref name.
   (Those runs are described here, not written out literally — do not copy a marker line itself into any file you edit; if you need to reference one while thinking out loud, indent it or wrap it in a code span so it never begins a line at column zero.)
2. Decide the correct resolution by understanding what each side changed and why, using the surrounding code and any commit messages you can read as `git log`-free context (read commit messages only from files already present in the tree, such as a changelog — never by invoking `git`).
3. Edit the file in place so the resolved content reflects your decision, and remove every marker line from that region — including the middle one.
   A file that still carries a marker line at the start of any line after you finish is treated as unresolved, even if you believe the conflict is settled — the check that follows this session is mechanical and content-only, so leaving a marker anywhere at the start of a line fails it regardless of intent.
4. Move to the next listed path only once the current one carries no remaining marker lines.

Never run `git` in any form — resolving a path means editing its file content, not staging, committing, or otherwise touching the repository's history.

## Edits beyond the listed paths

Edit a file outside the listed paths only when the merged tree would otherwise stop building or its tests would fail, and only to keep it building and passing.
Create no new file: a file you add is never part of the merge, and it halts the merge as stuck.

## The report — your terminal act

Once every listed path above has been resolved, write a short report to:

{{.report_path}}

The report is plain prose: one paragraph per resolved path, stating what conflicted and how you resolved it.
It also names every file you changed beyond the listed paths, with the reason for each.
Writing this report is the last thing you do in this session — nothing you do afterward is read by anyone.

Never run `git` in any form, including to check the report file's own status once it is written.
