<!-- This is the shed driver's repair guide: the reference the one-shot fork a driver spawns at an `error` or `interrupted` stop reads before it repairs or escalates.
     The driver stencil points at it by its deployed path through its guide_path marker and never paraphrases it; the guide is not an opening stencil and joins no role's list.
     It declares no marker and must stay marker-free, and, like the driver stencil, it names no recipe and no recipe-owned command.
     Refusal text and its ways forward live in the refusal spec the driver stencil points at, never restated here. -->

# Repair guide for a stopped run

You are the one-shot fork a driver spawned at an `error` or `interrupted` stop.
You read the stop's evidence, repair what a `lyx` verb can repair, write the stop report and any repair records, and edit nothing else.
Your reply is a short conclusion and exactly one action for the driver to run: a named repair verb, a re-launch of the loop, or an escalation.
The run id, the report path and the envelope arrive in the driver's brief; the evidence is the envelope's `loop.trace_copy`, `loop.stderr_path`, the `trace_file` it names and the child traces under the trace directory.

## Repairing

Read `loop.trace_copy` and the child traces it names.
Identify the half-finished mutation from the `fabric: mutation` records (attrs `kind`, `target`, `detail`) and the `shed: step`, `shed: step done` and `shed: step refused` boundary records.
Restore a state the next step can proceed from, then name the re-launch as the driver's action.

Repairs act through `lyx`'s own verbs, plus read-only git for diagnosis:

- Partial `lyx fabric add` (worktrees created, a later step failed): `lyx fabric remove [--force] [--remote] <slug>` removes the pair; a code branch it leaves behind falls under the stranded-branch exception.
- Partial `lyx fabric remove`: re-run `lyx fabric remove [--force] <slug>`, or `lyx fabric prune --apply [--force]` for an orphaned half-pair.
- Stranded remote records branch: `lyx fabric cleanup --apply [--force] --remote`.
- Stranded remote code branch the failed step created and pushed: the stranded-branch exception.
- Drifted or broken records side of a pair: `lyx fabric reconcile`.
- `lyx reed ...` only for strands the trace names as the failed step's own.

Never: `lyx shed pause` as a repair, hand-edit a status file or `seed.json`, re-seed, force-push, or delete a branch or worktree the trace does not name as created by the failed step.
A crash window outside this list, or one no `lyx` verb can repair, escalates by design.

Prefer not to merge the parent into a task worktree while its in-flight producer is still running;
when a parent fix is needed mid-run, `lyx fabric merge-in` is survivable and will warn.
Only a clean merge survives: a conflicted merge-in, once resolved, is refused by record-batch until HEAD returns to the batch report's `head_sha`.

Traces can be swept before a later read, so each repair record copies the trace lines it acted on.

## Stranded-branch exception

The one raw-git mutation: deleting a code branch the failed step's trace proves it created.
Code branches only; a stranded records branch goes through `lyx fabric cleanup`.

- Local delete: a `branch_created` entry for exactly that branch in the failed step's trace or a child trace sharing its id, and no later `branch_deleted` entry for it.
  Run `git -C <repo> branch -D <branch>`.
- Remote delete: a `branch_created` entry and a `branch_pushed` entry for exactly that branch, and no later `remote_branch_deleted` entry for it.
  Run `git -C <repo> push <remote> --delete <branch>`.
- `<repo>` and `<remote>` come from the entry's `detail` (`side=code repo=<abs> [remote=<name>]`).
  A missing `detail`, any other `side`, a missing `remote=` for a remote delete, or a repository path that no longer exists fails the rule, and you escalate.
- A `branch_pushed` entry alone never qualifies: it is recorded whenever an existing branch is pushed forward.
- Never delete a branch carrying commits the trace does not attribute to the failed step.

`PATTERN-fabric-git` binds `lyx`'s own code; you make no commits.
Each deletion is a repair record like any other.

## Repair cap

Make at most two repairs of the same row without the run advancing.
The key is the loop envelope's `loop.current_producer` and `loop.history_length`, which an error envelope's own keys do not carry.
A different pair resets the count, and the third failure escalates.
The automatic re-step budgets are separate from this cap and do not count toward it.
The `reinvoke` sub-case counts toward the cap.
`("", 0)` is a valid key for a run with no status file.

## Binary watch

The watch is a background shell loop that resolves the `lyx` executable once, checks its modification time about once a minute, and exits when it changes.
It never runs a long-lived `lyx` process, which would block a deploy's overwrite on Windows.
When it exits, read `lyx shed status <run-id>` once, and re-step only if `binary_changed` is true and the clean-tree guard passes; otherwise re-arm the watch.
A watch job that ends by the harness timeout is re-armed, never read as a resume.

## Interrupted invocations

An invocation that was killed, timed out or exited without a parseable envelope wrote no envelope.
Read `lyx shed status <run-id>` once and branch on that read:

- `current_producer` or `history_length` changed, or `state` is no longer running: the run advanced; continue from the fresh status.
- Unchanged and `interrupt_policy: reinvoke`: re-invoke the step; this counts toward the repair cap.
- Unchanged and `interrupt_policy: handback` or `""`: hand back unconditionally.
  A live agent may still be running, and re-invoking would restart it.

The trace of an interrupted step is found by the status envelope's `last_step.trace_id`: every file in `trace_dir` whose name carries that id, read in the order of the UTC timestamp in the name (`trace-<UTC>-<traceid>-<pid>.log`).
A child spawned into another worktree, or a process in a reed strand pane, is reached only through paths the parent trace names, never by id.

## Repair records and the stop report

`friction_dir` and `scratch_dir` come from the file at `envelope_path`, or from the printed envelope when it carries no `envelope_path`.
Write one record per repair into the envelope's `friction_dir` when it is non-empty, and under `<scratch_dir>/repairs/` only when it is empty: the failure, the trace lines acted on, the action taken and the outcome.
At every stop the report lists `friction_dir` and whichever of `friction_dir` or `<scratch_dir>/repairs/` held the records.
Every report names the run by the envelope's `run_id` and gives its position as `history_length` plus `progress` (`step` of `steps`, and `name`).
Each automatic re-step writes a record into the same place, holding the stop, the field that justified it (the `transient` class, or the build identity `last_step` recorded before the re-step and the one it records after), the action and the outcome.
