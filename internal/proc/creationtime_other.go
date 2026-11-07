//go:build !windows

// creationtime_other.go stubs the creation-time read, which only Windows needs.

package proc

import "time"

// CreationTime reports no creation time off Windows, where the process start time is read another way.
func CreationTime(pid int) (time.Time, bool) {
	return time.Time{}, false
}
