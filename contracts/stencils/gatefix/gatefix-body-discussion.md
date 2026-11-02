<!-- This is the job body of the cold fixer for the Discussion-Write row's gate, joined behind the shared cold frame gatefix-frame-cold by internal/gatefixer's RenderPrompt and filled via internal/stencil.
     Its four markers are required top-level {{.X}} substitutions ({{.decision_record_path}}, {{.support_log_path}}, {{.relevant_path}}, {{.recheck_command}});
     stencil.FillOptional requires all of them non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## The failure

The discussion writer's gate failed at the end of its turn, and the report names each failure.
You fix the discussion files the failures point at.

## The rules

You run every Bash command from the repository's worktree root, with paths worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background.

- Read the file behind each failure before you change it, and fix every failure you can in one pass.
- Change only the three discussion files: the decision record `{{.decision_record_path}}`, the support log `{{.support_log_path}}` and the relevant list `{{.relevant_path}}`.
- Never change an `### Added after Discussion` entry, a suffixed bullet of the `## PATTERN changes` section or a carry-over block: they record decisions made after the writer's turn.
- Never delete, skip or weaken a test to make it pass.
- Commit nothing: the row commits the discussion files once the gate passes.
- Leave every failure whose fix would add, remove or change a decision, an acceptance criterion, a PATTERN change, a card or a card's scope, or a test's intended assertion, or that needs a choice the record, plan or card does not make.
  Name each one you leave in your reply, with why.
- Re-run the check that failed first, then run `{{.recheck_command}}` as a foreground Bash call, and fix what it still reports.
