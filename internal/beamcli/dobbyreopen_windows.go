//go:build windows

// dobbyreopen_windows.go is the Windows counterpart of the output reopen, which selfexec's absence of an exec there makes unnecessary.

package beamcli

// reopenStdio does nothing on Windows, where a dobby never re-execs and so never outlives the spawner's file.
func reopenStdio(string) error { return nil }
