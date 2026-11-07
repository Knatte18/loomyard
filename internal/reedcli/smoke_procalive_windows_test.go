// smoke_procalive_windows_test.go exists so the runtime.GOOS branch in smoke_test.go compiles on Windows.
// That branch never calls posixProcessAlive there:
// Windows uses os.Process.Wait on a handle instead, which works for a pid that is not a child,
// so this body is unreachable in practice.
// The real implementation is in smoke_procalive_linux_test.go.

package reedcli

func posixProcessAlive(pid int) bool {
	panic("posixProcessAlive is POSIX-only and unreachable on windows (processGone never calls it there)")
}
