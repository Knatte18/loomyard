<!-- This is the job body of Weaver's plan-level verify fixer, joined behind the shared cold frame gatefix-frame-cold.
     internal/gatefixer's RenderPrompt joins it and fills it through internal/stencil.
     Its required markers are {{.findings}}, {{.journal_path}}, {{.plan_dir}} and {{.worktree_root}}.
     The frame fills report_path and reply_path.
     It holds no {{if}} or {{range}} conditional. -->

## The findings — read them first

The plan-level verify failed after every batch was committed.
Its findings follow:

{{.findings}}

The report at `{{.report_path}}` holds the same failure with the full verify log's path.
Read both before you edit anything.
A finding that lists dirty paths means the worktree held uncommitted changes: commit or remove exactly those paths.
A finding that names an entry other than the verify entry records the failure of a run-level entry, such as the `drift` entry's changed values.
You never edit `_lym/drift/expected.yaml`.
A value the plan or the decision record shows the change meant to move is intended: leave it as it is and say so in your reply.
Fix any other value in code, as you fix a failing test.

## The run journal

The run journal at `{{.journal_path}}` holds what each batch's attempt reported about its own work.
Read it for what the batches struggled with: the cause of a failure often sits in an entry there.

## The rules

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there and give paths in Bash commands worktree-relative.
{{template "engine-absolute-paths"}}
You never run a Bash call in the background, and you end your turn only after your final action.

- Read `_lym/plan/00-overview.md` before you edit anything: the task framing and `## Shared Decisions` say what the plan intends, which is what a test must assert.
- Fix the failing identities the findings name, at their cause in source.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what the plan intends, never to assert less.
- Never touch the plan directory `{{.plan_dir}}` or anything under `_lym`: they sit outside every code commit, and a write there corrupts the run's own record.
- Commit each fix with normal git from your working directory, with the subject `fix: <summary>`, where `<summary>` says what the fix repairs.
  Never commit a path under `_lym`.
- A fix that departs from a PATTERN entry because the entry forbids the way the findings call for records the departure as a `Pattern-Departure:` trailer in that `fix:` commit.
- Never run the plan-level verify yourself: Weaver runs it again once you end.
- Leave the worktree clean: no uncommitted change may remain when you end your turn.

{{template "loom-fragment-loosened-check"}}
