// launchscript_cleanup_test.go proves every path that forgets a strand also deletes its launch script, and that no path deletes a script whose strand survives.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// seedLaunchScripts writes a placeholder launch script for each guid and returns the paths in order.
func seedLaunchScripts(t *testing.T, e *Engine, guids ...string) []string {
	t.Helper()
	paths := make([]string, 0, len(guids))
	for _, guid := range guids {
		path := launchScriptPath(shell.ForGOOS(), e.stateDir(), guid)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte("true\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		paths = append(paths, path)
	}
	return paths
}

// assertScriptPresence fails unless the file at path exists iff want.
func assertScriptPresence(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if want && err != nil {
		t.Fatalf("launch script %s should exist: %v", path, err)
	}
	if !want && err == nil {
		t.Fatalf("launch script %s should be deleted", path)
	}
}

// newCleanupEngine returns a test engine whose tmux fake reports a live session and no pane output, with st saved as its state.
func newCleanupEngine(t *testing.T, st *ReedState) *Engine {
	t.Helper()
	e := newTestEngine(t)
	installFakeTmux(t, e).answer("display-message", "$0|4321|1787000000", nil)
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	return e
}

func hiddenStrand(guid, parent string) Strand {
	return Strand{GUID: guid, Name: guid, Parent: parent, Display: render.Display{Anchor: render.AnchorHidden}}
}

// TestLaunchScriptCleanup pins that every path forgetting a strand deletes exactly that strand's launch script (a recursive remove, the subtree's),
// leaves every surviving strand's script, and succeeds without a launch-script warning when the strand had no script.
//
//testtiming:keep pins every path forgetting a strand deleting exactly that strand's launch script, a recursive remove deleting the subtree's, a replace keeping the survivor's, and a remove without a script succeeding with no launch-script warning; its covering tests run this code without asserting it
func TestLaunchScriptCleanup(t *testing.T) {
	tests := []struct {
		name        string
		strands     []Strand
		seeded      []string
		forget      func(e *Engine) error
		wantDeleted []string
		wantKept    []string
	}{
		{
			name:    "RemoveDeletesLeafScriptKeepsSibling",
			strands: []Strand{hiddenStrand("a", ""), hiddenStrand("b", "")},
			seeded:  []string{"a", "b"},
			forget: func(e *Engine) error {
				_, err := e.RemoveStrand("a", false)
				return err
			},
			wantDeleted: []string{"a"},
			wantKept:    []string{"b"},
		},
		{
			name: "RecursiveRemoveDeletesSubtreeScripts",
			strands: []Strand{
				hiddenStrand("parent", ""), hiddenStrand("child", "parent"), hiddenStrand("grandchild", "child"), hiddenStrand("other", ""),
			},
			seeded: []string{"parent", "child", "grandchild", "other"},
			forget: func(e *Engine) error {
				_, err := e.RemoveStrand("parent", true)
				return err
			},
			wantDeleted: []string{"parent", "child", "grandchild"},
			wantKept:    []string{"other"},
		},
		{
			name:    "ReplaceDeletesReplacedScriptKeepsSurvivor",
			strands: []Strand{hiddenStrand("old", ""), hiddenStrand("keep", "")},
			seeded:  []string{"old", "keep"},
			forget: func(e *Engine) error {
				_, err := e.ReplaceStrand("old", AddSpec{Role: "worker", NameOverride: "new", Display: render.Display{Anchor: render.AnchorHidden}})
				return err
			},
			wantDeleted: []string{"old"},
			wantKept:    []string{"keep"},
		},
		{
			name:    "RemoveWithoutAScriptSucceedsSilently",
			strands: []Strand{hiddenStrand("a", "")},
			forget: func(e *Engine) error {
				_, err := e.RemoveStrand("a", false)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCleanupEngine(t, &ReedState{Strands: tt.strands})
			paths := map[string]string{}
			for i, path := range seedLaunchScripts(t, e, tt.seeded...) {
				paths[tt.seeded[i]] = path
			}
			buf := logcapture.CaptureVerbose(t)

			if err := tt.forget(e); err != nil {
				t.Fatalf("forgetting the strand: %v", err)
			}
			for _, guid := range tt.wantDeleted {
				assertScriptPresence(t, paths[guid], false)
			}
			for _, guid := range tt.wantKept {
				assertScriptPresence(t, paths[guid], true)
			}
			if strings.Contains(buf.String(), "launch script") {
				t.Fatalf("expected no launch-script warning, got %q", buf.String())
			}
		})
	}
}

func TestDown_RemovesStateAndLaunchDir(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{hiddenStrand("a", "")}})
	// A sibling session keeps Down off the server teardown path.
	installFakeTmux(t, e).answer("list-sessions", "sibling-session\n", nil)
	seedLaunchScripts(t, e, "a")

	if _, err := e.Down(); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.stateDir(), reedStateFileName)); err == nil {
		t.Fatalf("reed.json should be deleted")
	}
	if _, err := os.Stat(launchScriptDir(e.stateDir())); err == nil {
		t.Fatalf("launch directory should be deleted")
	}
	if _, err := e.Down(); err != nil {
		t.Fatalf("second Down: %v", err)
	}
}
