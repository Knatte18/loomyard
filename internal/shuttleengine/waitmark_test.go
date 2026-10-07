// waitmark_test.go covers ReadWaitMarker over hand-written run directories, with the liveness seam replaced so no process is spawned.
// Replacing the seam is process-global state, so these tests do not run in parallel.

package shuttleengine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeWaitMarkerFile writes body as the wait marker of the run directory named name under root.
func writeWaitMarkerFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, waitMarkerFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadWaitMarker(t *testing.T) {
	livePID := 4242
	prev := isAlive
	isAlive = func(pid int) bool { return pid == livePID }
	t.Cleanup(func() { isAlive = prev })

	started := time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC)
	liveBody := "kind: gate review\nstarted: 2026-10-03T09:15:00Z\npid: 4242\n"
	deadBody := "kind: background shells\nstarted: 2026-10-03T09:00:00Z\npid: 4343\n"
	tests := []struct {
		name  string
		runs  map[string]string
		want  WaitMarker
		found bool
	}{
		{name: "no runs"},
		{name: "a run directory with no marker", runs: map[string]string{"run-a": ""}},
		{
			name:  "a live marker",
			runs:  map[string]string{"run-a": liveBody},
			want:  WaitMarker{Kind: "gate review", Started: started, PID: livePID},
			found: true,
		},
		{
			name:  "a dead-pid marker is skipped in favour of a live one in another run",
			runs:  map[string]string{"run-a": deadBody, "run-b": liveBody},
			want:  WaitMarker{Kind: "gate review", Started: started, PID: livePID},
			found: true,
		},
		{name: "only a dead-pid marker reads as absent", runs: map[string]string{"run-a": deadBody}},
		{
			name:  "a torn marker is skipped",
			runs:  map[string]string{"run-a": "kind: [unterminated", "run-b": liveBody},
			want:  WaitMarker{Kind: "gate review", Started: started, PID: livePID},
			found: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range tt.runs {
				if body == "" {
					if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
						t.Fatal(err)
					}
					continue
				}
				writeWaitMarkerFile(t, root, name, body)
			}

			got, found, err := ReadWaitMarker(Config{RunDir: root}, t.TempDir())

			if err != nil || found != tt.found {
				t.Fatalf("ReadWaitMarker = (_, %v, %v); want (_, %v, nil)", found, err, tt.found)
			}
			if found && (got.Kind != tt.want.Kind || !got.Started.Equal(tt.want.Started) || got.PID != tt.want.PID) {
				t.Errorf("ReadWaitMarker = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestReadWaitMarker_AbsentRunDirectoryRootReadsAsNone(t *testing.T) {
	t.Parallel()

	_, found, err := ReadWaitMarker(Config{RunDir: filepath.Join(t.TempDir(), "absent")}, t.TempDir())
	if found || err != nil {
		t.Errorf("ReadWaitMarker = (_, %v, %v); want (_, false, nil)", found, err)
	}
}
