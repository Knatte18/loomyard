// reap.go holds the waits that keep a process's children from staying zombies once they exit.

package proc

import (
	"errors"
	"os"
	"os/exec"
)

// Reap waits on the started command in the background and discards its exit,
// so a child its starter never waits on leaves no zombie once it exits.
// When the starter exits first, the child is reparented and init reaps it.
func Reap(cmd *exec.Cmd) {
	go func() { _ = cmd.Wait() }()
}

// ReapInheritedChildren waits in the background on every child the calling process has, and returns their pids.
// A process that re-execs itself keeps its children but loses the goroutines that waited on them,
// so the new image calls it before it starts a child of its own:
// every child it finds then is one the replaced image started.
// Calling it after the process has started a child of its own races the wait on that child.
// It finds none where the platform cannot list processes.
func ReapInheritedChildren() ([]int, error) {
	stats, err := ListStats()
	if errors.Is(err, errors.ErrUnsupported) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	var pids []int
	for _, entry := range stats {
		if entry.PPID != self {
			continue
		}
		child, err := os.FindProcess(entry.PID)
		if err != nil {
			continue
		}
		go func() { _, _ = child.Wait() }()
		pids = append(pids, entry.PID)
	}
	return pids, nil
}
