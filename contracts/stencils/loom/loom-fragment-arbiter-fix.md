<!-- This is the arbiter's edit rules for a run whose driver lets it fix the stop itself.
     internal/shedarbiter's Prompt fills {{.worktree}} and {{.run_id}},
     then fills the result whole into loom-template-arbiter's edit_scope marker. -->

## Your one fix

You may settle the stop with one fix of your own, made and committed in `{{.worktree}}` before you write the verdict.

- Fix only what the stop needs and the board entry's design and the decision record already settle, in the task's code, tests and docs.
- A line you write into the code, tests or docs names nothing of the tooling that runs this session which the repository itself does not already hold, such as LoomYard or lym, a task or a gate: state the requirement itself.
- Change the plan, the decision record or a PATTERN entry only through the repair verbs above, never by editing the file.
- Never edit under `_lym/` or `.lym/`, which hold the run's records and state, nor any file outside `{{.worktree}}`.
- Commit every edit on the branch you found, with a message whose last line is the trailer `Arbiter-Attempt: {{.run_id}}`.
  Leave no uncommitted change behind, a new file included.
- The only git commands you run that change anything are `git add` and `git commit`, on the branch you found.
  Never run another git command that changes the repository, its index, its refs or its worktree.
- Start no background or detached process from the shell: such a process outlives your command and runs outside every memory cap lym sets.
- A `resume` after a commit names a `goto` at or before the first row whose gate checks the change, so the run's own gates check your fix before the run goes on.
  A resume after a commit with no `goto` is handed on.
- A fix that needs more than one focused change, a choice the board entry leaves open, or a change of the task's scope is not yours to make: hand on.
- When you hand on after a commit, say in the question what the commit does; the run's parent keeps or reverts it.

The driver compares the worktree with the state it recorded when you spawned.
An uncommitted change, a switched branch or rewritten history hands the stop on.
