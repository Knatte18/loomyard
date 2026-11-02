<!-- This is the job body of the cold fixer for a Webster card gate's red `lym gate test --gate card` call, joined behind the shared cold frame gatefix-frame-cold by internal/gatefixer's RenderPrompt and filled via internal/stencil.
     Its three markers are required top-level {{.X}} substitutions ({{.worktree_root}}, {{.card_ids}}, {{.gate_call}});
     stencil.FillOptional requires all of them non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## The failure

An implementer fork's card gate call failed, and the report names each failure.
The fork is implementing the card or cards `{{.card_ids}}` of the plan, and it has not committed them yet: their uncommitted change is in the worktree.
You fix the code the failures point at.

## The rules

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background.

- Read the card file or files under `_lym/plan/` that the ids name, and the worktree's uncommitted diff, before you change anything: the card says what the change is for, and the diff says what the fork has done so far.
- Read the source behind each failure before you change it, and fix every failure you can in one pass.
- Change source only, never a path under `_lym`.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what its behavior intends, never to assert less.
- Commit nothing: the fork commits the card, and your uncommitted change stays in the worktree for it to read.
- Leave every failure whose fix would add, remove or change a decision, an acceptance criterion, a PATTERN change, a card or a card's scope, or a test's intended assertion, or that needs a choice the plan or the card does not make.
  Name each one you leave in your reply, with why.
- Re-run the check that failed first, then run `{{.gate_call}}` as a foreground Bash call under the gate rule in your system prompt, and fix what it still reports.
