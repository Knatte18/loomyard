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
