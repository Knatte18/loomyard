// seam_test.go drives the worktree seam over temporary directories, without git.
// It covers a worktree with an anchor and one without.

package heddleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seamFixture is a seam over a store holding a claude-file and a codex-file entry.
// The worktree's anchor lies anchorRel below its root.
type seamFixture struct {
	renderFixture
	root  string
	event fabricengine.WorktreeEvent
}

func newSeamFixture(t *testing.T, anchorRel string) seamFixture {
	t.Helper()
	f := newRenderFixture(t, Store{Entries: []Entry{
		entryFor("claude", "claude text", 1, "claude-file"),
		entryFor("codex", "codex text", 2, "codex-file"),
	}})
	root := t.TempDir()
	f.anchor = filepath.Join(root, filepath.FromSlash(anchorRel))
	if err := os.MkdirAll(f.anchor, 0o755); err != nil {
		t.Fatal(err)
	}
	return seamFixture{
		renderFixture: f,
		root:          root,
		event:         fabricengine.WorktreeEvent{Root: root, Anchor: f.anchor, ScratchDir: f.scratch, BoltDir: f.bolt, Excluded: f.excluded},
	}
}

// tookInAfterClearing fires a took-in event over paths after removing both renders.
// Any file present afterwards was rendered by the event.
func (f seamFixture) tookInAfterClearing(t *testing.T, paths ...string) {
	t.Helper()
	for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
		if err := os.Remove(filepath.Join(f.anchor, name)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	event := f.event
	event.Paths = paths
	if err := (Seam{}).TookIn(event); err != nil {
		t.Fatalf("TookIn(%v) = %v", paths, err)
	}
}

func TestSeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MadeWritesBothFileTargetsAtTheAnchor", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			anchorRel string
		}{
			{"NoAnchor", "."},
			{"Anchored", "src/app"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newSeamFixture(t, tt.anchorRel)
				if err := (Seam{}).Made(f.event); err != nil {
					t.Fatalf("Made() = %v", err)
				}
				if !f.anchorHas("CLAUDE.local.md") || !f.anchorHas("AGENTS.override.md") {
					t.Error("Made did not write both file targets at the anchor")
				}
				if tt.anchorRel != "." {
					if _, err := os.Lstat(filepath.Join(f.root, "CLAUDE.local.md")); err == nil {
						t.Error("Made wrote CLAUDE.local.md at the worktree's root")
					}
				}
			})
		}
	})

	t.Run("TookInRendersTheTargetsBuiltFromAChangedPath", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			anchorRel    string
			paths        []string
			wantClaude   bool
			wantOverride bool
		}{
			{"AgentsMd", ".", []string{"AGENTS.md"}, true, true},
			{"UnrelatedPath", ".", []string{"src/x.go"}, false, false},
			{"ClaudeDirClaudeMd", ".", []string{".claude/CLAUDE.md"}, true, true},
			{"AnchoredAgentsMd", "src/app", []string{"src/app/AGENTS.md"}, true, true},
			{"ClaudeMd", ".", []string{"CLAUDE.md"}, true, true},
			{"AnchoredClaudeMd", "src/app", []string{"src/app/CLAUDE.md"}, true, true},
			{"ClaudeMdAboveTheAnchor", "src/app", []string{"CLAUDE.md"}, false, false},
			{"AgentsMdAboveTheAnchor", "src/app", []string{"AGENTS.md"}, false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newSeamFixture(t, tt.anchorRel)
				f.writeAnchor(t, "AGENTS.md", "new rules\n")
				f.tookInAfterClearing(t, tt.paths...)
				if f.anchorHas("CLAUDE.local.md") != tt.wantClaude || f.anchorHas("AGENTS.override.md") != tt.wantOverride {
					t.Errorf("after TookIn(%v): CLAUDE.local.md present = %v, AGENTS.override.md present = %v; want %v and %v",
						tt.paths, f.anchorHas("CLAUDE.local.md"), f.anchorHas("AGENTS.override.md"), tt.wantClaude, tt.wantOverride)
				}
				if tt.wantOverride && !strings.Contains(f.readAnchor(t, "AGENTS.override.md"), "new rules") {
					t.Errorf("AGENTS.override.md = %q; want it built from the changed AGENTS.md", f.readAnchor(t, "AGENTS.override.md"))
				}
			})
		}
	})
}
