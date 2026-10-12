// cpu.go declares the process-tree CPU sample type every platform shares; TreeCPU itself is per-platform.

package proc

import "time"

// ProcessCPU is one process of a sampled tree.
type ProcessCPU struct {
	// PID is the process id.
	PID int
	// Path is the process's executable path.
	Path string
	// CPU is the user plus system time the process has used.
	CPU time.Duration
}
