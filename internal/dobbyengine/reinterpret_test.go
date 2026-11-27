// reinterpret_test.go pins the rewrite of a hub beam.yaml's retired pull_request and land_notice values.
// It pins the rewrite of pull_request's renamed on and merge too.
// It also pins the flags raised for a retired value in the worktrees' own files it is told,
// and the carry of the board entries' beam settings into the worktrees that need them.

package dobbyengine

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReinterpretPerTaskSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const requiredFlag = "does not keep what required meant"
	tests := []struct {
		name     string
		existing string
		// primeWorktree and primeLocal are the prime's own files, written when set.
		primeWorktree, primeLocal string
		// wantValues are the top-level scalars the output must decode to.
		wantValues map[string]any
		// wantRewrote holds one substring per rewrote line, in order.
		wantRewrote []string
		// wantFlagged holds one substring list per flagged line, in order.
		// {worktree} and {local} stand for the paths of the prime's worktree and local files.
		wantFlagged [][]string
		// wantKept are texts the output must still hold, such as comments.
		wantKept []string
	}{
		{
			name:        "a required pull_request becomes review and is flagged for what review does not keep",
			existing:    "pull_request: required\n",
			wantValues:  map[string]any{"pull_request": "review"},
			wantRewrote: []string{"pull_request: required is retired; it now reads review"},
			wantFlagged: [][]string{{"pull_request: required", requiredFlag, "lym beam ready", "lym config beam --layer default --set pull_request=trail", "GitHub branch rules"}},
		},
		{
			name:        "an all land_notice becomes on",
			existing:    "land_notice: all\n",
			wantValues:  map[string]any{"land_notice": "on"},
			wantRewrote: []string{"land_notice: all is retired; it now reads on"},
		},
		{
			name:        "a per-task land_notice becomes off",
			existing:    "land_notice: per-task\n",
			wantValues:  map[string]any{"land_notice": "off"},
			wantRewrote: []string{"land_notice: per-task is retired; it now reads off"},
		},
		{
			name:        "comments and other keys survive the rewrite of every retired value",
			existing:    "# the beam's settings\nmax_runs: 3  # at most three\npull_request: required  # how a task lands\nland_notice: all\nadopt_fix_tries: 2\n",
			wantValues:  map[string]any{"max_runs": 3, "pull_request": "review", "land_notice": "on", "adopt_fix_tries": 2},
			wantRewrote: []string{"pull_request: required is retired", "land_notice: all is retired"},
			wantFlagged: [][]string{{"pull_request: required", requiredFlag}},
			wantKept:    []string{"# the beam's settings", "# at most three", "# how a task lands"},
		},
		{
			name:       "current values are left alone beside a retired one",
			existing:   "pull_request: merge-in\nland_notice: per-task\n",
			wantValues: map[string]any{"pull_request": "merge-in", "land_notice": "off"},
			wantRewrote: []string{
				"land_notice: per-task is retired",
			},
		},
		{
			name:       "a file with only current values comes back unchanged",
			existing:   "# keep this\nmax_runs:   3\npull_request: \"off\"\nland_notice: on\n",
			wantValues: map[string]any{"max_runs": 3, "pull_request": "off", "land_notice": "on"},
		},
		{
			name:       "an empty file comes back unchanged",
			existing:   "",
			wantValues: map[string]any{},
		},
		{
			name:          "a retired value in the prime's worktree file is flagged with its file and the way forward",
			existing:      "max_runs: 3\n",
			primeWorktree: "pull_request: required\n",
			wantValues:    map[string]any{"max_runs": 3},
			wantFlagged: [][]string{
				{"{worktree}", "pull_request: required", "lym config beam --set pull_request=review", "in the prime", requiredFlag},
				{"a worktree file's beam.pull_request now decides that worktree's own landing", "{worktree}"},
			},
		},
		{
			name:        "a retired value in the prime's local file is flagged with its file and the way forward",
			existing:    "max_runs: 3\n",
			primeLocal:  "land_notice: per-task\n",
			wantValues:  map[string]any{"max_runs": 3},
			wantFlagged: [][]string{{"{local}", "land_notice: per-task", "lym config beam --set land_notice=off"}},
		},
		{
			name:          "current values in the prime's files flag only that a worktree's pull_request decides its own landing",
			existing:      "max_runs: 3\n",
			primeWorktree: "pull_request: trail\n",
			primeLocal:    "land_notice: \"on\"\n",
			wantValues:    map[string]any{"max_runs": 3},
			wantFlagged:   [][]string{{"a worktree file's beam.pull_request now decides that worktree's own landing", "{worktree}"}},
		},
		{
			name:        "a prime file that does not read is flagged and the hub file is still rewritten",
			existing:    "land_notice: all\n",
			primeLocal:  "no_such_key: 1\n",
			wantValues:  map[string]any{"land_notice": "on"},
			wantRewrote: []string{"land_notice: all is retired"},
			wantFlagged: [][]string{{"could not be read", "{local}"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var worktrees []configengine.HubWorktree
			worktreeFile, localFile := "", ""
			if tt.primeWorktree != "" || tt.primeLocal != "" {
				anchor := t.TempDir()
				worktrees = []configengine.HubWorktree{{Name: "prime", Anchor: anchor, Main: true}}
				worktreeFile = configengine.ConfigFile(anchor, "beam")
				localFile = configengine.LocalFile(anchor, "beam")
				writeBeamLayer(t, anchor, tt.primeWorktree)
				if tt.primeLocal != "" {
					writeBeamLocalLayer(t, anchor, tt.primeLocal)
				}
			}

			got, rewrote, flagged, err := ReinterpretPerTaskSettings([]byte(tt.existing), worktrees)
			if err != nil {
				t.Fatalf("ReinterpretPerTaskSettings() error = %v; want nil", err)
			}
			if len(rewrote) != len(tt.wantRewrote) {
				t.Fatalf("ReinterpretPerTaskSettings() rewrote = %q; want %d lines", rewrote, len(tt.wantRewrote))
			}
			for i, want := range tt.wantRewrote {
				if !strings.Contains(rewrote[i], want) {
					t.Errorf("rewrote[%d] = %q; want it to contain %q", i, rewrote[i], want)
				}
			}
			if len(tt.wantRewrote) == 0 && string(got) != tt.existing {
				t.Errorf("ReinterpretPerTaskSettings() = %q; want the input back byte for byte %q", got, tt.existing)
			}
			if len(flagged) != len(tt.wantFlagged) {
				t.Fatalf("ReinterpretPerTaskSettings() flagged = %q; want %d lines", flagged, len(tt.wantFlagged))
			}
			for i, wants := range tt.wantFlagged {
				for _, want := range wants {
					want = strings.NewReplacer("{worktree}", worktreeFile, "{local}", localFile).Replace(want)
					if !strings.Contains(flagged[i], want) {
						t.Errorf("flagged[%d] = %q; want it to contain %q", i, flagged[i], want)
					}
				}
			}

			var decoded map[string]any
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("output %q does not parse: %v", got, err)
			}
			if len(decoded) != len(tt.wantValues) {
				t.Errorf("output decodes to %v; want %v", decoded, tt.wantValues)
			}
			for key, want := range tt.wantValues {
				if decoded[key] != want {
					t.Errorf("output %s = %v; want %v\n%s", key, decoded[key], want, got)
				}
			}
			for _, kept := range tt.wantKept {
				if !strings.Contains(string(got), kept) {
					t.Errorf("output = %q; want it to keep %q", got, kept)
				}
			}
		})
	}
}

// TestReinterpretPerTaskSettings_ToldWorktrees pins that the flags land at the anchors the reinterpretation is told.
// A worktree whose anchor differs from the prime's is read there, and a directory it is not told of is never read.
//
//testtiming:keep pins that a directory no told worktree names is never read for a retired value
func TestReinterpretPerTaskSettings_ToldWorktrees(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	primeAnchor := filepath.Join(hub, "prime")
	beamAnchor := filepath.Join(hub, "beam-a", "code")
	// The directory at the prime's own anchor offset, and a records sibling holding a .git entry, are not told.
	decoy := filepath.Join(hub, "beam-a")
	sibling := filepath.Join(hub, "beam-a-records")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", sibling, err)
	}
	if err := os.WriteFile(filepath.Join(sibling, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git) = %v; want nil", err)
	}
	writeBeamLayer(t, primeAnchor, "max_runs: 1\n")
	writeBeamLayer(t, beamAnchor, "pull_request: required\n")
	writeBeamLayer(t, decoy, "land_notice: all\n")
	writeBeamLayer(t, sibling, "land_notice: per-task\n")
	worktrees := []configengine.HubWorktree{
		{Name: "prime", Anchor: primeAnchor, Main: true},
		{Name: "beam-a", Anchor: beamAnchor},
	}

	_, _, flagged, err := ReinterpretPerTaskSettings(nil, worktrees)
	if err != nil {
		t.Fatalf("ReinterpretPerTaskSettings() error = %v; want nil", err)
	}

	wantFlag := []string{configengine.ConfigFile(beamAnchor, "beam"), "pull_request: required", "lym config beam --set pull_request=review", "in the worktree beam-a"}
	if !containsLineWith(flagged, wantFlag...) {
		t.Errorf("flagged = %q; want a line containing every one of %q", flagged, wantFlag)
	}
	for _, untold := range []string{configengine.ConfigFile(decoy, "beam"), configengine.ConfigFile(sibling, "beam")} {
		if containsLineWith(flagged, untold) {
			t.Errorf("flagged = %q; want %s never read, since it was not told", flagged, untold)
		}
	}
}

func TestReinterpretPullRequestModes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const onMeaning = "which keeps what on meant, except that the review now ends only on the operator's word " +
		"whatever beam.autonomy says, since no parent review ends it"
	tests := []struct {
		name     string
		existing string
		// worktreeFile and localFile are the own files of a worktree called beam-a, written when set.
		worktreeFile, localFile string
		// wantValues are the top-level scalars the output must decode to.
		wantValues map[string]any
		// wantRewrote holds the rewrote lines, in order.
		wantRewrote []string
		// wantFlagged holds the flagged lines, in order.
		// {worktree} and {local} stand for the paths of the worktree's own file and local file.
		wantFlagged []string
		// wantKept are texts the output must still hold, such as comments.
		wantKept []string
	}{
		{
			name:        "an on pull_request in the hub file becomes review",
			existing:    "pull_request: on\n",
			wantValues:  map[string]any{"pull_request": "review"},
			wantRewrote: []string{"beam.pull_request: on is retired; it now reads review, " + onMeaning},
		},
		{
			name:        "a merge pull_request in the hub file becomes trail",
			existing:    "pull_request: merge\n",
			wantValues:  map[string]any{"pull_request": "trail"},
			wantRewrote: []string{"beam.pull_request: merge is retired; it now reads trail, which keeps what it meant"},
		},
		{
			name:        "comments and other keys survive the rewrite",
			existing:    "# the beam's settings\nmax_runs: 3  # at most three\npull_request: merge  # how a task lands\nland_notice: \"on\"\n",
			wantValues:  map[string]any{"max_runs": 3, "pull_request": "trail", "land_notice": "on"},
			wantRewrote: []string{"beam.pull_request: merge is retired; it now reads trail, which keeps what it meant"},
			wantKept:    []string{"# the beam's settings", "# at most three", "# how a task lands"},
		},
		{
			name:       "a hub file with only current values comes back unchanged",
			existing:   "# keep this\nmax_runs:   3\npull_request: merge-in\nland_notice: on\n",
			wantValues: map[string]any{"max_runs": 3, "pull_request": "merge-in", "land_notice": "on"},
		},
		{
			name:       "a land_notice of on is not a renamed pull_request value",
			existing:   "land_notice: \"on\"\n",
			wantValues: map[string]any{"land_notice": "on"},
		},
		{
			name:       "an empty hub file comes back unchanged",
			existing:   "",
			wantValues: map[string]any{},
		},
		{
			name:         "a merge in a worktree's own file is flagged with its file and the way forward",
			existing:     "max_runs: 3\n",
			worktreeFile: "pull_request: merge\n",
			wantValues:   map[string]any{"max_runs": 3},
			wantFlagged: []string{
				"{worktree} sets beam.pull_request: merge, which is retired, so beam.yaml stops loading there; " +
					"way forward: run `lym config beam --set pull_request=trail` in the worktree beam-a, which keeps what it meant",
			},
		},
		{
			name:         "an on in a worktree's own file is flagged with the review way forward",
			existing:     "max_runs: 3\n",
			worktreeFile: "pull_request: \"on\"\n",
			wantValues:   map[string]any{"max_runs": 3},
			wantFlagged: []string{
				"{worktree} sets beam.pull_request: on, which is retired, so beam.yaml stops loading there; " +
					"way forward: run `lym config beam --set pull_request=review` in the worktree beam-a, " + onMeaning,
			},
		},
		{
			name:       "a merge in a worktree's local file is flagged with that file",
			existing:   "max_runs: 3\n",
			localFile:  "pull_request: merge\n",
			wantValues: map[string]any{"max_runs": 3},
			wantFlagged: []string{
				"{local} sets beam.pull_request: merge, which is retired, so beam.yaml stops loading there; " +
					"way forward: run `lym config beam --set pull_request=trail` in the worktree beam-a, which keeps what it meant",
			},
		},
		{
			name:         "current values in a worktree's files raise no flag",
			existing:     "max_runs: 3\n",
			worktreeFile: "pull_request: trail\n",
			localFile:    "land_notice: \"on\"\n",
			wantValues:   map[string]any{"max_runs": 3},
		},
		{
			name:         "the hub file is rewritten and a worktree's file is flagged in the same pass",
			existing:     "pull_request: on\n",
			worktreeFile: "pull_request: merge\n",
			wantValues:   map[string]any{"pull_request": "review"},
			wantRewrote:  []string{"beam.pull_request: on is retired; it now reads review, " + onMeaning},
			wantFlagged: []string{
				"{worktree} sets beam.pull_request: merge, which is retired, so beam.yaml stops loading there; " +
					"way forward: run `lym config beam --set pull_request=trail` in the worktree beam-a, which keeps what it meant",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var worktrees []configengine.HubWorktree
			worktreePath, localPath := "", ""
			if tt.worktreeFile != "" || tt.localFile != "" {
				anchor := t.TempDir()
				worktrees = []configengine.HubWorktree{{Name: "beam-a", Anchor: anchor}}
				worktreePath = configengine.ConfigFile(anchor, "beam")
				localPath = configengine.LocalFile(anchor, "beam")
				writeBeamLayer(t, anchor, tt.worktreeFile)
				if tt.localFile != "" {
					writeBeamLocalLayer(t, anchor, tt.localFile)
				}
			}

			got, rewrote, flagged, err := ReinterpretPullRequestModes([]byte(tt.existing), worktrees)
			if err != nil {
				t.Fatalf("ReinterpretPullRequestModes() error = %v; want nil", err)
			}
			if !slices.Equal(rewrote, tt.wantRewrote) {
				t.Errorf("ReinterpretPullRequestModes() rewrote = %q; want %q", rewrote, tt.wantRewrote)
			}
			if len(tt.wantRewrote) == 0 && string(got) != tt.existing {
				t.Errorf("ReinterpretPullRequestModes() = %q; want the input back byte for byte %q", got, tt.existing)
			}
			wantFlagged := make([]string, len(tt.wantFlagged))
			for i, line := range tt.wantFlagged {
				wantFlagged[i] = strings.NewReplacer("{worktree}", worktreePath, "{local}", localPath).Replace(line)
			}
			if !slices.Equal(flagged, wantFlagged) {
				t.Errorf("ReinterpretPullRequestModes() flagged = %q; want %q", flagged, wantFlagged)
			}

			var decoded map[string]any
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatalf("output %q does not parse: %v", got, err)
			}
			if len(decoded) != len(tt.wantValues) {
				t.Errorf("output decodes to %v; want %v", decoded, tt.wantValues)
			}
			for key, want := range tt.wantValues {
				if decoded[key] != want {
					t.Errorf("output %s = %v; want %v\n%s", key, decoded[key], want, got)
				}
			}
			for _, kept := range tt.wantKept {
				if !strings.Contains(string(got), kept) {
					t.Errorf("output = %q; want it to keep %q", got, kept)
				}
			}
		})
	}
}

func TestReinterpretProfilePullRequestModes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		existing string
		want     string
		// wantRewrote holds the opening of each rewrote line, in order.
		wantRewrote []string
	}{
		{
			name:        "a renamed value of a block is replaced and comments stay",
			existing:    "# profiles\nfast:\n  tapestry:\n    beam:\n      pull_request: merge  # lands at once\n      land_notice: \"on\"\n",
			want:        "# profiles\nfast:\n  tapestry:\n    beam:\n      pull_request: \"trail\" # lands at once\n      land_notice: \"on\"\n",
			wantRewrote: []string{"fast.tapestry beam.pull_request: merge is retired; it now reads trail"},
		},
		{
			name:     "a file with only current values comes back unchanged",
			existing: "fast:\n  darn:\n    beam: {pull_request: review}\n",
			want:     "fast:\n  darn:\n    beam: {pull_request: review}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, rewrote, flagged, err := ReinterpretProfilePullRequestModes([]byte(tt.existing), nil)
			if err != nil || len(flagged) != 0 {
				t.Fatalf("ReinterpretProfilePullRequestModes() flagged = %q, %v; want neither", flagged, err)
			}
			if string(got) != tt.want {
				t.Errorf("ReinterpretProfilePullRequestModes() = %q; want %q", got, tt.want)
			}
			if !slices.EqualFunc(rewrote, tt.wantRewrote, strings.HasPrefix) {
				t.Errorf("rewrote = %q; want lines opening %q", rewrote, tt.wantRewrote)
			}
		})
	}
}

// carryHub lays out a hub under a temp dir: a prime and one worktree for each name, each anchored one level down.
// It writes boardJSON as the board's board.json, and returns the board dir and the hub's worktrees, the prime first.
func carryHub(t *testing.T, names []string, boardJSON string) (boardDir string, worktrees []configengine.HubWorktree) {
	t.Helper()
	hub := t.TempDir()
	boardDir = filepath.Join(hub, "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", boardDir, err)
	}
	for _, name := range append([]string{"prime"}, names...) {
		anchor := filepath.Join(hub, name, "code")
		if err := os.MkdirAll(filepath.Join(anchor, "_lym"), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", anchor, err)
		}
		worktrees = append(worktrees, configengine.HubWorktree{Name: name, Anchor: anchor, Main: name == "prime"})
	}
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(boardJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(board.json) = %v; want nil", err)
	}
	return boardDir, worktrees
}

// anchorOf returns the anchor of the worktree called name among worktrees.
func anchorOf(t *testing.T, worktrees []configengine.HubWorktree, name string) string {
	t.Helper()
	for _, worktree := range worktrees {
		if worktree.Name == name {
			return worktree.Anchor
		}
	}
	t.Fatalf("no worktree called %q among %v", name, worktrees)
	return ""
}

// TestCarryBeamTaskSettings pins what the carry writes into the local layer of each worktree named after an entry.
// It never writes over a key that worktree already sets.
// A board save made before the carry drops a task's retired keys.
// The folded run settings and a project's kept keys carry the same.
func TestCarryBeamTaskSettings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const board = `{"version":1,"entries":[` +
		`{"id":0,"slug":"task-merge-parent","title":"M","kind":"task","pull_request":"merge-in","notify_on_land":true},` +
		`{"id":1,"slug":"task-already-set","title":"S","kind":"task","pull_request":"merge"},` +
		`{"id":2,"slug":"task-off","title":"O","kind":"task","pull_request":"off"},` +
		`{"id":3,"slug":"proj","title":"P","kind":"project","notify_on_land":true},` +
		`{"id":4,"slug":"gone","title":"G","kind":"task","pull_request":"merge"},` +
		`{"id":5,"slug":"task-seeded","title":"D","kind":"task","run_settings":{"beam":{"land_notice":"on"}}},` +
		`{"id":6,"slug":"task-run-off","title":"R","kind":"task","run_settings":{"beam":{"pull_request":"off"}}},` +
		`{"id":7,"slug":"task-run-merge","title":"T","kind":"task","run_settings":{"beam":{"pull_request":"merge"}}},` +
		`{"id":8,"slug":"task-legacy-on","title":"L","kind":"task","pull_request":"on"}]}`
	tests := []struct {
		name string
		// save loads and saves the board before the carry, as an unrelated board write does.
		save bool
		// wantOffFlag is whether the retired off of task-off is flagged, which only the unsaved file still holds.
		wantOffFlag bool
	}{
		{name: "the retired keys on disk carry and a retired off is flagged", wantOffFlag: true},
		{name: "the folded run settings and a project's kept keys carry after an unrelated board save", save: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			boardDir, worktrees := carryHub(t, []string{"task-merge-parent", "task-already-set", "task-off", "proj", "task-seeded", "task-run-off", "task-run-merge", "task-legacy-on"}, board)
			writeBeamLocalLayer(t, anchorOf(t, worktrees, "task-already-set"), "pull_request: \"review\"\n")
			if tt.save {
				store := boardengine.NewStore(boardDir)
				if err := store.Load(); err != nil {
					t.Fatalf("Load() error = %v; want nil", err)
				}
				if err := store.Save(); err != nil {
					t.Fatalf("Save() error = %v; want nil", err)
				}
			}

			carried, flagged, err := CarryBeamTaskSettings(boardDir, worktrees)
			if err != nil {
				t.Fatalf("CarryBeamTaskSettings() error = %v; want nil", err)
			}

			wantCarried := [][]string{
				{"task-merge-parent", "pull_request: merge-in"},
				{"task-merge-parent", "land_notice: on"},
				{"proj", "land_notice: on"},
				{"task-seeded", "land_notice: on"},
				{"task-run-off", "pull_request: off"},
				{"task-run-merge", "pull_request: trail"},
				{"task-legacy-on", "pull_request: review"},
			}
			if len(carried) != len(wantCarried) {
				t.Errorf("carried = %q, flagged = %q; want %d carried lines", carried, flagged, len(wantCarried))
			}
			for _, wants := range wantCarried {
				if !containsLineWith(carried, wants...) {
					t.Errorf("carried = %q; want a line containing every one of %q", carried, wants)
				}
			}
			wantOff := []string{"task-off", "beam.pull_request off", "lym config beam --set pull_request=off"}
			if got := containsLineWith(flagged, wantOff...); got != tt.wantOffFlag {
				t.Errorf("flagged = %q; want a line containing every one of %q: %v", flagged, wantOff, tt.wantOffFlag)
			}

			wantValues := []struct {
				name, worktree, key, want string
				set                       bool
			}{
				{"merge-in carried to the merge-parent task", "task-merge-parent", PullRequestKey, "merge-in", true},
				{"the notice carried to the same task", "task-merge-parent", LandNoticeKey, "on", true},
				{"the notice carried to the project's worktree", "proj", LandNoticeKey, "on", true},
				{"the notice of the seeded task's run settings carried", "task-seeded", LandNoticeKey, "on", true},
				{"an off in the run settings is a current value and carries", "task-run-off", PullRequestKey, "off", true},
				{"a renamed merge in the run settings carries as trail", "task-run-merge", PullRequestKey, "trail", true},
				{"a renamed on in the retired entry key carries as review", "task-legacy-on", PullRequestKey, "review", true},
				{"a key the worktree already sets is kept", "task-already-set", PullRequestKey, "review", true},
				{"an off carries nothing", "task-off", PullRequestKey, "", false},
			}
			for _, want := range wantValues {
				value, _, set, err := ownTaskSetting(configengine.Dirs{Anchor: anchorOf(t, worktrees, want.worktree)}, want.key)
				if err != nil || set != want.set || (set && value != want.want) {
					t.Errorf("%s: own %s = %v, set %v, %v; want %q, set %v", want.name, want.key, value, set, err, want.want, want.set)
				}
			}
		})
	}
}

// TestCarryBeamTaskSettings_UnreadableBoard pins that a board that cannot be read is an error, so the caller tries again.
func TestCarryBeamTaskSettings_UnreadableBoard(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boardDir, worktrees := carryHub(t, []string{"task-a"}, `{not json`)

	carried, _, err := CarryBeamTaskSettings(boardDir, worktrees)

	if err == nil {
		t.Errorf("CarryBeamTaskSettings() error = nil, carried = %q; want an error naming the board", carried)
	}
}

// containsLineWith reports whether one line holds every one of wants.
func containsLineWith(lines []string, wants ...string) bool {
	for _, line := range lines {
		matches := true
		for _, want := range wants {
			matches = matches && strings.Contains(line, want)
		}
		if matches {
			return true
		}
	}
	return false
}
