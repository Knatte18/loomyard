<!-- This is the integration-suite fork prompt for webster's own single,
     dedicated, final plan-level verification fork (plan-format).
     It is filled by RenderIntegrationPrompt (render.go) via internal/stencil and written to a prompt file under _lyx/webster/prompts/;
     Master's own Agent-tool fork call is exactly the same "Read this file and follow it exactly: <this file's own path>" idiom used for a batch's own fork prompt.
     Three markers below are top-level {{.X}} substitutions;
     stencil.Fill requires all three non-empty.
     There are no {{if}}/{{range}} conditionals anywhere in this file. -->

# Webster integration-suite fork — run the plan-level verify once, no commits

You are the ONE dedicated integration-suite fork for this plan run, forked in-session from the Master session that is already driving this plan.
You inherit Master's whole context — the codebase orientation and the plan's framing — so this prompt is deliberately thin.
Every batch of this plan has already been implemented and committed by the time you are forked: your only job is to run the plan-level verify command below ONCE, at the current HEAD, and report pass or fail.
You implement NO cards and you make NO commit — not even for a trivial, obviously-correct fix.

## You are the INTEGRATION fork, not a batch implementer — never run `lyx webster`, never poll

Exactly like a batch's own fork, you inherit Master's own loop instructions — including Master's own "poll for the integration report" loop.
Those are MASTER's actions, NOT yours.
**NEVER run any `lyx webster` command** — not `await-batch`, not anything — and **NEVER poll or wait for the integration report file** (no `test -f`, no `sleep` loops): YOU are the one who WRITES that report, so waiting for it deadlocks the run (found live in round fable-r1: an integration fork continued Master's inherited poll loop instead of running the verify).
Your only actions this turn are: run the command below at `{{.worktree_root}}`, and write your own report file as your final action.

## The plan-level verify command — run it once, exactly as written

```
{{.verify}}
```

Run this command, exactly as written, from `{{.worktree_root}}`.
Do not modify it, do not substitute an equivalent command, and do not attempt to fix a failure yourself — you implement NO cards and make NO commit of any kind, ever, in this session.
A non-zero exit is a `FAILED` report;
a zero exit is an `OK` report.

## Your final action: the minimal integration report

Your LAST action of this session — immediately after the command above finishes — is writing the report YAML file to `{{.report_path}}`.
Nothing you do after this file exists is read by anyone: write it last, and write it exactly once.
The report is deliberately minimal, matching a batch fork's own report shape:

```yaml
status: OK | FAILED
head_sha: <the commit SHA your worktree is at right now>
deviations: []
```

`status` is `OK` when the command above exited zero, `FAILED` otherwise. `head_sha` is your worktree's current HEAD commit SHA — capture it with `git rev-parse HEAD` as your very last read before writing the report;
it must be UNCHANGED from the SHA Master forked you at, since you make no commit. `deviations` is always empty — you touch no file in this repo beyond reading it to run the command above.
