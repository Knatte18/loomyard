<!-- This is the integration-fix strand prompt for webster's one-shot repair of an integration regression.
     It is filled by RenderIntegrationFixPrompt (render.go) via internal/stencil and written to a prompt file under _lyx/webster/prompts/.
     The strand is a separate cold-start session: it inherits no Master context, so this prompt carries everything it needs.
     Five markers below are required top-level {{.X}} substitutions: regressions, verify, worktree_root, plan_dir and report_path.
     {{.card_hint}} and {{.friction_directive}} are the optional markers (filled via stencil.FillOptional), each rendering as nothing when empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Webster integration-fix strand — repair the regressed integration verify, commit, report

You are the ONE integration-fix strand for this plan run, a fresh session that inherits no earlier context.
Every batch of the plan has been implemented and committed, and the plan's integration verify then regressed: it fails at the current HEAD of `{{.worktree_root}}` where it did not fail before the plan began.
Your job is to fix the regressions below at that HEAD, commit the fix, and report.

{{.friction_directive}}
## The regressions

These identities fail at HEAD and were not proven to fail before the plan began, each followed by the tail of its output:

{{.regressions}}

{{.card_hint}}

## The verify command

```
{{.verify}}
```

Run this command, exactly as written, from `{{.worktree_root}}` before you report.
Do not modify the command or substitute an equivalent one.

## Rules

- Work only in `{{.worktree_root}}`, at its current HEAD, and commit with normal dev git, in non-merge commits only.
- Never write under `{{.plan_dir}}`.
- Never commit anything under `_lyx`, and write nothing else under `_lyx` but `{{.report_path}}` and the friction note the directive above names, if it names one.
- Never reset, rebase or amend an existing commit, and never run any `lyx` command.
- Leave no uncommitted change behind: Go judges only the commits, and a dirty worktree fails the attempt.
- Never delete, skip or weaken a test to make it pass.
  A test changes only when the test itself is wrong, and the commit message says why.
- Fix the cause of the regression, not its symptom.

## Your final action: the minimal report

Your LAST action of this session, after the fix is committed and the verify command has been run, is writing the report YAML file to `{{.report_path}}`.
Nothing you do after this file exists is read by anyone: write it last, and write it exactly once.
The report carries exactly these two fields and nothing else:

```yaml
status: OK | FAILED
head_sha: <the output of git rev-parse HEAD>
```

`status` is `OK` when the verify command exited zero after your commits, and `FAILED` when you could not make it pass.
`head_sha` is your worktree's HEAD commit SHA, captured with `git rev-parse HEAD` as your very last read before writing the report.
