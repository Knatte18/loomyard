// render_integration_test.go proves a render leaves a hub's prime free of untracked files.

package heddleengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRender_LeavesNoUntrackedFile renders both file targets into a hub's prime from the event fabric builds, and expects git to see nothing untracked.
func TestRender_LeavesNoUntrackedFile(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
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
		Root: event.Root, ScratchDir: event.ScratchDir, BoltDir: event.BoltDir, Excluded: event.Excluded,
	})
	if err != nil {
		t.Fatalf("Render() = %v", err)
	}
	if len(report.Warnings) != 0 {
		t.Errorf("Render() warnings = %v; want none", report.Warnings)
	}
	for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
		if _, err := os.Stat(filepath.Join(hub.PrimeWorktree(), name)); err != nil {
			t.Errorf("%s was not rendered: %v", name, err)
		}
	}
	if status := gitkit.Git(t, hub.PrimeWorktree(), "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Errorf("git status in the prime = %q; want it empty", status)
	}
}
