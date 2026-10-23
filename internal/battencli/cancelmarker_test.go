// cancelmarker_test.go pins the cancel marker's record, its writer-liveness reading and the refusal words a reading earns.

package battencli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// TestCancelMarker walks one marker through its life over a temporary location: absent, taken, contended, left by a dead writer, taken over, removed and torn.
func TestCancelMarker(t *testing.T) {
	t.Parallel()

	location := locationkit.Location(t.TempDir(), "prime", ".")
	const slug = "slug"
	const takeWait = 200 * time.Millisecond

	reading, err := readCancelMarker(location, slug)
	if err != nil || reading.Present {
		t.Fatalf("readCancelMarker(no marker) = %+v, %v; want not present", reading, err)
	}

	held, err := takeCancelMarker(location, slug, takeWait)
	if err != nil {
		t.Fatalf("takeCancelMarker(no marker) error = %v, want nil", err)
	}
	if !held.fresh {
		t.Error("takeCancelMarker(no marker).fresh = false, want true")
	}

	reading, err = readCancelMarker(location, slug)
	if err != nil || !reading.Present || !reading.WriterAlive {
		t.Fatalf("readCancelMarker(taken) = %+v, %v; want present with the writer alive", reading, err)
	}
	if reading.Marker.PID != os.Getpid() || reading.Marker.StartedAt == "" {
		t.Errorf("readCancelMarker(taken).Marker = %+v, want this process's pid and a start time", reading.Marker)
	}
	wantAlive := fmt.Sprintf("way forward: wait for the cancel (pid %d) to end, seeing it with \"ps -p %d\"", os.Getpid(), os.Getpid())
	if got := cancelMarkerRefusal(slug, reading).Error(); !strings.Contains(got, wantAlive) || !strings.HasSuffix(got, "then run the verb again") {
		t.Errorf("cancelMarkerRefusal(writer alive) = %q, want it to end with the clause containing %q", got, wantAlive)
	}

	if _, err := takeCancelMarker(location, slug, 0); err == nil || !strings.Contains(err.Error(), "way forward: wait for the cancel") {
		t.Errorf("takeCancelMarker(held, no wait) error = %v, want the live-cancel refusal", err)
	}

	if err := held.release(false); err != nil {
		t.Fatalf("release(false): %v", err)
	}
	reading, err = readCancelMarker(location, slug)
	if err != nil || !reading.Present || reading.WriterAlive {
		t.Fatalf("readCancelMarker(writer gone) = %+v, %v; want present with the writer gone", reading, err)
	}
	wantGone := `way forward: run "lyx batten cancel slug" from the prime to finish it`
	if got := cancelMarkerRefusal(slug, reading).Error(); !strings.HasSuffix(got, wantGone) {
		t.Errorf("cancelMarkerRefusal(writer gone) = %q, want it to end with %q", got, wantGone)
	}

	held, err = takeCancelMarker(location, slug, takeWait)
	if err != nil {
		t.Fatalf("takeCancelMarker(dead writer's marker) error = %v, want nil", err)
	}
	if held.fresh {
		t.Error("takeCancelMarker(dead writer's marker).fresh = true, want false")
	}
	if err := held.release(true); err != nil {
		t.Fatalf("release(true): %v", err)
	}
	if _, statErr := os.Stat(shedrun.CancelMarker(location, slug)); !os.IsNotExist(statErr) {
		t.Errorf("stat of the marker after release(true) = %v, want not exist", statErr)
	}
	if reading, err = readCancelMarker(location, slug); err != nil || reading.Present {
		t.Errorf("readCancelMarker(removed) = %+v, %v; want not present", reading, err)
	}

	if err := os.WriteFile(shedrun.CancelMarker(location, slug), []byte(`{"pid":`), 0o644); err != nil {
		t.Fatal(err)
	}
	reading, err = readCancelMarker(location, slug)
	if err != nil || !reading.Present || reading.WriterAlive || reading.Marker != (cancelMarker{}) {
		t.Errorf("readCancelMarker(undecodable) = %+v, %v; want present with empty fields", reading, err)
	}
}

// TestJudgeCancelMarker pins each reading of the marker's three probes, the writer removing the marker between the first stat and the shared lock included.
func TestJudgeCancelMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                      string
		presentBefore, lockFree, presentUnderLock bool
		wantPresent, wantWriterAlive              bool
	}{
		{"AbsentBeforeReadsNotPresent", false, true, false, false, false},
		{"PresentWithTheLockHeldElsewhereReadsWriterAlive", true, false, false, true, true},
		{"PresentWithTheLockFreeAndStillThereReadsWriterGone", true, true, true, true, false},
		{"RemovedBetweenTheStatAndTheSharedLockReadsNotPresent", true, true, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			present, writerAlive := judgeCancelMarker(tt.presentBefore, tt.lockFree, tt.presentUnderLock)
			if present != tt.wantPresent || writerAlive != tt.wantWriterAlive {
				t.Errorf("judgeCancelMarker(%v, %v, %v) = %v, %v; want %v, %v",
					tt.presentBefore, tt.lockFree, tt.presentUnderLock, present, writerAlive, tt.wantPresent, tt.wantWriterAlive)
			}
		})
	}
}
