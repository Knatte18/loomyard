// Package shedverbs implements the generic cobra verb bodies -- run, step, status, pause, and goto -- shared by every module that arms a *shedengine.Shed onto a CLI subtree.
// goto only reads its Spec and calls shedengine.Goto, which owns the status-file mutation.
// It owns those verb bodies and nothing else: it derives no path of its own, imports no resolver, and never calls os.Getwd or internal/lymcwd -- every path this package touches reaches it told, through a Spec the arming module fills.
// It also imports no <module>cli package, which is what keeps a consuming
// module's own imports acyclic: internal/shedcli (or any other CLI module wiring shedverbs.Verbs
// onto its own subtree) can safely import shedverbs, but shedverbs never imports back.
// The one resolving import it admits is internal/logger, for boundary logging and the two
// sink-location accessors, per the Shed Verb-Set Invariant.
//
// pause with no flag requests a pause at the next producer boundary, unless the spec's BarePause hook refuses it, which loom's does.
// --before <producer> and --after <producer> record a stop condition the engine fires at that producer, and --clear removes both; none of the three touches the bare request, and --clear refuses beside the other two.
// --by <label> stores a free-text caller label beside the request it accompanies: pause_requested_by for a bare pause, pause_before_by with --before and pause_after_by with --after, each replaced with its condition and --clear dropping both condition labels.
// The label gates nothing, and the engine records it in the paused run's pause_reason;
// --by shed is refused before any write, since shed is the label of lym's own automatic steps.
// status's generic core is twelve keys, and pause_reason is the status file's value; the human view prints it as a "why" line for a paused run with a non-empty reason.
// Spec.GotoGuard is the recipe's told check of a goto target, which every goto passes on; nil admits every target.
// Spec.HandBack is the recipe's told check of a blocked stop that goes to the orch, which keeps the package recipe-generic: the loop's stop envelope and lym shed status report its answer as hand_back.
//
// step prints a short envelope by default.
// Before printing, it writes the full envelope to its per-invocation record under Spec.StepsDir, and the short envelope names that record's path as "envelope_path".
// The --full flag, or a record that could not be written, makes step print the full envelope instead, byte-identical to the record.
// The record, the exit code and the run's state are the same with or without --full.
//
// step's refusal-kind vocabulary is closed at six: busy, unseeded, ownership, bootstrap, producer and interrupted.
// A module arming a step subtree reports an arming error through ReportArmError, which prints a bootstrap error on step, with a trace file and a way forward, and a bare error line on any other verb.
// An error wrapped in KindlessRefusal, such as a missing seed, prints the bare line on step too.
//
// step --until-stop runs one child step after another, each under a fresh trace id, and prints one envelope at the first stop.
// A child's own record under Spec.StepsDir is its step's full envelope; a child that refused before its step body ran left an error envelope on stdout instead, and one that left neither is an interrupted stop.
// The invocation carrying the flag keeps no in-flight record, so last_step only ever names a child's step.
// A recipe that arms no loop (an empty Spec.Loop.EnvelopePath) refuses the flag with a bootstrap error.
//
// The stop is one of LoopStop's values.
// halted is a run that is no longer running: blocked, awaiting, done or paused by request.
// stop-condition is a pause_before or pause_after condition that fired.
// error is a step that ended in an error, and interrupted a child that ended without an envelope; both write the status file failed while it still reads running.
// An interrupted child's cause is its exit, its command line and an excerpt of its captured stderr: a Go crash's first lines when stderr holds one, and its last lines otherwise.
// The stop names the row the status file names once the child exited, since a child that skipped gated rows moved the run on before it died,
// and its trace file is the child's first trace file, or its captured stderr when it left none, or the loop process's own trace file when it left neither.
// A child start refused because the executable was open for writing, ETXTBSY, as while a deploy rewrites the lym binary in place, is tried again, five tries in all, waiting 100 ms before the first retry and doubling after through Spec.Loop.Sleep.
// A child that never started is interrupted with the cause start-failed after <n> tries: <error>, which for ETXTBSY adds what the refusal means.
// The loop process's own trace file, which Spec.Loop.LoopTraceFile names, holds the start failure of a child that never started.
// The failed write comes before Hooks.AfterInterrupt, so the hook reads the status the stop leaves: a child that died before its resume write leaves a paused run paused.
// interrupted is also the stop on the status read at an iteration's top, with the cause status-missing for an absent file and status-unreadable: <error> for an unreadable one.
// A decode error stops at once, since the file stays undecodable to this binary; any other read error gets one re-read through the same re-step the transient errors share, and stops when that fails or the re-step is already used.
// No child runs at such a stop, the trace file is the loop log, and the error names the status file and the loop log and ends with a way forward split by cause: the recipe's own reseed for an absent file, a lym binary that decodes the file for a decode error, and a re-run of the step for a read error.
// The status file is left as found, since writing it failed would destroy the evidence.
// busy is another holder of the run, or of the run lock at the failed write, which writes nothing.
// A transient error gets one immediate re-step, reported as restep.
//
// RunUntilStop and RunGoto are the exported entries an in-process driver calls: the same bodies the step --until-stop and goto verbs run after their flags are parsed.
// The goto and step bodies record Spec.Caller, the caller label, on the history; RunGoto and RunUntilStop take the label as an explicit argument.
// The goto verb's hidden --by flag admits shed alone, the label of lym's own automatic steps; any other value is refused before the status file is read.
// The package reads no environment and imports nothing to classify a caller; the arming module fills Spec.Caller.
// LastStepBuildChanged reads whether the last step ran on a known build other than the one passed, which an in-process driver judges against the lym on disk.
//
// The final envelope is the stop step's short envelope plus a loop object whose keys are closed:
// steps, first_producer, stop, detail, status_moved, hand_back, restep_hold, restep (only when a re-step ran), current_producer, history_length, state, interrupt_policy, trace_copy, stderr_path and pause_reason.
// hand_back is true when the stop is blocked and Spec.HandBack, the recipe's told check, answers true for the status's current producer and error, so a driver hands the halt to the orch; nil marks nothing.
// restep_hold is true under the same rule for Spec.RestepHold, the recipe's told check of a blocked stop that a driver's own build re-launch leaves alone, though such a stop still goes to the arbiter first.
// lym shed status reports the same hand_back and restep_hold keys at their top level, over the status file.
// detail is the stop's error or reason and the WARN and ERROR lines and last lines of the stop step's trace, capped by lines and bytes.
// trace_copy is a copy of the stop step's trace files, which outlives a sweep of the trace directory.
// pause_reason is the status file's pause reason as the stop leaves it, empty unless the run is paused, so a stop that leaves a paused run names why even when the stop step's own envelope carries no reason.
// A refusal of the --until-stop invocation's own arming goes through ReportLoopArmError, which prints the bootstrap error with a loop object that stopped with no step run.
//
// The loop runs detached, and the --until-stop invocation is a waiter on it.
// The waiter may carry --arbiter-attempt <N>, which names the arbiter attempt a driver's re-launch consumes.
// Before it reads the loop's files or spawns anything, the waiter calls Hooks.ConsumeAttempt with the value;
// a nil hook, or a flag without --until-stop, is refused with kind bootstrap, and a hook error is refused with its own message (kind busy for a busy error, bootstrap otherwise).
// Every such refusal carries arbiter_attempt on its envelope and starts no loop, and the flag is dropped from the loop's and its children's command lines, so the attempt is consumed once.
// The waiter then reads the loop lock, the pid file and the envelope file first.
// A free lock beside the teardown's mark, or beside a pid file and no envelope, runs the dead-loop stop.
// A held lock beside a pid file is a live loop: the waiter spawns nothing, adopts the loop id the pid file records and waits.
// A held lock with no pid file is an instant liveness probe or a loop between taking its lock and writing its pid file, so the waiter adopts nothing and reads again on its timer until a pid file appears or the lock reads free.
// A free lock beside an envelope delivers it while the envelope is current, that is while its loop.current_producer, loop.history_length and loop.state still match the status file, and retires it undelivered otherwise.
// Anything else spawns a loop and follows it.
//
// The spawn mints the loop id with logger.NewTraceID and starts Spec.Loop.Executable with the invocation's own command line plus --until-stop and the hidden --loop-detached=<loop id>.
// That flag marks the detached process, which re-runs its subtree's pre-run and then runLoop;
// it selects runLoop and nothing more, and runLoop itself refuses to step over a dead loop's pid file, the teardown's mark or an undelivered envelope, returning at once with no envelope.
// An arming refusal in that second pre-run reaches ReportLoopArmError with the loop id, which writes the refusal as the loop envelope file instead of printing it, so the waiter prints it.
//
// The start handshake: a waiter treats the loop it spawned as starting until the spawned process exits, so a free lock seen before the new loop took it never reads as a gone loop.
// A loop that cannot take the loop lock within the grace period exits at once, and its waiter, seeing its spawn exited and the lock held, adopts the holder's loop id from the pid file.
//
// The loop lock is the liveness record, held for the loop's life, and the pid file is the loop's record for the kill.
// The pid file holds the loop id, the loop's own pid and start time, and the in-flight child's process tree and trace id with the status file's current_producer and history_length when that child started;
// the loop writes it after taking the lock and removes it only after its envelope is on disk.
// The envelope file holds the loop id beside the envelope, and a waiter prints only the envelope.
// A waiter matches files by its adopted loop id: it prints the envelope file only when it carries that id, or the delivered record of that id, which only that loop's envelope ever reaches.
// Printing an envelope retires it by renaming the file onto the delivered record of its loop id, so the next invocation spawns a fresh loop, a second waiter on the same loop still prints it, and a later loop's delivery lands in its own record.
// With the lock free, retiring also removes a pid file beside it that names the same loop; the teardown's mark stays.
//
// The waiter looks again on a timer that starts at one second, doubles to thirty while nothing changes and returns to one second on a change.
// A file event in the steps directory, or the exit of its own spawn, only cuts the current wait short, so a loop that dies without touching a file is still seen at the next tick.
//
// The loop watches each child step for activity and kills one that shows none for the idle window, Spec.Loop.IdleTimeout;
// a zero window disarms the watch, and one below a minute is raised to a minute with one Warn.
// Activity is a write to any of the child's trace files, listed through Spec.Loop.TraceFiles on each wake since a child opens its trace after it starts, or a newer reading of Spec.Loop.Activity, the arming module's report of the run's agent activity.
// The watch counts the idle time from the child's start until the first activity, wakes at most once a second, doubles its wake to a minute while nothing changes and returns to a second on a change.
// The idle time is awake, unpaused time: the watch reads Spec.Loop.Awake (default the monotonic clock) against the wall clock on each wake to see a machine sleep, and the intervals Spec.Loop.Pause reports a pause covered the run, and neither counts.
// While Spec.Loop.Pause reports the run covered the watch judges nothing and never kills, and a pause record it cannot read reads uncovered with one Warn.
// When the window passes the loop asks the child to dump its goroutines with Quit so the dump lands in its captured stderr, gives it five seconds to exit, then kills it with its descendants.
// The stop is interrupted with a cause that names the watchdog, the awake idle time, the window and the last activity time, and loop.detail naming the idle time.
// Spec.Loop.OnWatchdogKill runs once after the kill, with what the watchdog read, and the note it returns ends the cause; both Warn lines of the kill carry the same reading in the step's trace.
// The loop never re-steps an interrupted step itself; loop.interrupt_policy and loop.status_moved let the driver's rule decide.
// A child that exits on its own with a non-zero status and no record keeps cause exited with the exit status.
// The watch kills only the loop's own in-flight child and its process group or job.
//
// Three optional seams let the arming module hook the loop.
// Spec.Loop.OnStart runs once, after the pid file is written and the start is logged and before the first step, so the arming module registers the loop as a run process without this package importing the owner record.
// Spec.Loop.OnWake runs on every wake of the child watch; with a zero idle window and OnWake set, the watch runs on the same wake schedule without ever killing the child, until the child exits.
// Spec.Loop.OnExit runs once at every stop, after the stop is final and a failed stop is written, with the stop and the status file as it then reads, and before the envelope is written.
// A loop that refuses before its first step, and a dead loop the waiter reports, never run OnStart or OnExit.
//
// A loop that is gone without an envelope ends in the dead-loop stop.
// It kills what is left of the loop's step tree from the pid file's child record, writes the run failed while the status file still reads running, and reports an interrupted stop with cause loop-exited and loop.status_moved from the recorded producer and history length against the status file.
// Its loop.detail names the loop log, the producer of the in-flight child and its trace id while the status file has not moved past that child, and whether that child's step tree was still running and was killed, which a loop killed from outside leaves behind.
// Spec.Hooks.AfterInterrupt gets loop-exited followed by that detail as its cause, and the in-flight child's first trace file, or the loop log when the record names no trace, no file is listed for it or the status file has moved past that child.
// It writes that stop as the loop's envelope under the dead loop's id, delivers it and retires the pid file.
// A record naming no child kills nothing and reports status_moved false.
// Another holder of the run lock makes the stop busy: nothing is written, the pid file stays and the next invocation retries.
// A waiter whose own spawn exited before any pid file named it, with the lock free, reports the same stop with loop.detail naming the loop log, and writes no loop envelope.
// The teardown's mark, which a pair's session end puts in the pid file in place of a loop's record, stops every invocation the same way, writes no envelope and keeps the mark, so no loop runs until the pair is removed.
//
// progress has two shapes.
// The step envelope's progress is compact: step, steps and name, with a zero step or steps and an empty name omitted, and no remaining list.
// The status envelope's progress keeps the full form including remaining, since the driver names producers from it.
//
// A failure stop is visible before its waiter returns:
// when an --until-stop loop stops on a failure and no other holder has the run lock, the status file no longer reads running by the time the waiting invocation returns.
// That covers a dead loop, a child that ended in an error and an arming refusal, so `lym loom resume` never answers "already running" for such a run, and no failure waits for the quiet notice.
// A busy stop, an absent or unreadable status file, the Go driver and a bare step are outside it, as the stops above describe.
// TestLoop_FailureStopLeavesStatusFailedUnlessBusy, TestUntilStop_DeadLoopStopsLoopExited and TestLoopIntegration_ArmingRefusalStopsAsBootstrap pin it.
package shedverbs
