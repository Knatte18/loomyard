// smoke_procalive_linux_test.go answers whether a pid is still running on Linux, via signal 0.
// Signal 0 checks existence without signaling: ESRCH means gone, while nil or EPERM means it still exists.
// This is the POSIX-correct check for an arbitrary pid;
// os.Process.Wait returns ECHILD at once for a pid that is not a child.
// Both this file and smoke_procalive_windows_test.go define posixProcessAlive,
// so the runtime.GOOS branch in smoke_test.go compiles on either platform.

package reedcli

import "syscall"

// posixProcessAlive reports whether pid names a running process.
func posixProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
