// Package gitexec runs git as a subprocess and reports the outcome in one of two shapes.
// Run is the checked default: a non-zero git exit is a failure, returned as *GitError.
// RunStdin is the second checked form.
// It feeds a stdin text to git, for a command such as `cat-file --batch-check` that reads its operands there.
// RunGit is the raw form, for the sites where a non-zero exit is an answer rather than a failure.
// All three wrap one unexported exec core.
// An exec-level failure, git not being runnable at all, is returned unwrapped.
// So errors.As on *GitError means "git ran and rejected this".
// A git below lym's minimum version returns a floor error, the other non-*GitError result.
// No git command ran to produce it.
//
// # Floor
//
// The exec core checks git's version once per process before its first command.
// It runs `git --version` directly and hands the line to toolfloor.
// A parsed version below the floor makes every entry point return the *toolfloor.FloorError.
// Nothing bypasses a parsed refusal.
// A probe that could not run returns its exec error and is not cached, so the next command probes again.
// An unparseable line is admitted with one Warn naming it, and that answer is cached.
// CheckFloor exposes the same check to a git spawn outside the exec core.
// Bound: a long-lived daemon keeps its cached answer after a git upgrade during its life.
//
// # Remote bounds
//
// A remote subcommand (push, fetch, pull, clone, ls-remote) runs in its own process group under a ten-minute deadline.
// git's HTTP low-speed limit is set to 1000 B/s for 60 s.
// When the deadline expires, the call kills the whole process group on Unix and git alone on Windows.
// It returns a *GitError whose Timeout is the deadline and whose exit code is -1, though git did not exit on its own.
// IsTransportFailure reports such a timeout, and a transfer the low-speed limit stopped, as a transport failure.
// A caller's existing transport handling then takes over.
// A local subcommand runs unbounded.
// Before the deadline starts, a remote subcommand waits through the process's network gate while a pause is on.
// That wait counts against no deadline, and with no gate installed there is none.
// The hold only delays the call's start and never fails it.
//
// # Stall bound
//
// The low-speed limit covers a transfer, never the connection's setup.
// A TLS handshake the remote never answers would run to curl's connect timeout or the deadline.
// A remote subcommand therefore runs with GIT_TRACE_PACKET pointed at a file in lymtemp's run directory.
// The call removes the file afterwards.
// git writes the file's first line only once the remote's first packet arrives.
// A file still empty 30 s into the call is a remote that never answered:
// the call is killed as at the deadline and run once more, with fresh bounds.
// A stalled call received no ref advertisement and changed nothing on the remote.
// That makes the retry safe for a push.
// A second stall returns a *GitError whose Stalled is set and whose Timeout is the stall bound.
// IsTransportFailure reports it as a transport failure.
// Bound: the stall bound covers the wait for the remote's first answer;
// a connection that stalls after it is held by the low-speed limit and the deadline alone.
// A deadline at or under the stall bound, as RunWithin may pass, leaves the stall check unarmed.
// An operator's own GIT_TRACE_PACKET is overridden for a remote subcommand.
//
// # Environment
//
// Every git child inherits the process environment except GIT_DIR and GIT_WORK_TREE.
// Both are dropped so the child follows the directory it is run in, as the in-process reads do.
// One command therefore never reads one repository and writes another.
// Bound: an operator who sets either variable to point `lym` at another repository has it ignored;
// `lym`'s own post-checkout hook already unsets them.
// Every git child also runs with GIT_OPTIONAL_LOCKS=0, whatever the process environment sets it to.
// A read-only command such as `git status` therefore never takes the index lock to write back a refreshed stat cache.
// Such a probe holds no lym lock.
// Its index lock would otherwise fail a concurrent `git add` on the worktree with "index.lock: File exists".
// Bound: a read probe leaves the stat cache unrefreshed, so a later probe may re-read files a refresh would have cached.
//
// The bounds are not configuration.
// A legitimate remote command that runs longer than the deadline fails as a transport failure.
// So does one that moves under the low-speed limit for the low-speed time.
//
// # Start retry
//
// A git start refused for EAGAIN, a temporary resource shortage in fork or exec, is retried:
// four tries in all, waiting 250 ms before the first retry and doubling the wait each time, each retry logged at Warn.
// git never ran on such a failure, so the retry repeats no side effect, for a local or a remote subcommand alike.
// A git that ran and exited nonzero, and every other start failure, returns at once.
// Each refusal is noted in proc's spawn-refusal record.
// A start still refused after the last try returns its exec error unwrapped, as any other exec-level failure.
//
// # Limits
//
// On Windows there is no process group to signal.
// A timed-out git's helper process, such as git-remote-https, may therefore outlive the kill.
// The wait on the command's output pipes is bounded separately, so such a helper cannot hang the call.
//
// # Kill outcome
//
// A kill's outcome reaches the logs through SetKillReporter.
// The process owner installs a reporter once.
// It receives the command's args, its pid and the group kill's error after each kill.
// It cannot stop, delay or retry the kill or change the call's result.
// With no reporter installed the outcome is dropped.
package gitexec
