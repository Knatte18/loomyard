// Package sheddriver is the shed layer's Go driver and its tackler.
// It steps a run to each stop, decides the stop by a closed rule table, asks a cold tackler agent about a stop no rule settles, and hands back to the run's parent everything it cannot settle.
// Loom and darn mount it through `lyx loom drive`, and batten's `lyx batten run` runs it in-process over its own run.
//
// # Told geometry
//
// The package is a told-geometry engine: it is handed every path it works on, and reaches recipe behaviour only through `Deps` and `TacklerDeps`.
// It imports no cwd resolver, no loom engine or shed, and no CLI package, and its other imports are a closed allowlist.
// PATTERN-shed-driver holds the rule, and `seam_enforcement_test.go` enforces the import bound.
//
// # The loop
//
// `Run` steps the run through `Deps.Step` and `Deps.Goto`, which the arming module points at `shedverbs`' `--until-stop` and goto bodies, so the driver restates neither.
// Each step prints one stop envelope; the driver saves it under the run's drive-reports directory and decides it.
// Before it acts on a stop it re-reads the status: a run another process is running, or whose run lock is held, ends the driver, and a run that moved to another stop is decided afresh from its status.
// A pause covering the run holds the driver before it spawns or re-steps anything.
// A failure the driver cannot act on is handed back; only a failed hand-back or a cancelled context is returned as an error.
// `DriveVerb` holds the run's driver lock for the driver's life, removes the stale park marker and resume report, and refuses a second driver for the run at once.
//
// # The rule table
//
// The first row that matches a stop decides it, and no row matches nothing:
//
//   - a finished run: done;
//   - a step that reported another holder of the run: exit and leave the run to the holder;
//   - an ownership error, or an error with no kind: hand back;
//   - a paused run: hold, and hand back;
//   - an awaiting stop the arbiter would admit: the arbiter decides;
//   - an awaiting stop with a parent notice: relay the notice and hand back;
//   - any other awaiting stop: hand back;
//   - a blocked stop matching the verify halt record: hand back under a watching batten, otherwise one goto to the record's target and a re-step, and hand back once that retry is spent;
//   - a stop the on-disk build could change, after the last step ran another build: hand over to a fresh driver of the on-disk build, once per build;
//   - a stop an applied tackler resume returned to: the arbiter decides;
//   - a transient error that outlived its re-step: the tackler decides;
//   - an interrupted step under the re-invoke policy, below the repair cap: re-step;
//   - an interrupted step under the hand-back policy: hand back;
//   - a producer, bootstrap or unseeded error, an interrupted step over the repair cap, or a blocked stop: the tackler decides;
//   - anything else: hand back.
//
// The settle records at `shedrun.SettleRecords` hold one line per action taken at a stop.
// The record format and its tolerant decoder are declared in `internal/shedrun`; this package appends and reads the file, and `SettleRecord` and `StopKey` are aliases of the types declared there.
// The repair cap, the one verify retry, the one hand-over per build and the open tackler attempt are counted from them, so a restarted driver keeps every cap.
//
// # Seams a recipe fills
//
// The required seams are the step and goto bodies, the status and run-lock reads, the saved envelope and report paths, the pause read and hold, the hand-back, the tackler and its consume, the open-attempts read and the clock.
// The rest are optional, and an unwired one only removes rows or agents from the table:
//
//   - no `Arbiter`: an escalation to it hands back, relaying the parent notice at an awaiting stop;
//   - no `Verify`: no stop is a verify halt;
//   - no `HandOver`: the driver reads no build and never hands over;
//   - no `Tamper`: a tackler verdict is acted on without comparing the worktree with its snapshot;
//   - no `Watched`: the run is never watched;
//   - no `CommitRecords`: a done stop commits nothing;
//   - no `WriteFrictionNote`: a paused stop with no reason writes none.
//
// A stop a removed row would have caught meets the later rows unchanged, so it ends in the tackler under its spawn budget, a re-step under the repair cap, or a hand-back.
// No unwired seam adds a resume path: a tackler escalation with no arbiter hands back, naming the tackler's case brief.
//
// # The hand-back
//
// A hand-back writes a stop report, sends the parent its notices and parks the run.
// The report is markdown: when it was written, the position, the reason, the friction and repair directories, the rules applied, the agents' attempts, the question for the parent when there is one, and one line per notice sent, declined, failed or left unqueued.
// The notices are the parent notice to relay, a question, a done run that reported friction `failed`, and, for a run no batten watches, one line naming the run and its report.
// `HandBackDeps.Notify` queues a line and may decline it at a stop the recipe sends no notice for, and no notice is queued when no orch strand is recorded.
// `HandBackDeps.Park` records the report as the run's park; a done run that reported friction `failed` is reported but never parked.
//
// # The tackler
//
// The tackler is a cold agent, spawned through shuttle for one stop with a prompt filled from the shed stencil `shed-template-tackler`, which is read at call time from the told stencils directory.
// `TacklerDeps.Recipe` is the one-line description of the recipe that the prompt names, and `GotoRoutedFrom` says whether a resume may name the row the halting history entry was routed from as well as the current row.
// Each spawn is an attempt directory holding its attempt record, prompt, verdict, case brief and envelope, and the attempt is counted against the spawn budget before the agent starts.
// A call first reuses the newest attempt for the stop key that was not yet handed to the parent: it waits on a live agent up to the attempt's deadline, and concludes from a verdict already written.
// The verdict is `resume`, `hand-back` or `escalate`.
// A resume is checked against the status and the goto rules before it is applied, and an agent that timed out, died, left no verdict or an unreadable one, or named a precondition the run does not meet, is a hand-back with that cause.
// A tackler switched off, or a spawn budget spent, hands the stop back without a spawn.
// `TacklerDeps.Snapshot` is optional: a recipe that sets it has the driver compare the worktree with the snapshot taken at spawn and refuse a verdict over a tampered tree.
//
// # The shed config module
//
// `shed.yaml` holds the defaults every shed-driven recipe inherits: `driver_kind`, `tackler`, `tackler_timeout_min` and `tackler_max_spawns`.
// A module's own keys of the same names are an `Override` of them: an empty string, a zero and a null inherit, and a set `tackler_max_spawns`, zero included, wins, since zero turns the tackler off.
// `ResolveDriverKind` and `ResolveTackler` take each key from the override, else `shed.yaml`, else the default: the `llm` driver, the `haiku` model-spec and a ten minute timeout.
// `ResolveTackler` returns `TacklerSettings`, which carry a model choice, engine included, beside the timeout and the spawn budget.
// `loomengine.RecipeOverride` picks the module override by recipe, `darn.yaml`'s for a darn run and `loom.yaml`'s otherwise; batten's tackler keys sit in `batten.yaml`.
// `CheckOverride` and `CheckLocalOverride` refuse a bad value in any layer, naming the module, the key and the way to set it.
package sheddriver
