// processscope_windows.go stands in for the scope cap of a running process, which Windows has no systemd for.

package proc

import (
	"errors"
	"fmt"
)

// capProcessMemoryPlatform always fails: Windows has no user manager to start a scope.
func capProcessMemoryPlatform(int, string, int64) (string, error) {
	return "", fmt.Errorf("%w: %w", ErrMemoryCap, errors.ErrUnsupported)
}

// processOOMKillsPlatform always fails: Windows has no cgroup to read.
func processOOMKillsPlatform(int) (string, int, error) {
	return "", 0, errors.ErrUnsupported
}

// oomKillsInCgroupPlatform always fails: Windows has no kernel journal to read.
func oomKillsInCgroupPlatform(string) ([]OOMKill, error) {
	return nil, errors.ErrUnsupported
}
