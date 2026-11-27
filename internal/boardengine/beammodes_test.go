// beammodes_test.go pins the beam modes a render reads through the installed reader.
// A mode line marks each value as the project's own or as inherited, and names where an inherited one comes from.

package boardengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRenderToDisk_ShowsTheInstalledBeamModes renders two open beams through Config.Outputs with a fixed-mode reader.
// It pins the mode line of the root and of each beam, in the README and in the project files.
// It pins how an inherited value names its source,
// and that a setting every place takes from the hub-wide default still shows on each mode line.
// It pins the mode line of an open beam the reader leaves out.
// It pins that the reader is asked for the open beams alone, and that a failed read renders no mode line.
// It installs the process-wide reader, so it does not run in parallel.
func TestRenderToDisk_ShowsTheInstalledBeamModes(t *testing.T) {
	tiergate.Require(t, 0)
	const pullRequestURL = "https://github.com/example/repo/pull/12"
	const unreadLine = "Autonomy and PR setting not read on this machine"
	done := boardengine.StatusDone
	tasks := []boardengine.Task{
		{ID: 0, Handle: "P1", Slug: "beam-a", Title: "Beam A", Kind: boardengine.KindProject, Beam: true},
		{ID: 1, Handle: "P2", Slug: "beam-b", Title: "Beam B", Kind: boardengine.KindProject, Beam: true},
		{ID: 2, Handle: "P3", Slug: "beam-done", Title: "Beam done", Kind: boardengine.KindProject, Beam: true, Status: &done},
		{ID: 3, Handle: "P4", Slug: "grouping", Title: "Grouping", Kind: boardengine.KindProject},
		{ID: 4, Handle: "P5", Slug: "beam-elsewhere", Title: "Beam elsewhere", Kind: boardengine.KindProject, Beam: true},
	}
	hubWide := boardengine.BeamMode{
		Autonomy:    boardengine.SettingValue{Value: "off"},
		PullRequest: boardengine.SettingValue{Value: "on"},
	}
	tests := []struct {
		name string
		// rootPR, beamAPR and beamBPR are each place's resolved pull_request.
		rootPR, beamAPR, beamBPR boardengine.SettingValue
		// wantLines are lines the README must hold, each whole.
		wantLines []string
		// wantProjectLines maps a project file to a line it must hold, whole.
		wantProjectLines map[string]string
		// readErr is the reader's error, nil for a read that answers.
		readErr error
		// wantAbsentPrefixes are openings no README line may have.
		wantAbsentPrefixes []string
	}{
		{
			name:    "own values are bold and inherited ones name their source",
			rootPR:  boardengine.SettingValue{Value: "on"},
			beamAPR: boardengine.SettingValue{Value: "merge", SetBy: "beam-a"},
			beamBPR: boardengine.SettingValue{Value: "on"},
			wantLines: []string{
				"Hub-wide default: autonomy `off` · PR setting `on`.",
				"Autonomy **on** · PR setting _on_ (hub-wide)",
				"Autonomy _on_ (P0) · PR setting **merge** · Parent branch `main` · Beam PR [#12](" + pullRequestURL + ")",
				"Autonomy **off** · PR setting _on_ (hub-wide) · Parent branch `beam-a`",
				unreadLine,
			},
			wantProjectLines: map[string]string{
				"project-root.md":           "Autonomy **on** · PR setting _on_ (hub-wide)",
				"project-beam-a.md":         "Autonomy _on_ (P0) · PR setting **merge** · Parent branch `main` · Beam PR [#12](" + pullRequestURL + ")",
				"project-beam-elsewhere.md": unreadLine,
			},
		},
		{
			name:    "a setting every place takes from the hub-wide default still shows on each mode line",
			rootPR:  boardengine.SettingValue{Value: "on"},
			beamAPR: boardengine.SettingValue{Value: "on"},
			beamBPR: boardengine.SettingValue{Value: "on"},
			wantLines: []string{
				"Hub-wide default: autonomy `off` · PR setting `on`.",
				"Autonomy **on** · PR setting _on_ (hub-wide)",
				"Autonomy _on_ (P0) · PR setting _on_ (hub-wide) · Parent branch `main` · Beam PR [#12](" + pullRequestURL + ")",
				"Autonomy **off** · PR setting _on_ (hub-wide) · Parent branch `beam-a`",
			},
		},
		{
			name:      "a value inherited from a beam names the beam's handle",
			rootPR:    boardengine.SettingValue{Value: "on"},
			beamAPR:   boardengine.SettingValue{Value: "merge", SetBy: "beam-a"},
			beamBPR:   boardengine.SettingValue{Value: "merge", SetBy: "beam-a"},
			wantLines: []string{"Autonomy **off** · PR setting _merge_ (P1) · Parent branch `beam-a`"},
		},
		{
			name:      "a value inherited from a worktree with no board entry names its slug",
			rootPR:    boardengine.SettingValue{Value: "on"},
			beamAPR:   boardengine.SettingValue{Value: "on"},
			beamBPR:   boardengine.SettingValue{Value: "merge", SetBy: "side-pair"},
			wantLines: []string{"Autonomy **off** · PR setting _merge_ (`side-pair`) · Parent branch `beam-a`"},
		},
		{
			name:               "a failed read renders no mode line",
			readErr:            errors.New("no prime"),
			wantAbsentPrefixes: []string{"Hub-wide default:", "Autonomy "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBeams []string
			boardengine.InstallBeamModeReader(func(boardDir string, beams []string) (boardengine.BeamModes, error) {
				gotBeams = beams
				return boardengine.BeamModes{
					HubWide: hubWide,
					Root: boardengine.BeamMode{
						Autonomy:    boardengine.SettingValue{Value: "on", SetBy: boardengine.RootSubject},
						PullRequest: tt.rootPR,
					},
					Beams: map[string]boardengine.BeamMode{
						"beam-a": {
							Autonomy:          boardengine.SettingValue{Value: "on", SetBy: boardengine.RootSubject},
							PullRequest:       tt.beamAPR,
							ParentBranch:      "main",
							PullRequestNumber: 12,
							PullRequestURL:    pullRequestURL,
						},
						"beam-b": {
							Autonomy:     boardengine.SettingValue{Value: "off", SetBy: "beam-b"},
							PullRequest:  tt.beamBPR,
							ParentBranch: "beam-a",
						},
					},
				}, tt.readErr
			})
			t.Cleanup(func() { boardengine.InstallBeamModeReader(nil) })

			dir := t.TempDir()
			out := boardengine.Config{Readme: "README.md", DesignPrefix: "proposal-"}.Outputs()
			if err := boardengine.RenderToDisk(dir, tasks, "", out, renderTime); err != nil {
				t.Fatalf("RenderToDisk() = %v; want nil", err)
			}
			if want := []string{"beam-a", "beam-b", "beam-elsewhere"}; !slices.Equal(gotBeams, want) {
				t.Errorf("reader beams = %v; want %v", gotBeams, want)
			}
			readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatalf("read README.md: %v", err)
			}
			lines := strings.Split(string(readme), "\n")
			for _, want := range tt.wantLines {
				if !slices.Contains(lines, want) {
					t.Errorf("README.md lacks the line %q; got:\n%s", want, readme)
				}
			}
			for _, prefix := range tt.wantAbsentPrefixes {
				if slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, prefix) }) {
					t.Errorf("README.md holds a line opening %q; want none; got:\n%s", prefix, readme)
				}
			}
			for file, want := range tt.wantProjectLines {
				content, err := os.ReadFile(filepath.Join(dir, file))
				if err != nil {
					t.Fatalf("read %s: %v", file, err)
				}
				if !slices.Contains(strings.Split(string(content), "\n"), want) {
					t.Errorf("%s lacks the line %q; got:\n%s", file, want, content)
				}
			}
		})
	}
}
