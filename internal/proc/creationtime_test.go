// creationtime_test.go covers CreationTime, defined with the same signature on both platforms, so this file needs no //go:build tag.

package proc

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// TestCreationTime pins CreationTime's per-platform answer: on Windows the calling process's creation time is found and a pid no process holds reads false, elsewhere nothing is found.
func TestCreationTime(t *testing.T) {
	t.Parallel()

	created, ok := CreationTime(os.Getpid())
	if runtime.GOOS != "windows" {
		if ok || !created.IsZero() {
			t.Errorf("CreationTime(self) = %v, %v; want zero time, false", created, ok)
		}
		return
	}
	if !ok {
		t.Fatalf("CreationTime(self) = %v, false; want the creation time found", created)
	}
	if created.After(time.Now()) {
		t.Errorf("CreationTime(self) = %v; is after now", created)
	}
	if got, ok := CreationTime(exitedChildPID(t)); ok {
		t.Errorf("CreationTime(exited pid) = %v, true; want false", got)
	}
}
