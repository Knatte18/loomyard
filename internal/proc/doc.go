// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment, and for reading a process's liveness, start time, CPU time and ancestry.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW.
// Detach additionally creates a new process group (CREATE_NEW_PROCESS_GROUP), so the child survives parent exit and is unaffected by the parent's Ctrl-C signal.
//
// ConfigureGroupKill makes a command's cancel SIGKILL its whole process group at once.
// ConfigureGroupTerminate makes it SIGTERM the group instead and SIGKILL it after a delay, and returns the awaitGone a caller runs after the command has been waited for, which returns once the group is empty or the SIGKILL is sent.
// Windows has no process group to signal here, so both kill the command alone and report ErrNoProcessGroup.
//
// TreeCPU samples the user plus system CPU time of a process and each of its descendants.
// IsAncestor reports whether a pid is one of the calling process's ancestors.
// ListProcesses lists every process with its start time, exe and cwd links and command line.
// All three read /proc, so they work on Linux only: on Windows TreeCPU and ListProcesses return errors.ErrUnsupported and IsAncestor is false.
package proc
