// engineworktree_integration_test.go drives EngineWorktree over one real hub from hubforge:
// the prime, a task pair, a beam marked through the origin record, a beam known only by the board's flag,
// and a pair whose directory is gone.

package hubgeom_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEngineWorktree pins the name and kind told for each worktree of a hub.
// It pins the account directory under the Bolt's scratch tree.
// It pins the worktree names the closure lists, which leave out a pair whose directory was removed.
// A pair whose board entry carries the beam flag before any origin mark reads as a beam.
func TestEngineWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location

	const (
		taskSlug   = "ew-task"
		markedSlug = "ew-beam-marked"
		boardSlug  = "ew-beam-board"
		goneSlug   = "ew-gone"
	)
	for _, slug := range []string{taskSlug, markedSlug, boardSlug, goneSlug} {
		hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})
	}
	origin, found, err := fabricengine.ReadOriginFor(prime, markedSlug)
	if err != nil || !found {
		t.Fatalf("ReadOriginFor(%q) = found %v, error %v; want a record", markedSlug, found, err)
	}
	origin.Beam = true
	if err := fabricengine.WriteOrigin(nil, prime, markedSlug, origin); err != nil {
		t.Fatalf("WriteOrigin(%q): %v", markedSlug, err)
	}
	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"` + boardSlug + `","title":"Board beam","kind":"project","labels":["bug"],"beam":true}]}`
	if err := os.WriteFile(filepath.Join(h.BoardDir(), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	locate := func(slug string) *lymcwd.Location {
		t.Helper()
		location, err := lymcwd.Resolve(fabricengine.WorktreePath(prime, slug))
		if err != nil {
			t.Fatalf("lymcwd.Resolve(%q): %v", slug, err)
		}
		return location
	}
	wantNames := []string{prime.WorktreeName, taskSlug, markedSlug, boardSlug}
	if err := os.RemoveAll(fabricengine.WorktreePath(prime, goneSlug)); err != nil {
		t.Fatalf("remove the directory of %q: %v", goneSlug, err)
	}
	wantAccountDir := filepath.Join(fabricengine.HubScratchDir(prime.HubPath), lymdirs.ClaudeAccountsDirName)

	tests := []struct {
		name     string
		location *lymcwd.Location
		wantName string
		wantKind shuttleengine.WorktreeKind
	}{
		{"Prime", prime, prime.WorktreeName, shuttleengine.WorktreePrime},
		{"TaskPair", locate(taskSlug), taskSlug, shuttleengine.WorktreeTask},
		{"BeamMarkedThroughTheOriginRecord", locate(markedSlug), markedSlug, shuttleengine.WorktreeBeam},
		{"BeamKnownOnlyByTheBoardFlag", locate(boardSlug), boardSlug, shuttleengine.WorktreeBeam},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hubgeom.EngineWorktree(tt.location)
			if got.Name != tt.wantName || got.Kind != tt.wantKind {
				t.Errorf("EngineWorktree(%q) = name %q, kind %q; want name %q, kind %q", tt.wantName, got.Name, got.Kind, tt.wantName, tt.wantKind)
			}
			if got.AccountDir != wantAccountDir {
				t.Errorf("EngineWorktree(%q).AccountDir = %q; want %q", tt.wantName, got.AccountDir, wantAccountDir)
			}
			names, err := got.Names()
			if err != nil {
				t.Fatalf("EngineWorktree(%q).Names() error = %v; want nil", tt.wantName, err)
			}
			sort.Strings(names)
			want := append([]string(nil), wantNames...)
			sort.Strings(want)
			if !reflect.DeepEqual(names, want) {
				t.Errorf("EngineWorktree(%q).Names() = %v; want %v", tt.wantName, names, want)
			}
		})
	}
}
