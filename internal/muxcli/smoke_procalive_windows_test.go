//go:build smoke

// smoke_procalive_windows.go exists only so smoke_test.go's
// runtime.GOOS-branching processGone compiles on Windows too: its
// GOOS == "windows" branch never calls posixProcessAlive (Windows uses
// os.Process.Wait() on a handle instead, which works for a non-child pid —
// see processGone's doc comment), so this body is unreachable in practice.
// See smoke_procalive_linux.go for the real implementation.

package muxcli

func posixProcessAlive(pid int) bool {
	panic("posixProcessAlive is POSIX-only and unreachable on windows (processGone never calls it there)")
}
