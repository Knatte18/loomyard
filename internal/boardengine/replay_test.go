// replay_test.go — FabricReplay, the by-entry merge of a local board commit onto origin's board.
//
// Every row builds origin's board directory, the merge base, the commit's parent and the commit, applies the commit and reads the board directory back.

package boardengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// replayEntry builds a task entry with the type label bug, the options applied after the defaults.
func replayEntry(slug string, options ...func(*Task)) Task {
	task := Task{
		Slug:      slug,
		Title:     slug,
		Kind:      KindTask,
		Labels:    []string{"bug"},
		Issues:    []int{},
		DependsOn: []string{},
	}
	for _, option := range options {
		option(&task)
	}
	return task
}

func withTitle(title string) func(*Task) {
	return func(task *Task) { task.Title = title }
}

func withHandle(handle string) func(*Task) {
	return func(task *Task) { task.Handle = handle }
}

func withBody(body string) func(*Task) {
	return func(task *Task) { task.Body = body }
}

func withStatus(status string) func(*Task) {
	return func(task *Task) { task.Status = &status }
}

func withLabels(labels ...string) func(*Task) {
	return func(task *Task) { task.Labels = labels }
}

func withDependsOn(slugs ...string) func(*Task) {
	return func(task *Task) { task.DependsOn = slugs }
}

func withIssues(numbers ...int) func(*Task) {
	return func(task *Task) { task.Issues = numbers }
}

func asIsolated(task *Task) { task.Isolated = true }

func asNote(task *Task) { task.Kind = KindNote }

func withOwner(owner string) func(*Task) {
	return func(task *Task) { task.OwnerMachine = owner }
}

// boardJSON encodes tasks as a board.json, numbering the entries by position.
func boardJSON(t *testing.T, tasks []Task) []byte {
	t.Helper()
	return boardJSONWithPrime(t, tasks, "")
}

// boardJSONWithPrime encodes tasks as a board.json that records prime as the hub's prime owner, none when empty.
func boardJSONWithPrime(t *testing.T, tasks []Task, prime string) []byte {
	t.Helper()
	numbered := make([]Task, len(tasks))
	for i, task := range tasks {
		task.ID = i
		numbered[i] = task
	}
	data, err := json.Marshal(struct {
		Version    int    `json:"version"`
		Entries    []Task `json:"entries"`
		PrimeOwner string `json:"prime_owner,omitempty"`
	}{1, numbered, prime})
	if err != nil {
		t.Fatalf("marshal board.json: %v", err)
	}
	return data
}

// hubWithBoardConfig returns a temp hub whose board directory holds the board config template.
func hubWithBoardConfig(t *testing.T) string {
	t.Helper()
	hub := t.TempDir()
	configFile := configengine.ConfigFile(fabricengine.BoardDir(hub), "board")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(configFile), err)
	}
	if err := os.WriteFile(configFile, []byte(ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", configFile, err)
	}
	return hub
}

// replayRow is one scenario: origin's board and files, the three sides of the local commit and the checks after the replay.
type replayRow struct {
	name        string
	extraLabel  bool
	origin      []Task
	base        []Task
	parent      []Task
	commit      []Task
	originFiles map[string]string
	designs     []fabricengine.BoardDesignFile
	// originPrime, basePrime, parentPrime and commitPrime are the prime_owner each side records, and wantPrime the one the board holds afterwards.
	originPrime, basePrime, parentPrime, commitPrime, wantPrime string
	wantEmpty                                                   bool
	wantLost                                                    []fabricengine.BoardReplayFinding
	// wantRefused maps an entry to a substring of its refusal reason.
	wantRefused map[string]string
	// wantEntries maps a slug to the title the board holds afterwards, "" meaning the entry is absent.
	wantEntries map[string]string
	check       func(t *testing.T, dir string, got map[string]Task)
}

func TestFabricReplay(t *testing.T) {
	t.Parallel()

	runLocked := RunStatus("running", "Implementer")
	ghostDependency := replayEntry("x", withDependsOn("ghost"))
	rows := []replayRow{
		{
			name:        "DifferentEntriesChangedOnTheTwoSidesBothLand",
			base:        []Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a", withTitle("A2")), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a", withTitle("A")), replayEntry("b", withTitle("B2"))},
			wantEntries: map[string]string{"a": "A2", "b": "B2"},
			check: func(t *testing.T, dir string, _ map[string]Task) {
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
			base:        []Task{replayEntry("a", withTitle("A"))},
			origin:      []Task{replayEntry("a", withTitle("origin"))},
			parent:      []Task{replayEntry("a", withTitle("A"))},
			commit:      []Task{replayEntry("a", withTitle("local"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "a", Field: "title", Reason: "origin changed the field too", Local: `"local"`}},
			wantEntries: map[string]string{"a": "origin"},
			check: func(t *testing.T, dir string, _ map[string]Task) {
				if _, err := os.Stat(filepath.Join(dir, "README.md")); !os.IsNotExist(err) {
					t.Errorf("Stat(README.md) = %v; want not-exist, since an empty replay writes nothing", err)
				}
			},
		},
		{
			name:        "CommitOriginAlreadyHoldsIsEmpty",
			base:        []Task{replayEntry("a", withTitle("A"))},
			origin:      []Task{replayEntry("a", withTitle("T"))},
			parent:      []Task{replayEntry("a", withTitle("A"))},
			commit:      []Task{replayEntry("a", withTitle("T"))},
			wantEmpty:   true,
			wantEntries: map[string]string{"a": "T"},
		},
		{
			name:        "BodyEditUnderRunStatusIsRefusedAndTheRestLands",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a", withStatus(runLocked)), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a", withBody("local body")), replayEntry("b", withTitle("B2"))},
			wantRefused: map[string]string{"a": "held by a run"},
			wantEntries: map[string]string{"a": "a", "b": "B2"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].Body != "" {
					t.Errorf("held entry body = %q; want it unchanged", got["a"].Body)
				}
			},
		},
		{
			name:        "DependsOnAnEntryARefusedChangeAddedIsRefusedToo",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a"), replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "nolabel")), replayEntry("y", withDependsOn("x"))},
			wantRefused: map[string]string{"x": "nolabel", "y": `"x"`},
			wantEntries: map[string]string{"b": "B2", "x": "", "y": ""},
		},
		{
			name:        "DanglingDependsOnIsRefused",
			base:        []Task{replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("b", withTitle("B2")), ghostDependency},
			wantRefused: map[string]string{"x": "dangling dependency"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "CyclicDependsOnIsRefused",
			base:        []Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C"))},
			commit:      []Task{replayEntry("a", withDependsOn("b")), replayEntry("b", withDependsOn("a")), replayEntry("c", withTitle("C2"))},
			wantRefused: map[string]string{"a": "cycle detected"},
			wantEntries: map[string]string{"c": "C2"},
		},
		{
			name:        "DependingOnAnIsolatedEntryIsRefused",
			base:        []Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a", asIsolated), replayEntry("b", withTitle("B2")), replayEntry("x", withDependsOn("a"))},
			wantRefused: map[string]string{"x": "isolated"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "NoteWithDependsOnIsRefused",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a"), replayEntry("b", withTitle("B2")), replayEntry("x", asNote, withDependsOn("a"))},
			wantRefused: map[string]string{"x": "only a task can depend"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "InvalidIssueNumberIsRefused",
			base:        []Task{replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("b", withTitle("B2")), replayEntry("x", withIssues(0))},
			wantRefused: map[string]string{"x": "issue number is a positive integer"},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "UnknownLabelIsRefused",
			base:        []Task{replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "area"))},
			wantRefused: map[string]string{"x": `label "area"`},
			wantEntries: map[string]string{"b": "B2", "x": ""},
		},
		{
			name:        "LabelKnownOnlyThroughTheBoardYamlLands",
			extraLabel:  true,
			base:        []Task{replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("b", withTitle("B2")), replayEntry("x", withLabels("bug", "area"))},
			wantEntries: map[string]string{"b": "B2", "x": "x"},
		},
		{
			name:        "AddedEntryTakesOriginsNextIDAndAFreeHandle",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a"), replayEntry("b", withHandle("T2"))},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a"), replayEntry("c", withHandle("T2"))},
			wantEntries: map[string]string{"a": "a", "b": "b", "c": "c"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["c"].ID != 2 {
					t.Errorf("added entry id = %d; want 2, origin's next", got["c"].ID)
				}
				if got["b"].Handle != "T2" || got["c"].Handle != "T3" {
					t.Errorf("handles b %q, c %q; want origin's T2 kept and the added entry on T3, free on origin", got["b"].Handle, got["c"].Handle)
				}
			},
		},
		{
			name:        "ChangeToAnEntryOriginRemovedIsLost",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a"), replayEntry("b", withTitle("B2"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "b", Reason: "origin removed the entry"}},
			wantEntries: map[string]string{"a": "a", "b": ""},
		},
		{
			name:        "RemovalOfAnEntryOriginHoldsUnchangedLands",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a")},
			wantEntries: map[string]string{"a": "a", "b": ""},
		},
		{
			name:        "RemovalOfAnEntryOriginChangedIsLost",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withTitle("origin"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a")},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "b", Reason: "origin changed the entry this commit removes"}},
			wantEntries: map[string]string{"a": "a", "b": "origin"},
		},
		{
			name:        "OwnerChangedOnBothSidesKeepsOriginsAndReportsTheLocalOneLost",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a", withOwner("box"))},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a", withOwner("neve"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "a", Field: "owner_machine", Reason: "origin changed the field too", Local: `"neve"`}},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].OwnerMachine != "box" {
					t.Errorf("owner_machine = %q; want origin's box", got["a"].OwnerMachine)
				}
			},
		},
		{
			name:        "OwnerOnlyTheCommitChangedLandsOnARunHeldEntry",
			base:        []Task{replayEntry("a", withStatus(runLocked))},
			origin:      []Task{replayEntry("a", withStatus(runLocked))},
			parent:      []Task{replayEntry("a", withStatus(runLocked))},
			commit:      []Task{replayEntry("a", withStatus(runLocked), withOwner("neve"))},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].OwnerMachine != "neve" {
					t.Errorf("owner_machine = %q; want the commit's neve", got["a"].OwnerMachine)
				}
			},
		},
		{
			name:        "PrimeOwnerChangedOnBothSidesKeepsOriginsAndReportsItLost",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			basePrime:   "base",
			originPrime: "box",
			parentPrime: "base",
			commitPrime: "neve",
			wantPrime:   "box",
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "prime", Field: "prime_owner", Reason: "origin changed the field too", Local: `"neve"`}},
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:        "PrimeOwnerOnlyTheCommitChangedLands",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			basePrime:   "base",
			originPrime: "base",
			parentPrime: "base",
			commitPrime: "neve",
			wantPrime:   "neve",
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:        "DesignFileChangedOnBothSidesKeepsOriginsFile",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			originFiles: map[string]string{"design-a.md": "origin"},
			designs:     []fabricengine.BoardDesignFile{{Name: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			wantEmpty:   true,
			wantLost:    []fabricengine.BoardReplayFinding{{Entry: "design-a.md", Field: "design file", Reason: "origin changed the file too"}},
			check: func(t *testing.T, dir string, _ map[string]Task) {
				if got, _ := os.ReadFile(filepath.Join(dir, "design-a.md")); string(got) != "origin" {
					t.Errorf("design-a.md = %q; want origin's file", got)
				}
			},
		},
		{
			name:        "DesignFileOnlyTheCommitChangedLands",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			originFiles: map[string]string{"design-a.md": "base"},
			designs:     []fabricengine.BoardDesignFile{{Name: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			check: func(t *testing.T, dir string, _ map[string]Task) {
				if got, _ := os.ReadFile(filepath.Join(dir, "design-a.md")); string(got) != "local" {
					t.Errorf("design-a.md = %q; want the commit's file", got)
				}
			},
		},
	}

	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := hubWithBoardConfig(t)
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
			if err := os.WriteFile(filepath.Join(dir, "board.json"), boardJSONWithPrime(t, tt.origin, tt.originPrime), 0o644); err != nil {
				t.Fatalf("WriteFile(board.json) = %v; want nil", err)
			}
			for name, content := range tt.originFiles {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
				}
			}

			result, err := FabricReplay{}.Apply(fabricengine.BoardReplayRequest{
				BoardDir:        dir,
				BaseBoardJSON:   boardJSONWithPrime(t, tt.base, tt.basePrime),
				ParentBoardJSON: boardJSONWithPrime(t, tt.parent, tt.parentPrime),
				CommitBoardJSON: boardJSONWithPrime(t, tt.commit, tt.commitPrime),
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

			board, err := OpenHub(hub)
			if err != nil {
				t.Fatalf("OpenHub() error = %v; want nil", err)
			}
			tasks, err := board.ListTasksFull()
			if err != nil {
				t.Fatalf("ListTasksFull() error = %v; want nil", err)
			}
			got := make(map[string]Task, len(tasks))
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
			stored := NewStore(dir)
			if err := stored.Load(); err != nil {
				t.Fatalf("Load() error = %v; want nil", err)
			}
			if stored.primeOwner != tt.wantPrime {
				t.Errorf("prime_owner = %q; want %q", stored.primeOwner, tt.wantPrime)
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
		data := boardJSON(t, []Task{held, replayEntry("b")})

		got, err := FabricReplay{}.Entries(data)
		if err != nil {
			t.Fatalf("Entries() error = %v; want nil", err)
		}
		want := []fabricengine.BoardEntry{{Slug: "a", Status: "done", Created: created}, {Slug: "b"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Entries() = %+v; want %+v", got, want)
		}

		if got, err := (FabricReplay{}).Entries(nil); err != nil || len(got) != 0 {
			t.Errorf("Entries(nil) = %+v, %v; want no entries and no error", got, err)
		}
	})
}
