//go:build integration && !windows

// naming_integration_test.go carries the standalone acceptance criterion at the engine `lyx webster run --target-dir` and `lyx burler run --target-dir` build,
// since a CLI-level run would spawn live Claude producers.
// On a plain checkout with no recorded code it builds the reed engine from standalonestate.Derive and ReedGeometry, as the standalone wiring does.

package standalonegeom_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

func TestNaming_StandaloneStrandIsCodeAndRoleWithNoParent(t *testing.T) {
	target := t.TempDir()
	if out, err := exec.Command("git", "-C", target, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll stateDir: %v", err)
	}

	cfg, err := reedengine.LoadConfig(stateDir, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, err := exec.LookPath(cfg.Tmux); err != nil {
		t.Skipf("configured multiplexer binary %q not found: %v", cfg.Tmux, err)
	}

	e := reedengine.New(cfg, standalonegeom.ReedGeometry(target, stateDir, hash8))
	t.Cleanup(func() {
		_, _ = e.Down()
		_ = exec.Command(cfg.Tmux, "-L", "lyx-"+hash8, "kill-server").Run()
	})

	probe := filepath.Join(t.TempDir(), "env.txt")
	cmd := "printf '%s|%s' \"$LYX_STRAND_NAME\" \"$LYX_PARENT\" > '" + probe + "'; sleep 300"
	strand, err := e.AddStrand(reedengine.AddSpec{Role: "worker", Cmd: cmd, Display: render.Display{Anchor: render.AnchorBelowParent}})
	if err != nil {
		t.Fatalf("AddStrand: %v", err)
	}

	want := agentname.StandaloneCode(hash8) + ":worker"
	if strand.Name != want {
		t.Errorf("strand name = %q, want %q", strand.Name, want)
	}

	out, err := exec.Command(cfg.Tmux, "-L", "lyx-"+hash8, "display-message", "-p", "-t", strand.PaneID, "#{pane_title}").Output()
	if err != nil {
		t.Fatalf("display-message: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("pane title = %q, want %q", got, want)
	}

	deadline := time.Now().Add(15 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(probe); err == nil && len(b) > 0 {
			got = string(b)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got != want+"|" {
		t.Errorf("process env = %q, want %q (LYX_PARENT unset)", got, want+"|")
	}
}
