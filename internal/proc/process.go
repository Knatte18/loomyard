// process.go declares the process-listing type every platform shares; ListProcesses itself is per-platform.

package proc

// Process is one process with its identity, executable, working directory and command line.
type Process struct {
	// PID is the process id.
	PID int
	// StartTime is the process's start time in the form Claude's session registry records.
	StartTime string
	// Exe is the target of the process's executable link as the kernel prints it, a " (deleted)" suffix included.
	Exe string
	// Cwd is the target of the process's working-directory link as the kernel prints it, a " (deleted)" suffix included.
	Cwd string
	// Argv is the process's command line, split at its NUL separators.
	Argv []string
}

// StatEntry is one process with the fields its stat record alone supplies.
type StatEntry struct {
	// PID is the process id.
	PID int
	// PPID is the parent process id.
	PPID int
	// PGID is the process-group id.
	PGID int
	// StartTime is the process's start time in the form Claude's session registry records.
	StartTime string
	// Zombie reports whether the process has ended and awaits its parent's wait.
	Zombie bool
}
