---
name: ly-drive
description: Drive a loom task through a loop over `lyx loom step`, reading each step's envelope and stopping on any non-running state. Explicit invocation only.
disable-model-invocation: true
---

# ly-drive

Drive one loom task by repeatedly invoking `lyx loom step`, reading the envelope it returns, and stopping the moment the task reaches a state a human needs to look at.
This skill carries no phase knowledge of its own.
It never names a producer, never predicts what comes next, and never decides what runs — every branch below is on a policy word or an envelope field the step verb itself hands over, never on a row name.

## Preconditions

The session's current working directory must already be the task worktree root, because `lyx loom step` derives everything from cwd, and `lyxcwd.Resolve` requires that cwd to be a git worktree root.
Verify this before the first step — for example, confirm the directory looks like a task worktree — rather than discovering the mismatch as a resolve failure mid-loop.

The operator does not need to open a side terminal for this: a worktree opened through `lyx ide spawn`'s generated VS Code task already starts `lyx reed up`, then `lyx reed add --if-absent --cmd claude --name claude --focus`, then `lyx reed attach`, in that sequence.
The session running this skill is therefore itself the strand named `claude`, and the panes loom spawns while this loop runs are its siblings inside that same reed session — not a second, independent layer this skill needs to ask the operator to open.

Run this self-check once, before the first step, to confirm that expectation holds for the session actually running: read `$TMUX_PANE` from the environment, then run `lyx reed status` and compare that pane id against the tracked strands the envelope reports.
Exactly three outcomes follow.

- `$TMUX_PANE` is set and its pane id appears among the tracked strands: proceed silently.
- `$TMUX_PANE` is set but its pane id does not appear among the tracked strands: tell the operator this session is running in a pane reed does not track, name the launch chain above as the fix, and offer the choice — as a numbered text list, per this skill's own `## Operator choices` section — of relaunching through that chain or proceeding without reed supervision.
- `$TMUX_PANE` is unset: report the check as unconfirmed, not failed.
  Say the check could not run, that this session may or may not be a strand, and that proceeding is fine.

That third outcome exists because on Windows reed drives psmux, a tmux-compatible port, and nothing in this repo verifies that psmux exports `TMUX_PANE` into a pane's environment — so a two-outcome check would tell every correctly-launched Windows operator to relaunch on a signal that never fires for them.
State the relaunch advice only in the tracked-absent branch above; the unconfirmed branch never recommends it.

A worktree created before this launch convention keeps its existing `.vscode/tasks.json`, because the generator never clobbers a file that already exists.
The manual upgrade is to delete `.vscode/tasks.json` and re-run `lyx ide spawn`.

## The pre-loop baseline

Before the first step, take one `lyx loom status` read and record its `current_producer` and `history_length` as the baseline.
It exists for one reason: the interrupted-step branch later in this loop compares against it.
The very first step of a session — or the first step after an operator re-invokes past the iteration cap — has no prior step envelope to compare with, so the baseline is what makes that comparison possible.

On a task that has never been bootstrapped there is no status file yet, and this read returns an error envelope naming `lyx loom start` as the remedy.
That is not a failure to report: record an empty baseline and proceed to the first step, which bootstraps the task itself.
Treat any *other* status error as a hand-back, the same as an error envelope from a step.

## How to invoke a step

Never invoke a step as a blocking foreground shell call.
Launch it in the background with stdout redirected to a per-step file under `.scratch/ly-drive/step-<n>.json`, then wait for that process to exit and read the envelope from the file.

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

Read the policy off `lyx loom status`, not off the previous step's envelope, even though the previous envelope's `next_interrupt_policy` names the same row and the same table.
The status read is a fresh fact taken after the interruption, and it is the only one available on the very first step of a session, where no previous envelope exists.
The two agreeing is the point, not a redundancy to optimise away: if they ever disagree, the status file is authoritative and something is wrong worth handing back over.

Nothing in loom enforces `handback`.
The policy is advisory metadata for this loop to act on — re-invoking a `handback` row is not refused, not warned about on the envelope, and not blocked in any way; it kills the in-flight agent and restarts that row's work from its own persisted state.
This loop is the only thing standing between an automated caller and that restart, which is why the branch above stops rather than deciding for the operator.

Cap the re-invoking branch at **two consecutive** interrupted-and-re-invoked steps against the same row.
On a third, stop and hand back — something is wrong with the invocation mechanism itself rather than with the run.

Outside the handback branch, print no orphaned-agent warning, because there is no orphan: every spawning row's adapter probes for a live agent and waits on it, so the next step attaches rather than abandoning.

## Self-report

Nothing files automatically while this loop is driving.
Loom's two automatic self-report tiers both hang off `lyx loom run`'s own run, and `lyx loom step` runs neither — so on a supervised task, a blocked halt, a producer failure, and a friction note left behind all pass unreported unless this skill reports them.
That is the trade this design makes: a live supervisor with an operator in the loop instead of a primitive filing public issues on its own, forty times a run.
Read it as a responsibility, not a gap.

The friction notes live at `.lyx/loom/friction/` under the task worktree root — a fixed location, not phase knowledge.
Whenever the loop stops — terminal state, blocked, hand-back, or iteration cap — list that directory and name any notes found in the stop report, because nothing else will: `step` never spawns the reflection pass that `run` runs, and the next task's own first seed clears the directory, so a note left unread here is dropped silently.
Reading the notes and judging whether one is worth a `lyx selfreport create` call is part of the same operator-gated flow below.

This skill may call `lyx selfreport create` only after the loop has stopped — terminal state, blocked, hand-back, or iteration cap — never between steps, and at most once per supervised run.
Draft the title and body, show both to the operator, and fire the call only on explicit operator approval.
The body goes in via `-b -` on stdin and states the row's policy-relevant facts, the envelope fields that were surprising, and the artifact path this loop read.
The default `bug` label stands for a defect; `--label enhancement` is for friction that is not a defect.

Record why the gate exists: the verb files a real public issue through the GitHub API, an outward-facing and hard-to-reverse act.

## Operator choices

Any point where this skill offers the operator a choice must present it as a numbered text list, one option per line in the form `1) Label — description`, never a mouse-driven prompt.
