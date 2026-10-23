//go:build !windows

// creationtime_other.go stubs the creation-time read, which only Windows needs.

package proc

import "time"

// CreationTime reports no creation time off Windows, where a process's start time is read through StartTime instead.
func CreationTime(pid int) (time.Time, bool) {
	return time.Time{}, false
}
