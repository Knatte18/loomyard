// Package proc provides cross-OS primitives for controlling child-process window visibility and detachment, and for reading a process's liveness, start time, CPU time and ancestry.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW.
// Detach additionally creates a new process group (CREATE_NEW_PROCESS_GROUP), so the child survives parent exit and is unaffected by the parent's Ctrl-C signal.
//
// TreeCPU samples the user plus system CPU time of a process and each of its descendants.
// IsAncestor reports whether a pid is one of the calling process's ancestors.
// ListProcesses lists every process with its start time, exe and cwd links and command line.
// All three read /proc, so they work on Linux only: on Windows TreeCPU and ListProcesses return errors.ErrUnsupported and IsAncestor is false.
package proc
