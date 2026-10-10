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
// progress has two shapes.
// The step envelope's progress is compact: step, steps and name, with a zero step or steps and an empty name omitted, and no remaining list.
// The status envelope's progress keeps the full form including remaining, since the driver names producers from it.
package shedverbs
