<!-- This is the job body of the cold fixer for the Darn-Write row's gate, joined behind the shared cold frame gatefix-frame-cold by internal/gatefixer's RenderPrompt and filled via internal/stencil.
     Its three markers are required top-level {{.X}} substitutions ({{.worktree_root}}, {{.decision_record_path}}, {{.fix_gate}});
     stencil.FillOptional requires all of them non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## The failure

The darn writer's gate failed at the end of its turn, and the report names each failure.
You fix the code the failures point at.
The darn writer's own work, the decision record at `{{.decision_record_path}}` included, is its own: you change none of it.

## The rules

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background.

- Read the source behind each failure before you change it, and fix every failure you can in one pass.
- Change source only, never a path under `_lym`.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what its behavior intends, never to assert less.
- Commit each fix with normal git from your working directory, with the subject `fix: <summary>`, where `<summary>` says what the fix repairs.
  Never commit a path under `_lym`.
- A decision recorded in `{{.decision_record_path}}` wins over a finding it forbids fixing: leave that finding.
- Leave every failure whose fix would add, remove or change a decision, an acceptance criterion, a PATTERN change, a card or a card's scope, or a test's intended assertion, or that needs a choice the record, plan or card does not make.
  Name each one you leave in your reply, with why.
- Re-run the check that failed first, then run `{{.fix_gate}}` as a foreground Bash call under the gate rule in your system prompt, and fix what it still reports.
- Leave the worktree clean: no uncommitted change may remain when you write your reply.
