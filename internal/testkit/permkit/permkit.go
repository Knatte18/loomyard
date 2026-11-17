// Package permkit changes a test path's permission bits for the length of one test.
//
// A test that needs a read-only or unreadable path takes it from Chmod,
// so the restore that lets the test's temp tree be removed is registered in one place.
// A test binary killed before its cleanups run leaves the path as Chmod set it;
// the dead-owner sweep of the lym temp root makes such a tree owner-writable before it removes it.
// The kit imports the standard library only.
package permkit

import (
	"io/fs"
	"os"
	"testing"
)

// Chmod sets path's permission bits to mode and registers a cleanup that restores the bits path had before.
// It fails the test when path cannot be read or changed.
// The restore drops its own error, so a test may remove path itself before it ends.
func Chmod(tb testing.TB, path string, mode fs.FileMode) {
	tb.Helper()
	info, err := os.Stat(path)
	if err != nil {
		tb.Fatalf("permkit: stat %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		tb.Fatalf("permkit: chmod %s to %v: %v", path, mode, err)
	}
	tb.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
}
