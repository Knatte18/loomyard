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
// ConfigureGroupKill and ConfigureGroupTerminate make a command's cancel end its whole process group.
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
// The process readers are TreeSample, ResidentInCgroup, ListProcesses, ReadProcess, ListStats and PIDsWithEnv.
// ResidentInCgroup lists the processes of a cgroup and its descendants with the memory each holds resident.
// They, the signal senders and the cgroup readers read /proc or signal by pid, so they work on Linux only.
// On Windows TreeSample, ResidentInCgroup, ListProcesses, ListStats and PIDsWithEnv return errors.ErrUnsupported.
// ReadProcess and IsAncestor are false there.
// SignalProcess only kills, and SignalGroup returns ErrNoProcessGroup.
// SignalProcess and SignalGroup return the raw syscall error.
// CreationTime is the exception: it reads a process's creation time by pid on Windows, where StartTime is unreadable.
// Elsewhere it is always the zero time and false.
//
// BootID changes at a boot and not with the clock, so two reads equal tell that no boot lay between them.
// It reports false on Windows.
//
// ReadTaskLimit and ReadMemoryLimit return the cgroup v2 limit closest to being reached.
// They look over the calling process's cgroup and its ancestors.
// A caller uses them to hold back new work before every fork fails or the memory runs out.
// They find none on Windows or a cgroup v1 host.
//
// RunMemoryCapped runs a command with the memory of its process tree capped.
// On Linux the command runs in a transient scope of the systemd user manager, started through `systemd-run`.
// The scope's `MemoryMax` is the cap and it has no swap, so the kernel kills the command alone.
// The scope checks its own `memory.max` before it runs the command, and a scope not held to the cap never runs it.
// The call reports a kill at the scope's own limit.
// A kill at the limit of an enclosing cgroup or of the host is no such kill.
// The scope's `OOMPolicy` is `continue`, so the kill never fails the scope with the result `oom-kill`.
// KDE Plasma's out-of-memory notifier reports each unit that ends with that result as a host low on memory.
// The command keeps its pid and process group, so a group kill still reaches it.
// Where the user manager cannot cap a scope's memory, `RLIMIT_DATA` limits each process's data segment instead.
// An allocation past that limit fails in the process, and the call reports no kill.
// On Windows the command runs in a kill-on-close job object with a job memory limit.
// That limit fails an allocation the same way.
// RunMemoryCappedVia runs a command under the same cap through a caller's own start and wait.
// A cap that could not be set returns an error wrapping `ErrMemoryCap`.
// So does a scope that never ran the command, or one whose `memory.max` is not the cap.
// MemoryCapCoversTree reports whether the cap bounds a command's whole tree as one.
// Only the data segment limit does not.
// proc cannot log, so the caller logs the command it hands over and its end.
// The one-time probe's `systemd-run` spawn goes unlogged; a caller sees only its outcome, through MemoryCapCoversTree.
//
// CapProcessMemory moves a process that is already running into a new transient scope with a memory cap and no swap.
// It asks the systemd user manager over D-Bus through `busctl`.
// It waits until the process has joined the scope and checks the scope's `memory.max`.
// The scope's `OOMPolicy` is `continue`, so a kill at the cap ends one process and leaves the rest running.
// ProcessOOMKills reads a process's cgroup and the OOM kills its `memory.events` counts.
// OOMKillsInCgroup reads the name and pid of each process the kernel journal records as OOM-killed in a cgroup.
// It reads through `journalctl`.
// All three are Linux only, and the caller logs their spawns.
//
// IsSpawnAgain reports whether a start failed on a temporary resource shortage, EAGAIN from fork or exec.
// NoteSpawnAgain records the time of each such refusal process-wide, and LastSpawnAgain reads it back,
// so a caller that sees a later failure can judge whether a host resource shortage preceded it.
// IsSpawnBusy reports whether a start failed because the executable was open for writing, ETXTBSY.
// A deploy that rewrites the binary in place causes it.
//
// BuildServerOffEnv is the environment that stops a .NET build leaving MSBuild nodes or servers running.
package proc
