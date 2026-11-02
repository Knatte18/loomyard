<!-- This is the job body of the cold fixer for the Plan-Write row's gate, joined behind the shared cold frame gatefix-frame-cold by internal/gatefixer's RenderPrompt and filled via internal/stencil.
     Its three markers are required top-level {{.X}} substitutions ({{.plan_dir}}, {{.lens_review_path}}, {{.recheck_command}});
     stencil.FillOptional requires all of them non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## The failure

The plan writer's gate failed at the end of its turn, and the report names each failure.
You fix the plan files the failures point at.

## The rules

You run every Bash command from the repository's worktree root, with paths worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background.

- Read the plan file behind each failure before you change it, and fix every failure you can in one pass.
- Change only the files under `{{.plan_dir}}`, and never the lens review record `{{.lens_review_path}}`.
  That record is the writer's own, and so is a lens review that is missing or incomplete: leave both, and name the missing or incomplete lens review in your reply.
- Never delete, skip or weaken a test to make it pass.
  A test a card names keeps its intended assertion.
- Commit nothing: the row commits the plan once the gate passes.
- Leave every failure whose fix would add, remove or change a decision, an acceptance criterion, a PATTERN change, a card or a card's scope, or a test's intended assertion, or that needs a choice the record, plan or card does not make.
  Name each one you leave in your reply, with why.
- Re-run the check that failed first, then run `{{.recheck_command}}` as a foreground Bash call, and fix what it still reports.
