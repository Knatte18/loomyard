// render_integration_test.go proves a render writes the file targets at a hub prime's anchor, with an anchor and without one, and leaves no untracked file.

package heddleengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRender_WritesAtTheAnchorAndLeavesNoUntrackedFile renders both file targets into a hub's prime from the event fabric builds.
// It expects both files at the prime's anchor, none at its root when the two differ, and git to see nothing untracked.
func TestRender_WritesAtTheAnchorAndLeavesNoUntrackedFile(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	tests := []struct {
		name   string
		anchor string
	}{
		{"NoAnchor", "."},
		{"Anchored", "backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: tt.anchor})
			store := heddleengine.Store{Entries: []heddleengine.Entry{
				{Slug: "claude", Text: "claude text", Order: 1, Targets: []string{"claude-file"}},
				{Slug: "codex", Text: "codex text", Order: 2, Targets: []string{"codex-file"}},
			}}
			if err := heddleengine.Save(hub.BoardDir(), store); err != nil {
				t.Fatal(err)
			}

			event, err := fabricengine.EventFor(hub.Location, nil)
			if err != nil {
				t.Fatalf("EventFor() = %v", err)
			}
			report, err := heddleengine.Renderer{}.Render(heddleengine.RenderRequest{
				Anchor: event.Anchor, ScratchDir: event.ScratchDir, BoltDir: event.BoltDir, Excluded: event.Excluded,
			})
			if err != nil {
				t.Fatalf("Render() = %v", err)
			}
			if want := []string{"CLAUDE.local.md", "AGENTS.override.md"}; !slices.Equal(report.Written, want) || len(report.Warnings) != 0 {
				t.Errorf("Render() report = %+v; want %v written and no warnings", report, want)
			}
			for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
				if _, err := os.Stat(filepath.Join(hub.Location.AnchorPath(), name)); err != nil {
					t.Errorf("%s was not rendered at the anchor: %v", name, err)
				}
				if tt.anchor != "." {
					if _, err := os.Stat(filepath.Join(hub.PrimeWorktree(), name)); err == nil {
						t.Errorf("%s was rendered at the worktree's root", name)
					}
				}
			}
			if status := gitkit.Git(t, hub.PrimeWorktree(), "status", "--porcelain", "--untracked-files=all"); status != "" {
				t.Errorf("git status in the prime = %q; want it empty", status)
			}
		})
	}
}
