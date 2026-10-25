// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment, and for reading a process's liveness, start time, CPU time and ancestry.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW.
// Detach additionally creates a new process group (CREATE_NEW_PROCESS_GROUP), so the child survives parent exit and is unaffected by the parent's Ctrl-C signal.
//
// The per-child and named Windows jobs admit breakaway, so only a DetachBreakaway start leaves them and every other descendant still ends with its job.
//
// ConfigureGroupKill makes a command's cancel SIGKILL its whole process group at once.
// ConfigureGroupTerminate makes it SIGTERM the group instead and SIGKILL it after a delay, and returns the awaitGone a caller runs after the command has been waited for, which returns once the group is empty or the SIGKILL is sent.
// Windows has no process group to signal here, so both kill the command alone and report ErrNoProcessGroup.
//
// TreeCPU samples the user plus system CPU time of a process and each of its descendants.
// IsAncestor reports whether a pid is one of the calling process's ancestors.
// ListProcesses lists every process with its start time, exe and cwd links and command line.
// ListStats lists every process with its parent, group, start time and zombie state, read from its stat alone, so it also lists a process whose links the caller cannot read.
// SignalProcess and SignalGroup send SIGTERM, or SIGKILL when asked, to one pid or to one process group and return the raw syscall error.
// All of these read /proc or signal by pid, so they work on Linux only: on Windows TreeCPU, ListProcesses and ListStats return errors.ErrUnsupported, IsAncestor is false, SignalProcess only kills and SignalGroup returns ErrNoProcessGroup.
// CreationTime is the exception: it reads a process's creation time by pid on Windows, where StartTime is unreadable, and is always the zero time and false elsewhere.
package proc
