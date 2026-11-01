//go:build tier0

// seam_test.go drives the worktree seam over temporary directories, without git.

package heddleengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// seamFixture is a seam over a store holding a claude-file and a codex-file entry.
type seamFixture struct {
	renderFixture
	event fabricengine.WorktreeEvent
}

func newSeamFixture(t *testing.T) seamFixture {
	t.Helper()
	f := newRenderFixture(t, Store{Entries: []Entry{
		entryFor("claude", "claude text", 1, "claude-file"),
		entryFor("codex", "codex text", 2, "codex-file"),
	}})
	return seamFixture{
		renderFixture: f,
		event:         fabricengine.WorktreeEvent{Root: f.root, ScratchDir: f.scratch, BoltDir: f.bolt, Excluded: f.excluded},
	}
}

// tookInAfterClearing fires a took-in event over paths after removing both renders, so a file present afterwards was rendered by the event.
func (f seamFixture) tookInAfterClearing(t *testing.T, paths ...string) {
	t.Helper()
	for _, name := range []string{"CLAUDE.local.md", "AGENTS.override.md"} {
		if err := os.Remove(filepath.Join(f.root, name)); err != nil && !os.IsNotExist(err) {
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
	t.Parallel()

	t.Run("MadeWritesBothFileTargets", func(t *testing.T) {
		t.Parallel()
		f := newSeamFixture(t)
		if err := (Seam{}).Made(f.event); err != nil {
			t.Fatalf("Made() = %v", err)
		}
		if !f.rootHas("CLAUDE.local.md") || !f.rootHas("AGENTS.override.md") {
			t.Error("Made did not write both file targets")
		}
	})

	t.Run("TookInRendersTheTargetsBuiltFromAChangedPath", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name         string
			paths        []string
			wantClaude   bool
			wantOverride bool
		}{
			{"AgentsMd", []string{"AGENTS.md"}, true, true},
			{"UnrelatedPath", []string{"src/x.go"}, false, false},
			{"ClaudeDirClaudeMd", []string{".claude/CLAUDE.md"}, true, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				f := newSeamFixture(t)
				f.writeRoot(t, "AGENTS.md", "new rules\n")
				f.tookInAfterClearing(t, tt.paths...)
				if f.rootHas("CLAUDE.local.md") != tt.wantClaude || f.rootHas("AGENTS.override.md") != tt.wantOverride {
					t.Errorf("after TookIn(%v): CLAUDE.local.md present = %v, AGENTS.override.md present = %v; want %v and %v",
						tt.paths, f.rootHas("CLAUDE.local.md"), f.rootHas("AGENTS.override.md"), tt.wantClaude, tt.wantOverride)
				}
				if tt.wantOverride && !strings.Contains(f.readRoot(t, "AGENTS.override.md"), "new rules") {
					t.Errorf("AGENTS.override.md = %q; want it built from the changed AGENTS.md", f.readRoot(t, "AGENTS.override.md"))
				}
			})
		}
	})
}
