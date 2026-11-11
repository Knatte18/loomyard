// Package shedverbs implements the generic cobra verbs run, step, status, pause and goto.
// Every module that arms a *shedengine.Shed onto a CLI subtree shares them.
// goto only reads its Spec and calls shedengine.Goto, which owns the status-file mutation.
// The package owns those verb bodies and nothing else.
// It derives no path of its own, imports no resolver, and never calls os.Getwd or internal/lymcwd;
// every path it touches reaches it through a Spec the arming module fills.
// It imports no <module>cli package, which keeps a consuming module's own imports acyclic:
// a CLI module such as internal/shedcli can import shedverbs, but shedverbs never imports back.
// The one resolving import it admits is internal/logger, per the Shed Verb-Set Invariant.
// It serves boundary logging and the two sink-location accessors.
//
// pause with no flag requests a pause at the next producer boundary, unless the spec's BarePause hook refuses it.
// --before <producer> and --after <producer> record a stop condition the engine fires at that producer;
// --clear removes both.
// None of the three touches the bare request, and --clear refuses beside the other two.
// --by <label> stores a free-text caller label beside the request it accompanies:
// pause_requested_by for a bare pause, pause_before_by with --before and pause_after_by with --after.
// Each label is replaced with its condition, and --clear drops both condition labels.
// The label gates nothing, and the engine records it in the paused run's pause_reason;
// --by shed is refused before any write, since shed is the label of lym's own automatic steps.
// status's human view prints pause_reason as a "why" line for a paused run with a non-empty reason.
// Spec.GotoGuard is the recipe's check of a goto target, which every goto passes; nil admits every target.
// Spec.HandBack is the recipe's check of a blocked stop that goes to the orch; it keeps the package recipe-generic.
// The loop's stop envelope and lym shed status report its answer as hand_back.
//
// step prints a short envelope by default.
// Before printing, it writes the full envelope to its per-invocation record under Spec.StepsDir,
// and the short envelope names that record's path as "envelope_path".
// The --full flag makes step print the full envelope instead, byte-identical to the record.
// A record that could not be written does the same.
// The record, the exit code and the run's state are the same with or without --full.
//
// step's refusal-kind vocabulary is closed: busy, unseeded, ownership, bootstrap, producer and interrupted.
// A module arming a step subtree reports an arming error through ReportArmError.
// It prints a bootstrap error on step, with a trace file and a way forward, and a bare error line on any other verb.
// An error wrapped in KindlessRefusal, such as a missing seed, prints the bare line on step too.
//
// step --until-stop runs one child step after another, each under a fresh trace id.
// It prints one envelope at the first stop.
// A child's own record under Spec.StepsDir is its step's full envelope.
// A child that refused before its step body ran left an error envelope on stdout instead.
// A child that left neither is an interrupted stop.
// The invocation carrying the flag keeps no in-flight record, so last_step only ever names a child's step.
// A recipe that arms no loop (an empty Spec.Loop.EnvelopePath) refuses the flag with a bootstrap error.
//
// The stop is one of LoopStop's values.
// halted is a run that is no longer running: blocked, awaiting, done or paused by request.
// stop-condition is a pause_before or pause_after condition that fired.
// error is a step that ended in an error, and interrupted a child that ended without an envelope;
// both write the status file failed while it still reads running.
// An interrupted child's cause is its exit, its command line and an excerpt of its captured stderr:
// a Go crash's first lines when stderr holds one, and its last lines otherwise.
// The stop names the row the status file names once the child exited.
// A child that skipped gated rows moved the run on before it died.
// Its trace file is the child's first trace file;
// with none, it is the child's captured stderr; with neither, the loop process's own trace file.
// A child start refused with ETXTBSY, because the executable was open for writing, is tried again.
// That happens while a deploy rewrites the lym binary in place.
// There are five tries in all, waiting 100 ms before the first retry and doubling after through Spec.Loop.Sleep.
// A child that never started is interrupted with the cause start-failed after <n> tries: <error>.
// For ETXTBSY the cause adds what the refusal means.
// Spec.Loop.LoopTraceFile names the loop process's own trace file.
// It holds the start failure of a child that never started.
// The failed write comes before Hooks.AfterInterrupt, so the hook reads the status the stop leaves:
// a child that died before its resume write leaves a paused run paused.
// interrupted is also the stop on the status read at an iteration's top.
// Its cause is status-missing for an absent file and status-unreadable: <error> for an unreadable one.
// A decode error stops at once, since the file stays undecodable to this binary;
// any other read error gets one re-read through the same re-step the transient errors share,
// and it stops when that fails or the re-step is already used.
// No child runs at such a stop, and the trace file is the loop log.
// The error names the status file and the loop log, and it ends with a way forward split by cause:
// the recipe's own reseed for an absent file;
// a lym binary that decodes the file for a decode error;
// a re-run of the step for a read error.
// The status file is left as found, since writing it failed would destroy the evidence.
// busy is another holder of the run, or of the run lock at the failed write, which writes nothing.
// A transient error gets one immediate re-step, reported as restep.
//
// RunUntilStop and RunGoto are the exported entries an in-process driver calls.
// They are the bodies the step --until-stop and goto verbs run after their flags are parsed.
// The goto and step bodies record Spec.Caller, the caller label, on the history;
// RunGoto and RunUntilStop take the label as an explicit argument.
// The goto verb's hidden --by flag admits shed alone, the label of lym's own automatic steps;
// any other value is refused before the status file is read.
// The package reads no environment and imports nothing to classify a caller; the arming module fills Spec.Caller.
// LastStepBuildChanged reports whether the last step ran on a known build other than the one passed.
// An in-process driver judges that against the lym on disk.
//
// The final envelope is the stop step's short envelope plus a loop object whose keys are closed.
// The keys are steps, first_producer, stop, detail, status_moved, hand_back and restep_hold.
// restep is present only when a re-step ran.
// The others are current_producer, history_length, state, interrupt_policy, trace_copy, stderr_path and pause_reason.
// hand_back is true when the stop is blocked and Spec.HandBack answers true for the status's current producer and error.
// A driver then hands the halt to the orch; a nil Spec.HandBack marks nothing.
// restep_hold follows the same rule for Spec.RestepHold.
// That is the recipe's check of a blocked stop that a driver's own build re-launch leaves alone;
// such a stop still goes to the arbiter first.
// lym shed status reports the same hand_back and restep_hold keys at their top level, over the status file.
// detail is the stop's error or reason, then the WARN and ERROR lines and the last lines of the stop step's trace.
// It is capped by lines and bytes.
// trace_copy is a copy of the stop step's trace files, which outlives a sweep of the trace directory.
// pause_reason is the status file's pause reason as the stop leaves it, empty unless the run is paused.
// A stop that leaves a paused run so names why, even when the stop step's own envelope carries no reason.
// A refusal of the --until-stop invocation's own arming goes through ReportLoopArmError.
// It prints the bootstrap error with a loop object that stopped with no step run.
//
// The loop runs detached, and the --until-stop invocation is a waiter on it.
// The waiter may carry --arbiter-attempt <N>, which names the arbiter attempt a driver's re-launch consumes.
// Before it reads the loop's files or spawns anything, the waiter calls Hooks.ConsumeAttempt with the value;
// a nil hook, or a flag without --until-stop, is refused with kind bootstrap,
// and a hook error is refused with its own message (kind busy for a busy error, bootstrap otherwise).
// Every such refusal carries arbiter_attempt on its envelope and starts no loop,
// and the flag is dropped from the loop's and its children's command lines, so the attempt is consumed once.
// The waiter then reads the loop lock, the pid file and the envelope file first.
// A free lock beside the teardown's mark, or beside a pid file and no envelope, runs the dead-loop stop.
// A held lock beside a pid file is a live loop:
// the waiter spawns nothing, adopts the loop id the pid file records and waits.
// A held lock with no pid file is an instant liveness probe or a loop between taking its lock and writing its pid file.
// The waiter then adopts nothing and reads again on its timer until a pid file appears or the lock reads free.
// A free lock beside an envelope delivers it while the envelope is current:
// its loop.current_producer, loop.history_length and loop.state still match the status file.
// Otherwise the envelope is retired undelivered.
// Anything else spawns a loop and follows it.
//
// The spawn mints the loop id with logger.NewTraceID.
// It starts the loop's keeper, Spec.Loop.Executable with these arguments:
// the invocation's own command line, --until-stop and the hidden --loop-keeper=<loop id>.
// The keeper re-runs its subtree's pre-run and then starts the loop process in a session of its own.
// The loop process is the same executable with the same command line, carrying the hidden --loop-detached=<loop id> in place of --loop-keeper.
// It re-runs the pre-run too and then runLoop; the flag selects runLoop and nothing more.
// The keeper waits for the loop process and exits once it ends, so it lives as long as the loop and outlives the waiter.
// It catches SIGINT, SIGTERM and SIGHUP and keeps waiting, so a signal that ends the run's processes ends the keeper only through the loop's end.
// runLoop itself refuses to step over a dead loop's pid file, the teardown's mark or an undelivered envelope;
// it returns at once with no envelope.
// An arming refusal in the keeper's or the loop's pre-run reaches ReportLoopArmError with the loop id, which DetachedLoopID reads from either flag.
// That writes the refusal as the loop envelope file instead of printing it, so the waiter prints it.
//
// The start handshake: a waiter treats the loop it spawned as starting until the spawned keeper exits.
// A free lock seen before the new loop took it therefore never reads as a gone loop.
// A loop that cannot take the loop lock within the grace period exits at once.
// Its waiter sees its spawn exited and the lock held, and it adopts the holder's loop id from the pid file.
//
// The loop lock is the liveness record, held for the loop's life, and the pid file is the loop's record for the kill.
// The pid file holds the loop id and the loop's own pid and start time.
// It also holds the in-flight child's record:
// its process tree, its trace id, and the status file's current_producer and history_length when that child started.
// The loop writes the pid file after taking the lock and removes it only after its envelope is on disk.
// The envelope file holds the loop id beside the envelope, and a waiter prints only the envelope.
// A waiter matches files by its adopted loop id:
// it prints the envelope file only when it carries that id.
// It also prints the delivered record of that id, which only that loop's envelope ever reaches.
// Printing an envelope retires it by renaming the file onto the delivered record of its loop id.
// That has three effects:
// the next invocation spawns a fresh loop;
// a second waiter on the same loop still prints the envelope;
// a later loop's delivery lands in its own record.
// With the lock free, retiring also removes a pid file beside it that names the same loop; the teardown's mark stays.
//
// The waiter looks again on a timer that starts at one second and doubles to thirty while nothing changes.
// A change returns it to one second.
// A file event in the steps directory, or the exit of its own spawn, only cuts the current wait short.
// A loop that dies without touching a file is therefore still seen at the next tick.
//
// The loop watches each child step for activity.
// It kills a child that shows none for the idle window, Spec.Loop.IdleTimeout;
// a zero window disarms the watch, and one below a minute is raised to a minute with one Warn.
// Activity is a write to any of the child's trace files, or a newer reading of Spec.Loop.Activity.
// Spec.Loop.Activity is the arming module's report of the run's agent activity.
// The trace files are listed through Spec.Loop.TraceFiles on each wake, since a child opens its trace after it starts.
// The watch counts the idle time from the child's start until the first activity.
// It wakes at most once a second.
// The wake interval doubles to a minute while nothing changes and returns to a second on a change.
// The idle time is awake, unpaused time.
// On each wake the watch reads Spec.Loop.Awake (default the monotonic clock) against the wall clock.
// A difference between them shows a machine sleep.
// A machine sleep and the intervals Spec.Loop.Pause reports a pause covered the run both stay out of the count.
// While Spec.Loop.Pause reports the run covered, the watch judges nothing and never kills.
// A pause record it cannot read counts as uncovered, with one Warn.
// When the window passes, the loop asks the child to dump its goroutines with Quit,
// so the dump lands in its captured stderr.
// It gives the child five seconds to exit, then kills it with its descendants.
// The stop is interrupted with a cause naming the watchdog, the awake idle time, the window and the last activity time.
// loop.detail names the idle time.
// Spec.Loop.OnWatchdogKill runs once after the kill, with what the watchdog read.
// The note it returns ends the cause.
// Both Warn lines of the kill carry the same reading in the step's trace.
// The loop never re-steps an interrupted step itself;
// loop.interrupt_policy and loop.status_moved let the driver's rule decide.
// A child that exits on its own with a non-zero status and no record keeps cause exited with the exit status.
// The watch kills only the loop's own in-flight child and its process group or job.
//
// A loop ends without an envelope by a signal that kills it.
// A failure of the machine does the same, and so does a panic outside the stepping, in a hook or in the code after it.
// It also ends without one by an early return before its first step:
// the loop lock not taken, the teardown's mark, an undelivered envelope or a dead loop's pid file.
// A panic while stepping is recovered:
// the in-flight child's step tree is killed, and the stop is interrupted with the cause loop-panicked: <value>.
// A terminating signal is SIGINT, SIGTERM or SIGHUP, received through Spec.Loop.Signals.
// The loop writes it to the loop log as a mark of the loop, and then ends the process through Spec.Loop.Raise.
// By default Spec.Loop.Raise sends the signal again with its default action.
// Such a signal comes from whatever ends the run's processes, as a cancel, a teardown or a reap does.
// Once the loop has seen the signal it writes no failed stop, runs no interrupt hook and writes no envelope;
// the dead-loop stop below reports it.
// A mark opens its own line in the loop log, even after a last line of output that has no newline.
// The waiter that spawns a loop marks in the loop log that the loop is starting, before the keeper starts.
// It also marks the keeper's pid and exit status once it reaps the keeper.
// The keeper marks the loop process's pid and its exit status, or the signal that ended it, once that process ends;
// a loop process that does not start gets a mark naming the start error.
// The keeper's mark is written even when the waiter is gone, as after a driver handed over to a successor while its loop ran.
// lym's marks open with "lym shed: loop <loop id> ", which tells them from the output of the loop and its children.
//
// Optional seams let the arming module hook the loop.
// Spec.Loop.OnStart runs once in the loop, after the pid file is written and the start is logged and before the first step.
// It also runs once in the keeper, after the loop process has started.
// The arming module registers the loop and its keeper as run processes there, without this package importing the owner record.
// Spec.Loop.OnWake runs on every wake of the child watch.
// With a zero idle window and OnWake set, the watch keeps the same wake schedule and never kills the child.
// It runs until the child exits.
// Spec.Loop.OnExit runs once at every stop except one that a terminating signal ends while the loop steps.
// It runs after the stop is final and a failed stop is written, and before the envelope is written.
// It receives the stop and the status file as it then reads.
// A loop that refuses before its first step, and a dead loop the waiter reports, never run OnStart or OnExit.
//
// A loop that is gone without an envelope ends in the dead-loop stop.
// It kills what is left of the loop's step tree from the pid file's child record.
// It writes the run failed while the status file still reads running.
// It reports an interrupted stop with cause loop-exited.
// loop.status_moved compares the recorded producer and history length against the status file.
// The in-flight child is the one the pid file records as running; the loop clears the child from the record once it exits.
// Its loop.detail names:
// the loop log;
// the producer the in-flight child started at and its trace id,
// with the status file's producer when the status file has moved since, as a child that skips gated rows moves it;
// how the loop ended;
// an excerpt of the in-flight child's captured stderr;
// and whether that child's step tree was still running and was killed, which a loop killed from outside leaves behind.
// How the loop ended is read from the loop log's lines after the loop's last starting mark:
// the marks of the loop, its keeper and the waiter's spawn, such as a signal the loop received and the loop's exit status,
// or "loop exit: not recorded" when there are none;
// then an excerpt of its other lines, a Go crash's first lines or the last lines.
// A log with no starting mark for the loop gives the first part alone.
// With a child in flight, loop.stderr_path names its captured stderr and loop.trace_copy holds a copy of its trace files;
// with none, both are empty.
// Spec.Hooks.AfterInterrupt gets loop-exited followed by that detail as its cause.
// Its trace file is the in-flight child's first trace file.
// It is the loop log when no child was in flight, the record names no trace or no file is listed for it.
// The dead-loop stop is written as the loop's envelope under the dead loop's id, delivered, and the pid file is retired.
// A record naming no child kills nothing and reports status_moved false.
// Another holder of the run lock makes the stop busy:
// nothing is written, the pid file stays and the next invocation retries.
// A waiter whose own spawn exited before any pid file named it, with the lock free, reports the same stop.
// Its loop.detail names the loop log and how the loop ended, and it writes no loop envelope.
// The teardown's mark is what a pair's session end puts in the pid file in place of a loop's record.
// It stops every invocation the same way and writes no envelope.
// The mark stays, so no loop runs until the pair is removed.
//
// progress has two shapes.
// The step envelope's progress is compact: step, steps and name, and no remaining list.
// A zero step or steps and an empty name are omitted.
// The status envelope's progress keeps the full form including remaining, since the driver names producers from it.
//
// A failure stop is visible before its waiter returns:
// the condition is an --until-stop loop that stops on a failure while no other holder has the run lock.
// Then the status file no longer reads running by the time the waiting invocation returns.
// That covers a dead loop, a child that ended in an error and an arming refusal.
// So `lym loom resume` never answers "already running" for such a run, and no failure waits for the quiet notice.
// A busy stop, an absent or unreadable status file, the Go driver and a bare step are outside it.
package shedverbs
