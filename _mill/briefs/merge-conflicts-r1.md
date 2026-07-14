# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/mux-server-crash rm <file>`.



## Conflicting files

- `internal/muxcli/add.go`
- `internal/muxengine/render/types.go`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Run `git -C /home/knatte/Code/loomyard/wts/mux-server-crash add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/mux-server-crash rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/mux-server-crash rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. The `mill-merge-in` frontend reads this field and surfaces any losses to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C /home/knatte/Code/loomyard/wts/mux-server-crash` for any git commands; do not `cd`. Worktree cwd is `/home/knatte/Code/loomyard/wts/mux-server-crash`.
