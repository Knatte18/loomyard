---
name: ly-drive
description: Drive one seeded run through repeated `lyx shed step [<run-id>]`, repairing failures from each step's trace and escalating what it cannot repair. Runs only when an operator, a launch prompt or an orchestrator's fork prompt names it.
argument-hint: "[run-id]"
---

# ly-drive

## What it does

Drive one addressed run by repeatedly invoking `lyx shed step [<run-id>]`; the run-id defaults to `self`, an alias for the worktree's slug.
Read each envelope, repair what can be repaired from the step's trace, and escalate what cannot.
This skill carries no phase knowledge.
Which recipe runs is a property of the run's seed alone, so every branch below is on an envelope field, a policy word or one of the five error kinds, never on a row or recipe name.

## Drive directory

The drive directory is the directory the run was seeded from.
A fork prompt names it; otherwise it is the session's own cwd.
Run every `lyx` call as `(cd <drive-dir> && lyx ...)` in a subshell, so the session's own shell cwd never changes.
A wrong directory surfaces as the recipe's own refusal text, which the driver reports verbatim.

## How to invoke a step

Run `lyx shed step <run-id>` as one background job.
Mint no `LYX_TRACE_ID`, create no `mktemp` step directory and add no redirects: `lyx shed step` mints its own trace id and keeps its own record of each invocation under the run's untracked `.lyx` scratch directory, so the records never dirty the worktree.
Never run a step as a blocking foreground call: a step can block for a whole agent run, longer than any foreground shell call allows.
Launch one step per background job, and read its envelope and its trace before launching the next; never a loop that runs several steps without the session reading each one in between, since a trace read only after the loop ends may already be swept.
Wait for that background job's own exit, by its completion notice or by `wait <pid>` in the shell that launched it, then read the envelope from the job's own output.
When the job left no output, read `lyx shed status <run-id>` and take the envelope from its `last_step`: the file named by `envelope_path`, present once `finished` is true.
Never wait by matching text of any command line (`pgrep -f`, `ps | grep`), whatever the text is: the step's own verb, a wrapper script, the step directory's path.
The waiting shell's own command line carries the same text, so such a wait never ends.

## Baseline

Before the first step, read `lyx shed status [<run-id>]` once and record `current_producer` and `history_length`.
An envelope with `found: false`, success or error, is an empty baseline `("", 0)`; proceed to the first step, whose own bootstrap seeds the status file.
Any other status error is handed back.
A baseline `state` of `blocked`, `awaiting`, `paused` or `failed` is not a stop: starting a driver on such a run is how an operator resumes it after resolving the cause (for `awaiting`, after approving), so proceed to the first step, which resumes the run.
Label that read as taken before the driver acted, for example "before step: blocked at Publish".

## The loop

There is no step cap; the run's own bounce budgets and the repair cap bound the loop.
Continue while the envelope's `continue` is true, reading the artifact `output` names when it is non-empty.
After every step, read its `trace_file` right away, because traces are swept by retention (see Repairing).

## Stopping on a non-running state

On `continue: false`, `state: done` stops the loop.
`blocked` and `paused` are handed back with the envelope's `reason` and are never repaired: a Go gate concluded a human is needed.
`awaiting` is handed back the same way, as a planned hand-off with the envelope's `reason`: the run waits on a person by design, so the report words it as a hand-off and never as a failure,
and nothing is repaired.

## Error envelopes

An error envelope carries `kind`, one of five values, plus `trace_file`, `friction_dir`, `scratch_dir` and `transient`.
`transient` is a class name, or empty when the failure is not transient; the driver reads that key and never matches error text.

- `producer`, `bootstrap`, `unseeded`: go down the repair path, under the repair cap.
- `busy`: hand back.
  The lock holder may be a live driver or a sibling fork, and the driver never pauses, kills or unlocks another driver.
- `ownership`: hand back; a slug mismatch is an operator decision, not a crash state.
- No `kind` (an unseeded run-id, an unsupported verb): always escalate with the refusal text verbatim, after reading its trace for the report.
  The driver never re-seeds, and no `lyx` verb fixes either.

## Automatic re-steps

The driver makes exactly two kinds of automatic re-step, a closed list.
Each has its own budget, separate from the repair cap.

- **Transient re-step**: an error envelope with a non-empty `transient` gets one immediate re-step, with no repair action and no wait.
  Record `current_producer` from a status read first; if that re-step stops again at the same `current_producer`, for any reason, hand back.
- **Binary-change re-step**: when `lyx shed status`'s `last_step.binary_changed` is true, re-step once.
  It is checked at the stop itself, and again each time the binary watch fires while parked.
  Eligible stops:
  - `blocked`;
  - a `producer`, `bootstrap` or `unseeded` error the driver escalated (repair cap exhausted or nothing to repair);
  - a transient stop whose own re-step failed.

  Never eligible: `awaiting`, `paused`, `busy`, `ownership`, a no-kind refusal, and an interrupted invocation under `interrupt_policy: handback`.
  `awaiting` and `paused` are never auto-resumed, by either re-step.
- **Clean-tree guard**: skip the binary-change re-step, and stay parked, when `git -C <warp> status --porcelain` in the task's code worktree (the warp) lists any uncommitted change.
  The driver's own stop reports, repair records and park marker never count: they live in the weft, which the warp reaches only through its `_lyx` and `.lyx` links, and the warp's git excludes both links, so porcelain there never lists them.
  A skipped re-step is not retried when the tree turns clean; it waits for the next binary change or a resume.

Before any self-initiated re-step, remove the park marker.

## Parking

Only a session whose launch prompt says to park does so.
Orchestrator forks and operator-launched sessions keep their stop behaviour and get the transient re-step and the stop-time binary-change re-step only.

At a hand-back, a parking driver does three things, in order:

1. writes its stop report;
2. runs the park command the launch prompt names, which commits the run records and writes the park marker, named `driver-parked`, under `scratch_dir`, holding the stop report's path;
3. starts the binary watch as a background job when the stop is binary-change eligible.

Never write the park marker by hand: the park command owns it.

It then ends its turn with the session open.
At `done` and at `busy` it runs the launch prompt's end-of-session command instead, as today.
On every wake (a resume line, a background job's exit, a context compaction), the marker on disk is the truth of whether the driver is parked.

## Binary watch

The watch is a background shell loop that resolves the `lyx` executable once, checks its modification time about once a minute, and exits when it changes.
It never runs a long-lived `lyx` process, which would block a deploy's overwrite on Windows.
When it exits, read `lyx shed status` once, and re-step only if `binary_changed` is true and the clean-tree guard passes; otherwise re-arm the watch.
A watch job that ends by the harness timeout is re-armed, never read as a resume.

## Resume

A resume line names a report path.
On it, stop the driver's own watch job, remove the marker if still present, take a fresh baseline, reset the repair and re-step budgets, and write every later stop report of this attempt to that path.
A resume line that arrives while a step job is in flight starts no second step; its report path and budget reset apply to the next stop.

## Interrupted invocations

An invocation that was killed, timed out or exited without a parseable envelope wrote no envelope.
Read `lyx shed status [<run-id>]` once and branch on that read:

- `current_producer` or `history_length` changed, or `state` is no longer running: the run advanced; continue from the fresh status.
- Unchanged and `interrupt_policy: reinvoke`: re-invoke the step; this counts toward the repair cap.
- Unchanged and `interrupt_policy: handback` or `""`: hand back unconditionally.
  A live agent may still be running, and re-invoking would restart it.

The trace of an interrupted step is found by the status envelope's `last_step.trace_id`: every file in `trace_dir` whose name carries that id, read in the order of the UTC timestamp in the name (`trace-<UTC>-<traceid>-<pid>.log`).
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

Prefer not to merge the parent into a task worktree while its Webster phase is in flight;
when a parent fix is needed mid-run, `lyx fabric merge-in` is survivable and will warn.
Only a clean merge survives: a conflicted merge-in, once resolved, is refused by record-batch until HEAD returns to the batch report's `head_sha`.

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
The automatic re-step budgets are separate from this cap and do not count toward it.
The `reinvoke` sub-case counts toward the cap.
`("", 0)` is a valid key for a run with no status file.

## Ways forward

A refusal's message ends in a `way forward:` clause, and `contracts/specs/refusal-spec.md` lists every refusal with the way forward it names.
When a stop report quotes a refusal, it looks the refusal text up in that table and names the way forward beside it, so the operator reads a next step rather than a diagnosis alone.
A refusal the table does not list is reported verbatim with no way forward added.

Two stops name `lyx shed goto --to <producer>` as their way forward:

- a `current_producer` that is no longer in the recipe;
- an exhausted bounce budget.

The driver reports `goto` as the way forward and never runs it: it is an operator and orchestrator verb, and a run moved by the driver itself would hide the decision from the person who owns it.

## Repair records and the stop report

Write one record per repair under `<scratch_dir>/repairs/`: the failure, the trace lines acted on, the action taken and the outcome.
Write the stop report under `scratch_dir` too, or to the path a launch prompt names.
At every stop the report lists `friction_dir` and `<scratch_dir>/repairs/`.
Every report names the run by the envelope's `run_id` and gives its position as `history_length` plus `progress` (`step` of `steps`, and `name`).
It never cites the driver's own step count.
A launch prompt from the orchestrator names a report path under the run's durable drive-reports directory.
Each automatic re-step writes a record under `<scratch_dir>/repairs/` holding:

- the stop;
- the field that justified it: the `transient` class, or the build identity `last_step` recorded before the re-step and the one it records after;
- the action and the outcome.

After a self-initiated re-step, the next stop rewrites the same report file to cover the whole attempt, listing every automatic re-step since the attempt began.
Its end-of-session command commits the stop report and the friction notes through the orchestrator's own records-commit verb before ending the session, so this skill itself still makes no commits.
When the launch prompt names an end-of-session command, run it as the last act, after writing the stop report: the teardown command at `done` or `busy`, and at a park the park command (step 2 of parking), with the session then left open.

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
