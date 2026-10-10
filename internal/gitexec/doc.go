// Package gitexec runs git as a subprocess and reports the outcome in one of two shapes.
// Run is the checked default: a non-zero git exit is a failure, returned as *GitError.
// RunGit is the raw form, for the sites where a non-zero exit is an answer rather than a failure.
// Both wrap one unexported exec core, and an exec-level failure, git not being runnable at all, is returned unwrapped so errors.As on *GitError means "git ran and rejected this".
//
// # Remote bounds
//
// A remote subcommand, one in remoteSubcommands (push, fetch, pull, clone, ls-remote), runs under a ten-minute deadline, in its own process group, with git's HTTP low-speed limit set to 1000 B/s for 60 s.
// When the deadline expires the whole process group is killed on Unix, and git alone on Windows, and the call returns a *GitError whose Timeout is the deadline and whose exit code is -1, though git did not exit on its own.
// IsTransportFailure reports such a timeout, and a transfer the low-speed limit stopped, as a transport failure, so a caller's existing transport handling takes over.
// A local subcommand runs unbounded.
//
// # Environment
//
// Every git child inherits the process environment except GIT_DIR and GIT_WORK_TREE, which are dropped so the child follows the directory it is run in, as the in-process reads do, and one command never reads one repository and writes another.
// Bound: an operator who sets either variable to point `lyx` at another repository has it ignored; `lyx`'s own post-checkout hook already unsets them.
//
// The bounds are not configuration.
// A legitimate remote command that runs longer than the deadline, or moves under the low-speed limit for the low-speed time, fails as a transport failure.
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
