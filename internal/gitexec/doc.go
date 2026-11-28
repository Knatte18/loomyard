// Package gitexec runs git as a subprocess and reports the outcome in one of two shapes.
// Run is the checked default: a non-zero git exit is a failure, returned as *GitError.
// RunStdin is the second checked form, feeding a stdin text to git.
// RunGit is the raw form, for the sites where a non-zero exit is an answer rather than a failure.
// An exec-level failure, git not being runnable at all, is returned unwrapped.
// So errors.As on *GitError means "git ran and rejected this".
// A git below lym's minimum version returns a floor error, the other non-*GitError result.
//
// # Floor
//
// The version is checked once per process before the first command,
// and a parsed version below the floor refuses every call.
// CheckFloor exposes the same check to a git spawn outside the exec core.
// Bound: a long-lived daemon keeps its cached answer after a git upgrade during its life.
//
// # Remote bounds
//
// A remote subcommand (push, fetch, pull, clone, ls-remote) runs in its own process group under a ten-minute deadline,
// with git's HTTP low-speed limit set to 1000 B/s for 60 s.
// At the deadline the call kills the whole process group on Unix and git alone on Windows.
// It returns a *GitError whose Timeout is the deadline and whose exit code is -1.
// IsTransportFailure reports such a timeout, and a transfer the low-speed limit stopped, as a transport failure.
// It reports a remote whose repository path is gone from this machine as one too.
// A removed origin directory, or one below an unmounted mount point, then reads as a refused connection does.
// Bound: a mount point left as an empty directory reads as a refusal.
// A local subcommand runs unbounded.
// A remote subcommand first waits through the process's network gate while a pause is on.
// That wait counts against no deadline.
// The bounds are not configuration.
//
// # Stall bound
//
// The low-speed limit never covers the connection's setup, so a remote that never answers is bounded separately.
// A remote call whose remote has sent no packet 30 s in is killed and run once more.
// Such a call changed nothing on the remote, so the retry is safe for a push.
// A second stall returns a *GitError whose Stalled is set, which IsTransportFailure also reports.
// Bound: a connection that stalls after the first answer is held by the low-speed limit and the deadline alone.
// An operator's own GIT_TRACE_PACKET is overridden for a remote subcommand.
//
// # Environment
//
// Every git child drops GIT_DIR and GIT_WORK_TREE so it follows the directory it is run in.
// One command therefore never reads one repository and writes another.
// Bound: an operator who sets either variable to point `lym` at another repository has it ignored.
// Every git child runs with GIT_OPTIONAL_LOCKS=0,
// so a read-only command never takes the index lock and fails a concurrent `git add`.
// Bound: a read probe leaves the stat cache unrefreshed.
//
// # Start retry
//
// A git start refused for EAGAIN is retried a few times.
// git never ran on such a failure, so the retry repeats no side effect.
// A start still refused after the last try returns its exec error unwrapped.
//
// # Limits
//
// On Windows there is no process group to signal, so a timed-out git's helper process may outlive the kill.
// The wait on the command's output pipes is bounded separately, so such a helper cannot hang the call.
//
// # Kill outcome
//
// A kill's outcome reaches the logs through SetKillReporter, which cannot change the kill or the call's result.
package gitexec
