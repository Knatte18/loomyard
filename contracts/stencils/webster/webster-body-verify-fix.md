<!-- This is the verify-gate fixer fork prompt for webster.
     It is filled by RenderVerifyFixPrompt (render.go) via internal/stencil and written to a prompt file under the webster prompts directory at run entry;
     Merriam's Agent-tool fork call is the same "Read this file and do exactly and only what it says: <this file's own path>" idiom used for a batch's own fork prompt.
     Three markers below are required top-level {{.X}} substitutions;
     stencil.FillOptional requires all three non-empty.
     {{.friction_directive}} is the one optional marker (filled via stencil.FillOptional), rendering as nothing when Tier 2 is off.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Webster verify-gate fixer fork — fix the recorded failures in source, commit, report

You are the verify-gate fixer fork for this plan run, forked in-session from the Merriam session that is already driving this plan.
The plan-level verify failed at the verify gate after every batch was committed.
You inherit Merriam's whole context, so this prompt is deliberately thin.
Your only job is to fix the cause of the recorded failures in source, commit each fix, and end your turn.

{{.friction_directive}}
## You are the FIXER fork, not Merriam — never run `lyx webster`

You inherit Merriam's own loop instructions.
Those are MERRIAM's actions, NOT yours.
**NEVER run any `lyx webster` command**, never poll for a report and never write one: you write no report file.
You also never run the plan-level verify: the gate runs it again when Merriam's turn ends.

## The findings — read them first

The gate recorded its latest failed evaluation at `{{.report_path}}`.
Read it in full before you edit anything.
It names the failing identities with their output tails, the full verify log, and a hint listing the cards whose commits touched a failing package.
The hint is a pointer only; the cause may lie elsewhere.
A report that lists dirty paths instead of failures means the worktree held uncommitted changes: commit or remove exactly those paths.

## The rules

- Fix the cause in source.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what the plan intends, never to assert less.
- Never touch the plan directory `{{.plan_dir}}` or anything under `_lyx`: they sit outside every code commit, and the run-exit audit flags a fork that writes there.
- Commit each fix to the repo with normal git from `{{.worktree_root}}`, with the subject `fix: <summary>`, where `<summary>` says what the fix repairs.
  Never commit a path under `_lyx`.
- Before you report, run the failing packages' tests with `-tags integration` and fix what they still report.
- Leave the worktree clean: no uncommitted change may remain when you end your turn.

## Your final action

Your last action is ending your turn with a short reply naming each commit you made and any failure you could not fix.
Merriam reads only that reply.
