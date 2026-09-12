---
name: ly-supervise
description: Drive a loom task through a supervised loop over `lyx loom step`, reading each step's envelope and stopping on any non-running state. Explicit invocation only.
disable-model-invocation: true
---

# ly-supervise

Drive one loom task by repeatedly invoking `lyx loom step`, reading the envelope it returns, and stopping the moment the task reaches a state a human needs to look at.
This skill carries no phase knowledge of its own.
It never names a producer, never predicts what comes next, and never decides what runs — every branch below is on a policy word or an envelope field the step verb itself hands over, never on a row name.

## Preconditions

The session's current working directory must already be the task worktree root, because `lyx loom step` derives everything from cwd, and `lyxcwd.Resolve` requires that cwd to be a git worktree root.
Verify this before the first step — for example, confirm the directory looks like a task worktree — rather than discovering the mismatch as a resolve failure mid-loop.
Tell the operator to open `lyx reed attach` in a side terminal before starting.
That is a second, independent watching layer, not a substitute for this loop.

## The pre-loop baseline

Before the first step, take one `lyx loom status` read and record its `current_producer` and `history_length` as the baseline.
This baseline always exists, and it exists for one reason: the interrupted-step branch later in this loop compares against it.
The very first step of a session — or the first step after an operator re-invokes past the iteration cap — has no prior step envelope to compare with, so the baseline is what makes that comparison possible.

## How to invoke a step

Never invoke a step as a blocking foreground shell call.
Launch it in the background with stdout redirected to a per-step file under `.scratch/ly-supervise/step-<n>.json`, then wait for that process to exit and read the envelope from the file.

This matters for a concrete reason: a single step blocks for a whole producer call, loom's LLM rows are minutes-to-an-hour agent spawns, and every agent shell tool caps a foreground call in the single-digit minutes.
A foreground call would be killed mid-producer on exactly the rows this loop exists to watch.
Redirecting to a file also keeps each envelope out of the transcript until this loop chooses to read it, which matters across a long run.
`.scratch/` is relative to the same cwd, is already gitignored repo-wide, and is the mandated scratch location — never the OS temp directory.

## The loop

Run a hard cap of **40 steps**.
Each iteration: invoke a step, read its envelope, read the artifact its `output` field names when that field is non-empty, and continue only while the envelope's `continue` field is true.

40 is a typical-run margin, not the mechanical ceiling, which is far higher.
Loom's list is seventeen rows, a review round inside a segment costs two steps, and a run taking three review rounds in each of three segments walks thirty-five steps, rounded up to 40.
The six review rows carry their own budget of five bounces and the three validator rows bounce at the inherited default of ten, which puts the worst case near a hundred steps — so treat 40 as a sane default to stop and check in at, not proof that anything is wrong past it.

On reaching the cap, stop and report to the operator rather than failing anything.
Loom's own state is untouched by the cap; the operator re-invokes this skill to continue from where the loop left off.

## Stopping

On any envelope whose `continue` field is false, and on any error envelope, stop looping and hand back to the operator with a report.
Never clear `state`, never edit the status file, never re-seed, never push.

The reason: a non-running state means a Go gate concluded a human is needed, and this loop deciding otherwise would be exactly the judgment-override the whole design avoids.

## Error envelopes

An error envelope carries a `kind` field with exactly one of five values.

On `kind: producer`, re-invoke the step **once**.
If the retry also errors, stop and report.

On every other kind, hand straight back to the operator with no retry.
None of the other kinds can be fixed by running the same command again, and retrying would read to the operator as though this loop were trying something it has no basis to try.
Name the remedy the envelope itself gives for the `busy` kind: the operator has a driver running and must pause it before this loop can proceed.

State the hard limit on cleanup plainly: this loop never kills reed panes, removes strands, deletes lock files, touches git, or edits any task artifact.
It reports debris; the operator resolves it.

## Interrupted invocations

An invocation that was killed, timed out, or exited without writing a parseable envelope is a distinct case from an error envelope, and it writes no envelope of its own.

On detecting one, read `lyx loom status` once and branch on that single read:

- If `current_producer` or `history_length` differs from the baseline, or `state` is no longer running, the producer finished and only the invocation died.
  Continue from the fresh status and do not count the step twice.
- If nothing differs, `state` is still running, and the status envelope's `interrupt_policy` field is `reinvoke`, re-invoke the step for the same row.
  This is loom's own designed crash-resume, not a retry of something unknown.
- If nothing differs and the policy is `handback`, stop and hand back, saying plainly that a live agent may still be running in its pane, and that a later re-invocation would restart it rather than attach to it.

Cap the re-invoking branch at **two consecutive** interrupted-and-re-invoked steps against the same row.
On a third, stop and hand back — something is wrong with the invocation mechanism itself rather than with the run.

Outside the handback branch, print no orphaned-agent warning, because there is no orphan: the next step attaches to the agent rather than abandoning it.

## Self-report

This skill may call `lyx selfreport create` only after the loop has stopped — terminal state, blocked, hand-back, or iteration cap — never between steps, and at most once per supervised run.
Draft the title and body, show both to the operator, and fire the call only on explicit operator approval.
The body goes in via `-b -` on stdin and states the row's policy-relevant facts, the envelope fields that were surprising, and the artifact path this loop read.
The default `bug` label stands for a defect; `--label enhancement` is for friction that is not a defect.

Record why the gate exists: the verb files a real public issue through the GitHub API, an outward-facing and hard-to-reverse act.

## Operator choices

Any point where this skill offers the operator a choice must present it as a numbered text list, one option per line in the form `1) Label — description`, never a mouse-driven prompt.
