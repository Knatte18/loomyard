//go:build tier1

// dobbypairanchor_test.go pins that dobby's two pair-reading callers place a task pair at the pair's own anchor, over a hubforge hub.

package beamcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// commitAnchorMarker commits a .lym-anchor holding anchor, and an empty file under the anchor directory, in the pair's records worktree.
func commitAnchorMarker(t *testing.T, h *hubforge.Hub, slug, anchor string) {
	t.Helper()

	root := h.PairRecordsSibling(slug)
	if err := os.WriteFile(filepath.Join(root, lymcwd.AnchorFileName), []byte(anchor+"\n"), 0o644); err != nil {
		t.Fatalf("write the marker of %s: %v", slug, err)
	}
	if !strings.HasPrefix(anchor, "..") {
		moved := filepath.Join(root, anchor)
		if err := os.MkdirAll(moved, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", moved, err)
		}
		if err := os.WriteFile(filepath.Join(moved, ".keep"), nil, 0o644); err != nil {
			t.Fatalf("write the placeholder of %s: %v", slug, err)
		}
	}
	gitkit.Git(t, root, "add", "-A")
	gitkit.Git(t, root, "commit", "-m", "anchor "+slug+" at "+anchor)
}

// seedBattenRun gives slug a batten seed and status under the prime, so a pass reads it as a run of the prime's.
func seedBattenRun(t *testing.T, prime *lymcwd.Location, slug string) {
	t.Helper()

	for path, content := range map[string]string{
		shedrun.SeedFile(prime, slug):   `{"recipe":"batten","driver":"go"}`,
		shedrun.StatusFile(prime, slug): `{}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func TestDobbyRuns_PairAtItsOwnAnchor(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "moved")
	hubforge.AddPair(t, h, "broken")
	prime := h.Location
	for _, slug := range []string{"moved", "broken"} {
		seedBattenRun(t, prime, slug)
	}
	commitAnchorMarker(t, h, "moved", "sub")

	movedStatus := shedrun.StatusFile(&lymcwd.Location{HubPath: h.Path, WorktreeName: "moved", AnchorRel: "sub"}, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(movedStatus), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(movedStatus), err)
	}
	if err := os.WriteFile(movedStatus, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write %s: %v", movedStatus, err)
	}
	brokenStatus := shedrun.StatusFile(&lymcwd.Location{HubPath: h.Path, WorktreeName: "broken", AnchorRel: "."}, shedrun.SelfRunID)

	pauseRecord := filepath.Join(t.TempDir(), "pause.json")
	geom := dobbyengine.Geometry{}
	newParents := func() *battencli.DobbyParents { return battencli.NewDobbyParents(prime) }

	// The tests share one hub and its marker state, so the cases run in sequence.
	t.Run("ChildStatusAndWatchDirUnderThePairsAnchor", func(t *testing.T) {
		views, err := battencli.DobbyRuns(prime, geom, newParents(), pauseRecord)
		if err != nil {
			t.Fatalf("dobbyRuns() = _, %v; want nil", err)
		}
		modified := map[string]bool{}
		for _, view := range views {
			modified[view.Slug] = !view.ChildStatusModified.IsZero()
		}
		if want := map[string]bool{"moved": true, "broken": false}; len(modified) != len(want) || modified["moved"] != want["moved"] || modified["broken"] != want["broken"] {
			t.Errorf("dobbyRuns() child status seen = %v; want %v", modified, want)
		}

		dirs := dobbyWatchDirs(prime, pauseRecord, h.BoardDir())()
		if _, listed := dirs[filepath.Dir(movedStatus)]; !listed {
			t.Errorf("dobbyWatchDirs() = %v; want the moved pair's status directory %s", dirs, filepath.Dir(movedStatus))
		}
		if _, listed := dirs[filepath.Dir(brokenStatus)]; !listed {
			t.Errorf("dobbyWatchDirs() = %v; want the other pair's status directory %s", dirs, filepath.Dir(brokenStatus))
		}
		if _, listed := dirs[filepath.Dir(shedrun.StatusFile(&lymcwd.Location{HubPath: h.Path, WorktreeName: "moved", AnchorRel: "."}, shedrun.SelfRunID))]; listed {
			t.Errorf("dobbyWatchDirs() = %v; want no status directory of the moved pair at the prime's anchor", dirs)
		}
	})

	t.Run("InvalidMarkerRefusesTheRunsAndLeavesThePairOutOfTheWatch", func(t *testing.T) {
		commitAnchorMarker(t, h, "broken", "../out")

		views, err := battencli.DobbyRuns(prime, geom, newParents(), pauseRecord)
		if !errors.Is(err, lymcwd.ErrInvalidAnchor) || !strings.Contains(err.Error(), "broken") {
			t.Fatalf("dobbyRuns() = %v, %v; want an error naming the pair and wrapping ErrInvalidAnchor", views, err)
		}

		dirs := dobbyWatchDirs(prime, pauseRecord, h.BoardDir())()
		if _, listed := dirs[filepath.Dir(movedStatus)]; !listed {
			t.Errorf("dobbyWatchDirs() = %v; want the moved pair's status directory %s", dirs, filepath.Dir(movedStatus))
		}
		if _, listed := dirs[filepath.Dir(brokenStatus)]; listed {
			t.Errorf("dobbyWatchDirs() = %v; want the pair with the invalid marker left out", dirs)
		}
	})
}
