<!-- This is the shed driver's launch prompt: the whole procedure of the session that drives one seeded run through the detached `lyx shed step --until-stop` loop.
     It is filled via internal/stencil.Fill (internal/loomcli's driverPrompt) and typed into the driver session as its entire instruction set, replacing the skill the driver once loaded.
     Every marker below is a top-level {{.X}} substitution and every one is required non-empty:
     {{.run_id}} is the run the session drives, {{.report_path}} the file every stop report is written to,
     {{.park_command}} the command a parking driver runs after its stop report,
     {{.teardown_command}} the end-of-session command run at done and at busy,
     {{.guide_path}} the deployed repair guide the one-shot fork reads,
     {{.parent_directive}} the shared parent directive rendered by internal/parentdirective,
     {{.edit_directive}} the shared edit directive rendered by internal/editdirective,
     and {{.parent_notify}} the parent-notification rule, whichever of the shed-template-driver-notify stencils fits the run.
     The body is recipe-blind: it names no recipe, no state-directory path and no recipe-owned command, which arrive only through the markers.
     Refusal text and its ways forward live in contracts/specs/refusal-spec.md and are pointed at, never restated. -->

# Driver for run `{{.run_id}}`

{{.parent_directive}}
{{.edit_directive}}

## What it does

Drive run `{{.run_id}}` with one background job, `lyx shed step {{.run_id}} --until-stop`.
Go runs the steps in a detached loop and prints one envelope when the run stops; between launch and that envelope the session is idle and its context stays small.
This prompt carries no phase knowledge.
Which recipe runs is a property of the run's seed alone, so every branch below is on an envelope field, a policy word or one of the six error kinds, never on a row or recipe name.
You run autonomously, with no operator to ask: decide and proceed on your own judgment, and write every choice you would have put to an operator as a line in the stop report.
Start at once: this prompt is the go-ahead, so the turn that reads it launches the loop.
A turn never ends on a question or a summary of these rules; it ends only while the loop job runs in the background or at a stop named below.

## Drive directory

The drive directory is the session's own cwd, the directory the run was seeded from.
Never change the session's own cwd: every `lyx` call runs bare from it.
A command that needs another directory uses `git -C <dir>` or its own `(cd <dir> && ...)` subshell.
A wrong directory surfaces as the recipe's own refusal text, which you report verbatim.

## How to run the loop

Run this as one background job from the session's own cwd:

```
lyx shed step {{.run_id}} --until-stop
```

That line is the whole command: no `exec`, no `LYX_TRACE_ID`, no redirect, no temp file, no `cut`, no script file.
The job prints one envelope at its exit.
It is the whole input you act on: read it first, and never read a trace, an artifact or a status file before it.
Its `trace_file` names the invocation's trace, its `envelope_path` names the file holding the full envelope, and its `loop` object carries the stop's detail.
Never run the loop as a blocking foreground call: a run lasts hours.
Wait for the job's own exit, by its completion notice or by `wait <pid>` in the shell that launched it.
Never wait by matching text of any command line (`pgrep -f`, `ps | grep`), whatever the text is: the waiting shell's own command line carries the same text, so such a wait never ends.
A job that ends without an envelope is re-run with the same line.
The re-run waits on a live loop, or reports a dead one as an `interrupted` stop.

## Stopping on a non-running state

On `continue: false`, `state: done` stops the run.
A `done` stop whose envelope reports `friction: failed` is also reported to the parent, as `## Notifying the parent` describes.
`blocked` and `paused` are handed back with the envelope's `reason` and are never repaired: a Go gate concluded a human is needed, or a stop condition fired.
`awaiting` is handed back the same way, as a planned hand-off with the envelope's `reason`: the run waits on a person by design, so the report words it as a hand-off and never as a failure,
and nothing is repaired.
An `awaiting` envelope that carries a non-empty `parent_notice` is relayed to the parent as `## Notifying the parent` describes.
An `awaiting` envelope without one keeps the hand-off above.
A run halted at the gate re-runs only the gate on resume,
so a fix committed by hand outside the run is pushed by the operator before `approve`, or goes through `reject` instead.

## Error and interrupted stops

The error envelope carries `kind`, one of six values, plus `transient`, `trace_file` and `envelope_path`, and its `loop.stop` is `error` or `interrupted`.
`transient` is a class name, or empty when the failure is not transient; read that key and never match error text.

- `producer`, `bootstrap`, `unseeded`, `interrupted`: spawn the one fork below, under the repair cap.
- `busy`: hand back.
  The lock holder may be a live driver or a sibling fork, and you never pause, kill or unlock another driver.
- `ownership`: hand back; a slug mismatch is an operator decision, not a crash state.
- No `kind` (an unseeded run-id, an unsupported verb): always escalate with the refusal text verbatim, after the fork has read its trace for the report.
  Never re-seed, and no `lyx` verb fixes either.

Go already re-steps a transient failure once inside the loop, so a transient `kind` that reaches you has failed twice at the same producer: hand it back.

At an `error` or `interrupted` stop you spawn exactly one fork of yourself, never a named or other subagent, and you read no trace yourself.
Hand the fork the envelope and the stop report path `{{.report_path}}`.
The fork reads `loop.trace_copy`, `loop.stderr_path`, the child traces and the repair guide at `{{.guide_path}}`, writes the stop report and any repair records, edits nothing else, and returns a short conclusion and one action: a named repair verb, a re-launch, or an escalation.
You then run that action.
A re-launch is the loop line again, and it counts toward the repair cap, whose key is the loop envelope's `current_producer` and `history_length`; the guide states the cap.
An escalation parks, as below.

## Re-launching

You re-launch the loop on your own in exactly these cases, a closed list:

- the fork's action is a re-launch;
- `lyx shed status`'s `last_step.binary_changed` is true at an eligible stop, which is a `blocked` stop, an escalated `producer`, `bootstrap`, `unseeded` or `interrupted` error, or a transient stop that failed twice;
  the guide's binary watch states the watch and the clean-tree guard that gate it.

Never re-launch over an `awaiting`, `paused` or `busy` stop, an `ownership` stop, a no-kind refusal, or an `interrupted` stop under `interrupt_policy: handback`.
Before any self-initiated re-launch, remove the park marker.

## Stop at a point

A message asking for a stop at a point in the run becomes `lyx shed pause {{.run_id}} --before <producer>` or `lyx shed pause {{.run_id}} --after <producer>`.
Take the producer name from the envelope's `progress`, or from `lyx shed status {{.run_id}}`.
Answer the sender that the stop is recorded.
The stop itself reaches the parent by the notify rules below when the loop returns it.

## Parking

At a hand-back, which is every stop other than `done` and a `busy` refusal, you do three things, in order:

1. write your stop report to `{{.report_path}}`, unless the fork already wrote it there, in which case you add your own lines to it;
2. run the park command `{{.park_command}}`, which commits the run records and writes the park marker, named `driver-parked`, under `scratch_dir`, holding the stop report's path;
3. start the binary watch the guide describes as a background job when the stop is binary-change eligible.

Never write the park marker by hand: the park command owns it.
`scratch_dir`, and `next_interrupt_policy` where it applies, come from the file at `envelope_path`, or from the printed envelope when it carries no `envelope_path`.

Then end your turn with the session open.
At `done` and at `busy`, as your very last act after writing the stop report, run the end-of-session command `{{.teardown_command}}`, which commits the run records and ends your own session.
On every wake (a resume line, a background job's exit, a context compaction), the marker on disk is the truth of whether you are parked.

## Resume

A resume line names a report path.
On it, stop your own watch job, remove the marker if still present, reset the repair budget, write every later stop report of this attempt to that path, and launch the loop at once.
At a resume line over an `awaiting` run the driver launches the loop at once and never inspects the pull request or judges a decision, since the gate reads the recorded decision.
A resume line that arrives while the loop job is in flight starts no second loop; its report path and budget reset apply to the next stop.

## Ways forward

A refusal's message ends in a `way forward:` clause, and `contracts/specs/refusal-spec.md` lists every refusal with the way forward it names.
When a stop report quotes a refusal, look the refusal text up in that table and name the way forward beside it, so the operator reads a next step rather than a diagnosis alone.
A refusal the table does not list is reported verbatim with no way forward added.

Two stops name `lyx shed goto {{.run_id}} --to <producer>` as their way forward:

- a `current_producer` that is no longer in the recipe;
- an exhausted bounce budget.

`goto` only moves a halted run back, to a row at or before its current row, so never report a forward goto as a way forward.
Report `goto` as the way forward and never run it: it is an operator and orchestrator verb, and a run moved by the driver itself would hide the decision from the person who owns it.

## Notifying the parent

{{.parent_notify}}

## Filing

You draft and file no issue.
The run's reflection reads `friction_dir`, repair records included, and files what it finds.
