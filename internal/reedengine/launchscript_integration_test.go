//go:build integration && !windows

// launchscript_integration_test.go proves against a real tmux that sourcing a strand's launch script runs the same statements in the pane shell's own scope that typing them did.
// The unit tests around panebin.go and spawn.go pin strings and files;
// only a live pane shell can show that the command ran and that the prelude's exports outlive it.
// The test reads the command's side effects, never pane text, so nothing matches pane output against the launch line's content.

package reedengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// readTrimmed returns path's content, or "" when it cannot be read yet.
func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestLaunchScript_SourcedScriptRunsInThePaneShellScope adds a strand whose command writes a marker file, then checks the script's content, the command's effect, and that LYX_BIN and PATH still carry the prelude's values in the pane shell after the command returned.
func TestLaunchScript_SourcedScriptRunsInThePaneShellScope(t *testing.T) {
	e := newColdScratchEngine(t)

	work := t.TempDir()
	fakeExe := filepath.Join(work, "fakebin", "lyx")
	withInjectedExecutablePath(t, func() (string, error) { return fakeExe, nil })

	sh := shell.ForGOOS()
	q := shell.Posix()
	marker := filepath.Join(work, "marker.txt")
	cmd := "printf '%s\\n' launched > " + q.Quote(marker)

	strand, err := e.AddStrand(AddSpec{
		Cmd:          cmd,
		NameOverride: "launch-script-strand",
		Display:      render.Display{Anchor: render.AnchorBelowParent},
	})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}

	waitUntil(t, 10*time.Second, "strand command never wrote its marker", func() bool {
		return strings.TrimSpace(readTrimmed(marker)) == "launched"
	})

	scriptPath := launchScriptPath(sh, e.stateDir(), strand.GUID)
	got, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read launch script %q: %v", scriptPath, err)
	}
	if want := composePaneLaunchLine(sh, cmd, strand.GUID) + "\n"; string(got) != want {
		t.Errorf("launch script = %q, want %q", got, want)
	}

	probe := filepath.Join(work, "probe.txt")
	probeCmd := "printf '%s\\n%s\\n' \"$LYX_BIN\" \"${PATH%%:*}\" > " + q.Quote(probe)
	if err := e.tmux.run("send-keys", "-t", strand.PaneID, "-l", sendKeysLiteralArg(probeCmd)); err != nil {
		t.Fatalf("send probe: %v", err)
	}
	if err := e.tmux.run("send-keys", "-t", strand.PaneID, "Enter"); err != nil {
		t.Fatalf("send Enter: %v", err)
	}
	waitUntil(t, 10*time.Second, "probe never wrote its output", func() bool {
		return strings.Count(readTrimmed(probe), "\n") >= 2
	})
	lines := strings.Split(strings.TrimRight(readTrimmed(probe), "\n"), "\n")
	if lines[0] != fakeExe {
		t.Errorf("LYX_BIN in pane = %q, want %q", lines[0], fakeExe)
	}
	if lines[1] != filepath.Dir(fakeExe) {
		t.Errorf("first PATH entry in pane = %q, want %q", lines[1], filepath.Dir(fakeExe))
	}

	if _, err := e.Down(); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.stateDir(), reedStateFileName)); !os.IsNotExist(err) {
		t.Errorf("reed.json after Down: stat err = %v, want not-exist", err)
	}
	if _, err := os.Stat(launchScriptDir(e.stateDir())); !os.IsNotExist(err) {
		t.Errorf("launch dir after Down: stat err = %v, want not-exist", err)
	}
}
