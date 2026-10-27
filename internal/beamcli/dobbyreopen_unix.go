//go:build !windows

// dobbyreopen_unix.go moves a dobby process's standard output and error onto a file off Windows.

package beamcli

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// reopenStdio opens the file at path for append and duplicates it onto this process's standard output and error.
// A dobby whose descriptors an older binary pointed at `dobby.log` so leaves that file to its own lines.
func reopenStdio(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	for _, target := range []int{int(os.Stdout.Fd()), int(os.Stderr.Fd())} {
		if err := unix.Dup2(int(file.Fd()), target); err != nil {
			return fmt.Errorf("duplicate %s onto descriptor %d: %w", path, target, err)
		}
	}
	return nil
}
