// Package gitexec runs git as a subprocess and reports the outcome in one of two shapes.
// Run is the checked default: a non-zero git exit is a failure, returned as *GitError.
// RunStdin is the second checked form, Run with a stdin text fed to git, for a command such as `cat-file --batch-check` that reads its operands there.
// RunGit is the raw form, for the sites where a non-zero exit is an answer rather than a failure.
// Both wrap one unexported exec core, and an exec-level failure, git not being runnable at all, is returned unwrapped so errors.As on *GitError means "git ran and rejected this".
// The floor error of a git below lym's minimum is the other non-*GitError result, since no git command ran to produce it.
//
// # Floor
//
// The exec core checks git's version once per process before its first command, by running `git --version` directly and handing the line to toolfloor.
// A parsed version below the floor makes every entry point return the *toolfloor.FloorError, and nothing bypasses a parsed refusal.
// A probe that could not run returns its exec error and is not cached, so the next command probes again.
// An unparseable line is admitted with one Warn naming it, and that answer is cached.
// CheckFloor exposes the same check to a git spawn outside the exec core.
// Bound: a long-lived daemon keeps its cached answer after a git upgrade during its life.
//
// # Remote bounds
//
// A remote subcommand, one in remoteSubcommands (push, fetch, pull, clone, ls-remote), runs under a ten-minute deadline, in its own process group, with git's HTTP low-speed limit set to 1000 B/s for 60 s.
// When the deadline expires the whole process group is killed on Unix, and git alone on Windows, and the call returns a *GitError whose Timeout is the deadline and whose exit code is -1, though git did not exit on its own.
// IsTransportFailure reports such a timeout, and a transfer the low-speed limit stopped, as a transport failure, so a caller's existing transport handling takes over.
// A local subcommand runs unbounded.
// Before the deadline starts, a remote subcommand waits while a pause covers the process, through the process's network gate: the wait counts against no deadline, and with no gate installed there is none.
// The hold only delays the call's start and never fails it.
//
// # Stall bound
//
// The low-speed limit covers a transfer, never the connection's setup, so a TLS handshake the remote never answers would run to curl's connect timeout or the deadline.
// A remote subcommand therefore runs with GIT_TRACE_PACKET pointed at a file in lymtemp's run directory, removed after the call.
// git writes the file's first line only once the remote's first packet arrives, so a file still empty 30 s into the call is a remote that never answered:
// the call is killed as at the deadline and run once more, with fresh bounds.
// A stalled call received no ref advertisement and so changed nothing on the remote, which is what makes the retry safe for a push.
// A second stall returns a *GitError whose Stalled is set and whose Timeout is the stall bound, which IsTransportFailure reports as a transport failure.
// Bound: the stall bound covers only the wait for the remote's first answer; a connection that stalls after it is held by the low-speed limit and the deadline alone.
// A deadline at or under the stall bound, as RunWithin may pass, leaves the stall check unarmed.
// An operator's own GIT_TRACE_PACKET is overridden for a remote subcommand.
//
// # Environment
//
// Every git child inherits the process environment except GIT_DIR and GIT_WORK_TREE, which are dropped so the child follows the directory it is run in, as the in-process reads do, and one command never reads one repository and writes another.
// Bound: an operator who sets either variable to point `lym` at another repository has it ignored; `lym`'s own post-checkout hook already unsets them.
// Every git child also runs with GIT_OPTIONAL_LOCKS=0, whatever the process environment sets it to, so a read-only command such as `git status` never takes the index lock to write back a refreshed stat cache.
// Such a probe holds no lym lock, and its index lock would otherwise fail a concurrent locked commit's `git add` on the same worktree with "index.lock: File exists".
// Bound: a read probe leaves the stat cache unrefreshed, so a later probe may re-read files a refresh would have cached.
//
// The bounds are not configuration.
// A legitimate remote command that runs longer than the deadline, or moves under the low-speed limit for the low-speed time, fails as a transport failure.
//
// # Start retry
//
// A git the host could not start for EAGAIN, a fork or exec refused for a temporary resource shortage, is started again: four tries in all, waiting 250 ms before the first retry and doubling the wait each time, each retry logged at Warn.
// git never ran on such a failure, so the retry repeats no side effect, for a local or a remote subcommand alike; a git that ran and exited nonzero, and every other start failure, returns at once.
// Each refusal is noted in proc's spawn-refusal record, and a start still refused after the last try returns its exec error unwrapped, as any other exec-level failure.
//
// # Limits
//
// On Windows there is no process group to signal, so a timed-out git's helper process, such as git-remote-https, may outlive the kill.
// The wait on the command's output pipes is bounded separately, so such a helper cannot hang the call.
//
// # Kill outcome
//
// A kill's outcome reaches the logs through SetKillReporter.
// The process owner installs a reporter once; it receives the command's args, its pid and the group kill's error after each kill, and it cannot stop, delay or retry the kill or change the call's result.
// With no reporter installed the outcome is dropped.
package gitexec
