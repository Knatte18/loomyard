// dobbyreading_test.go pins what the pass wiring reads from the parent worktree's dobby: its pid, its recorded sleeps and the shared awake clock, and which runs take no slot.

package battencli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestDobbyReading(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	base := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	slept := pauserecord.Interval{Start: base.Add(10 * time.Minute), End: base.Add(40 * time.Minute)}
	const dobbyPID = 424242

	tests := []struct {
		name       string
		live       bool
		record     func(t *testing.T, path string)
		wantPID    int
		wantSleeps []pauserecord.Interval
	}{
		{name: "DeadDobbyReadsThisProcess", live: false, record: writeDobbyState(dobbyPID, slept), wantPID: os.Getpid()},
		{name: "MissingRecordReadsThisProcess", live: true, wantPID: os.Getpid()},
		{name: "LiveDobbyReadsItsPIDAndSleeps", live: true, record: writeDobbyState(dobbyPID, slept), wantPID: dobbyPID, wantSleeps: []pauserecord.Interval{slept}},
		{name: "UndecodableRecordReadsAsNoDobby", live: true, record: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write undecodable record: %v", err)
			}
		}, wantPID: os.Getpid()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "state.json")
			if tt.record != nil {
				tt.record(t, path)
			}
			pid, sleeps, err := dobbyStateReading(path, func() (bool, error) { return tt.live, nil })
			if err != nil {
				t.Fatalf("dobbyStateReading() error = %v; want nil", err)
			}
			if pid != tt.wantPID || !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("dobbyStateReading() = %d, %v; want %d, %v", pid, sleeps, tt.wantPID, tt.wantSleeps)
			}
		})
	}

	t.Run("AwakeReadingsDifferByTheWallGapLessTheSleep", func(t *testing.T) {
		t.Parallel()
		before, after := base, base.Add(time.Hour)
		gap := awakeReading(after, []pauserecord.Interval{slept}) - awakeReading(before, []pauserecord.Interval{slept})
		if want := time.Hour - slept.End.Sub(slept.Start); gap != want {
			t.Errorf("awake readings an hour apart with a thirty-minute sleep between differ by %v; want %v", gap, want)
		}
	})
}

// writeDobbyState returns a record writer that stores a state holding pid and sleeps.
func writeDobbyState(pid int, sleeps ...pauserecord.Interval) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		err := dobbyengine.UpdateState(path, func(s *dobbyengine.State) error {
			s.PID, s.Sleeps = pid, sleeps
			return nil
		})
		if err != nil {
			t.Fatalf("write dobby state: %v", err)
		}
	}
}

func TestDobbyTasks_ReadsDependenciesParentAndNotifyOnLand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	boardDir := t.TempDir()
	entries := strings.Join([]string{
		`{"id":0,"slug":"released","title":"R","kind":"task","labels":["bug"],"depends_on":["open-dep","done-dep","pruned-dep","beam-dep"],"notify_on_land":true}`,
		`{"id":1,"slug":"open-dep","title":"O","kind":"task","labels":["bug"],"status":"draft"}`,
		`{"id":2,"slug":"done-dep","title":"D","kind":"task","labels":["bug"],"status":"done"}`,
		`{"id":3,"slug":"beam-dep","title":"B","kind":"project","labels":["bug"],"beam":true,"status":"running · Beam-Await"}`,
		`{"id":4,"slug":"beam-member","title":"M","kind":"task","labels":["bug"],"project":"beam-dep","status":"done"}`,
		`{"id":5,"slug":"landed","title":"L","kind":"task","labels":["bug"],"status":"done","notify_on_land":true}`,
		`{"id":6,"slug":"landed-here","title":"H","kind":"task","labels":["bug"],"status":"done","notify_on_land":true}`,
		`{"id":7,"slug":"beam-b","title":"B2","kind":"project","labels":["bug"],"beam":true,"owner_machine":"another-machine","status":"running · Beam-Await"}`,
	}, ",")
	content := `{"version":1,"entries":[` + entries + `]}`
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write board.json: %v", err)
	}
	prime := &lymcwd.Location{WorktreeName: "main"}
	parents := NewDobbyParents(prime)
	parents.known["released"] = "beam-a"
	parents.known["landed"] = "beam-b"
	parents.known["landed-here"] = "beam-a"
	parents.known["beam-b"] = ""

	views, err := DobbyTasks(prime, boardDir, parents)
	if err != nil {
		t.Fatalf("DobbyTasks() error = %v; want nil", err)
	}
	bySlug := map[string]dobbyengine.TaskView{}
	for _, view := range views {
		bySlug[view.Slug] = view
	}
	released := bySlug["released"]
	if want := []string{"open-dep", "beam-dep"}; !slices.Equal(released.PendingDependencies, want) {
		t.Errorf("DobbyTasks() released PendingDependencies = %v; want %v", released.PendingDependencies, want)
	}
	if released.Parent != "beam-a" {
		t.Errorf("DobbyTasks() released Parent = %q; want %q", released.Parent, "beam-a")
	}
	if !released.NotifyOnLand || bySlug["open-dep"].NotifyOnLand {
		t.Errorf("DobbyTasks() NotifyOnLand = %v, %v; want true for the entry carrying it and false for the other", released.NotifyOnLand, bySlug["open-dep"].NotifyOnLand)
	}
	if draft := bySlug["open-dep"]; draft.Parent != "" {
		t.Errorf("DobbyTasks() draft Parent = %q; want none read for a task with a status", draft.Parent)
	}
	if landed := bySlug["landed"]; landed.Parent != "beam-b" || !landed.OwnedElsewhere {
		t.Errorf("DobbyTasks() landed Parent, OwnedElsewhere = %q, %v; want %q, true for a done entry carrying notify_on_land under a beam another machine owns", landed.Parent, landed.OwnedElsewhere, "beam-b")
	}
	if landed := bySlug["landed-here"]; landed.Parent != "beam-a" || landed.OwnedElsewhere {
		t.Errorf("DobbyTasks() landed-here Parent, OwnedElsewhere = %q, %v; want %q, false for a done entry carrying notify_on_land under a beam no machine owns", landed.Parent, landed.OwnedElsewhere, "beam-a")
	}
}

func TestDobbyTasks_AnUnreadableParentFailsOnlyAReleasedTask(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		entry   string
		wantErr bool
	}{
		{name: "ADoneEntryCarryingTheRequestIsLeftWithNoParent", entry: `{"id":1,"slug":"broken","title":"B","kind":"task","labels":["bug"],"status":"done","notify_on_land":true}`},
		{name: "AReleasedTaskFailsTheRead", entry: `{"id":1,"slug":"broken","title":"B","kind":"task","labels":["bug"]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir := t.TempDir()
			content := `{"version":1,"entries":[{"id":0,"slug":"fine","title":"F","kind":"task","labels":["bug"],"status":"draft"},` + tt.entry + `]}`
			if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(content), 0o644); err != nil {
				t.Fatalf("write board.json: %v", err)
			}
			// The hub's own board is unreadable, which is what fails the parent read of a slug with no origin record.
			prime := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			hubBoard := fabricengine.BoardDir(prime.HubPath)
			if err := os.MkdirAll(hubBoard, 0o755); err != nil {
				t.Fatalf("create the hub board directory: %v", err)
			}
			if err := os.WriteFile(filepath.Join(hubBoard, "board.json"), []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write the hub board: %v", err)
			}

			views, err := DobbyTasks(prime, boardDir, NewDobbyParents(prime))
			if (err != nil) != tt.wantErr {
				t.Fatalf("DobbyTasks() error = %v; want an error = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			bySlug := map[string]dobbyengine.TaskView{}
			for _, view := range views {
				bySlug[view.Slug] = view
			}
			if _, ok := bySlug["fine"]; !ok || len(views) != 2 {
				t.Errorf("DobbyTasks() slugs = %v; want fine and broken both returned", bySlug)
			}
			if broken := bySlug["broken"]; broken.Parent != "" || broken.OwnedElsewhere {
				t.Errorf("DobbyTasks() broken Parent, OwnedElsewhere = %q, %v; want none read", broken.Parent, broken.OwnedElsewhere)
			}
		})
	}
}

func TestDobbyTasks_ReadsTheStartRefusalOfAFinishedRunRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		status      string
		recipe      string
		childRecipe string
		// childRecipeUnreadable makes the child-recipe record a directory, so reading it fails.
		childRecipeUnreadable bool
		// defaultRecipe is the board's default_recipe, empty leaving the template's.
		defaultRecipe string
		wantRefused   bool
	}{
		{name: "NoRunRecordHoldsNothing"},
		{name: "ADoneRunRecordHoldsTheStart", status: `{"state":"done"}`, wantRefused: true},
		{name: "ARunningRunRecordHoldsNothing", status: `{"state":"running"}`},
		{name: "AnUndecodableRunRecordHoldsNothing", status: `{not json`},
		{name: "ADoneMotifRunOfARaddleEntryHoldsNothing", status: `{"state":"done"}`, recipe: shedrun.RecipeRaddle, childRecipe: shedrun.RecipeMotif},
		{name: "ADoneRaddleRunOfARaddleEntryHoldsTheStart", status: `{"state":"done"}`, recipe: shedrun.RecipeRaddle, childRecipe: shedrun.RecipeRaddle, wantRefused: true},
		{name: "ADoneRunOfARaddleEntryWhoseChildRecipeDoesNotReadHoldsTheStart", status: `{"state":"done"}`, recipe: shedrun.RecipeRaddle, childRecipeUnreadable: true, wantRefused: true},
		{name: "ADoneMotifRunOfADarnEntryHoldsTheStart", status: `{"state":"done"}`, recipe: shedrun.RecipeDarn, childRecipe: shedrun.RecipeMotif, wantRefused: true},
		{name: "ADoneMotifRunOfAnEntryWithNoRecipeUnderARaddleDefaultHoldsNothing", status: `{"state":"done"}`, childRecipe: shedrun.RecipeMotif, defaultRecipe: shedrun.RecipeRaddle},
		{name: "ADoneMotifRunOfAnEntryWithNoRecipeUnderTheTemplateDefaultHoldsTheStart", status: `{"state":"done"}`, childRecipe: shedrun.RecipeMotif, wantRefused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "main", AnchorRel: "."}
			writeBoardHub(t, prime, tt.defaultRecipe, `{"version":1,"entries":[{"id":0,"slug":"released","title":"R","kind":"task","labels":["bug"],"recipe":"`+tt.recipe+`"}]}`)
			if tt.status != "" {
				path := shedrun.StatusFile(prime, "released")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("create the run directory: %v", err)
				}
				if err := os.WriteFile(path, []byte(tt.status), 0o644); err != nil {
					t.Fatalf("write the run status: %v", err)
				}
			}
			if tt.childRecipe != "" {
				if err := shedrun.WriteChildRecipe(prime, "released", tt.childRecipe); err != nil {
					t.Fatalf("write the child recipe: %v", err)
				}
			}
			if tt.childRecipeUnreadable {
				if err := os.MkdirAll(shedrun.ChildRecipeFile(prime, "released"), 0o755); err != nil {
					t.Fatalf("create the unreadable child recipe: %v", err)
				}
			}
			parents := NewDobbyParents(prime)
			parents.known["released"] = "main"

			views, err := DobbyTasks(prime, fabricengine.BoardDir(prime.HubPath), parents)
			if err != nil {
				t.Fatalf("DobbyTasks() error = %v; want nil", err)
			}
			want := ""
			if tt.wantRefused {
				want = doneSlugRefusal(prime, "released").Error()
			}
			if len(views) != 1 || views[0].StartRefusal != want {
				t.Errorf("DobbyTasks() = %+v; want one task with StartRefusal %q", views, want)
			}
		})
	}
}

func TestRunTakesNoSlot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	status := func(producer string) *string {
		value := boardengine.RunStatus("running", producer)
		return &value
	}
	tests := []struct {
		name string
		task boardengine.Task
		want bool
	}{
		{"BeamProjectAtBeamAwait", boardengine.Task{Kind: boardengine.KindProject, Beam: true, Status: status("Beam-Await")}, true},
		{"BeamProjectAtWorktreeCreate", boardengine.Task{Kind: boardengine.KindProject, Beam: true, Status: status("Worktree-Create")}, true},
		{"BeamProjectAtParentAlign", boardengine.Task{Kind: boardengine.KindProject, Beam: true, Status: status("Parent-Align")}, false},
		{"BeamProjectWithNoRunStatus", boardengine.Task{Kind: boardengine.KindProject, Beam: true}, false},
		{"TaskAtBeamAwait", boardengine.Task{Kind: boardengine.KindTask, Status: status("Beam-Await")}, false},
		{"TaskWithNoRunStatus", boardengine.Task{Kind: boardengine.KindTask}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := runTakesNoSlot(tt.task); got != tt.want {
				t.Errorf("runTakesNoSlot(%+v) = %v; want %v", tt.task, got, tt.want)
			}
		})
	}
}
