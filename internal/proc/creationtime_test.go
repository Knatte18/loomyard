// creationtime_test.go covers CreationTime, which has the same signature on both platforms.
// The file therefore carries no platform constraint.

package proc

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCreationTime pins CreationTime's per-platform answer.
// On Windows the calling process's creation time is found, and a pid no process holds reads false.
// Elsewhere nothing is found.
func TestCreationTime(t *testing.T) {
	tiergate.Require(t, 0)
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
