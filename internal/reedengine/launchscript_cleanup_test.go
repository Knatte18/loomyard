// launchscript_cleanup_test.go proves every path that forgets a strand also deletes its launch script, and that no path deletes a script whose strand survives.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
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
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "display-message":
			return "$0|4321|1787000000", nil
		default:
			return "", nil
		}
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	return e
}

func hiddenStrand(guid, parent string) Strand {
	return Strand{GUID: guid, Name: guid, Parent: parent, Display: render.Display{Anchor: render.AnchorHidden}}
}

func TestRemoveStrand_DeletesLeafScriptKeepsSibling(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{hiddenStrand("a", ""), hiddenStrand("b", "")}})
	paths := seedLaunchScripts(t, e, "a", "b")

	if _, err := e.RemoveStrand("a", false); err != nil {
		t.Fatalf("RemoveStrand: %v", err)
	}
	assertScriptPresence(t, paths[0], false)
	assertScriptPresence(t, paths[1], true)
}

func TestRemoveStrand_RecursiveDeletesSubtreeScripts(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{
		hiddenStrand("parent", ""), hiddenStrand("child", "parent"), hiddenStrand("grandchild", "child"), hiddenStrand("other", ""),
	}})
	paths := seedLaunchScripts(t, e, "parent", "child", "grandchild", "other")

	if _, err := e.RemoveStrand("parent", true); err != nil {
		t.Fatalf("RemoveStrand: %v", err)
	}
	for _, p := range paths[:3] {
		assertScriptPresence(t, p, false)
	}
	assertScriptPresence(t, paths[3], true)
}

func TestReplaceStrand_DeletesReplacedScriptKeepsSurvivor(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{hiddenStrand("old", ""), hiddenStrand("keep", "")}})
	paths := seedLaunchScripts(t, e, "old", "keep")

	if _, err := e.ReplaceStrand("old", AddSpec{Role: "worker", NameOverride: "new", Display: render.Display{Anchor: render.AnchorHidden}}); err != nil {
		t.Fatalf("ReplaceStrand: %v", err)
	}
	assertScriptPresence(t, paths[0], false)
	assertScriptPresence(t, paths[1], true)
}

func TestRemoveStrand_NoScriptSucceedsSilently(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{hiddenStrand("a", "")}})
	buf := captureLogOutput(t)

	if _, err := e.RemoveStrand("a", false); err != nil {
		t.Fatalf("RemoveStrand: %v", err)
	}
	if strings.Contains(buf.String(), "launch script") {
		t.Fatalf("expected no launch-script warning, got %q", buf.String())
	}
}

func TestDown_RemovesStateAndLaunchDir(t *testing.T) {
	e := newCleanupEngine(t, &ReedState{Strands: []Strand{hiddenStrand("a", "")}})
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		if args[0] == "list-sessions" {
			// A sibling session keeps Down off the server teardown path.
			return "sibling-session\n", nil
		}
		return "", nil
	}
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
