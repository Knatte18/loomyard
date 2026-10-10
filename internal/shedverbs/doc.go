// Package shedverbs implements the generic cobra verb bodies -- run, step, status, pause, and goto -- shared by every module that arms a *shedengine.Shed onto a CLI subtree.
// goto only reads its Spec and calls shedengine.Goto, which owns the status-file mutation.
// It owns those verb bodies and nothing else: it derives no path of its own, imports no resolver, and never calls os.Getwd or internal/lyxcwd -- every path this package touches reaches it told, through a Spec the arming module fills.
// It also imports no <module>cli package, which is what keeps a consuming
// module's own imports acyclic: internal/shedcli (or any other CLI module wiring shedverbs.Verbs
// onto its own subtree) can safely import shedverbs, but shedverbs never imports back.
// The one resolving import it admits is internal/logger, for boundary logging and the two
// sink-location accessors, per the Shed Verb-Set Invariant.
//
// pause with no flag requests a pause at the next producer boundary.
// --before <producer> and --after <producer> record a stop condition the engine fires at that producer, and --clear removes both; none of the three touches the bare request, and --clear refuses beside the other two.
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
// error is a step that ended in an error, and interrupted a child that ended without an envelope, including a status file that vanished; both write the status file failed while it still reads running.
// busy is another holder of the run, or of the run lock at the failed write, which writes nothing.
// A transient error gets one immediate re-step, reported as restep.
//
// The final envelope is the stop step's short envelope plus a loop object whose keys are closed:
// steps, first_producer, stop, detail, status_moved, restep (only when a re-step ran), current_producer, history_length, state, interrupt_policy, trace_copy and stderr_path.
// detail is the stop's error or reason and the WARN and ERROR lines and last lines of the stop step's trace, capped by lines and bytes.
// trace_copy is a copy of the stop step's trace files, which outlives a sweep of the trace directory.
// A refusal of the --until-stop invocation's own arming goes through ReportLoopArmError, which prints the bootstrap error with a loop object that stopped with no step run.
//
// The loop runs detached, and the --until-stop invocation is a waiter on it.
// The waiter reads the loop lock, the pid file and the envelope file first.
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
// The pid file holds the loop id, the loop's own pid and start time, and the in-flight child's process tree with the status file's current_producer and history_length when that child started;
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
// When the window passes the loop asks the child to dump its goroutines with Quit so the dump lands in its captured stderr, gives it five seconds to exit, then kills it with its descendants.
// The stop is interrupted with cause watchdog and loop.detail naming the idle time.
// The loop never re-steps an interrupted step itself; loop.interrupt_policy and loop.status_moved let the driver's rule decide.
// A child that exits on its own with a non-zero status and no record keeps cause exited with the exit status.
// The watch kills only the loop's own in-flight child and its process group or job.
//
// A loop that is gone without an envelope ends in the dead-loop stop.
// It kills what is left of the loop's step tree from the pid file's child record, writes the run failed while the status file still reads running, and reports an interrupted stop with cause loop-exited and loop.status_moved from the recorded producer and history length against the status file.
// It writes that stop as the loop's envelope under the dead loop's id, delivers it and retires the pid file.
// A record naming no child kills nothing and reports status_moved false.
// Another holder of the run lock makes the stop busy: nothing is written, the pid file stays and the next invocation retries.
// A waiter whose own spawn exited before any pid file named it, with the lock free, reports the same stop with loop.detail naming the loop log, and writes no loop envelope.
// The teardown's mark, which a pair's session end puts in the pid file in place of a loop's record, stops every invocation the same way, writes no envelope and keeps the mark, so no loop runs until the pair is removed.
//
// progress has two shapes.
// The step envelope's progress is compact: step, steps and name, with a zero step or steps and an empty name omitted, and no remaining list.
// The status envelope's progress keeps the full form including remaining, since the driver names producers from it.
package shedverbs
