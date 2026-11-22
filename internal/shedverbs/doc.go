// Package shedverbs implements the generic cobra verbs run, step, status, pause and goto.
// Every module that arms a *shedengine.Shed onto a CLI subtree shares them.
// The package owns those verb bodies and nothing else.
// It derives no path of its own, imports no resolver, and never calls os.Getwd or internal/lymcwd;
// every path it touches reaches it through a Spec the arming module fills.
// It imports no <module>cli package, so a CLI module such as internal/shedcli can import it without a cycle.
// The one resolving import it admits is internal/logger, per the Shed Verb-Set Invariant.
// run and step carry `clihelp.RunProcessAnnotation`, so a pause holds their processes;
// status, pause and goto are an operator's and pass a pause.
//
// pause with no flag requests a pause at the next producer boundary, unless the spec's BarePause hook refuses it.
// --before <producer> and --after <producer> record a stop condition the engine fires at that producer;
// --clear removes both and refuses beside the other two.
// --by <label> stores a free-text caller label beside the request; it gates nothing.
// --by shed is refused before any write, since shed is the label of lym's own automatic steps.
// status's idle_gaps name every stretch of the history of at least idleGapFloor in which no step ran.
// Spec.GotoGuard checks a goto target, and nil admits every target.
// Spec.GotoAdmitted is called with the target once a goto is written, and nil calls nothing.
// Spec.HandBack marks a blocked stop that goes to the orch, which keeps the package recipe-generic;
// the loop's stop envelope and lym shed status report its answer as hand_back.
//
// step prints a short envelope by default and writes the full one to its record under Spec.StepsDir.
// The short envelope names that record as "envelope_path".
// --full prints the full envelope instead, which a record that could not be written also does.
// The record, the exit code and the run's state are the same either way.
//
// step's refusal-kind vocabulary is closed: busy, unseeded, ownership, bootstrap, producer and interrupted.
// An arming error goes through ReportArmError: a bootstrap error on step and a bare error line on any other verb.
// An error wrapped in KindlessRefusal prints the bare line on step too.
//
// RunUntilStop and RunGoto are the exported entries an in-process driver calls.
// They take the caller label as an argument where the verbs read Spec.Caller.
// The package reads no environment to classify a caller; the arming module fills Spec.Caller.
// The goto verb's hidden --by flag admits shed alone.
//
// step --until-stop runs one child step after another and prints one envelope at the first stop.
// A recipe that arms no loop (an empty Spec.Loop.EnvelopePath) refuses the flag with a bootstrap error.
// The stop is one of LoopStop's values:
// halted is a run that is no longer running, stop-condition a pause_before or pause_after that fired;
// error a step that ended in an error, and interrupted a child that ended without an envelope.
// error and interrupted write the status file failed while it still reads running.
// busy is another holder of the run lock, and it writes nothing.
// A transient error gets one immediate re-step, reported as restep.
// An interrupted stop also covers an absent or unreadable status file at an iteration's top;
// the status file is left as found, and the error ends with a way forward split by cause.
// The loop never re-steps an interrupted step itself;
// loop.interrupt_policy and loop.status_moved let the driver's rule decide.
// The failed write comes before Hooks.AfterInterrupt, so the hook reads the status the stop leaves.
//
// The final envelope is the stop step's short envelope plus a loop object with a closed key set.
// hand_back is true when the stop is blocked and Spec.HandBack answers true for the status's producer and error.
// restep_hold follows the same rule for Spec.RestepHold:
// a blocked stop that a driver's own build re-launch leaves alone, which still goes to the arbiter first.
// trace_copy outlives a sweep of the trace directory, and detail is capped by lines and bytes.
// A refusal of the invocation's own arming goes through ReportLoopArmError, as a loop object with no step run.
// progress is compact on the step envelope and keeps remaining on the status envelope, which drivers read.
//
// The loop runs detached, and the --until-stop invocation is a waiter on it.
// --arbiter-attempt <N> names the arbiter attempt a driver's re-launch consumes.
// The waiter calls Hooks.ConsumeAttempt with it before it reads the loop's files or spawns anything.
// A nil hook, or the flag without --until-stop, is refused with kind bootstrap;
// a hook error is refused with its own message (kind busy for a busy error, bootstrap otherwise).
// A refusal carries arbiter_attempt and starts no loop, and the flag is dropped from the loop's command line,
// so the attempt is consumed once.
// The waiter then reads the loop lock, the pid file and the envelope file:
// a live loop (lock held beside a pid file) is adopted and waited on, never respawned;
// a free lock beside a current envelope delivers it, and a stale one is retired undelivered;
// a free lock beside the teardown's mark, or beside a pid file and no envelope, runs the dead-loop stop;
// anything else spawns a loop and follows it.
// The spawned keeper outlives the waiter and catches SIGINT, SIGTERM and SIGHUP,
// so a signal that ends the run's processes ends the keeper only through the loop's end.
// DetachedLoopID reads the loop id from the hidden --loop-keeper and --loop-detached flags.
// A waiter treats a loop it spawned as starting until its keeper exits,
// so a free lock seen before the new loop took it never reads as a gone loop.
// Printing an envelope retires it by renaming it onto the delivered record of its loop id:
// the next invocation spawns a fresh loop, and a second waiter on the same loop still prints the envelope.
//
// The loop kills a child step that shows no activity for Spec.Loop.IdleTimeout, and a zero window disarms the watch.
// Activity is a write to any trace file of the child, or a newer reading of Spec.Loop.Activity.
// The idle time counts awake, unpaused time only; a run Spec.Loop.Pause reports covered is never killed.
// The stop is interrupted with a cause naming the watchdog, and Spec.Loop.OnWatchdogKill adds a note to it.
// A child that exits non-zero with no record keeps cause exited and goes to Spec.Loop.OnStepOrphaned.
// Spec.Loop.ReconcileOrphanedSteps runs once before the first child starts, when no step of the run is in flight.
// It settles what any earlier step whose process is gone left behind.
// The watch kills only the loop's own in-flight child and its process group or job.
//
// A loop ends without an envelope by a signal that kills it, a machine failure or a panic outside the stepping.
// An early return before its first step ends it the same way:
// the loop lock not taken, the teardown's mark, an undelivered envelope or a dead loop's pid file.
// A panic while stepping is recovered, the in-flight child's step tree is killed,
// and the stop is interrupted with the cause loop-panicked.
// A terminating signal received through Spec.Loop.Signals is marked in the loop log,
// and the process ends through Spec.Loop.Raise with no failed stop, no interrupt hook and no envelope.
// Every such end marks its reason in the loop log, with marks opening "lym shed: loop <loop id> ".
// The loop appends a finished child's output to the loop log, each line opening "lym shed: step <trace id> output: ".
//
// Spec.Loop.OnStart runs once in the loop and once in its keeper.
// The arming module registers them there as run processes.
// This package never imports the owner record.
// Spec.Loop.OnWake runs on every wake of the child watch, and with a zero idle window it never kills the child.
// Spec.Loop.OnExit runs once at every stop except one a terminating signal ends while the loop steps.
// It runs after a failed stop is written and before the envelope is written.
// A loop that refuses before its first step, and a dead loop the waiter reports, never run OnStart or OnExit in the loop.
//
// A loop that is gone without an envelope ends in the dead-loop stop.
// It kills what is left of the loop's step tree from the pid file's child record.
// It hands the in-flight child's trace id to Spec.Loop.OnStepOrphaned.
// It writes the run failed while the status file still reads running.
// It reports an interrupted stop with cause loop-exited, which Spec.Hooks.AfterInterrupt receives with the detail.
// loop.detail names the loop log, the in-flight child, how the loop ended and an excerpt of the child's stderr.
// How the loop ended comes from the marks in the loop log.
// With none, it says so and compares the boot ids, the pid file's against Spec.Loop.BootID.
// The comparison tells a stop of the machine from a kill.
// The stop is written as the loop's envelope under the dead loop's id, delivered, and the pid file is retired.
// Another holder of the run lock makes the stop busy: nothing is written and the next invocation retries.
// The teardown's mark is what a pair's session end puts in the pid file in place of a loop's record.
// It stops every invocation without an envelope, and no loop runs until the pair is removed.
//
// A failure stop is visible before its waiter returns:
// an --until-stop loop that stops on a failure leaves a status file that no longer reads running.
// This holds while no other holder has the run lock.
// That covers a dead loop, a child that ended in an error and an arming refusal.
// So `lym loom resume` never answers "already running" for such a run, and no failure waits for the quiet notice.
// A busy stop, an absent or unreadable status file, the Go driver and a bare step are outside it.
package shedverbs
