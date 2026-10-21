// replay_test.go — FabricReplay, the by-entry merge of a local board commit onto origin's board.
//
// Every row builds origin's board directory, the merge base, the commit's parent and the commit, applies the commit and reads the board directory back.

package boardengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
)

// replayEntry builds a task entry with the type label bug, the options applied after the defaults.
func replayEntry(slug string, options ...func(*boardengine.Task)) boardengine.Task {
	task := boardengine.Task{
		Slug:      slug,
		Title:     slug,
		Kind:      boardengine.KindTask,
		Labels:    []string{"bug"},
		Issues:    []int{},
		DependsOn: []string{},
	}
	for _, option := range options {
		option(&task)
	}
	return task
}

func withTitle(title string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Title = title }
}

func withBody(body string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Body = body }
}

func withStatus(status string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Status = &status }
}

func withLabels(labels ...string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Labels = labels }
}

func withDependsOn(slugs ...string) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.DependsOn = slugs }
}

func withIssues(numbers ...int) func(*boardengine.Task) {
	return func(task *boardengine.Task) { task.Issues = numbers }
}

func asIsolated(task *boardengine.Task) { task.Isolated = true }

func asNote(task *boardengine.Task) { task.Kind = boardengine.KindNote }

// boardJSON encodes tasks as a board.json, numbering the entries by position.
func boardJSON(t *testing.T, tasks []boardengine.Task) []byte {
	t.Helper()
	numbered := make([]boardengine.Task, len(tasks))
	for i, task := range tasks {
		task.ID = i
		numbered[i] = task
	}
	data, err := json.Marshal(struct {
		Version int                `json:"version"`
		Entries []boardengine.Task `json:"entries"`
	}{1, numbered})
	if err != nil {
		t.Fatalf("marshal board.json: %v", err)
	}
	return data
}

// replayRow is one scenario: origin's board and files, the three sides of the local commit and the checks after the replay.
type replayRow struct {
	name        string
	extraLabel  bool
	origin      []boardengine.Task
	base        []boardengine.Task
	parent      []boardengine.Task
	commit      []boardengine.Task
	originFiles map[string]string
	designs     []fabricengine.BoardDesignFile
	wantEmpty   bool
	wantLost    []fabricengine.BoardReplayFinding
	// wantRefused maps an entry to a substring of its refusal reason.
	wantRefused map[string]string
	// wantEntries maps a slug to the title the board holds afterwards, "" meaning the entry is absent.
	wantEntries map[string]string
	check       func(t *testing.T, dir string, got map[string]boardengine.Task)
}

func TestFabricReplay(t *testing.T) {
	t.Parallel()

	runLocked := boardengine.RunStatus("running", "Implementer")
	ghostDependency := replayEntry("x", withDependsOn("ghost"))
	rows := []replayRow{
		{
			name:        "DifferentEntriesChangedOnTheTwoSidesBothLand",
			base:        []boardengine.Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a", withTitle("A2")), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B2"))},
			wantEntries: map[string]string{"a": "A2", "b": "B2"},
			check: func(t *testing.T, dir string, _ map[string]boardengine.Task) {
				readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
				if err != nil {
					t.Fatalf("ReadFile(README.md) = %v; want nil", err)
				}
				if !strings.Contains(string(readme), "B2") {
					t.Errorf("README.md = %q; want it re-rendered with the title B2", readme)
				}
			},
		},
		{
			name:        "SameFieldChangedOnBothSidesKeepsOriginsValue",
			base:        []boardengine.Task{replayEntry("a", withTitle("A"))},
			origin:      []boardengine.Task{replayEntry("a", withTitle("origin"))},
			parent:      []boardengine.Task{replayEntry("a", withTitle("A"))},
			commit:      []boardengine.Task{replayEntry("a", withTitle("local"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "a", Field: "title", Reason: "origin changed the field too", Local: `"local"`}},
			wantEntries: map[string]string{"a": "origin"},
			check: func(t *testing.T, dir string, _ map[string]boardengine.Task) {
				if _, err := os.Stat(filepath.Join(dir, "README.md")); !os.IsNotExist(err) {
					t.Errorf("Stat(README.md) = %v; want not-exist, since an empty replay writes nothing", err)
				}
			},
		},
		{
			name:        "CommitOriginAlreadyHoldsIsEmpty",
			base:        []boardengine.Task{replayEntry("a", withTitle("A"))},
			origin:      []boardengine.Task{replayEntry("a", withTitle("T"))},
			parent:      []boardengine.Task{replayEntry("a", withTitle("A"))},
			commit:      []boardengine.Task{replayEntry("a", withTitle("T"))},
			wantEmpty:   true,
			wantEntries: map[string]string{"a": "T"},
		},
		{
			name:        "BodyEditUnderRunStatusIsRefusedAndTheRestLands",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a", withStatus(runLocked)), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a", withBody("local body")), replayEntry("b", withTitle("B2"))},
			wantRefused: map[string]string{"a": "held by a run"},
			wantEntries: map[string]string{"a": "a", "b": "B2"},
			check: func(t *testing.T, _ string, got map[string]boardengine.Task) {
				if got["a"].Body != "" {
					t.Errorf("held entry body = %q; want it unchanged", got["a"].Body)
				}
			},
		},
		{
			name:        "DependsOnAnEntryARefusedChangeAddedIsRefusedToo",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "nolabel")), replayEntry("y", withDependsOn("x"))},
			wantRefused: map[string]string{"x": "nolabel", "y": `"x"`},
			wantEntries: map[string]string{"b": "B2", "x": "", "y": ""},
		},
		{
			name:        "DanglingDependsOnIsRefused",
			base:        []boardengine.Task{replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("b", withTitle("B2")), ghostDependency},
			wantRefused: map[string]string{"x": "dangling dependency"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "CyclicDependsOnIsRefused",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			commit:      []boardengine.Task{replayEntry("a", withDependsOn("b")), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C2"))},
			wantRefused: map[string]string{"a": "cycle detected"},
			wantEntries: map[string]string{"c": "C2"},
		},
		{
			name:        "DependingOnAnIsolatedEntryIsRefused",
			base:        []boardengine.Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B2")), replayEntry("x", withDependsOn("a"))},
			wantRefused: map[string]string{"x": "isolated"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "NoteWithDependsOnIsRefused",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B2")), replayEntry("x", asNote, withDependsOn("a"))},
			wantRefused: map[string]string{"x": "only a task can depend"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "InvalidIssueNumberIsRefused",
			base:        []boardengine.Task{replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("b", withTitle("B2")), replayEntry("x", withIssues(0))},
			wantRefused: map[string]string{"x": "issue number is a positive integer"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "UnknownLabelIsRefused",
			base:        []boardengine.Task{replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "area"))},
			wantRefused: map[string]string{"x": `label "area"`},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "LabelKnownOnlyThroughTheBoardYamlLands",
			extraLabel:  true,
			base:        []boardengine.Task{replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "area"))},
			wantEntries: map[string]string{"b": "B2", "x": "x"},
		},
		{
			name:        "AddedEntryTakesOriginsNextID",
			base:        []boardengine.Task{replayEntry("a")},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b")},
			parent:      []boardengine.Task{replayEntry("a")},
			commit:      []boardengine.Task{replayEntry("a"), replayEntry("c")},
			wantEntries: map[string]string{"a": "a", "b": "b", "c": "c"},
			check: func(t *testing.T, _ string, got map[string]boardengine.Task) {
				if got["c"].ID != 2 {
					t.Errorf("added entry id = %d; want 2, origin's next", got["c"].ID)
				}
			},
		},
		{
			name:        "ChangeToAnEntryOriginRemovedIsLost",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a")},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B2"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "b", Reason: "origin removed the entry"}},
			wantEntries: map[string]string{"a": "a", "b": ""},
		},
		{
			name:        "RemovalOfAnEntryOriginHoldsUnchangedLands",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a")},
			wantEntries: map[string]string{"a": "a", "b": ""},
		},
		{
			name:        "RemovalOfAnEntryOriginChangedIsLost",
			base:        []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("origin"))},
			parent:      []boardengine.Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []boardengine.Task{replayEntry("a")},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "b", Reason: "origin changed the entry this commit removes"}},
			wantEntries: map[string]string{"a": "a", "b": "origin"},
		},
		{
			name:        "DesignFileChangedOnBothSidesKeepsOriginsFile",
			base:        []boardengine.Task{replayEntry("a")},
			origin:      []boardengine.Task{replayEntry("a")},
			parent:      []boardengine.Task{replayEntry("a")},
			commit:      []boardengine.Task{replayEntry("a")},
			originFiles: map[string]string{"design-a.md": "origin"},
			designs:     []fabricengine.BoardDesignFile{{Name: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "design-a.md", Field: "design file", Reason: "origin changed the file too"}},
			check: func(t *testing.T, dir string, _ map[string]boardengine.Task) {
				if got, _ := os.ReadFile(filepath.Join(dir, "design-a.md")); string(got) != "origin" {
					t.Errorf("design-a.md = %q; want origin's file", got)
				}
			},
		},
		{
			name:        "DesignFileOnlyTheCommitChangedLands",
			base:        []boardengine.Task{replayEntry("a")},
			origin:      []boardengine.Task{replayEntry("a")},
			parent:      []boardengine.Task{replayEntry("a")},
			commit:      []boardengine.Task{replayEntry("a")},
			originFiles: map[string]string{"design-a.md": "base"},
			designs:     []fabricengine.BoardDesignFile{{Name: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			check: func(t *testing.T, dir string, _ map[string]boardengine.Task) {
				if got, _ := os.ReadFile(filepath.Join(dir, "design-a.md")); string(got) != "local" {
					t.Errorf("design-a.md = %q; want the commit's file", got)
				}
			},
		},
	}

	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := boardkit.HubWithBoardConfig(t)
			dir := fabricengine.BoardDir(hub)
			if tt.extraLabel {
				configFile := configengine.ConfigFile(dir, "board")
				template, err := os.ReadFile(configFile)
				if err != nil {
					t.Fatalf("ReadFile(%q) = %v; want nil", configFile, err)
				}
				withLabel := strings.Replace(string(template), "labels: {}", "labels: {area: An area}", 1)
				if err := os.WriteFile(configFile, []byte(withLabel), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", configFile, err)
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "board.json"), boardJSON(t, tt.origin), 0o644); err != nil {
				t.Fatalf("WriteFile(board.json) = %v; want nil", err)
			}
			for name, content := range tt.originFiles {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
				}
			}

			result, err := boardengine.FabricReplay{}.Apply(fabricengine.BoardReplayRequest{
				BoardDir:        dir,
				BaseBoardJSON:   boardJSON(t, tt.base),
				ParentBoardJSON: boardJSON(t, tt.parent),
				CommitBoardJSON: boardJSON(t, tt.commit),
				DesignFiles:     tt.designs,
			})
			if err != nil {
				t.Fatalf("Apply() error = %v; want nil", err)
			}

			if result.Empty != tt.wantEmpty {
				t.Errorf("Apply().Empty = %v; want %v", result.Empty, tt.wantEmpty)
			}
			if !reflect.DeepEqual(result.Lost, tt.wantLost) {
				t.Errorf("Apply().Lost = %+v; want %+v", result.Lost, tt.wantLost)
			}
			if len(result.Refused) != len(tt.wantRefused) {
				t.Errorf("Apply().Refused = %+v; want one refusal per entry of %v", result.Refused, tt.wantRefused)
			}
			for _, refusal := range result.Refused {
				want, ok := tt.wantRefused[refusal.Entry]
				if !ok || !strings.Contains(refusal.Reason, want) {
					t.Errorf("Apply().Refused entry %q reason = %q; want a refusal containing %q", refusal.Entry, refusal.Reason, want)
				}
			}

			board, err := boardengine.OpenHub(hub)
			if err != nil {
				t.Fatalf("OpenHub() error = %v; want nil", err)
			}
			tasks, err := board.ListTasksFull()
			if err != nil {
				t.Fatalf("ListTasksFull() error = %v; want nil", err)
			}
			got := make(map[string]boardengine.Task, len(tasks))
			for _, task := range tasks {
				got[task.Slug] = task
			}
			for slug, wantTitle := range tt.wantEntries {
				task, held := got[slug]
				if wantTitle == "" {
					if held {
						t.Errorf("board holds entry %q; want it absent", slug)
					}
					continue
				}
				if !held || task.Title != wantTitle {
					t.Errorf("board entry %q = %+v (held %v); want title %q", slug, task, held, wantTitle)
				}
			}
			if tt.check != nil {
				tt.check(t, dir, got)
			}
		})
	}

	t.Run("EntriesReturnsEachEntrysSlugStatusAndCreatedStamp", func(t *testing.T) {
		t.Parallel()
		created := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
		held := replayEntry("a", withStatus("done"))
		held.Created = created
		data := boardJSON(t, []boardengine.Task{held, replayEntry("b")})

		got, err := boardengine.FabricReplay{}.Entries(data)
		if err != nil {
			t.Fatalf("Entries() error = %v; want nil", err)
		}
		want := []fabricengine.BoardEntry{{Slug: "a", Status: "done", Created: created}, {Slug: "b"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Entries() = %+v; want %+v", got, want)
		}

		if got, err := (boardengine.FabricReplay{}).Entries(nil); err != nil || len(got) != 0 {
			t.Errorf("Entries(nil) = %+v, %v; want no entries and no error", got, err)
		}
	})
}
