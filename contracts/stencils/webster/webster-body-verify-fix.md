<!-- This is the shared job body of webster's two verify-gate fixers: the fixer fork Merriam spawns in its own session, and the cold fixer strand `lyx webster fix-verify` spawns at the recovery role.
     RenderVerifyFixPrompt joins it behind webster-prefix-verify-fix-fork, and RenderVerifyFixColdPrompt behind webster-prefix-verify-fix-cold (render.go), then fills it via internal/stencil.
     Its six markers are required top-level {{.X}} substitutions ({{.report_path}}, {{.plan_dir}}, {{.worktree_root}}, {{.outcome_path}}, {{.summary_path}}, {{.fix_gate}});
     stencil.FillOptional requires all of them non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

## The findings — read them first

The plan-level verify failed at the verify gate after every batch was committed.
The gate recorded its latest failed evaluation at `{{.report_path}}`.
Read it in full before you edit anything.
It names the failing identities with their output tails, the full verify log, and a hint listing the cards whose commits touched a failing package.
The hint is a pointer only;
the cause may lie elsewhere.
A report that lists dirty paths instead of failures means the worktree held uncommitted changes: commit or remove exactly those paths.
A report that lists fix commits names the commits earlier fixers made above the pre-fix head: they did not clear the failure, so check them before you build on them.

## The rules

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in your system prompt for `cd` and for quick checks.
Paths you pass to the Read, Write and Edit tools stay absolute.
You never run a Bash call in the background, and you end your turn only after your final action.

- Fix the cause in source.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what the plan intends, never to assert less.
- Never touch the plan directory `{{.plan_dir}}` or anything under `_lyx`: they sit outside every code commit, and a write there corrupts the run's own record.
- Commit each fix to the repo with normal git from your working directory, with the subject `fix: <summary>`, where `<summary>` says what the fix repairs.
  Never commit a path under `_lyx`.
- A fix that departs from a PATTERN entry because the entry forbids the way the findings call for records the departure as a `Pattern-Departure:` trailer in that `fix:` commit, in the shape `loom-plan-spec.md`'s `## Pattern-Departure trailer` section defines.
- Never write, create or delete Merriam's contract files `{{.outcome_path}}` and `{{.summary_path}}`.
  A fixer writing either forges the run's terminal judgment and halts the run at its exit audit.
- Never run the plan-level verify: the gate runs it again once Merriam's turn ends.
- Before your final action, run `{{.fix_gate}}` as a foreground Bash call under the gate rule in your system prompt, and fix what it still reports.
- Leave the worktree clean: no uncommitted change may remain when you end your turn.
