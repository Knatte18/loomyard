// Package sheddriver is the shed layer's Go driver and its tackler.
// It steps a run to each stop and decides the stop by a closed rule table.
// It asks a cold tackler agent about a stop no rule settles.
// It hands back to the run's parent everything it cannot settle.
// Tapestry, darn and sampler mount it through `lym loom drive`.
// Batten's `lym batten run` runs it in-process over its own run.
//
// # Told geometry
//
// The package is a told-geometry engine: it is handed every path it works on,
// and it reaches recipe behaviour only through `Deps` and `TacklerDeps`.
// It imports no cwd resolver, no loom engine or shed, and no CLI package, and its other imports are a closed allowlist.
// PATTERN-shed-driver holds the rule, and `seam_enforcement_test.go` enforces the import bound.
//
// # The loop
//
// `Run` steps the run through `Deps.Step` and `Deps.Goto`.
// The arming module points them at `shedverbs`' `--until-stop` and goto bodies, so the driver restates neither.
// Each step prints one stop envelope;
// the driver saves each envelope it decides under the run's drive-reports directory.
// Before it acts on a stop it re-reads the status.
// A run another process is running, or whose run lock is held, ends the driver,
// and a run that moved to another stop is decided afresh from its status.
// A driver that starts at a blocked stop with no open agent attempt first asks `Deps.StartCheck` about the stop's row.
// It asks once, before its first step, and a refusal hands the run back quoting it with nothing stepped.
// A step that yielded ends the driver with `ExitYield` and nothing decided.
// A yield is an envelope that reports outcome `yield` with the run still running.
// No saved envelope, rule row, settle record, report, repair, bounce or tackler budget applies,
// and the run is left as it was.
// A pause covering the run holds the driver before it spawns or re-steps anything.
// A failure the driver cannot act on is handed back.
// Only a failed hand-back or a cancelled context is returned as an error.
// `DriveVerb` holds the run's driver lock for the driver's life and removes the stale park marker and resume report.
// It refuses a second driver for the run at once.
//
// # The rule table
//
// The first row that matches a stop decides it, and no row matches nothing:
//
//   - a finished run: done;
//   - a step that reported another holder of the run: exit and leave the run to the holder;
//   - an ownership error, or an error with no kind: hand back;
//   - a paused run: hold, and hand back;
//     the hand-back asks for the friction note when neither the envelope nor the status records a reason;
//   - a blocked stop the loop's envelope marks `hand_back`, or the told `Deps.HandsBack` check marks:
//     hand back, ahead of every budget, precondition, hand-over, arbiter and tackler row;
//     only the orch's resume lifts a landing halt;
//   - a blocked stop whose reason (a status-built stop's error) starts with the bounce-budget-exhausted prefix:
//     hand back, ahead of every hand-over, arbiter and tackler row;
//     only the parent's resume lifts it;
//   - a blocked stop whose reason (a status-built stop's error) starts with the precondition-unmet prefix:
//     hand back, ahead of every hand-over, arbiter and tackler row;
//     no tackler settles it;
//   - an awaiting stop the arbiter would admit: the arbiter decides;
//   - an awaiting stop with a parent notice: relay the notice and hand back;
//   - any other awaiting stop: hand back;
//   - a stop the on-disk build could change, after the last step ran another build:
//     hand over to a fresh driver of the on-disk build, once per build;
//   - a stop an applied tackler resume returned to: the arbiter decides;
//   - a blocked stop the told `ArbiterSeam.JudgesFirst` marks for its row and reason (a status-built stop's error):
//     the arbiter decides, with no tackler call;
//   - a transient error that outlived its re-step: the tackler decides;
//   - an interrupted step under the re-invoke policy, below the repair cap: re-step;
//   - an interrupted step under the hand-back policy: hand back;
//   - a producer, bootstrap or unseeded error, an interrupted step over the repair cap, or a blocked stop:
//     the tackler decides;
//   - anything else: hand back.
//
// A stop built from a paused status takes the status's `pause_reason` as its reason.
// A stop whose loop envelope leaves the run paused with no reason of its own takes the loop object's `pause_reason`.
// A stop report for a paused stop with none says `paused with no recorded reason`.
// The landing-halt, budget-exhausted and precondition-unmet rows only hand back; they never resume a run.
// A recipe's told `GotoGuard` reaches the tackler through `TacklerDeps`:
// a verdict whose goto the guard refuses, or cannot decide, is rejected as an invalid resume with the guard's text.
//
// The settle records at `shedrun.SettleRecords` hold one line per action taken at a stop.
// The record format and its tolerant decoder are declared in `internal/shedrun`;
// this package appends and reads the file, and `SettleRecord` and `StopKey` are aliases of the types declared there.
// The repair cap, the one hand-over per build and the open tackler attempt are counted from them,
// so a restarted driver keeps every cap.
// A recipe's told `CompletedWake` restarts the repair count of a stop key.
// The count is zero again when the row's completed-wake stamp is `wakeRestartGap` or more after the key's last re-step.
// A row that waits long, as a pawl run's Beam-Await does, is then re-stepped after each loop that lived through a wake.
// A loop that dies soon after each re-step still reaches the tackler.
// A younger stamp, an absent one and a failed read leave the count as the records give it,
// and a nil `CompletedWake` reads none.
// `RestepRecorded` reads a record list for a re-step of the stop a status names,
// so a reader outside the driver tells a re-stepped stop from one still open.
//
// # Seams a recipe fills
//
// The required seams are the step and goto bodies, the status and run-lock reads, and the envelope and report paths.
// They also include the pause read and hold, the hand-back, and the tackler and its consume.
// The open-attempts read and the clock are required too.
// The rest are optional, and an unwired one only removes rows or agents from the table:
//
//   - no `Arbiter`: an escalation to it hands back, relaying the parent notice at an awaiting stop;
//   - no `Arbiter.JudgesFirst`: every blocked stop meets the tackler before the arbiter;
//   - no `HandOver`: the driver reads no build and never hands over;
//   - no `Tamper`: a tackler verdict is acted on without comparing the worktree with its snapshot;
//   - no `StartCheck`: a driver starting at a blocked stop steps it unchecked;
//   - no `HandsBack`: a stop built from a status is never marked, so only the loop's envelope marks a landing halt;
//   - no `Watched`: the run is never watched;
//   - no `CommitRecords`: a done stop commits nothing;
//   - no `WriteFrictionNote`: a paused stop with no reason writes none.
//
// A stop a removed row would have caught meets the later rows unchanged,
// so it ends in the tackler under its spawn budget, a re-step under the repair cap, or a hand-back.
// No unwired seam adds a resume path: a tackler escalation with no arbiter hands back, naming the tackler's case brief.
// An arbiter `resume` at an awaiting stop sends the parent one outcome line naming the run and the verdict file.
// The line goes through `HandBackDeps.Notify`.
// It is sent when an orch strand is recorded and once the step has consumed the resume.
// The resume asks nothing and parks nothing, and a resume the step refuses hands back with no outcome line.
// Loom's, darn's and batten's drivers all wire the arbiter after the tackler.
// Batten's wiring uses `batten.yaml`'s arbiter overrides of `shed.yaml`.
// The loom CLI's wiring, shared by tapestry, darn and sampler, marks these blocked stops for the arbiter to judge first.
// One is the Webster row, whose blocked stop is Merriam's stuck outcome.
// The other is a tapestry, darn or sampler landing escalate, a merge fixer's halt that opens with the escalate prefix.
//
// # The hand-back
//
// A hand-back writes a stop report, sends the parent its notices and parks the run.
// The report is markdown.
// It records when it was written, the position, the reason, and the friction and repair directories.
// It also records the rules applied, the agents' attempts, and the question for the parent when there is one.
// It lists one line per notice sent, declined, failed or left unqueued.
// The notices are the parent notice to relay, a question, and a done run that reported friction `failed`.
// A run no batten watches also gets one line naming the run and its report.
// `HandBackDeps.Notify` queues a line and may decline it at a stop the recipe sends no notice for,
// and no notice is queued when no orch strand is recorded.
// `HandBackDeps.Park` records the report as the run's park;
// a done run that reported friction `failed` is reported but never parked.
//
// # The tackler
//
// The tackler is a cold agent, spawned through shuttle for one stop.
// Its prompt is filled from the shed stencil `shed-template-tackler`.
// The stencil is read at call time from the told stencils directory.
// `TacklerDeps.Recipe` is the one-line description of the recipe that the prompt names.
// `GotoRoutedFrom` says whether a resume may also name the row the halting history entry was routed from.
// Each spawn is an attempt directory holding its attempt record, prompt, verdict, case brief and envelope,
// and the attempt is counted against the spawn budget before the agent starts.
// A call first reuses the newest attempt for the stop key that was not yet handed to the parent:
// it waits on a live agent up to the attempt's deadline, and concludes from a verdict already written.
// The verdict is `resume`, `hand-back` or `escalate`.
// A resume is checked against the status and the goto rules before it is applied.
// The stop is handed back with the cause when the agent timed out, died, or left no verdict or an unreadable one.
// It is also handed back when the verdict names a precondition the run does not meet.
// Shuttle re-sends a turn the provider's API ends on an error `SettlerAPIErrorRetries` times.
// The error past them is a hand-back with cause `api-error`.
// So is a model the provider rejects, or a start whose skill-load hold reached its cap of API errors.
// Its question names the error, so the parent hears of the stop at once rather than at the deadline.
// A tackler switched off, or a spawn budget spent, hands the stop back without a spawn.
// `TacklerDeps.Snapshot` is optional.
// A recipe that sets it has the driver compare the worktree with the snapshot taken at spawn.
// The driver refuses a verdict over a tampered tree.
//
// # The shed config module
//
// `shed.yaml` holds the defaults every shed-driven recipe inherits.
// Its keys are `driver_kind`, `tackler`, `tackler_timeout_min` and `tackler_max_spawns`.
// The arbiter keys are `arbiter`, `arbiter_timeout_min` and `arbiter_max_spawns`.
// `batten.yaml`'s keys of the same names are an `Override` of them,
// and a loom recipe's own values come through its profile block's `shed` object.
// In an `Override` an empty string, a zero and a null inherit.
// A set `tackler_max_spawns` or `arbiter_max_spawns`, zero included, wins, since zero turns that role off.
// `ResolveDriverKind`, `ResolveTackler` and `ResolveArbiter` take each key from the override.
// Failing that they take it from `shed.yaml`, and failing that from the default.
// The defaults are the `llm` driver, the `haiku` and `opus[high]` model-specs, and a ten and a thirty minute timeout.
// `ResolveTackler` and `ResolveArbiter` return `TacklerSettings` and `ArbiterSettings`.
// Each carries a model choice, engine included, beside the timeout and the spawn budget.
// `ArbiterLimits` returns the arbiter's timeout and spawn budget alone.
// Batten's tackler and arbiter keys sit in `batten.yaml`.
// `CheckOverride` and `CheckLocalOverride` refuse a bad value in any layer.
// The refusal names the module, the key and the way to set it.
package sheddriver
