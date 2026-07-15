//go:build smoke

// smoke_procalive_linux.go answers "is this pid still running" on Linux via
// signal 0 (syscall.Kill with sig=0 checks existence/permission without
// actually signaling: ESRCH means gone, nil or EPERM means it still
// exists) — the POSIX-correct way to check an arbitrary pid's liveness,
// unlike os.Process.Wait(), which only ever succeeds for a true CHILD of
// the calling process (wait4/waitid return ECHILD immediately otherwise;
// see processGone's doc comment in smoke_test.go). Filename-suffixed to
// match internal/muxengine/proctree_linux.go's / proctree_windows.go's own
// GOOS-split pattern: this and smoke_procalive_windows.go provide the same
// posixProcessAlive symbol so smoke_test.go's runtime.GOOS-branching
// processGone compiles on both platforms.

package muxcli

import "syscall"

// posixProcessAlive reports whether pid names a running process, tolerating
// a caller that lacks permission to signal it (EPERM still means it
// exists).
func posixProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
