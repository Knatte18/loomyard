// sample.go declares the process-tree sample type every platform shares; TreeSample itself is per-platform.

package proc

import "time"

// ProcessSample is one process of a sampled tree.
type ProcessSample struct {
	// PID is the process id.
	PID int
	// Parent is the process id of the process's parent.
	Parent int
	// Path is the process's executable path.
	Path string
	// CPU is the user plus system time the process has used.
	CPU time.Duration
	// RSS is the bytes of memory the process holds resident.
	RSS int64
}
