// unlessname_integration_test.go drives `lym reed add --unless-name` on a real hub and tmux server.
// It proves that a matching strand, live or dormant, makes the add a no-op:
// neither the strand list nor the pane geometry is disturbed.
// With no matching strand, the add runs as normal.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// unlessRun runs one reed verb in worktree and returns its exit code and decoded envelope.
func unlessRun(t *testing.T, worktree string, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(worktree, &out, args)
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("lym reed %v output is not valid JSON: %v; got: %q", args, err, out.String())
	}
	return code, env
}

// unlessStatus runs `lym reed status` in worktree and returns the guid of every strand it lists.
// It also returns the envelope's session and the pane id of the strand named paneOf.
func unlessStatus(t *testing.T, worktree, paneOf string) (guids []string, session, pane string) {
	t.Helper()
	code, env := unlessRun(t, worktree, "status")
	if code != 0 {
		t.Fatalf("lym reed status = %d, envelope: %v", code, env)
	}
	session, _ = env["session"].(string)
	strands, _ := env["strands"].([]any)
	for _, s := range strands {
		strand, _ := s.(map[string]any)
		guid, _ := strand["guid"].(string)
		guids = append(guids, guid)
		if name, _ := strand["name"].(string); name == paneOf {
			pane, _ = strand["paneId"].(string)
		}
	}
	return guids, session, pane
}

// unlessPanes returns the multiplexer's pane listing for the whole session, read on the hub's socket.
// Each line carries the pane's id, height and active flag.
func unlessPanes(t *testing.T, tmux, socket, session string) string {
	t.Helper()
	out, err := exec.Command(tmux, "-L", socket, "list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{pane_height} #{pane_active}").Output()
	if err != nil {
		t.Fatalf("tmux list-panes: %v", err)
	}
	return string(out)
}

// TestUnlessName runs the --unless-name claims against one hub; its prime worktree accumulates strands across the steps.
// Without an orch, the add proceeds.
// A live orch makes the add a no-op, and so does a dormant orch whose pane is gone.
// The steps run serially in a fixed order, and each later step relies on the earlier step's session.
// The scenario calls t.Parallel, but no step does.
// Every step shares the one hub, session and strand table, so none of them runs in parallel.
func TestUnlessName(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	skipWithoutMultiplexer(t, h)
	worktree := h.PrimeWorktree()
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})
	cfg, err := reedengine.LoadConfig(configengine.Dirs{Anchor: h.Location.AnchorPath()}, "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	socket := reedengine.ServerName(h.Path)

	if !t.Run("NoOrchAddsAsToday", func(t *testing.T) {
		code, env := unlessRun(t, worktree, "add", "--unless-name", "orch", "--if-absent", "--name", "claude", "--cmd", coldAddLaunchCmd())
		if code != 0 {
			t.Fatalf("add --unless-name orch with no orch = %d, envelope: %v", code, env)
		}
		if skipped, _ := env["skipped"].(bool); skipped {
			t.Fatalf("envelope = %v, want an ordinary add", env)
		}
		if name, _ := env["name"].(string); name != hubforge.TestShortname+":claude" {
			t.Errorf("name = %q, want the claude strand", name)
		}
	}) {
		return
	}

	// The orch strand stays behind for the dormant-orch step.
	// The claude strand of the first step keeps that step's session alive once the orch pane is gone.
	var orchName, orchGUID string
	if !t.Run("LiveOrchSkipsAndDisturbsNothing", func(t *testing.T) {
		code, orch := unlessRun(t, worktree, "add", "--name", "orch", "--cmd", coldAddLaunchCmd())
		if code != 0 {
			t.Fatalf("add orch = %d, envelope: %v", code, orch)
		}
		orchGUID, _ = orch["guid"].(string)
		orchName, _ = orch["name"].(string)

		guidsBefore, session, _ := unlessStatus(t, worktree, orchName)
		panesBefore := unlessPanes(t, cfg.Tmux, socket, session)

		code, env := unlessRun(t, worktree, "add", "--unless-name", "orch", "--if-absent", "--name", "claude-live", "--cmd", coldAddLaunchCmd())
		if code != 0 {
			t.Fatalf("add --unless-name orch = %d, envelope: %v", code, env)
		}
		if skipped, _ := env["skipped"].(bool); !skipped {
			t.Fatalf("envelope = %v, want skipped: true", env)
		}
		unless, _ := env["unless"].(map[string]any)
		if guid, _ := unless["guid"].(string); guid != orchGUID {
			t.Errorf("unless.guid = %q, want the orch strand's %q", guid, orchGUID)
		}

		guidsAfter, _, _ := unlessStatus(t, worktree, orchName)
		if strings.Join(guidsAfter, ",") != strings.Join(guidsBefore, ",") {
			t.Errorf("strand list after skip = %v, want unchanged %v", guidsAfter, guidsBefore)
		}
		if panesAfter := unlessPanes(t, cfg.Tmux, socket, session); panesAfter != panesBefore {
			t.Errorf("panes after skip = %q, want unchanged %q (height and active pane included)", panesAfter, panesBefore)
		}
	}) {
		return
	}

	t.Run("DormantOrchSkips", func(t *testing.T) {
		guidsBefore, _, orchPane := unlessStatus(t, worktree, orchName)
		if orchPane == "" {
			t.Fatalf("no pane id for %q in status", orchName)
		}
		if err := exec.Command(cfg.Tmux, "-L", socket, "kill-pane", "-t", orchPane).Run(); err != nil {
			t.Fatalf("tmux kill-pane: %v", err)
		}

		code, env := unlessRun(t, worktree, "add", "--unless-name", "orch", "--if-absent", "--name", "claude-dormant", "--cmd", coldAddLaunchCmd())
		if code != 0 {
			t.Fatalf("add --unless-name orch with a dormant orch = %d, envelope: %v", code, env)
		}
		if skipped, _ := env["skipped"].(bool); !skipped {
			t.Fatalf("envelope = %v, want skipped: true over a dormant orch", env)
		}
		unless, _ := env["unless"].(map[string]any)
		if guid, _ := unless["guid"].(string); guid != orchGUID {
			t.Errorf("unless.guid = %q, want the dormant orch strand's %q", guid, orchGUID)
		}
		guidsAfter, _, _ := unlessStatus(t, worktree, orchName)
		if strings.Join(guidsAfter, ",") != strings.Join(guidsBefore, ",") {
			t.Errorf("strand list after skip = %v, want unchanged %v", guidsAfter, guidsBefore)
		}
	})
}
