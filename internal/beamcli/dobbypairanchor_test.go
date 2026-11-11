// dobbypairanchor_test.go pins that dobby's two pair-reading callers place a task pair at the pair's own anchor.
// It also pins what DobbyRuns reads of a run's place against its Land row, over a hubforge hub.

package beamcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// commitAnchorMarker commits a .lym-anchor holding anchor, in the pair's records worktree.
// It also commits an empty file under the anchor directory unless anchor starts with `..`.
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
	tiergate.Require(t, 1)
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

		requestPath := filepath.Join(h.Path, "hibernate", "request.json")
		dirs := dobbyWatchDirs(prime, pauseRecord, h.BoardDir(), requestPath)()
		if names := dirs[filepath.Dir(requestPath)]; len(names) != 1 || names[0] != "request.json" {
			t.Errorf("dobbyWatchDirs() = %v; want the hibernate request's directory with its file name", dirs)
		}
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

		dirs := dobbyWatchDirs(prime, pauseRecord, h.BoardDir(), "")()
		if _, listed := dirs[filepath.Dir(movedStatus)]; !listed {
			t.Errorf("dobbyWatchDirs() = %v; want the moved pair's status directory %s", dirs, filepath.Dir(movedStatus))
		}
		if _, listed := dirs[filepath.Dir(brokenStatus)]; listed {
			t.Errorf("dobbyWatchDirs() = %v; want the pair with the invalid marker left out", dirs)
		}
	})

	t.Run("HoldCallerEntryReadsHeldAndOwnerEntryDoesNot", func(t *testing.T) {
		commitAnchorMarker(t, h, "broken", ".")
		for _, row := range []struct {
			caller   string
			wantHeld bool
		}{
			{pauserecord.HoldCaller, true},
			{pauserecord.OwnerCaller, false},
		} {
			content, err := json.Marshal(pauserecord.Record{Live: []pauserecord.Entry{{Scope: "moved", Caller: row.caller, PausedAt: time.Now()}}})
			if err != nil {
				t.Fatalf("marshal the pause record: %v", err)
			}
			if err := os.WriteFile(pauseRecord, content, 0o644); err != nil {
				t.Fatalf("write %s: %v", pauseRecord, err)
			}
			views, err := battencli.DobbyRuns(prime, geom, newParents(), pauseRecord)
			if err != nil {
				t.Fatalf("dobbyRuns() = _, %v; want nil", err)
			}
			for _, view := range views {
				if view.Slug == "moved" && (!view.Covered || view.Held != row.wantHeld) {
					t.Errorf("caller %q: moved view Covered, Held = %v, %v; want true, %v", row.caller, view.Covered, view.Held, row.wantHeld)
				}
				if view.Slug == "broken" && (view.Covered || view.Held) {
					t.Errorf("caller %q: broken view Covered, Held = %v, %v; want false, false", row.caller, view.Covered, view.Held)
				}
			}
		}
	})
}

func TestDobbyLandings_CountsOnlyThisMachine(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location
	const landDone = `{"history":[{"producer":"Land","outcome":"done","output":"","at":"2026-10-27T10:00:00Z"%s}]}`
	for slug, machine := range map[string]string{"here": `,"machine":"m1"`, "renamed": `,"machine":"m0"`, "elsewhere": `,"machine":"m2"`, "unstamped": ""} {
		seedBattenRun(t, prime, slug)
		status := fmt.Sprintf(landDone, machine)
		if err := os.WriteFile(shedrun.StatusFile(prime, slug), []byte(status), 0o644); err != nil {
			t.Fatalf("write the status of %s: %v", slug, err)
		}
	}

	got, err := battencli.DobbyLandings(prime, dobbyengine.Geometry{}, battencli.NewDobbyParents(prime), []string{"m1", "m0"})
	slices.Sort(got)

	if err != nil || !reflect.DeepEqual(got, []string{"here", "renamed"}) {
		t.Errorf("DobbyLandings([m1 m0]) = %v, %v; want [here renamed], nil", got, err)
	}
}

func TestDobbyRuns_ReadsWhetherAPausedRunsWorkReachedItsParent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location
	type fixture struct{ batten, child string }
	fixtures := map[string]fixture{
		"teardown":     {batten: `{"state":"paused","current_producer":"Worktree-Teardown"}`},
		"finalizedkid": {batten: `{"state":"paused","current_producer":"Run-Shed"}`, child: `{"state":"paused","current_producer":"Finalize","history":[{"producer":"Finalize","outcome":"done","output":"","at":"2026-10-31T10:00:00Z"}]}`},
		"midrun":       {batten: `{"state":"paused","current_producer":"Run-Shed"}`},
		"running":      {batten: `{"state":"running","current_producer":"Worktree-Teardown"}`},
	}
	for slug, f := range fixtures {
		hubforge.AddPair(t, h, slug)
		seedBattenRun(t, prime, slug)
		if err := os.WriteFile(shedrun.StatusFile(prime, slug), []byte(f.batten), 0o644); err != nil {
			t.Fatalf("write the status of %s: %v", slug, err)
		}
		if f.child == "" {
			continue
		}
		childStatus := shedrun.StatusFile(&lymcwd.Location{HubPath: h.Path, WorktreeName: slug, AnchorRel: "."}, shedrun.SelfRunID)
		if err := os.MkdirAll(filepath.Dir(childStatus), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(childStatus), err)
		}
		if err := os.WriteFile(childStatus, []byte(f.child), 0o644); err != nil {
			t.Fatalf("write the child status of %s: %v", slug, err)
		}
	}

	views, err := battencli.DobbyRuns(prime, dobbyengine.Geometry{}, battencli.NewDobbyParents(prime), filepath.Join(t.TempDir(), "pause.json"))
	if err != nil {
		t.Fatalf("DobbyRuns() = _, %v; want nil", err)
	}
	got := map[string]bool{}
	for _, view := range views {
		got[view.Slug] = view.ReachedParent
	}
	want := map[string]bool{"teardown": true, "finalizedkid": true, "midrun": false, "running": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DobbyRuns() ReachedParent = %v; want %v", got, want)
	}
}

func TestDobbyRuns_ReadsTheLandOutcomeAndTheLandingCommit(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location
	statuses := map[string]string{
		"ahead":      `{"state":"running","current_producer":"Run-Shed"}`,
		"landed":     `{"state":"done","current_producer":"Worktree-Teardown","history":[{"producer":"Land","outcome":"done","output":"","at":"2026-10-31T10:00:00Z"}]}`,
		"unlanded":   `{"state":"done","current_producer":"Worktree-Teardown"}`,
		"unreadable": `{"state":"done","current_producer":"Worktree-Teardown","history":[{"producer":"Land","outcome":"done","output":"","at":"2026-10-31T10:00:00Z"}]}`,
	}
	for slug, status := range statuses {
		hubforge.AddPair(t, h, slug)
		seedBattenRun(t, prime, slug)
		if err := os.WriteFile(shedrun.StatusFile(prime, slug), []byte(status), 0o644); err != nil {
			t.Fatalf("write the status of %s: %v", slug, err)
		}
	}
	if err := shedrun.WriteLandedCommit(prime, "landed", "abc123"); err != nil {
		t.Fatalf("WriteLandedCommit() = %v; want nil", err)
	}

	// A directory where the landed-commit file belongs makes the read fail for a reason other than a missing file.
	if err := os.MkdirAll(shedrun.LandedCommitFile(prime, "unreadable"), 0o755); err != nil {
		t.Fatalf("make the landed-commit path a directory: %v", err)
	}

	views, err := battencli.DobbyRuns(prime, dobbyengine.Geometry{}, battencli.NewDobbyParents(prime), filepath.Join(t.TempDir(), "pause.json"))
	if err != nil {
		t.Fatalf("DobbyRuns() = _, %v; want nil", err)
	}
	type landing struct {
		pastLand, landed bool
		commit           string
	}
	got := map[string]landing{}
	for _, view := range views {
		got[view.Slug] = landing{view.PastLand, view.Landed, view.LandingCommit}
	}
	want := map[string]landing{"ahead": {}, "landed": {true, true, "abc123"}, "unlanded": {pastLand: true}, "unreadable": {pastLand: true, landed: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DobbyRuns() land readings = %+v; want %+v", got, want)
	}
}
