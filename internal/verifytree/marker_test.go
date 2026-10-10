// marker_test.go covers ReadMarker without spawning anything.

package verifytree

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withAlive replaces the liveness seam for one test.
func withAlive(t *testing.T, alive bool) {
	t.Helper()
	prev := isAlive
	isAlive = func(int) bool { return alive }
	t.Cleanup(func() { isAlive = prev })
}

func TestReadMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "running.yaml")

	t.Run("absent", func(t *testing.T) {
		_, ok, err := ReadMarker(path)
		if ok || err != nil {
			t.Fatalf("ReadMarker = (_, %v, %v); want (_, false, nil)", ok, err)
		}
	})

	t.Run("live pid", func(t *testing.T) {
		withAlive(t, true)
		want := Marker{Site: "Finalize", Attempt: 1, Command: "go test ./...", Started: time.Now().Round(time.Second), PID: 4242}
		if err := writeMarker(path, want); err != nil {
			t.Fatal(err)
		}
		got, ok, err := ReadMarker(path)
		if !ok || err != nil {
			t.Fatalf("ReadMarker = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		if got.Site != want.Site || got.Attempt != want.Attempt || got.Command != want.Command || got.PID != want.PID || !got.Started.Equal(want.Started) {
			t.Errorf("ReadMarker = %+v; want %+v", got, want)
		}
	})

	t.Run("state and wait start round-trip", func(t *testing.T) {
		withAlive(t, true)
		want := Marker{Site: "Publish", Command: "go test ./...", Started: time.Now().Round(time.Second), PID: 4242, State: MarkerStateWaiting, WaitStarted: time.Now().Round(time.Second)}
		if err := writeMarker(path, want); err != nil {
			t.Fatal(err)
		}
		got, ok, err := ReadMarker(path)
		if !ok || err != nil || got.State != MarkerStateWaiting || !got.WaitStarted.Equal(want.WaitStarted) {
			t.Fatalf("ReadMarker = (%+v, %v, %v); want state %q with wait start %v", got, ok, err, MarkerStateWaiting, want.WaitStarted)
		}
	})

	t.Run("a marker without a state reads as running", func(t *testing.T) {
		withAlive(t, true)
		legacy := "site: Finalize\nattempt: 0\ncommand: go test ./...\nstarted: 2026-01-01T00:00:00Z\npid: 4242\n"
		if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok, err := ReadMarker(path)
		if !ok || err != nil || got.State != MarkerStateRunning {
			t.Fatalf("ReadMarker = (%+v, %v, %v); want state %q", got, ok, err, MarkerStateRunning)
		}
	})

	t.Run("dead pid", func(t *testing.T) {
		withAlive(t, false)
		if err := writeMarker(path, Marker{Site: "Publish", PID: 4242}); err != nil {
			t.Fatal(err)
		}
		_, ok, err := ReadMarker(path)
		if ok || err != nil {
			t.Fatalf("ReadMarker = (_, %v, %v); want (_, false, nil)", ok, err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("site: [unterminated"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := ReadMarker(path); ok || err == nil {
			t.Fatalf("ReadMarker = (_, %v, %v); want (_, false, error)", ok, err)
		}
	})
}
