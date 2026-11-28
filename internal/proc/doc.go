// Package proc provides cross-OS primitives for child-process window visibility and detachment.
// It also reads a process's liveness, start time, CPU time and ancestry.
//
// On Windows, HideWindow suppresses the console window using CREATE_NO_WINDOW.
// Detach additionally creates a new process group (CREATE_NEW_PROCESS_GROUP),
// so the child survives parent exit and is unaffected by the parent's Ctrl-C signal.
//
// The per-child and named Windows jobs admit breakaway.
// Only a DetachBreakaway start leaves them, and every other descendant still ends with its job.
//
// ConfigureGroupKill makes a command's cancel SIGKILL its whole process group at once.
// ConfigureGroupTerminate makes it SIGTERM the group instead and SIGKILL it after a delay.
// It returns the awaitGone a caller runs after the command has been waited for;
// awaitGone returns once the group is empty or the SIGKILL is sent.
// Windows has no process group to signal here, so both kill the command alone and report ErrNoProcessGroup.
//
// IsAlive reports whether a pid answers a signal, which a zombie still does;
// IsRunning reports whether it is alive and not a zombie,
// so a process that exited but was not yet reaped counts as gone.
//
// Reap waits on a started command in the background,
// so a detached child its starter never waits on leaves no zombie.
// ReapInheritedChildren waits in the background on every child the calling process has:
// a process that re-execs itself keeps its children but not the goroutines that waited on them,
// so the new image calls it before it starts a child of its own.
//
// TreeSample samples a process and each of its descendants:
// its parent, its user plus system CPU time and the memory it holds resident.
// IsAncestor reports whether a pid is one of the calling process's ancestors.
// ListProcesses lists every process with its start time, exe and cwd links and command line.
// ReadProcess reads the same of one process.
// ListStats lists every process with its parent, group, start time and zombie state, read from its stat alone.
// It also lists a process whose links the caller cannot read.
// PIDsWithEnv lists every process whose environment holds a given `NAME=value` entry.
// SignalProcess and SignalGroup send SIGTERM, or SIGKILL when asked, to one pid or to one process group.
// They return the raw syscall error.
// All of these read /proc or signal by pid, so they work on Linux only.
// On Windows TreeSample, ListProcesses, ListStats and PIDsWithEnv return errors.ErrUnsupported.
// ReadProcess and IsAncestor are false there.
// SignalProcess only kills, and SignalGroup returns ErrNoProcessGroup.
// CreationTime is the exception: it reads a process's creation time by pid on Windows, where StartTime is unreadable.
// Elsewhere it is always the zero time and false.
//
// BootID reads the id the kernel drew when the machine last booted, from /proc/sys/kernel/random/boot_id on Linux.
// It changes at a boot and not with the clock, so two reads equal tell that no boot lay between them.
// It reports false on Windows.
//
// ReadTaskLimit reads `pids.current` and `pids.max` over the calling process's cgroup and its ancestors.
// It returns the cgroup v2 task limit closest to being reached.
// A caller uses it to hold back new work before every fork fails.
// It finds none on Windows or a cgroup v1 host.
//
// ReadMemoryLimit reads `memory.high`, `memory.max` and `memory.current` over the same ancestry.
// It returns the cgroup v2 memory limit with the least memory left, the file cache not counted as used.
// A caller uses it to start no more memory-capped work than the limit leaves room for.
// It finds none on Windows or a cgroup v1 host.
//
// RunMemoryCapped runs a command with the memory of its process tree capped.
// On Linux the command runs in a transient scope of the systemd user manager, started through `systemd-run`.
// The scope's `MemoryMax` is the cap and it has no swap, so the kernel kills the command alone.
// The scope checks its own `memory.max` before it runs the command, and a scope not held to the cap never runs it.
// The call reports a kill at the scope's own limit, read from the scope's `memory.events` once the command has ended.
// A holder process in the scope keeps the scope, and that count, until the read is done,
// so the read never races the scope's removal.
// A kill at the limit of an enclosing cgroup or of the host is no such kill.
// After a SIGKILL of the command or a kill at the cap, it kills what is left in the scope through its `cgroup.kill`.
// The scope's `OOMPolicy` is `continue`, so the kill never fails the scope with the result `oom-kill`.
// KDE Plasma's out-of-memory notifier reports each unit that ends with that result as a host low on memory.
// The command keeps its pid and process group, so a group kill still reaches it.
// One probe per process tells whether the user manager can cap a scope's memory.
// Where it cannot, `RLIMIT_DATA` limits each process's data segment instead, set once the command started.
// An allocation past that limit fails in the process, and the call reports no kill.
// On Windows the command runs in a kill-on-close job object with a job memory limit.
// That limit fails an allocation the same way.
// A cancel there ends the whole job, and the job ends with the command.
// RunMemoryCappedVia runs a command under the same cap through a caller's own start and wait.
// An example is a start that ties the child to the caller's life.
// It reports whether the kernel killed any process of the tree at the cap.
// A cap that could not be set returns an error wrapping `ErrMemoryCap`.
// So does a scope that never ran the command, or one whose `memory.max` is not the cap.
// MemoryCapCoversTree reports whether the cap bounds a command's whole tree as one.
// Only the data segment limit does not.
// proc cannot log, so the caller logs the command it hands over and its end.
// The one-time probe's `systemd-run` spawn goes unlogged; a caller sees only its outcome, through MemoryCapCoversTree.
//
// IsSpawnAgain reports whether a start failed on a temporary resource shortage, EAGAIN from fork or exec.
// NoteSpawnAgain records the time of each such refusal process-wide, and LastSpawnAgain reads it back,
// so a caller that sees a later failure can judge whether a host resource shortage preceded it.
// IsSpawnBusy reports whether a start failed because the executable was open for writing, ETXTBSY.
// A deploy that rewrites the binary in place causes it.
//
// BuildServerOffEnv is the environment that stops a .NET build leaving MSBuild nodes or servers running.
// A process that sets it for its builds leaves no thread-holding servers behind when parallel agents start them.
package proc
