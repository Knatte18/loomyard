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
// PATTERN-shed-driver holds the rule, and `seam_enforcement_test.go` enforces the import bound.
//
// # The loop
//
// `Run` steps the run through `Deps.Step` and `Deps.Goto` and decides each stop's envelope.
// Before it acts on a stop it re-reads the status.
// A run another process is running, or whose run lock is held, ends the driver.
// A step that yielded ends the driver with `ExitYield`, nothing decided and the run left as it was.
// A pause covering the run holds the driver before it spawns or re-steps anything.
// Only a failed hand-back or a cancelled context is returned as an error.
// `DriveVerb` holds the run's driver lock for the driver's life and refuses a second driver at once.
//
// # The rule table
//
// The first row that matches a stop decides it:
//
//   - a finished run: done;
//   - a step that reported another holder of the run: exit and leave the run to the holder;
//   - an ownership error, or an error with no kind: hand back;
//   - a paused run: hold, and hand back;
//   - a blocked stop the loop's envelope marks `hand_back`, or the told `Deps.HandsBack` check marks: hand back;
//   - a blocked stop whose reason starts with the bounce-budget-exhausted or the precondition-unmet prefix: hand back;
//   - an awaiting stop the arbiter would admit: the arbiter decides;
//   - an awaiting stop with a parent notice: relay the notice and hand back;
//   - any other awaiting stop: hand back;
//   - a stop the on-disk build could change, after the last step ran another build:
//     hand over to a fresh driver of the on-disk build, once per build;
//   - a stop an applied tackler resume returned to: the arbiter decides;
//   - a blocked stop the told `ArbiterSeam.JudgesFirst` marks: the arbiter decides, with no tackler call;
//   - a git transport or GitHub API failure: wait for the remote, then re-step, with no cap on the re-steps;
//   - any other transient error that outlived its re-step: the tackler decides;
//   - an interrupted step under the re-invoke policy, below the repair cap: re-step;
//   - an interrupted step under the hand-back policy: hand back;
//   - a producer, bootstrap or unseeded error, an interrupted step over the repair cap, or a blocked stop:
//     the tackler decides;
//   - anything else: hand back.
//
// Only `git-transport` and `github-api` stops are re-stepped with waits, for as long as they stay network-class;
// every other class goes to the tackler.
// The waits start at 15 s, double with each re-step of the stop and stop growing at `Deps.NetworkWaitCeiling`.
// While the driver waits one out it keeps a `DriveWait` record at `Deps.WaitRecordPath`.
// The landing-halt, budget-exhausted and precondition-unmet rows only hand back; they never resume a run.
// A recipe's told `GotoGuard` rejects a tackler verdict whose goto it refuses or cannot decide.
//
// The settle records at `shedrun.SettleRecords` hold one line per action taken at a stop.
// The repair cap, the one hand-over per build and the open tackler attempt are counted from them,
// so a restarted driver keeps every cap.
//
// # Seams a recipe fills
//
// The step, status, run-lock, envelope, report, pause, hand-back, tackler, open-attempts and clock seams are required.
// The rest are optional, and an unwired one only removes rows or agents from the table, `Wait` excepted:
// `Arbiter`, `Arbiter.JudgesFirst`, `HandOver`, `Tamper`, `StartCheck`, `HandsBack` and `Watched`;
// also `CommitRecords` and `WriteFrictionNote`.
// An unwired `Wait` removes no row: the driver then waits on the real clock before a re-step.
// A stop a removed row would have caught meets the later rows unchanged.
// No unwired seam adds a resume path: a tackler escalation with no arbiter hands back.
//
// # The hand-back
//
// A hand-back writes a markdown stop report, sends the parent its notices and parks the run.
// No notice is queued when no orch strand is recorded.
// While a batten watches the run, the batten announces a plain stop.
// The driver still names a blocked stop the tackler or the arbiter was asked to judge.
// Batten still sends its own notice once the run parks, so the parent hears both at such a stop.
// A done run that reported friction `failed` is reported but never parked.
//
// # The tackler
//
// The tackler is a cold agent, spawned through shuttle for one stop from the shed stencil `shed-template-tackler`.
// Each spawn is counted against the spawn budget before the agent starts.
// Its verdict is `resume`, `hand-back` or `escalate`; a resume is checked against the status and the goto rules.
// The stop is handed back with the cause when the agent timed out, died, or left no verdict or an unreadable one.
// A tackler switched off, or a spawn budget spent, hands the stop back without a spawn.
// A recipe that sets `TacklerDeps.Snapshot` has the driver refuse a verdict over a tampered tree.
//
// # The shed config module
//
// `shed.yaml` holds the defaults every shed-driven recipe inherits.
// `batten.yaml`'s keys of the same names are an `Override` of them,
// and a loom recipe's own values come through its profile block's `shed` object.
// In an `Override` an empty string, a zero and a null inherit.
// A set `shed.tackler_max_spawns` or `shed.arbiter_max_spawns`, zero included, wins, since zero turns that role off.
// `CheckOverride` and `CheckLocalOverride` refuse a bad value in any layer.
package sheddriver
