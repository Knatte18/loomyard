// leftoverpairs_integration_test.go pins what the dobby reads over the pairs no run's own teardown takes.
// It drives DobbyOrphans over hubforge hubs.

package battencli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDobbyOrphans_ListsOnlyAPairNoBoardEntryAndNoRunDirectoryNames(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name            string
		registerProcess bool
		wantProcessLive bool
	}{
		{"NoProcessRegistered", false, false},
		{"ProcessRegisteredUnderTheSlug", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			for _, slug := range []string{"on-board", "has-run-directory", "orphan"} {
				hubforge.AddPair(t, h, slug)
			}
			writeHubBoard(t, h, `{"version":1,"entries":[{"id":0,"slug":"on-board","title":"B","kind":"task","labels":["bug"],"status":"draft"}]}`)
			if err := os.MkdirAll(shedrun.RunDir(h.Location, "has-run-directory"), 0o755); err != nil {
				t.Fatalf("create the run directory: %v", err)
			}
			if tt.registerProcess {
				if err := runproc.Register(fabricengine.RunProcDir(h.Path), "orphan", runproc.SelfRecord(runproc.RoleBatten)); err != nil {
					t.Fatalf("register a process under the orphan: %v", err)
				}
			}

			got, err := DobbyOrphans(h.Location, h.BoardDir())

			if err != nil {
				t.Fatalf("DobbyOrphans() error = %v; want nil", err)
			}
			if len(got) != 1 || got[0].Slug != "orphan" {
				t.Fatalf("DobbyOrphans() = %+v; want the pair named orphan alone", got)
			}
			if got[0].LastCommit.IsZero() {
				t.Error("DobbyOrphans() LastCommit = the zero time; want the newer tip's commit time")
			}
			if got[0].ProcessLive != tt.wantProcessLive {
				t.Errorf("DobbyOrphans() ProcessLive = %v; want %v", got[0].ProcessLive, tt.wantProcessLive)
			}
		})
	}
}

// writeHubBoard overwrites the hub board's board.json with content.
func writeHubBoard(t *testing.T, h *hubforge.Hub, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}
