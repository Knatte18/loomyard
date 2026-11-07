// projectrecord_test.go covers where a run's decision record lives by its recipe.
// It also covers the project records a member's run reads from the beams above its pair.

package loomcli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRunRecordOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	location := &lymcwd.Location{HubPath: filepath.Join("hub"), WorktreeName: "a-beam", AnchorRel: "."}

	tests := []struct {
		recipe      string
		wantPath    string
		wantRel     string
		wantProject bool
		// wantUnderLym is the record path inside _lym that the restore and differs ways forward name.
		wantUnderLym string
	}{
		{shedrun.RecipePawl, loomengine.ProjectDecisionRecord(location, "a-beam"), loomengine.ProjectDecisionRecordRel("a-beam"), true, "project/a-beam/decision-record.md"},
		{shedrun.RecipeDarn, loomengine.DiscussionDecisionRecord(location), loomengine.DiscussionDecisionRecordRel(), false, "discussion/decision-record.md"},
		{shedrun.RecipeTapestry, loomengine.DiscussionDecisionRecord(location), loomengine.DiscussionDecisionRecordRel(), false, "discussion/decision-record.md"},
	}
	for _, tt := range tests {
		t.Run(tt.recipe, func(t *testing.T) {
			t.Parallel()
			got := runRecordOf(location, tt.recipe)
			if got.path != tt.wantPath || got.rel != tt.wantRel || got.project != tt.wantProject {
				t.Errorf("runRecordOf(%s) = %q, %q, project %v; want %q, %q, project %v", tt.recipe, got.path, got.rel, got.project, tt.wantPath, tt.wantRel, tt.wantProject)
			}
			if filepath.Dir(got.path) != got.dir || filepath.Dir(got.supportLog) != got.dir {
				t.Errorf("runRecordOf(%s) record %q and support log %q; want both in %q", tt.recipe, got.path, got.supportLog, got.dir)
			}
			underLym, err := filepath.Rel(lymdirs.LymDirName, got.rel)
			if err != nil || filepath.ToSlash(underLym) != tt.wantUnderLym {
				t.Errorf("runRecordOf(%s) record %q inside _lym = %q, %v; want %q", tt.recipe, got.rel, underLym, err, tt.wantUnderLym)
			}
		})
	}
}

// TestProjectRecordPaths pins that a member reads the record of every beam above it, nearest first;
// from each beam's own worktree, and skips a beam whose record is absent.
func TestProjectRecordPaths(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	writeBeamBoard(t, hub)
	record := writeBeamRecords(t, hub, "outer", "inner")

	tests := []struct {
		name     string
		worktree string
		want     []string
	}{
		{name: "AMemberReadsEveryBeamAboveItNearestFirst", worktree: "member", want: []string{record("inner"), record("outer")}},
		{name: "ANestedBeamReadsTheBeamAboveIt", worktree: "inner", want: []string{record("outer")}},
		{name: "AnEntryUnderNoBeamReadsNone", worktree: "loose"},
		{name: "AnEntryTheBoardLacksReadsNone", worktree: "nobody"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := projectRecordPaths(&lymcwd.Location{HubPath: hub, WorktreeName: tt.worktree, AnchorRel: "."})
			if err != nil {
				t.Fatalf("projectRecordPaths(%s) error = %v; want nil", tt.worktree, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("projectRecordPaths(%s) = %q; want %q", tt.worktree, got, tt.want)
			}
		})
	}

	t.Run("ABeamWithNoRecordIsLeftOut", func(t *testing.T) {
		t.Parallel()
		other := t.TempDir()
		writeBeamBoard(t, other)
		got, err := projectRecordPaths(&lymcwd.Location{HubPath: other, WorktreeName: "member", AnchorRel: "."})
		if err != nil || len(got) != 0 {
			t.Errorf("projectRecordPaths(member) with no records = %q, %v; want none, nil", got, err)
		}
	})
}

// writeBeamBoard writes a board holding two nested beams, a member of the inner beam and a task under no beam into hub.
// A board write cannot make a beam, so the entries are written to the board file directly.
func writeBeamBoard(t *testing.T, hub string) {
	t.Helper()
	board := `{"version":1,"entries":[` +
		`{"id":1,"slug":"outer","title":"Outer","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":2,"slug":"inner","title":"Inner","kind":"project","labels":["bug"],"beam":true,"project":"outer"},` +
		`{"id":3,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"inner"},` +
		`{"id":4,"slug":"loose","title":"Loose","kind":"task","labels":["bug"]}]}`
	if err := os.MkdirAll(fabricengine.BoardDir(hub), 0o755); err != nil {
		t.Fatalf("mkdir board: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(hub), "board.json"), []byte(board), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
}

// writeBeamRecords writes a project record into the worktree of each beam slug under hub.
// It returns the lookup of a beam's record path.
func writeBeamRecords(t *testing.T, hub string, slugs ...string) func(slug string) string {
	t.Helper()
	record := func(slug string) string {
		t.Helper()
		beam, err := lymcwd.NamedWorktree(hub, slug)
		if err != nil {
			t.Fatalf("NamedWorktree(%s) error = %v", slug, err)
		}
		return loomengine.ProjectDecisionRecord(beam, slug)
	}
	for _, slug := range slugs {
		path := record(slug)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("## Goal\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return record
}

// TestWithProjectDirective pins that a member under a beam with a record gets a directive naming that record.
// A pair under no beam keeps the marker empty and every other table value untouched.
func TestWithProjectDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hub := t.TempDir()
	writeBeamBoard(t, hub)
	record := writeBeamRecords(t, hub, "inner")
	stencilsDir := stencilkit.Seed(t)

	tests := []struct {
		name     string
		worktree string
		// wantRecord is the record path the directive names, empty when the directive must be empty.
		wantRecord string
	}{
		{name: "AMemberUnderABeamWithARecordGetsADirectiveNamingIt", worktree: "member", wantRecord: record("inner")},
		{name: "APairUnderNoBeamKeepsTheMarkerEmpty", worktree: "loose"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			table := seatengine.Table{Values: map[string]string{"slug": tt.worktree}, Optional: []string{loomengine.ProjectDirectiveMarker}}
			got, err := withProjectDirective(table, &lymcwd.Location{HubPath: hub, WorktreeName: tt.worktree, AnchorRel: "."}, stencilsDir)
			if err != nil {
				t.Fatalf("withProjectDirective(%s) error = %v; want nil", tt.worktree, err)
			}
			directive := got.Values[loomengine.ProjectDirectiveMarker]
			if tt.wantRecord == "" {
				if directive != "" {
					t.Errorf("withProjectDirective(%s) directive = %q; want empty", tt.worktree, directive)
				}
			} else if !strings.Contains(directive, tt.wantRecord) {
				t.Errorf("withProjectDirective(%s) directive = %q; want one naming %q", tt.worktree, directive, tt.wantRecord)
			}
			want := map[string]string{"slug": tt.worktree, loomengine.ProjectDirectiveMarker: directive}
			if !maps.Equal(got.Values, want) {
				t.Errorf("withProjectDirective(%s) values = %v; want %v", tt.worktree, got.Values, want)
			}
		})
	}
}
