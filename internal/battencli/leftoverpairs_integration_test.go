// leftoverpairs_integration_test.go pins what the dobby reads and settles over the pairs no run's own teardown takes.
// It drives DobbyOrphans and MarkKeptCancels over hubforge hubs.

package battencli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
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

// seedCancelledRunWithPair adds the pair slug and seeds a committed record of a run cancelled with no cancel intent.
// With pairStands false the pair's task worktree is removed again.
func seedCancelledRunWithPair(t *testing.T, h *hubforge.Hub, slug string, pairStands bool) {
	t.Helper()
	hubforge.AddPair(t, h, slug)
	seedCancelRun(t, h, slug, battenrecipe.NameRunShed, shedengine.StateCancelled, "running · Run-Shed")
	if !pairStands {
		if err := os.RemoveAll(h.PairCodeWorktree(slug)); err != nil {
			t.Fatalf("remove the task worktree of %s: %v", slug, err)
		}
	}
}

// primeRunViews reads the runs of the prime's dobby, keyed by slug.
func primeRunViews(t *testing.T, h *hubforge.Hub) map[string]dobbyengine.RunView {
	t.Helper()
	geom := dobbyengine.Geometry{BoardDir: h.BoardDir()}
	views, err := DobbyRuns(h.Location, geom, NewDobbyParents(h.Location), filepath.Join(t.TempDir(), "pause.json"))
	if err != nil {
		t.Fatalf("DobbyRuns() error = %v; want nil", err)
	}
	bySlug := make(map[string]dobbyengine.RunView, len(views))
	for _, view := range views {
		bySlug[view.Slug] = view
	}
	return bySlug
}

func TestMarkKeptCancels_MarksOnlyACancelledRunWhosePairStands(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seedCancelledRunWithPair(t, h, "pair-stands", true)
	seedCancelledRunWithPair(t, h, "pair-gone", false)
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machinename.Name() error = %v; want nil", err)
	}
	prime, geom, parents := h.Location, dobbyengine.Geometry{BoardDir: h.BoardDir()}, NewDobbyParents(h.Location)
	if kept := primeRunViews(t, h)["pair-stands"].PairKept; kept {
		t.Fatal("DobbyRuns() PairKept of a cancelled run with no cancel intent = true before the mark; want false")
	}

	got, err := MarkKeptCancels(prime, geom, parents)

	if err != nil {
		t.Fatalf("MarkKeptCancels() error = %v; want nil", err)
	}
	if want := []string{"pair-stands"}; !slices.Equal(got, want) {
		t.Fatalf("MarkKeptCancels() = %q; want %q", got, want)
	}
	record, found := committedRecord(t, h, "pair-stands")
	if !found || record.State != shedengine.StateCancelled || record.Cancel == nil || record.Cancel.Machine != machine || record.Cancel.Remove || record.Cancel.Finish == nil || record.Cancel.Finish.Remove {
		t.Errorf("committed record of pair-stands = %+v, found %v; want state cancelled with a finished keep intent of %q", record, found, machine)
	}
	if record, found := committedRecord(t, h, "pair-gone"); !found || record.Cancel != nil {
		t.Errorf("committed record of pair-gone = %+v, found %v; want it left with no cancel intent", record, found)
	}
	views := primeRunViews(t, h)
	if view := views["pair-stands"]; !view.PairKept || view.KeptSince.IsZero() {
		t.Errorf("DobbyRuns() pair-stands PairKept, KeptSince = %v, %v; want true and a finish time", view.PairKept, view.KeptSince)
	}
	if view := views["pair-gone"]; view.PairKept {
		t.Error("DobbyRuns() pair-gone PairKept = true; want false")
	}
	if again, err := MarkKeptCancels(prime, geom, parents); err != nil || len(again) != 0 {
		t.Errorf("MarkKeptCancels() after the mark = %q, %v; want none and nil", again, err)
	}
}

func TestMarkKeptCancels_CommitsAFinishedKeepIntentThisMachineWroteButDidNotCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seedCancelledRunWithPair(t, h, "uncommitted", true)
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machinename.Name() error = %v; want nil", err)
	}
	if written, err := shedrun.WriteKeptCancel(h.Location, "uncommitted", machine, time.Now()); err != nil || !written {
		t.Fatalf("WriteKeptCancel() = %v, %v; want true, nil", written, err)
	}
	if record, _ := committedRecord(t, h, "uncommitted"); record.Cancel != nil {
		t.Fatalf("committed record = %+v; want no cancel intent before the retry", record)
	}

	got, err := MarkKeptCancels(h.Location, dobbyengine.Geometry{BoardDir: h.BoardDir()}, NewDobbyParents(h.Location))

	if err != nil {
		t.Fatalf("MarkKeptCancels() error = %v; want nil", err)
	}
	if want := []string{"uncommitted"}; !slices.Equal(got, want) {
		t.Errorf("MarkKeptCancels() = %q; want %q", got, want)
	}
	record, found := committedRecord(t, h, "uncommitted")
	if !found || record.Cancel == nil || record.Cancel.Machine != machine || record.Cancel.Finish == nil {
		t.Errorf("committed record = %+v, found %v; want the finished keep intent of %q committed", record, found, machine)
	}
}
