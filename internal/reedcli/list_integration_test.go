//go:build integration && !windows

// list_integration_test.go drives `lyx reed list` on a real hub: strands in the prime and in a task worktree all appear in one listing from the prime, and retitling a pane by hand flips only that row's drift.

package reedcli

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// listRows returns `lyx reed list` run in worktree, keyed by strand name.
func listRows(t *testing.T, worktree string) map[string]map[string]any {
	t.Helper()
	env := runVerb(t, worktree, "list")
	rows := map[string]map[string]any{}
	strands, _ := env["strands"].([]any)
	for _, s := range strands {
		row, _ := s.(map[string]any)
		name, _ := row["name"].(string)
		rows[name] = row
	}
	return rows
}

func TestList_HubWideDirectoryAndDrift(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	skipWithoutMultiplexer(t, h)

	const slug = "list-task"
	hubforge.AddPair(t, h, slug)
	prime := h.PrimeWorktree()
	task := h.PairWarpWorktree(slug)
	for _, wt := range []string{prime, task} {
		wt := wt
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(wt, &buf, []string{"down"})
		})
	}

	runVerb(t, prime, "add", "--role", "worker", "--cmd", "sleep 300")
	runVerb(t, task, "add", "--role", "worker", "--cmd", "sleep 300")

	primeName := hubforge.TestCode + ":worker"
	taskName := hubforge.TestCode + ":" + slug + ":worker"

	rows := listRows(t, prime)
	for _, name := range []string{primeName, taskName} {
		row, ok := rows[name]
		if !ok {
			t.Fatalf("list from the prime has no row %q; got %v", name, rows)
		}
		if live, _ := row["live"].(bool); !live {
			t.Errorf("row %q live = false, want true", name)
		}
		if title, _ := row["title"].(string); title != name {
			t.Errorf("row %q title = %q, want its name", name, title)
		}
		if drift, _ := row["drift"].(bool); drift {
			t.Errorf("row %q drift = true before any retitle", name)
		}
		if paneID, _ := row["pane_id"].(string); paneID == "" {
			t.Errorf("row %q has no pane id", name)
		}
	}
	if wt, _ := rows[taskName]["worktree"].(string); wt == "" || wt == rows[primeName]["worktree"] {
		t.Errorf("task row worktree = %q, prime row worktree = %v; want each its own", wt, rows[primeName]["worktree"])
	}

	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	paneID, _ := rows[taskName]["pane_id"].(string)
	if err := exec.Command(cfg.Tmux, "-L", reedengine.ServerName(h.Path), "select-pane", "-t", paneID, "-T", "hand-set").Run(); err != nil {
		t.Fatalf("tmux select-pane: %v", err)
	}

	rows = listRows(t, prime)
	if drift, _ := rows[taskName]["drift"].(bool); !drift {
		t.Errorf("retitled row %q drift = false, want true", taskName)
	}
	if title, _ := rows[taskName]["title"].(string); title != "hand-set" {
		t.Errorf("retitled row title = %q, want %q", title, "hand-set")
	}
	if drift, _ := rows[primeName]["drift"].(bool); drift {
		t.Errorf("untouched row %q drift = true", primeName)
	}
}
