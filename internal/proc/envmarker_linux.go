// envmarker_linux.go finds processes by an entry of their environment, read from /proc.

package proc

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// PIDsWithEnv returns every process whose environment holds entry, a whole `NAME=value` string.
// A process keeps the environment it started with, so a marker set for a run follows each of its processes.
// That includes one that left the run's process tree, such as a tmux server's panes.
// A process whose environment the caller may not read, or that ends during the scan, is skipped;
// an unreadable /proc is an error.
func PIDsWithEnv(entry string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("proc: list /proc: %w", err)
	}
	want := []byte(entry)
	var pids []int
	for _, dirEntry := range entries {
		pid, err := strconv.Atoi(dirEntry.Name())
		if err != nil {
			continue
		}
		environ, err := os.ReadFile("/proc/" + dirEntry.Name() + "/environ")
		if err != nil {
			continue
		}
		for field := range bytes.SplitSeq(environ, []byte{0}) {
			if bytes.Equal(field, want) {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, nil
}
