//go:build integration

// notify_integration_test.go proves NotifyPrime over a real hub: a notice from a task worktree lands in the prime's notice queue and nowhere else, and with no orch strand recorded nothing is queued.

package orchcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

func TestNotifyPrime_QueuesInThePrimeOnly(t *testing.T) {
	t.Parallel()

	const line = "Shuttle notice: hold"
	tests := []struct {
		name        string
		strand      string
		wantNotices int
	}{
		{"strand recorded queues one notice", "g1", 1},
		{"no strand recorded queues nothing", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.NewHub(t, ".")
			hubforge.AddPair(t, h, "notify-task")
			task, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("notify-task"))
			if err != nil {
				t.Fatalf("ResolveWorktree task: %v", err)
			}
			primePaths := PrimePaths(h.Location)
			if tt.strand != "" {
				if err := orchengine.SaveState(primePaths, orchengine.State{Phase: orchengine.PhaseIdle, Strand: tt.strand}); err != nil {
					t.Fatalf("SaveState: %v", err)
				}
			}

			if err := NotifyPrime(task, line); err != nil {
				t.Fatalf("NotifyPrime() error = %v; want nil", err)
			}

			assertNotices(t, primePaths.NoticesDir, tt.wantNotices, line)
			assertNotices(t, PrimePaths(task).NoticesDir, 0, line)
		})
	}
}

// assertNotices asserts dir holds exactly want notice files, each holding line.
// An absent dir holds none.
func assertNotices(t *testing.T, dir string, want int, line string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != want {
		t.Fatalf("%s holds %d notice files; want %d", dir, len(entries), want)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read notice: %v", err)
		}
		if string(data) != line {
			t.Errorf("notice = %q; want %q", data, line)
		}
	}
}
