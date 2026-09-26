---
name: ly-drive
description: Drive one seeded run through repeated `lyx shed step [<run-id>]`, repairing failures from each step's trace and escalating what it cannot repair. Runs only when an operator, a launch prompt or an orchestrator's fork prompt names it.
argument-hint: "[run-id]"
---

# ly-drive

## What it does

Drive one addressed run by repeatedly invoking `lyx shed step [<run-id>]`; the run-id defaults to `self`.
Read each envelope, repair what can be repaired from the step's trace, and escalate what cannot.
This skill carries no phase knowledge.
Which recipe runs is a property of the run's seed alone, so every branch below is on an envelope field, a policy word or one of the five error kinds, never on a row or recipe name.

## Drive directory

The drive directory is the directory the run was seeded from.
A fork prompt names it; otherwise it is the session's own cwd.
Run every `lyx` call as `(cd <drive-dir> && lyx ...)` in a subshell, so the session's own shell cwd never changes.
A wrong directory surfaces as the recipe's own refusal text, which the driver reports verbatim.

## How to invoke a step

Mint a fresh 16-lowercase-hex `LYX_TRACE_ID` for each invocation and export it into the subshell.
Never run a step as a blocking foreground call: a step can block for a whole agent run, longer than any foreground shell call allows.
Launch it in the background with stdout redirected to `<step-dir>/step-<n>.json`, where `<step-dir>` is a private directory created once per drive with `mktemp -d` and recorded as an absolute path.
Never put step output under the drive directory, or under the session's cwd when that is the drive directory, as it is for a driver a recipe's bootstrap verb launches: any file there dirties the run's worktree and fails its clean-tree gates.
Wait for that background job's own exit, by its completion notice or its PID, then read the envelope from that file.
Never wait by matching a command line (`pgrep -f 'lyx shed step'`): the waiting shell's own command line carries the same text, so the wait never ends.

## Baseline

Before the first step, read `lyx shed status [<run-id>]` once and record `current_producer` and `history_length`.
An envelope with `found: false`, success or error, is an empty baseline `("", 0)`; proceed to the first step, whose own bootstrap seeds the status file.
Any other status error is handed back.

## The loop

There is no step cap; the run's own bounce budgets and the repair cap bound the loop.
Continue while the envelope's `continue` is true, reading the artifact `output` names when it is non-empty.
After every step, read its `trace_file` right away, because traces are swept by retention (see Repairing).

## Stopping on a non-running state

On `continue: false`, `state: done` stops the loop.
`blocked` and `paused` are handed back with the envelope's `reason` and are never repaired: a Go gate concluded a human is needed.

## Error envelopes

An error envelope carries `kind`, one of five values, plus `trace_file`, `friction_dir` and `scratch_dir`.

- `producer`, `bootstrap`, `unseeded`: go down the repair path, under the repair cap.
- `busy`: hand back.
  The lock holder may be a live driver or a sibling fork, and the driver never pauses, kills or unlocks another driver.
- `ownership`: hand back; a slug mismatch is an operator decision, not a crash state.
- No `kind` (an unseeded run-id, an unsupported verb): always escalate with the refusal text verbatim, after reading its trace for the report.
  The driver never re-seeds, and no `lyx` verb fixes either.

## Interrupted invocations

An invocation that was killed, timed out or exited without a parseable envelope wrote no envelope.
Read `lyx shed status [<run-id>]` once and branch on that read:

- `current_producer` or `history_length` changed, or `state` is no longer running: the run advanced; continue from the fresh status.
- Unchanged and `interrupt_policy: reinvoke`: re-invoke the step; this counts toward the repair cap.
- Unchanged and `interrupt_policy: handback` or `""`: hand back unconditionally.
  A live agent may still be running, and re-invoking would restart it.

The trace of an interrupted step is every file in the status envelope's `trace_dir` whose name carries that step's `LYX_TRACE_ID`, read in the order of the UTC timestamp in the name (`trace-<UTC>-<traceid>-<pid>.log`).
A child spawned into another worktree, or a process in a reed strand pane, is reached only through paths the parent trace names, never by id.

## Repairing

Read `trace_file` and the child traces it names.
Identify the half-finished mutation from the `fabric: mutation` records (attrs `kind`, `target`, `detail`) and the `shed: step`, `shed: step done` and `shed: step refused` boundary records.
Restore a state the next step can proceed from, then step again.

Repairs act through `lyx`'s own verbs, plus read-only git for diagnosis:

- Partial `lyx fabric add` (worktrees created, a later step failed): `lyx fabric remove [--force] [--remote] <slug>` removes the pair; a warp branch it leaves behind falls under the stranded-branch exception.
- Partial `lyx fabric remove`: re-run `lyx fabric remove [--force] <slug>`, or `lyx fabric prune --apply [--force]` for an orphaned half-pair.
- Stranded remote weft branch: `lyx fabric cleanup --apply [--force] --remote`.
- Stranded remote warp branch the failed step created and pushed: the stranded-branch exception.
- Drifted or broken weft side of a pair: `lyx fabric reconcile`.
- `lyx reed ...` only for strands the trace names as the failed step's own.

Never: `lyx shed pause` as a repair, hand-edit a status file or `seed.json`, re-seed, force-push, or delete a branch or worktree the trace does not name as created by the failed step.
A crash window outside this list, or one no `lyx` verb can repair, escalates by design.

Traces can be swept before a later read, so each repair record copies the trace lines it acted on.

## Stranded-branch exception

The one raw-git mutation: deleting a warp branch the failed step's trace proves it created.
Warp branches only; a stranded weft branch goes through `lyx fabric cleanup`.

- Local delete: a `branch_created` entry for exactly that branch in the failed step's trace or a child trace sharing its id, and no later `branch_deleted` entry for it.
  Run `git -C <repo> branch -D <branch>`.
- Remote delete: a `branch_created` entry and a `branch_pushed` entry for exactly that branch, and no later `remote_branch_deleted` entry for it.
  Run `git -C <repo> push <remote> --delete <branch>`.
- `<repo>` and `<remote>` come from the entry's `detail` (`side=warp repo=<abs> [remote=<name>]`).
  A missing `detail`, a `side` other than `warp`, a missing `remote=` for a remote delete, or a repository path that no longer exists fails the rule, and the driver escalates.
- A `branch_pushed` entry alone never qualifies: it is recorded whenever an existing branch is pushed forward.
- Never delete a branch carrying commits the trace does not attribute to the failed step.

The Fabric Git Invariant in `CONSTRAINTS.md` binds `lyx`'s own code; this skill makes no commits.
Each deletion is a repair record like any other.

## Repair cap

Make at most two repairs of the same row without the run advancing.
The key is `(current_producer, history_length)` from a status read taken before the first repair, because an error envelope carries neither field.
A different pair resets the count, and the third failure escalates.
The `reinvoke` sub-case counts toward the cap.
`("", 0)` is a valid key for a run with no status file.

## Repair records and the stop report

Write one record per repair under `<scratch_dir>/repairs/`: the failure, the trace lines acted on, the action taken and the outcome.
Write the stop report under `scratch_dir` too, or to the path a launch prompt names.
At every stop the report lists `friction_dir` and `<scratch_dir>/repairs/`.

## Self-report

After the loop stops, read the `friction_dir` notes and the repair records; every repair record is itself friction data.
In operator-driven mode, draft at most one `lyx selfreport create` call per supervised run, with the body on stdin via `-b -` and `--label enhancement` for non-defects.
Fire it only on explicit operator approval: it files a public issue, an outward-facing and hard-to-reverse act.
In autonomous mode, file nothing and put the draft in the report.

## Operator choices

Present any choice as a numbered text list, one option per line in the form `1) Label — description`, never a mouse prompt.

## Autonomous mode

When the launch prompt says the session runs autonomously, there is no operator to ask.
Each choice this skill would offer becomes a line in the stop report, the report goes to the path the prompt names, and every other rule stays absolute.

## Driven from an orchestrator

An orchestrator session forks one `Agent` per run, with a prompt naming this skill, the run-id and the drive directory.
The fork runs the loop and returns the stop report path plus a short summary.
Recipe-specific directory knowledge lives in the orchestrator's own prompt, never here.
A fork's lifetime is the orchestrator session's; this is a limit, not something the skill solves.
