// process.go declares the process-listing type every platform shares; ListProcesses itself is per-platform.

package proc

// Process is one process as ListProcesses read it.
type Process struct {
	// PID is the process id.
	PID int
	// StartTime is the process's start time in the form StartTime returns.
	StartTime string
	// Exe is the target of the process's executable link as the kernel prints it, a " (deleted)" suffix included.
	Exe string
	// Cwd is the target of the process's working-directory link as the kernel prints it, a " (deleted)" suffix included.
	Cwd string
	// Argv is the process's command line, split at its NUL separators.
	Argv []string
}
