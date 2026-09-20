---
name: ly-drive
description: Drive an addressed run (defaulting to self, which arms loom) through a loop over `lyx shed step [<run-id>]`, reading each step's envelope and stopping on any non-running state. Explicit invocation only.
argument-hint: "[run-id]"
disable-model-invocation: true
---

# ly-drive

Drive one addressed run by repeatedly invoking `lyx shed step [<run-id>]` — the run-id argument defaults to `self`, which arms `loom`, when the operator gives none — reading the envelope it returns, and stopping the moment the run reaches a state a human needs to look at.
This skill carries no phase knowledge of its own.
It never names a producer, never predicts what comes next, and never decides what runs — every branch below is on a policy word or an envelope field the step verb itself hands over, never on a row name.

`loom` and `batten` are the two recipes shipped today whose table entries support `step` — see `## Error envelopes`'s closing note for what happens when the addressed run's own recipe does not.
The run-id argument exists so this loop can address any seeded run, not only `self`; which recipe it drives is a property of the addressed run's own seed, not of this loop.

## Preconditions

The session's current working directory must already match the driven recipe's own resolution requirement, because `lyx shed step [<run-id>]` derives everything from cwd through that recipe's own `Arm`.
For `loom`, that requirement is the task worktree root, because `lyxcwd.Resolve` requires cwd to be a git worktree root.
Verify this before the first step — for example, confirm the directory looks like a task worktree — rather than discovering the mismatch as a resolve failure mid-loop.
A different recipe may carry a different requirement — `batten`, for instance, refuses from anywhere but the hub's prime worktree, so driving a batten-seeded run-id with this loop is reachable only from there.

The rest of this section describes `loom`'s own launch convention; a different recipe's own driving session has no equivalent convention recorded here yet.

The operator does not need to open a side terminal for `loom`: a worktree opened through `lyx ide spawn`'s generated VS Code task already starts `lyx reed up`, then `lyx reed add --if-absent --cmd claude --name claude --focus`, then `lyx reed attach`, in that sequence.
The session running this skill is therefore itself the strand named `claude`, and the panes loom spawns while this loop runs are its siblings inside that same reed session — not a second, independent layer this skill needs to ask the operator to open.

Run this self-check once, before the first step, when driving `loom`, to confirm that expectation holds for the session actually running: read `$TMUX_PANE` from the environment, then run `lyx reed status` and compare that pane id against the tracked strands the envelope reports.
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

Before the first step, take one `lyx shed status [<run-id>]` read and record its `current_producer` and `history_length` as the baseline.
It exists for one reason: the interrupted-step branch later in this loop compares against it.
The very first step of a session — or the first step after an operator re-invokes past the iteration cap — has no prior step envelope to compare with, so the baseline is what makes that comparison possible.

On a run that has never been bootstrapped there is no status file yet, and what this read returns depends on the recipe.
For `loom`, it is an error envelope naming `lyx loom start` as the remedy, and that is not a failure to report: record an empty baseline and proceed to the first step, which bootstraps the run itself.
A different recipe's absent-file disposition may not be an error at all — read whatever remedy, if any, that recipe's own envelope names, and treat an absent-file read that names no error as an empty baseline too.
Treat any *other* status error as a hand-back, the same as an error envelope from a step.

## How to invoke a step

Never invoke a step as a blocking foreground shell call.
Launch `lyx shed step [<run-id>]` in the background with stdout redirected to a per-step file under `.scratch/ly-drive/step-<n>.json`, then wait for that process to exit and read the envelope from the file.

This matters for a concrete reason: a single step blocks for a whole producer call, and every agent shell tool caps a foreground call in the single-digit minutes.
For `loom`, the LLM rows are minutes-to-an-hour agent spawns, so a foreground call would be killed mid-producer on exactly the rows this loop exists to watch; a different recipe's own producers may run faster or slower, but the underlying blocking-call risk is the same regardless of which recipe is driven.
Redirecting to a file also keeps each envelope out of the transcript until this loop chooses to read it, which matters across a long run.
`.scratch/` is relative to the same cwd, is already gitignored repo-wide, and is the mandated scratch location — never the OS temp directory.

## The loop

Run a hard cap of **40 steps**.
Each iteration: invoke a step, read its envelope, read the artifact its `output` field names when that field is non-empty, and continue only while the envelope's `continue` field is true.

40 is a typical-run margin for `loom`, not a mechanical ceiling that holds for every recipe — loom's own ceiling is itself far higher than 40.
Loom's list is fourteen rows, a review round inside a segment costs two steps, and a run taking three review rounds in each of three segments walks thirty-five steps, rounded up to 40.
The six review rows carry their own budget of five bounces and the three validator rows bounce at the inherited default of ten, which puts loom's own worst case near a hundred steps — so treat 40 as a sane default to stop and check in at when driving loom, not proof that anything is wrong past it.
A recipe with a graph shaped differently from loom's fourteen rows would need this cap re-derived from its own arithmetic rather than inheriting loom's; no other recipe's graph is worked out here.

On reaching the cap, stop and report to the operator rather than failing anything.
The driven recipe's own state is untouched by the cap; the operator re-invokes this skill to continue from where the loop left off.

## Stopping

On any envelope whose `continue` field is false, and on any error envelope, stop looping and hand back to the operator with a report.
Never clear `state`, never edit the status file, never re-seed, never push.

The reason: a non-running state means a Go gate concluded a human is needed, and this loop deciding otherwise would be exactly the judgment-override the whole design avoids.

## Error envelopes

An error envelope carries a `kind` field with exactly one of five values, when the invoked recipe supports `step` at all.

On `kind: producer`, re-invoke the step **once**.
If the retry also errors, stop and report.

On every other of the five kinds, hand straight back to the operator with no retry.
None of the other kinds can be fixed by running the same command again, and retrying would read to the operator as though this loop were trying something it has no basis to try.
Name the remedy the envelope itself gives for the `busy` kind: the operator has a driver running and must pause it before this loop can proceed.

A recipe with no `step` verb at all is refused before any of the five kinds is ever reached.
`loom` and `batten` are the two recipes shipped today whose table entries include `step`; any future recipe absent from that entry is refused by `shed`'s own pre-run the same bare-envelope way.
A run-id that has never been seeded is refused earlier still, before any recipe is even resolved: the envelope names the addressed run-id and lists every seeded run-id found, and carries no `kind` field either — like the excluded-verb refusal, it sits outside the five-kind vocabulary above and is not a retryable condition.
On an error envelope with no `kind` field, hand it straight back to the operator with no retry, naming the refusal text verbatim.
Do not widen the five-kind vocabulary to explain it: that set is closed by a test and by the Shed Verb-Set Invariant, and this refusal is raised above the verb body that owns those kinds, not inside it.

State the hard limit on cleanup plainly: this loop never kills reed panes, removes strands, deletes lock files, touches git, or edits any task artifact.
It reports debris; the operator resolves it.

## Interrupted invocations

An invocation that was killed, timed out, or exited without writing a parseable envelope is a distinct case from an error envelope, and it writes no envelope of its own.

On detecting one, read `lyx shed status [<run-id>]` once and branch on that single read:

- If `current_producer` or `history_length` differs from the baseline, or `state` is no longer running, the producer finished and only the invocation died.
  Continue from the fresh status and do not count the step twice.
- If nothing differs, `state` is still running, and the status envelope's `interrupt_policy` field is `reinvoke`, re-invoke the step for the same row.
  For `loom`, this is its own designed crash-resume, not a retry of something unknown.
- If nothing differs and the policy is `handback`, or the envelope carries no `interrupt_policy` field at all (empty or absent), stop and hand back, saying plainly that a live agent may still be running in its pane, and that a later re-invocation would restart it rather than attach to it.
  Treating an absent policy as `handback` is the conservative arm — it is the one that never restarts in-flight work sight unseen, which is what a recipe with no interrupt-policy table of its own needs, since its status envelope carries no `interrupt_policy` field at all rather than a present-and-empty one.

Read the policy off `lyx shed status [<run-id>]`, not off the previous step's envelope, even though the previous envelope's `next_interrupt_policy` names the same row and the same table.
The status read is a fresh fact taken after the interruption, and it is the only one available on the very first step of a session, where no previous envelope exists.
The two agreeing is the point, not a redundancy to optimise away: if they ever disagree, the status file is authoritative and something is wrong worth handing back over.

For `loom`, nothing in the engine enforces `handback`.
The policy is advisory metadata for this loop to act on — re-invoking a `handback` row is not refused, not warned about on the envelope, and not blocked in any way; for loom specifically, it kills the in-flight agent and restarts that row's work from its own persisted state.
This loop is the only thing standing between an automated caller and that restart, which is why the branch above stops rather than deciding for the operator.
A different recipe's own producers need not share this exact restart behavior; nothing here claims they do.

Cap the re-invoking branch at **two consecutive** interrupted-and-re-invoked steps against the same row.
On a third, stop and hand back — something is wrong with the invocation mechanism itself rather than with the run.

Outside the handback branch, print no orphaned-agent warning for `loom`, because there is no orphan there: every one of loom's spawning rows' adapters probes for a live agent and waits on it, so the next step attaches rather than abandoning.
A different recipe's own producers need not share this adapter behavior, and this loop makes no claim about them.

## Self-report

This whole section describes `loom`'s own friction machinery.
A different recipe carries no analogous friction directory recorded here today, so treat every claim below as loom-specific until a second recipe's own shape is documented.

Nothing files automatically while this loop is driving `loom`.
Loom's two automatic self-report tiers both hang off `lyx loom run`'s own run, and `lyx shed step` against a loom-seeded run-id runs neither — so on a supervised task, a blocked halt, a producer failure, and a friction note left behind all pass unreported unless this skill reports them.
That is the trade this design makes: a live supervisor with an operator in the loop instead of a primitive filing public issues on its own, forty times a run.
Read it as a responsibility, not a gap.

Loom's friction notes live at `.lyx/loom/friction/` under the task worktree root — a fixed location, not phase knowledge.
Whenever the loop stops — terminal state, blocked, hand-back, or iteration cap — list that directory and name any notes found in the stop report, because nothing else will: `step` never spawns the reflection pass that `run` runs, and the next task's own first seed clears the directory, so a note left unread here is dropped silently.
Reading the notes and judging whether one is worth a `lyx selfreport create` call is part of the same operator-gated flow below.

This skill may call `lyx selfreport create` only after the loop has stopped — terminal state, blocked, hand-back, or iteration cap — never between steps, and at most once per supervised run, when driving `loom`.
Draft the title and body, show both to the operator, and fire the call only on explicit operator approval.
The body goes in via `-b -` on stdin and states the row's policy-relevant facts, the envelope fields that were surprising, and the artifact path this loop read.
The default `bug` label stands for a defect; `--label enhancement` is for friction that is not a defect.

Record why the gate exists: the verb files a real public issue through the GitHub API, an outward-facing and hard-to-reverse act.

## Operator choices

Any point where this skill offers the operator a choice must present it as a numbered text list, one option per line in the form `1) Label — description`, never a mouse-driven prompt.

## Autonomous driver

Enter this section instead of the operator-driven posture above when the launch prompt says this session runs autonomously.
Exactly four things change; everything else in this skill — every stop condition, every retry rule, every cleanup limit — stays exactly as written above.

**No operator choices.**
There is no operator in the session to answer a question, so every numbered-list prompt this skill would otherwise raise — including the pane self-check's tracked-absent branch and the `## Operator choices` section itself — becomes a line in the stop report instead of a question.
Decide nothing on the operator's behalf that the prose above reserves for the operator; record the choice that would have been offered, and the reasoning available for it, and move on to stopping and reporting.

**The report goes to the file.**
Every place the skill above says to report to the operator or hand back with a report, the autonomous path instead writes that report to the output-file path named in its own launch prompt, and then stops.
Nothing here changes what belongs in the report — only where it goes and that no further action follows it in this session.

**The step cap is a budget, not a check-in.**

autonomous step cap: 120

This section's own loop runs a hard cap of the number above rather than the operator-driven cap of 40 stated in `## The loop`.
The number is derived, not a bare round figure: `## The loop` already works out that loom's own worst case — three review rounds in each of three segments, plus the review rows' own bounce budget — lands near a hundred steps, and 120 keeps a margin over that worst case rather than cutting an unlucky-but-legitimate run off mid-task.
The operator-driven cap of 40 exists so a human can look in on a run in progress; with no human to look in on it, that same cap would stop a healthy autonomous run three times over before it ever finished, for no benefit anyone would see.
On exhausting the autonomous budget, the driver writes the report and stops, leaving the driven recipe's own state exactly as it is — the same as the operator-driven cap's own disposition in `## The loop`, just with nobody to re-invoke the loop afterward.

**Stop conditions are otherwise unchanged and remain absolute.**
A halting envelope stops the loop exactly as `## Stopping` describes.
An error envelope stops with the single retry `## Error envelopes` already allows on `kind: producer`, and with no retry on every other kind.
The skill still never clears `state`, never edits the status file, never re-seeds, never pushes, never kills a reed pane, and never touches git — autonomy changes only who reads the output and what happens when there is nobody to ask, never what this skill is willing to do to the run or the repo.
