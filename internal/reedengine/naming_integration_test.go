//go:build integration && !windows

// naming_integration_test.go proves against a real tmux that a strand's full name reaches its state record, its pane title and its process environment.
// The pane title is a display mirror, so the test reads it back through list-panes and never resolves anything by it.

package reedengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// addEnvProbeStrand adds a role strand whose command writes $LYX_STRAND_NAME and $LYX_PARENT to a file and returns the strand with the file's path.
func addEnvProbeStrand(t *testing.T, e *Engine, role string) (Strand, string) {
	t.Helper()
	withInjectedExecutablePath(t, func() (string, error) { return filepath.Join(t.TempDir(), "lyx"), nil })

	probe := filepath.Join(t.TempDir(), "env.txt")
	cmd := "printf '%s\\n%s\\n' \"$LYX_STRAND_NAME\" \"$LYX_PARENT\" > " + shell.Posix().Quote(probe)
	strand, err := e.AddStrand(AddSpec{Role: role, Cmd: cmd, Display: render.Display{Anchor: render.AnchorBelowParent}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}
	return strand, probe
}

// paneTitle returns the title list-panes reports for paneID.
func paneTitle(t *testing.T, e *Engine, paneID string) string {
	t.Helper()
	live, err := e.tmux.listPanes(e.SessionName())
	if err != nil {
		t.Fatalf("listPanes: %v", err)
	}
	for _, p := range live {
		if p.ID == paneID {
			return p.Title
		}
	}
	t.Fatalf("pane %s not in list-panes %+v", paneID, live)
	return ""
}

func TestNaming_TaskWorktreeStrandNameTitleAndEnv(t *testing.T) {
	e := newColdScratchEngine(t)
	e.geom.ParentName = "tc:tslug:orch"

	strand, probe := addEnvProbeStrand(t, e, "worker")

	const want = "tc:tslug:worker"
	if strand.Name != want {
		t.Errorf("strand name = %q, want %q", strand.Name, want)
	}
	if got := paneTitle(t, e, strand.PaneID); got != want {
		t.Errorf("pane title = %q, want %q", got, want)
	}

	out, err := e.tmux.output("show-options", "-p", "-v", "-t", strand.PaneID, "allow-set-title")
	if err != nil {
		t.Fatalf("show-options: %v", err)
	}
	if got := strings.TrimSpace(out); got != "off" {
		t.Errorf("allow-set-title = %q, want off", got)
	}

	waitUntil(t, 10*time.Second, "strand command never wrote its env probe", func() bool {
		return strings.Count(readOrEmpty(probe), "\n") >= 2
	})
	lines := strings.Split(strings.TrimRight(readOrEmpty(probe), "\n"), "\n")
	if lines[0] != want {
		t.Errorf("LYX_STRAND_NAME in pane = %q, want %q", lines[0], want)
	}
	if lines[1] != "tc:tslug:orch" {
		t.Errorf("LYX_PARENT in pane = %q, want %q", lines[1], "tc:tslug:orch")
	}
}

func TestNaming_EmptySlugGivesShortnameAndRole(t *testing.T) {
	e := newColdScratchEngine(t)
	e.geom.NameSlug = ""

	strand, _ := addEnvProbeStrand(t, e, "worker")

	const want = "tc:worker"
	if strand.Name != want {
		t.Errorf("strand name = %q, want %q", strand.Name, want)
	}
	if got := paneTitle(t, e, strand.PaneID); got != want {
		t.Errorf("pane title = %q, want %q", got, want)
	}
}

func TestNaming_RepairNamesRestoresAHandChangedTitle(t *testing.T) {
	logs := logcapture.CaptureVerbose(t)
	e := newColdScratchEngine(t)
	strand, _ := addEnvProbeStrand(t, e, "worker")

	if err := e.tmux.run("select-pane", "-t", strand.PaneID, "-T", "hand-edited"); err != nil {
		t.Fatalf("select-pane: %v", err)
	}
	if got := paneTitle(t, e, strand.PaneID); got != "hand-edited" {
		t.Fatalf("pane title = %q after the hand edit, want hand-edited", got)
	}

	if err := e.repairNames(nil); err != nil {
		t.Fatalf("repairNames: %v", err)
	}
	if got := paneTitle(t, e, strand.PaneID); got != strand.Name {
		t.Errorf("pane title = %q after repair, want %q", got, strand.Name)
	}
	if !strings.Contains(logs.String(), "reed: repaired pane title") {
		t.Errorf("log = %q, want the title repair line", logs.String())
	}
}
