//go:build windows

// dobbyreopen_windows.go holds the Windows no-op of the stdio reopen, since a dobby there never re-execs.

package beamcli

// reopenStdio does nothing on Windows, where a dobby never re-execs and so never outlives the spawner's file.
func reopenStdio(string) error { return nil }
