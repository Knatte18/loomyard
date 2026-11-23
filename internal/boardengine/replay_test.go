// replay_test.go — FabricReplay, the by-entry merge of a local board commit onto origin's board.
//
// Every row builds origin's board directory, the merge base, the commit's parent and the commit.
// It applies the commit and reads the board directory back.

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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

func asProject(task *Task) { task.Kind = KindProject }

func withProject(project string) func(*Task) {
	return func(task *Task) { task.Project = project }
}

func withOrder(slugs ...string) func(*Task) {
	return func(task *Task) { task.Order = slugs }
}

func asBeam(task *Task) { task.Beam = true }

func asDiscussed(task *Task) { task.Discussed = true }

func withRecordStamp(stamp string) func(*Task) {
	return func(task *Task) { task.RecordBodySHA256 = stamp }
}

// wantOriginFile returns a row check asserting that origin's board directory holds want under name.
// An empty want asserts that the file is absent.
func wantOriginFile(name, want string) func(t *testing.T, dir string, got map[string]Task) {
	return func(t *testing.T, dir string, _ map[string]Task) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dir, name))
		switch {
		case want == "" && !os.IsNotExist(err):
			t.Errorf("%s = %q, %v; want it absent", name, got, err)
		case want != "" && (err != nil || string(got) != want):
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

// withUnknownKey gives the entry one board.json key this binary does not declare, with raw as its JSON value.
func withUnknownKey(key, raw string) func(*Task) {
	return func(task *Task) { task.Unknown = map[string]json.RawMessage{key: json.RawMessage(raw)} }
}

func withBeamColor(color string) func(*Task) {
	return func(task *Task) { task.BeamColor = color }
}

// boardJSON encodes tasks as a board.json, numbering the entries by position.
func boardJSON(t *testing.T, tasks []Task) []byte {
	t.Helper()
	return boardJSONWithPrime(t, tasks, "")
}

// boardJSONWithPrime encodes tasks as a board.json that records prime as the hub's prime owner, none when empty.
func boardJSONWithPrime(t *testing.T, tasks []Task, prime string) []byte {
	t.Helper()
	return boardJSONWithHolds(t, tasks, prime, nil, nil)
}

// boardJSONWithHolds encodes tasks as a board.json that records prime as the hub's prime owner.
// It adds the top-level holds and machines keys.
// Each key is absent when empty.
func boardJSONWithHolds(t *testing.T, tasks []Task, prime string, holds []Hold, machines Registry) []byte {
	t.Helper()
	numbered := make([]Task, len(tasks))
	for i, task := range tasks {
		task.ID = i
		numbered[i] = task
	}
	data, err := json.Marshal(struct {
		Version    int      `json:"version"`
		Entries    []Task   `json:"entries"`
		PrimeOwner string   `json:"prime_owner,omitempty"`
		Machines   Registry `json:"machines,omitempty"`
		Holds      []Hold   `json:"holds,omitempty"`
	}{1, numbered, prime, machines, holds})
	if err != nil {
		t.Fatalf("marshal board.json: %v", err)
	}
	return data
}

// recordJSON encodes a holds list or a registry for comparison, empty when it holds none.
func recordJSON(t *testing.T, record any, empty bool) string {
	t.Helper()
	if empty {
		return ""
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	return string(data)
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

// replayRow is one scenario.
// It holds origin's board and files, the three sides of the local commit, and the checks after the replay.
type replayRow struct {
	name        string
	extraLabel  bool
	origin      []Task
	base        []Task
	parent      []Task
	commit      []Task
	originFiles map[string]string
	designs     []fabricengine.ReplayFile
	// originPrime, basePrime, parentPrime and commitPrime are each side's prime_owner; wantPrime is held afterwards.
	originPrime, basePrime, parentPrime, commitPrime, wantPrime string
	wantEmpty                                                   bool
	// originHolds, baseHolds, parentHolds and commitHolds are each side's holds, nil leaving the key absent.
	// wantHolds is held afterwards.
	originHolds, baseHolds, parentHolds, commitHolds, wantHolds []Hold
	// originMachines, baseMachines, parentMachines and commitMachines are each side's registry, nil leaving the key absent.
	originMachines, baseMachines, parentMachines, commitMachines, wantMachines Registry
	wantLost                                                                   []fabricengine.ReplayFinding
	// wantRefused maps an entry to a substring of its refusal reason.
	wantRefused map[string]string
	// wantEntries maps a slug to the title the board holds afterwards, "" meaning the entry is absent.
	wantEntries map[string]string
	check       func(t *testing.T, dir string, got map[string]Task)
}

func TestFabricReplay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	runLocked := RunStatus("running", "Implementer")
	ghostDependency := replayEntry("x", withDependsOn("ghost"))
	// A commit adds the raddle store directory.
	// store.json records the filed slugs p1 and p2, and proposal.yaml is a plain staged file.
	const (
		storeJSON         = `{"created":"2026-10-01T08:00:00Z","hash":"h","filed":["p1","p2"]}`
		storeJSONPath     = "raddle/plan/store.json"
		storeProposalPath = "raddle/plan/proposal.yaml"
	)
	storeFiles := []fabricengine.ReplayFile{
		{Path: storeJSONPath, After: []byte(storeJSON)},
		{Path: storeProposalPath, After: []byte("proposal")},
	}
	holdAt := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	adoptHold := func(machine string, subjects ...string) Hold {
		hold := Hold{Kind: HoldKindAdopt, Machine: machine, At: holdAt}
		for _, subject := range subjects {
			hold.Subjects = append(hold.Subjects, HoldSubject{Subject: subject})
		}
		return hold
	}
	oneEntry := []Task{replayEntry("a")}
	beamEntry := []Task{replayEntry("a", asProject, asBeam)}
	legacyPullRequest := []Task{replayEntry("a", withUnknownKey("pull_request", `"merge"`))}
	rows := []replayRow{
		{
			name:        "ACommitThatSetsOnlyALegacyNotifyOnLandLandsAsBeamLandNotice",
			base:        oneEntry,
			origin:      oneEntry,
			parent:      oneEntry,
			commit:      []Task{replayEntry("a", withUnknownKey("notify_on_land", "true"))},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				t.Helper()
				want := map[string]any{"beam": map[string]any{"land_notice": "on"}}
				if !reflect.DeepEqual(got["a"].RunSettings, want) {
					t.Errorf("entry a run_settings = %v; want %v", got["a"].RunSettings, want)
				}
			},
		},
		{
			name:        "RemovingAnUnchangedEntryThatCarriesALegacyPullRequestMakesNoConflict",
			base:        legacyPullRequest,
			origin:      legacyPullRequest,
			parent:      legacyPullRequest,
			commit:      nil,
			wantEntries: map[string]string{"a": ""},
		},
		{
			name:        "AHoldTheCommitAddedLandsOnOrigin",
			base:        oneEntry,
			origin:      oneEntry,
			parent:      oneEntry,
			commit:      oneEntry,
			commitHolds: []Hold{adoptHold("neve", "beam")},
			wantHolds:   []Hold{adoptHold("neve", "beam")},
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:        "AHoldRemovedByTheCommitLeavesOrigin",
			base:        oneEntry,
			origin:      oneEntry,
			parent:      oneEntry,
			commit:      oneEntry,
			baseHolds:   []Hold{adoptHold("neve", "beam")},
			parentHolds: []Hold{adoptHold("neve", "beam")},
			originHolds: []Hold{adoptHold("neve", "beam")},
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:        "AHoldTheCommitRewroteToThisMachineReplacesOriginsCopy",
			base:        oneEntry,
			origin:      oneEntry,
			parent:      oneEntry,
			commit:      oneEntry,
			baseHolds:   []Hold{adoptHold("box", "beam")},
			parentHolds: []Hold{adoptHold("box", "beam")},
			originHolds: []Hold{adoptHold("box", "beam")},
			commitHolds: []Hold{adoptHold("neve", "beam")},
			wantHolds:   []Hold{adoptHold("neve", "beam")},
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:        "AParentHoldOriginNoLongerHoldsIsLost",
			base:        oneEntry,
			origin:      oneEntry,
			parent:      oneEntry,
			commit:      oneEntry,
			baseHolds:   []Hold{adoptHold("neve", "beam")},
			parentHolds: []Hold{adoptHold("neve", "beam")},
			wantEmpty:   true,
			wantLost:    []fabricengine.ReplayFinding{{Entry: "holds", Field: "hold", Reason: "origin no longer holds the hold this commit changes"}},
			wantEntries: map[string]string{"a": "a"},
		},
		{
			name:           "AnAdoptCommitMeetingAnotherMachinesNewHoldIsRefusedWhole",
			base:           beamEntry,
			origin:         beamEntry,
			parent:         beamEntry,
			commit:         []Task{replayEntry("a", asProject, asBeam, withOwner("neve"))},
			basePrime:      "base",
			originPrime:    "base",
			parentPrime:    "base",
			commitPrime:    "neve",
			wantPrime:      "base",
			originHolds:    []Hold{adoptHold("box", "beam")},
			commitHolds:    []Hold{adoptHold("neve", "beam")},
			originMachines: Registry{"id-box": {Name: "box"}},
			commitMachines: Registry{"id-neve": {Name: "neve"}},
			wantHolds:      []Hold{adoptHold("box", "beam")},
			wantMachines:   Registry{"id-box": {Name: "box"}},
			wantEmpty:      true,
			wantRefused:    map[string]string{"holds": `machine "box" written at 2026-11-16T10:00:00Z`},
			wantEntries:    map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].OwnerMachine != "" {
					t.Errorf("owner_machine = %q; want it unchanged, since the commit is refused whole", got["a"].OwnerMachine)
				}
			},
		},
		{
			name:        "ACommitTouchingNoHoldReplaysOverAHoldOriginGained",
			base:        []Task{replayEntry("a", withTitle("A"))},
			origin:      []Task{replayEntry("a", withTitle("A"))},
			parent:      []Task{replayEntry("a", withTitle("A"))},
			commit:      []Task{replayEntry("a", withTitle("A2"))},
			originHolds: []Hold{adoptHold("box", "beam")},
			wantHolds:   []Hold{adoptHold("box", "beam")},
			wantEntries: map[string]string{"a": "A2"},
		},
		{
			name:           "AHoldOfTheSameMachineUnderAFormerNameIsNotAnotherMachines",
			base:           oneEntry,
			origin:         oneEntry,
			parent:         oneEntry,
			commit:         oneEntry,
			originMachines: Registry{"m1": {Name: "neve", Former: []string{"neve-old"}}},
			originHolds:    []Hold{adoptHold("neve-old", "beam")},
			commitHolds:    []Hold{adoptHold("neve", "root")},
			wantMachines:   Registry{"m1": {Name: "neve", Former: []string{"neve-old"}}},
			wantHolds:      []Hold{adoptHold("neve-old", "beam"), adoptHold("neve", "root")},
			wantEntries:    map[string]string{"a": "a"},
		},
		{
			name:           "ARegistryEntryTheCommitAddedLands",
			base:           oneEntry,
			origin:         oneEntry,
			parent:         oneEntry,
			commit:         oneEntry,
			commitMachines: Registry{"m1": {Name: "neve"}},
			wantMachines:   Registry{"m1": {Name: "neve"}},
			wantEntries:    map[string]string{"a": "a"},
		},
		{
			name:           "ARegistryEntryOriginChangedSinceTheBaseIsLost",
			base:           oneEntry,
			origin:         oneEntry,
			parent:         oneEntry,
			commit:         oneEntry,
			baseMachines:   Registry{"m1": {Name: "neve"}},
			parentMachines: Registry{"m1": {Name: "neve"}},
			originMachines: Registry{"m1": {Name: "neve", Former: []string{"x"}}},
			commitMachines: Registry{"m1": {Name: "neve", Former: []string{"y"}}},
			wantMachines:   Registry{"m1": {Name: "neve", Former: []string{"x"}}},
			wantEmpty:      true,
			wantLost:       []fabricengine.ReplayFinding{{Entry: "machines", Field: "m1", Reason: "origin changed the machine's entry since the merge base"}},
			wantEntries:    map[string]string{"a": "a"},
		},
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
			wantLost:    []fabricengine.ReplayFinding{{Entry: "a", Field: "title", Reason: "origin changed the field too", Local: `"local"`}},
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
			name:        "HeldTaskBecomingASketchIsNotRefused",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a", withStatus(runLocked))},
			parent:      []Task{replayEntry("a", withStatus(runLocked))},
			commit:      []Task{replayEntry("a", withLabels("sketch"), asNote)},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].Kind != KindNote || got["a"].Status != nil {
					t.Errorf("a kind, status = %q, %v; want note, none", got["a"].Kind, got["a"].Status)
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
			name:        "NoteWithDependsOnLands",
			base:        []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("a"), replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("a"), replayEntry("b", withTitle("B2")), replayEntry("x", asNote, withDependsOn("a"))},
			wantEntries: map[string]string{"b": "B2", "x": "x"},
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
			wantLost:    []fabricengine.ReplayFinding{{Entry: "b", Reason: "origin removed the entry"}},
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
			wantLost:    []fabricengine.ReplayFinding{{Entry: "b", Reason: "origin changed the entry this commit removes"}},
			wantEntries: map[string]string{"a": "a", "b": "origin"},
		},
		{
			name:        "OwnerChangedOnBothSidesKeepsOriginsAndReportsTheLocalOneLost",
			base:        []Task{replayEntry("a", asProject, asBeam)},
			origin:      []Task{replayEntry("a", asProject, asBeam, withOwner("box"))},
			parent:      []Task{replayEntry("a", asProject, asBeam)},
			commit:      []Task{replayEntry("a", asProject, asBeam, withOwner("neve"))},
			wantEmpty:   true,
			wantLost:    []fabricengine.ReplayFinding{{Entry: "a", Field: "owner_machine", Reason: "origin changed the field too", Local: `"neve"`}},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].OwnerMachine != "box" {
					t.Errorf("owner_machine = %q; want origin's box", got["a"].OwnerMachine)
				}
			},
		},
		{
			name:        "BeamColorOnlyTheCommitChangedLandsOnABeamOriginLacksItOn",
			base:        []Task{replayEntry("a", asProject, asBeam)},
			origin:      []Task{replayEntry("a", asProject, asBeam)},
			parent:      []Task{replayEntry("a", asProject, asBeam)},
			commit:      []Task{replayEntry("a", asProject, asBeam, withBeamColor("#7d2d6b"))},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].BeamColor != "#7d2d6b" {
					t.Errorf("beam_color = %q; want the commit's #7d2d6b", got["a"].BeamColor)
				}
			},
		},
		{
			name:        "BeamFlagClearedByTheCommitDropsTheColourOriginRecorded",
			base:        []Task{replayEntry("a", asProject, asBeam)},
			origin:      []Task{replayEntry("a", asProject, asBeam, withBeamColor("#7d2d6b"))},
			parent:      []Task{replayEntry("a", asProject, asBeam)},
			commit:      []Task{replayEntry("a", asProject)},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].Beam || got["a"].BeamColor != "" {
					t.Errorf("beam = %v, beam_color = %q; want a non-beam entry with no colour", got["a"].Beam, got["a"].BeamColor)
				}
			},
		},
		{
			name:        "OwnerOnlyTheCommitChangedLandsOnARunHeldEntry",
			base:        []Task{replayEntry("a", asProject, asBeam, withStatus(runLocked))},
			origin:      []Task{replayEntry("a", asProject, asBeam, withStatus(runLocked))},
			parent:      []Task{replayEntry("a", asProject, asBeam, withStatus(runLocked))},
			commit:      []Task{replayEntry("a", asProject, asBeam, withStatus(runLocked), withOwner("neve"))},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].OwnerMachine != "neve" {
					t.Errorf("owner_machine = %q; want the commit's neve", got["a"].OwnerMachine)
				}
			},
		},
		{
			name:        "ProjectAndOrderChangesLand",
			base:        []Task{replayEntry("p", asProject), replayEntry("a")},
			origin:      []Task{replayEntry("p", asProject), replayEntry("a")},
			parent:      []Task{replayEntry("p", asProject), replayEntry("a")},
			commit:      []Task{replayEntry("p", asProject, withOrder("a")), replayEntry("a", withProject("p"))},
			wantEntries: map[string]string{"p": "p", "a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].Project != "p" || !reflect.DeepEqual(got["p"].Order, []string{"a"}) {
					t.Errorf("a.project = %q, p.order = %v; want p and [a]", got["a"].Project, got["p"].Order)
				}
			},
		},
		{
			name:        "OrderOrBeamOnATaskOrANoteIsRefusedNamingTheField",
			base:        []Task{replayEntry("b", withTitle("B"))},
			origin:      []Task{replayEntry("b", withTitle("B"))},
			parent:      []Task{replayEntry("b", withTitle("B"))},
			commit:      []Task{replayEntry("b", withTitle("B2")), replayEntry("task-order", withOrder("b")), replayEntry("note-order", asNote, withOrder("b")), replayEntry("task-beam", asBeam), replayEntry("note-beam", asNote, asBeam)},
			wantRefused: map[string]string{"task-order": "order", "note-order": "order", "task-beam": "beam", "note-beam": "beam"},
			wantEntries: map[string]string{"b": "B2", "task-order": "", "note-order": "", "task-beam": "", "note-beam": ""},
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
			wantLost:    []fabricengine.ReplayFinding{{Entry: "prime", Field: "prime_owner", Reason: "origin changed the field too", Local: `"neve"`}},
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
			designs:     []fabricengine.ReplayFile{{Path: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			wantEmpty:   true,
			wantLost:    []fabricengine.ReplayFinding{{Entry: "design-a.md", Field: "design file", Reason: "origin changed the file too"}},
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
			designs:     []fabricengine.ReplayFile{{Path: "design-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			check: func(t *testing.T, dir string, _ map[string]Task) {
				if got, _ := os.ReadFile(filepath.Join(dir, "design-a.md")); string(got) != "local" {
					t.Errorf("design-a.md = %q; want the commit's file", got)
				}
			},
		},
		{
			name:    "RecordFileAddedLandsWhenOriginLacksIt",
			base:    []Task{replayEntry("a")},
			origin:  []Task{replayEntry("a")},
			parent:  []Task{replayEntry("a")},
			commit:  []Task{replayEntry("a")},
			designs: []fabricengine.ReplayFile{{Path: "decision-a.md", After: []byte("record")}},
			check:   wantOriginFile("decision-a.md", "record"),
		},
		{
			name:        "RecordFileChangedLandsWhenOriginHoldsTheParentsFile",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			originFiles: map[string]string{"decision-a.md": "base"},
			designs:     []fabricengine.ReplayFile{{Path: "decision-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			check:       wantOriginFile("decision-a.md", "local"),
		},
		{
			name:        "RecordFileRemovedLandsWhenOriginsFileIsUntouched",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			originFiles: map[string]string{"decision-a.md": "base"},
			designs:     []fabricengine.ReplayFile{{Path: "decision-a.md", Base: []byte("base"), Before: []byte("base")}},
			check:       wantOriginFile("decision-a.md", ""),
		},
		{
			name:        "RecordFileChangedOnBothSidesKeepsOriginsFileAndReportsItLost",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a")},
			originFiles: map[string]string{"decision-a.md": "origin"},
			designs:     []fabricengine.ReplayFile{{Path: "decision-a.md", Base: []byte("base"), Before: []byte("base"), After: []byte("local")}},
			wantEmpty:   true,
			wantLost:    []fabricengine.ReplayFinding{{Entry: "decision-a.md", Field: "decision record", Reason: "origin changed the file too"}},
			check:       wantOriginFile("decision-a.md", "origin"),
		},
		{
			name:        "RecordFileWrittenForAnEntryOriginLacksIsLostAndNotWritten",
			base:        []Task{replayEntry("a", withTitle("A")), replayEntry("b")},
			origin:      []Task{replayEntry("b")},
			parent:      []Task{replayEntry("a", withTitle("A")), replayEntry("b")},
			commit:      []Task{replayEntry("a", withTitle("A2")), replayEntry("b")},
			designs:     []fabricengine.ReplayFile{{Path: "decision-a.md", After: []byte("record")}},
			wantEmpty:   true,
			wantLost:    []fabricengine.ReplayFinding{{Entry: "a", Reason: "origin removed the entry"}, {Entry: "decision-a.md", Field: "decision record", Reason: "origin does not hold the entry"}},
			wantEntries: map[string]string{"a": "", "b": "b"},
			check:       wantOriginFile("decision-a.md", ""),
		},
		{
			name:        "RemovedEntryAndItsRecordFileLeaveNoRecordOnOrigin",
			base:        []Task{replayEntry("a"), replayEntry("b")},
			origin:      []Task{replayEntry("a"), replayEntry("b")},
			parent:      []Task{replayEntry("a"), replayEntry("b")},
			commit:      []Task{replayEntry("b")},
			originFiles: map[string]string{"decision-a.md": "record"},
			designs:     []fabricengine.ReplayFile{{Path: "decision-a.md", Base: []byte("record"), Before: []byte("record")}},
			wantEntries: map[string]string{"a": "", "b": "b"},
			check:       wantOriginFile("decision-a.md", ""),
		},
		{
			name:        "RenameCommitLeavesNoOldRecordFileOnOrigin",
			base:        []Task{replayEntry("a", asDiscussed)},
			origin:      []Task{replayEntry("a", asDiscussed)},
			parent:      []Task{replayEntry("a", asDiscussed)},
			commit:      []Task{replayEntry("c", asDiscussed)},
			originFiles: map[string]string{"decision-a.md": "record"},
			designs: []fabricengine.ReplayFile{
				{Path: "decision-a.md", Base: []byte("record"), Before: []byte("record")},
				{Path: "decision-c.md", After: []byte("record")},
			},
			wantEntries: map[string]string{"a": "", "c": "c"},
			check: func(t *testing.T, dir string, _ map[string]Task) {
				wantOriginFile("decision-a.md", "")(t, dir, nil)
				wantOriginFile("decision-c.md", "record")(t, dir, nil)
			},
		},
		{
			name:        "DiscussedFlagSetByTheCommitLandsOnOriginsEntry",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a", asDiscussed)},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if !got["a"].Discussed {
					t.Errorf("a.discussed = false; want true")
				}
			},
		},
		{
			name:        "RecordStampChangedWithTheBodyLandsOnOriginsEntry",
			base:        []Task{replayEntry("a", asDiscussed, withBody("first"), withRecordStamp("stamp-1"))},
			origin:      []Task{replayEntry("a", asDiscussed, withBody("first"), withRecordStamp("stamp-1"))},
			parent:      []Task{replayEntry("a", asDiscussed, withBody("first"), withRecordStamp("stamp-1"))},
			commit:      []Task{replayEntry("a", asDiscussed, withBody("second"), withRecordStamp("stamp-2"))},
			wantEntries: map[string]string{"a": "a"},
			check: func(t *testing.T, _ string, got map[string]Task) {
				if got["a"].Body != "second" || got["a"].RecordBodySHA256 != "stamp-2" {
					t.Errorf("a body, record_body_sha256 = %q, %q; want second, stamp-2", got["a"].Body, got["a"].RecordBodySHA256)
				}
			},
		},
		{
			name:        "RaddleStoreDirectoryAndItsFiledEntriesReplayTogether",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a"), replayEntry("p1"), replayEntry("p2")},
			designs:     storeFiles,
			wantEntries: map[string]string{"a": "a", "p1": "p1", "p2": "p2"},
			check: func(t *testing.T, dir string, got map[string]Task) {
				wantOriginFile(storeJSONPath, storeJSON)(t, dir, got)
				wantOriginFile(storeProposalPath, "proposal")(t, dir, got)
			},
		},
		{
			name:        "RaddleStoreReplaysBesideAConcurrentWriteOnOrigin",
			base:        []Task{replayEntry("a", withTitle("A"))},
			origin:      []Task{replayEntry("a", withTitle("A2"))},
			parent:      []Task{replayEntry("a", withTitle("A"))},
			commit:      []Task{replayEntry("a", withTitle("A")), replayEntry("p1"), replayEntry("p2")},
			designs:     storeFiles,
			wantEntries: map[string]string{"a": "A2", "p1": "p1", "p2": "p2"},
			check: func(t *testing.T, dir string, got map[string]Task) {
				wantOriginFile(storeJSONPath, storeJSON)(t, dir, got)
				wantOriginFile(storeProposalPath, "proposal")(t, dir, got)
			},
		},
		{
			name:    "RaddleStoreIsSetAsideWhenAFiledEntryOriginAlreadyHoldsUnderThatSlug",
			base:    []Task{replayEntry("a")},
			origin:  []Task{replayEntry("a"), replayEntry("p1", withTitle("other"))},
			parent:  []Task{replayEntry("a")},
			commit:  []Task{replayEntry("a"), replayEntry("p1"), replayEntry("p2")},
			designs: storeFiles,
			wantLost: []fabricengine.ReplayFinding{
				{Entry: "p1", Field: "title", Reason: "origin added an entry with this slug too", Local: `"p1"`},
			},
			wantRefused: map[string]string{storeJSONPath: `Sketch "plan"`, storeProposalPath: "`_lym/raddle/`"},
			wantEntries: map[string]string{"a": "a", "p1": "other", "p2": "p2"},
			check: func(t *testing.T, dir string, got map[string]Task) {
				wantOriginFile(storeJSONPath, "")(t, dir, got)
				wantOriginFile(storeProposalPath, "")(t, dir, got)
			},
		},
		{
			name:        "RaddleStoreIsSetAsideWhenTheReplayRefusesAFiledEntry",
			base:        []Task{replayEntry("a")},
			origin:      []Task{replayEntry("a")},
			parent:      []Task{replayEntry("a")},
			commit:      []Task{replayEntry("a"), replayEntry("p1", withDependsOn("ghost")), replayEntry("p2")},
			designs:     storeFiles,
			wantRefused: map[string]string{"p1": "dangling dependency", storeJSONPath: `filed entry "p1" was refused`, storeProposalPath: "lym board upsert"},
			wantEntries: map[string]string{"a": "a", "p1": "", "p2": "p2"},
			check: func(t *testing.T, dir string, got map[string]Task) {
				wantOriginFile(storeJSONPath, "")(t, dir, got)
				wantOriginFile(storeProposalPath, "")(t, dir, got)
			},
		},
		{
			name:      "RaddleStorePathOneLevelDeeperIsNotClaimed",
			base:      []Task{replayEntry("a")},
			origin:    []Task{replayEntry("a")},
			parent:    []Task{replayEntry("a")},
			commit:    []Task{replayEntry("a")},
			designs:   []fabricengine.ReplayFile{{Path: "raddle/plan/deeper/store.json", After: []byte(storeJSON)}},
			wantEmpty: true,
			check:     wantOriginFile("raddle/plan/deeper/store.json", ""),
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
			if err := os.WriteFile(filepath.Join(dir, "board.json"), boardJSONWithHolds(t, tt.origin, tt.originPrime, tt.originHolds, tt.originMachines), 0o644); err != nil {
				t.Fatalf("WriteFile(board.json) = %v; want nil", err)
			}
			for name, content := range tt.originFiles {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", name, err)
				}
			}

			boardFileSides := fabricengine.ReplayFile{
				Path:   boardFile,
				Base:   boardJSONWithHolds(t, tt.base, tt.basePrime, tt.baseHolds, tt.baseMachines),
				Before: boardJSONWithHolds(t, tt.parent, tt.parentPrime, tt.parentHolds, tt.parentMachines),
				After:  boardJSONWithHolds(t, tt.commit, tt.commitPrime, tt.commitHolds, tt.commitMachines),
			}
			result, err := FabricReplay{}.Apply(fabricengine.ReplayRequest{
				Dir:   dir,
				Files: append([]fabricengine.ReplayFile{boardFileSides}, tt.designs...),
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
			if got, want := recordJSON(t, stored.holds, len(stored.holds) == 0), recordJSON(t, tt.wantHolds, len(tt.wantHolds) == 0); got != want {
				t.Errorf("holds = %s; want %s", got, want)
			}
			if got, want := recordJSON(t, stored.machines, len(stored.machines) == 0), recordJSON(t, tt.wantMachines, len(tt.wantMachines) == 0); got != want {
				t.Errorf("machines = %s; want %s", got, want)
			}
			if !tt.wantEmpty {
				readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
				if err != nil {
					t.Fatalf("ReadFile(README.md) = %v; want nil", err)
				}
				wantLine := "Prime owner: none recorded.\n"
				if tt.wantPrime != "" {
					wantLine = "Prime owner: `" + tt.wantPrime + "`.\n"
				}
				if !strings.Contains(string(readme), wantLine) {
					t.Errorf("README.md = %q; want it to contain %q", readme, wantLine)
				}
			}
			if tt.check != nil {
				tt.check(t, dir, got)
			}
		})
	}

	t.Run("RunsReturnsEachEntrysSlugEndedAndCreatedStamp", func(t *testing.T) {
		t.Parallel()
		created := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
		held := replayEntry("a", withStatus("done"))
		held.Created = created
		data := boardJSON(t, []Task{held, replayEntry("b")})

		got, err := FabricReplay{}.Runs(data)
		if err != nil {
			t.Fatalf("Runs() error = %v; want nil", err)
		}
		want := []fabricengine.LedgerRun{{Slug: "a", Ended: true, Created: created}, {Slug: "b"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Runs() = %+v; want %+v", got, want)
		}

		if got, err := (FabricReplay{}).Runs(nil); err != nil || len(got) != 0 {
			t.Errorf("Runs(nil) = %+v, %v; want no runs and no error", got, err)
		}
	})
}

func TestFabricReplay_Ledger(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("Claims", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			path string
			want bool
		}{
			{"Readme", "README.md", true},
			{"ProjectFile", "project-x.md", true},
			{"ProjectFileWithDashedSlug", "project-a-b.md", true},
			{"EmptySlug", "project-.md", false},
			{"SlugWithSlash", "project-a/b.md", false},
			{"ProjectFileInSubdirectory", "dir/project-x.md", false},
			{"NotMarkdown", "project-x.json", false},
			{"EntryStore", "board.json", true},
			{"DesignFile", "design-a.md", true},
			{"RecordFile", "decision-x.md", true},
			{"ViewProjection", "board-data.js", true},
			{"ViewerPage", "board.html", true},
			{"ViewerPageInSubdirectory", "sub/board.html", false},
			{"RecordFileInSubdirectory", "sub/decision-x.md", false},
			{"RecordFileEmptySlug", "decision-.md", false},
			{"RaddleStoreFile", "raddle/x/store.json", true},
			{"RaddleStoreFileOneLevelDeeper", "raddle/x/y/store.json", false},
			{"RaddleStoreDirectory", "raddle/x", false},
			{"RaddleStoreFileOfAnEmptySketch", "raddle//store.json", false},
			{"NestedReadme", "dir/README.md", false},
			{"SubdirectoryReadme", "sub/README.md", false},
			{"OtherStore", "heddle.json", false},
			{"NestedNotes", "notes/x.md", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := (FabricReplay{}).Claims(tt.path); got != tt.want {
					t.Errorf("Claims(%q) = %v; want %v", tt.path, got, tt.want)
				}
			})
		}
	})

	t.Run("CommitRuns", func(t *testing.T) {
		t.Parallel()
		two := boardJSON(t, []Task{replayEntry("a"), replayEntry("b")})
		retitled := boardJSON(t, []Task{replayEntry("a", withTitle("A2")), replayEntry("b")})
		onlyA := boardJSON(t, []Task{replayEntry("a")})
		tests := []struct {
			name          string
			before, after []byte
			paths         []string
			want          []string
		}{
			{"EntryChanged", two, retitled, []string{"board.json"}, []string{"a"}},
			{"EntryAdded", onlyA, two, []string{"board.json"}, []string{"b"}},
			{"EntryRemoved", two, onlyA, []string{"board.json"}, []string{"b"}},
			{"EntryStoreUnchanged", two, two, []string{"board.json"}, []string{}},
			{"DesignFileNamesItsEntry", nil, nil, []string{"design-a.md"}, []string{"a"}},
			{"RecordFileNamesItsEntry", nil, nil, []string{"decision-a.md"}, []string{"a"}},
			{"OverviewFileNamesNone", nil, nil, []string{"README.md", "project-x.md"}, []string{}},
			{"DesignFileAndEntryChange", two, retitled, []string{"board.json", "design-b.md"}, []string{"a", "b"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				got, err := (FabricReplay{}).CommitRuns(tt.before, tt.after, tt.paths)
				if err != nil {
					t.Fatalf("CommitRuns() error = %v; want nil", err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("CommitRuns() = %v; want %v", got, tt.want)
				}
			})
		}
	})
}
