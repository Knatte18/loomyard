// project_test.go covers the project kind's write rules in project.go:
// what a write may set on a project and its members, through a Board built from a board.yaml.

package boardengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newProjectBoard returns a board whose config LoadConfig reads from a board.yaml under t.TempDir().
// That board.yaml names the bug and enhancement types and the Home.md output.
// configure, when non-nil, edits the loaded config before the board is built, so a test sets the seams it needs.
func newProjectBoard(t *testing.T, configure func(cfg *boardengine.Config)) *boardengine.Board {
	t.Helper()
	configDir := writeBoardConfig(t, "readme: Home.md\ndesign_prefix: proposal-\ntypes:\n  bug: Broken\n  enhancement: New\nlabels:\n  undecided: Not decided\n")
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: configDir}, "board")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v; want nil", err)
	}
	cfg.Path = t.TempDir()
	cfg.SkipGit = true
	cfg.Clock = func() time.Time { return time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC) }
	if configure != nil {
		configure(&cfg)
	}
	return boardengine.New(cfg)
}

// seedBoardJSON writes a board.json holding entries into boardPath, for a state no upsert key can reach.
func seedBoardJSON(t *testing.T, boardPath string, entries ...string) {
	t.Helper()
	content := `{"version":1,"entries":[` + strings.Join(entries, ",") + `]}`
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed board.json: %v", err)
	}
}

// newDependencyBoard returns a board holding proj, an open project with the member task member.
// It also holds outsider, a task under no project.
func newDependencyBoard(t *testing.T) *boardengine.Board {
	t.Helper()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) { boardPath = cfg.Path })
	seedBoardJSON(t, boardPath,
		`{"id":0,"slug":"proj","title":"Proj","kind":"project","labels":["bug"]}`,
		`{"id":1,"slug":"member","title":"Member","kind":"task","labels":["bug"],"project":"proj"}`,
		`{"id":2,"slug":"outsider","title":"Outsider","kind":"task","labels":["bug"]}`,
	)
	return b
}

// TestProjectDependencies pins that every write path may make a task depend on a project,
// and that a dependency leading from a project's member back to the project is refused as a cycle.
func TestProjectDependencies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	writes := []struct {
		name    string
		written string
		write   func(b *boardengine.Board) error
	}{
		{"AnUpsertOfANewTask", "fresh", func(b *boardengine.Board) error {
			_, err := b.UpsertTask(map[string]any{"slug": "fresh", "kind": "task", "labels": bugLabels, "depends_on": []string{"proj"}})
			return err
		}},
		{"SetDeps", "outsider", func(b *boardengine.Board) error {
			_, err := b.SetDeps("outsider", []string{"proj"})
			return err
		}},
		{"ABatch", "fresh", func(b *boardengine.Board) error {
			_, err := b.UpsertTasksBatch([]map[string]any{{"slug": "fresh", "kind": "task", "labels": bugLabels, "depends_on": []string{"proj"}}})
			return err
		}},
	}
	for _, tt := range writes {
		t.Run("Accepted_"+tt.name, func(t *testing.T) {
			t.Parallel()
			b := newDependencyBoard(t)
			if err := tt.write(b); err != nil {
				t.Fatalf("write error = %v; want nil", err)
			}
			got, _, err := b.GetTask(tt.written)
			if err != nil || !slices.Equal(got.DependsOn, []string{"proj"}) {
				t.Errorf("GetTask(%s) depends_on = %v, %v; want [proj]", tt.written, got.DependsOn, err)
			}
		})
	}

	t.Run("Refused_AMemberDependingOnItsOwnProject", func(t *testing.T) {
		t.Parallel()
		b := newDependencyBoard(t)
		if _, err := b.SetDeps("member", []string{"proj"}); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Errorf("SetDeps(member, [proj]) error = %v; want a cycle refusal", err)
		}
	})

	t.Run("Refused_AMemberWaitingOnATaskThatWaitsOnItsProject", func(t *testing.T) {
		t.Parallel()
		b := newDependencyBoard(t)
		if _, err := b.SetDeps("outsider", []string{"proj"}); err != nil {
			t.Fatalf("SetDeps(outsider, [proj]) error = %v; want nil", err)
		}
		if _, err := b.SetDeps("member", []string{"outsider"}); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Errorf("SetDeps(member, [outsider]) error = %v; want a cycle refusal", err)
		}
	})
}

// newMembershipBoard returns a board holding p1, an open project with the member m and a project member p2.
// It also holds the done project pd and the run-held project ph.
// The tasks a and b have no project, and paired is a member of p1 whose pair exists.
func newMembershipBoard(t *testing.T) *boardengine.Board {
	t.Helper()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) {
		boardPath = cfg.Path
		cfg.PairExists = func(slug string) bool { return slug == "paired" }
	})
	seedBoardJSON(t, boardPath,
		`{"id":0,"slug":"p1","title":"P1","kind":"project","labels":["bug"]}`,
		`{"id":1,"slug":"p2","title":"P2","kind":"project","labels":["bug"],"project":"p1"}`,
		`{"id":2,"slug":"pd","title":"Pd","kind":"project","labels":["bug"],"status":"done"}`,
		`{"id":3,"slug":"ph","title":"Ph","kind":"project","labels":["bug"],"beam":true,"status":"running · Webster"}`,
		`{"id":4,"slug":"a","title":"A","kind":"task","labels":["bug"]}`,
		`{"id":5,"slug":"b","title":"B","kind":"task","labels":["bug"]}`,
		`{"id":6,"slug":"m","title":"M","kind":"task","labels":["bug"],"project":"p1"}`,
		`{"id":7,"slug":"paired","title":"Paired","kind":"task","labels":["bug"],"project":"p1"}`,
		`{"id":8,"slug":"pa","title":"Pa","kind":"project","labels":["bug"],"beam":true,"status":"running · Beam-Await"}`,
		`{"id":9,"slug":"pp","title":"Pp","kind":"project","labels":["bug"],"beam":true,"status":"running · Parent-Align"}`,
	)
	return b
}

// TestProjectRules_Membership pins each refusal of a write that sets or changes project or order.
// Each refusal names the entry, the field and the slug at fault.
// It also pins the writes that pass:
// a valid membership, a valid order and a done-mark of a project its members still name.
func TestProjectRules_Membership(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	refused := []struct {
		name    string
		fields  map[string]any
		wantHas []string
	}{
		{"ProjectNamingNoEntry", map[string]any{"slug": "a", "project": "ghost"}, []string{`"a"`, "project", `"ghost"`, "names no entry"}},
		{"ProjectNamingANonProject", map[string]any{"slug": "a", "project": "b"}, []string{`"a"`, "project", `"b"`, "is not a project"}},
		{"ProjectNamingItself", map[string]any{"slug": "p1", "project": "p1"}, []string{`"p1"`, "project", "is the entry itself"}},
		{"ProjectNamingADoneProject", map[string]any{"slug": "a", "project": "pd"}, []string{`"a"`, "project", `"pd"`, "is done"}},
		{"ProjectNamingARunHeldProject", map[string]any{"slug": "a", "project": "ph"}, []string{`"a"`, "project", `"ph"`, "held by a run"}},
		{"ProjectNamingABeamHeldFromParentAlign", map[string]any{"slug": "a", "project": "pp"}, []string{`"a"`, "project", `"pp"`, "held by a run"}},
		{"ProjectChainWithACycle", map[string]any{"slug": "p1", "project": "p2"}, []string{`"p1"`, "project", `"p2"`, "loops back"}},
		{"OrderNamingAnotherProjectsEntry", map[string]any{"slug": "p1", "order": []string{"m", "a"}}, []string{`"p1"`, "order", `"a"`, "no entry of this project"}},
		{"OrderNamingAnEntryTwice", map[string]any{"slug": "p1", "order": []string{"m", "m"}}, []string{`"p1"`, "order", `"m"`, "named twice"}},
		{"ProjectChangeOfAnEntryWithAPair", map[string]any{"slug": "paired", "project": "p2"}, []string{`"paired"`, "lym batten cancel paired --remove"}},
	}
	for _, tt := range refused {
		t.Run("Refused_"+tt.name, func(t *testing.T) {
			t.Parallel()
			b := newMembershipBoard(t)
			_, err := b.UpsertTask(tt.fields)
			if err == nil {
				t.Fatalf("UpsertTask(%v) error = nil; want a refusal", tt.fields)
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("UpsertTask(%v) error = %q; want it to contain %q", tt.fields, err, want)
				}
			}
		})
	}

	t.Run("ValidMembershipAndOrderPass", func(t *testing.T) {
		t.Parallel()
		b := newMembershipBoard(t)
		if _, err := b.UpsertTask(map[string]any{"slug": "a", "project": "p1"}); err != nil {
			t.Fatalf("UpsertTask(a project p1) error = %v; want nil", err)
		}
		project, err := b.UpsertTask(map[string]any{"slug": "p1", "order": []string{"m", "a"}})
		if err != nil || !slices.Equal(project.Order, []string{"m", "a"}) {
			t.Errorf("UpsertTask(p1 order) = %+v, %v; want order [m a]", project, err)
		}
		if _, err := b.UpsertTasksBatch([]map[string]any{
			{"slug": "b", "project": "p1"},
			{"slug": "p1", "order": []string{"b", "m", "a"}},
		}); err != nil {
			t.Errorf("UpsertTasksBatch(b into p1, then p1 order) error = %v; want nil, since the batch is judged as one write", err)
		}
	})

	t.Run("TaskJoiningABeamAtBeamAwaitPasses", func(t *testing.T) {
		t.Parallel()
		b := newMembershipBoard(t)
		if _, err := b.UpsertTask(map[string]any{"slug": "b", "project": "pa"}); err != nil {
			t.Errorf("UpsertTask(b project pa) error = %v; want nil, since a beam at Beam-Await holds no member out", err)
		}
	})

	t.Run("DoneMarkOfAProjectItsMembersStillNamePasses", func(t *testing.T) {
		t.Parallel()
		b := newMembershipBoard(t)
		if err := b.SetStatus("p1", strPtr("done")); err != nil {
			t.Errorf("SetStatus(p1, done) error = %v; want nil, since no member's project changed", err)
		}
	})
}
