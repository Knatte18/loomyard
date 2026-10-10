//go:build tmux && !windows

// naming_integration_test.go drives the reed verbs on a real hub to prove a strand's full name and parent reach its process, survive a server kill and `resume`, address the right strand on `remove`, and that `lyx reed list` shows strands of the prime and of a task worktree in one listing from the prime and flags a hand-retitled pane as drift.

package reedcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// runVerb runs one reed verb in worktree and returns the decoded envelope; it fails the test on a non-zero exit.
func runVerb(t *testing.T, worktree string, args ...string) map[string]any {
	t.Helper()
	var out bytes.Buffer
	if code := RunCLIIn(worktree, &out, args); code != 0 {
		t.Fatalf("lyx reed %v = %d, output: %s", args, code, out.String())
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
		t.Fatalf("lyx reed %v output is not valid JSON: %v; got: %q", args, err, out.String())
	}
	return env
}

// statusNames returns every strand name `lyx reed status` lists in worktree.
func statusNames(t *testing.T, worktree string) []string {
	t.Helper()
	env := runVerb(t, worktree, "status")
	var names []string
	strands, _ := env["strands"].([]any)
	for _, s := range strands {
		strand, _ := s.(map[string]any)
		name, _ := strand["name"].(string)
		names = append(names, name)
	}
	return names
}

// envProbeCmd returns a --cmd that records "$LYX_STRAND_NAME|$LYX_PARENT" into file, then stays alive.
func envProbeCmd(file string) string {
	return "printf '%s|%s' \"$LYX_STRAND_NAME\" \"$LYX_PARENT\" > '" + file + "'; sleep 300"
}

// waitForProbe polls file until it holds content and returns it.
func waitForProbe(t *testing.T, file string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(file); err == nil && len(b) > 0 {
			return string(b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("strand process never wrote %s", file)
	return ""
}

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

// TestStrandNamingAndList runs the naming and listing claims against one hub with two task worktrees.
// The steps run serially in a fixed order: the prime strand step adds the prime's worker strand, the listing step reuses it and adds a task strand, and the task naming step kills the hub's tmux server and resumes only its own worktree.
// The scenario calls t.Parallel but no step does, because every step shares the one hub and its tmux server.
func TestStrandNamingAndList(t *testing.T) {
	t.Parallel()
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	skipWithoutMultiplexer(t, h)

	const listSlug = "list-task"
	const namingSlug = "naming-task"
	hubforge.AddPair(t, h, listSlug)
	hubforge.AddPair(t, h, namingSlug)
	prime := h.PrimeWorktree()
	listTask := h.PairCodeWorktree(listSlug)
	namingTask := h.PairCodeWorktree(namingSlug)
	for _, wt := range []string{prime, listTask, namingTask} {
		t.Cleanup(func() {
			var buf bytes.Buffer
			RunCLIIn(wt, &buf, []string{"down"})
		})
	}

	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	primeName := hubforge.TestShortname + ":worker"

	if !t.Run("PrimeStrandHasShortnameAndRoleAndNoParent", func(t *testing.T) {
		probe := filepath.Join(t.TempDir(), "prime.txt")
		env := runVerb(t, prime, "add", "--role", "worker", "--cmd", envProbeCmd(probe))

		if name, _ := env["name"].(string); name != primeName {
			t.Errorf("prime strand name = %q, want %q", name, primeName)
		}
		if got := waitForProbe(t, probe); got != primeName+"|" {
			t.Errorf("prime strand env = %q, want %q (no LYX_PARENT)", got, primeName+"|")
		}
	}) {
		return
	}

	// HubWideDirectoryAndDrift relies on the previous step's prime strand: the listing from the prime shows it and the task strand added here.
	if !t.Run("HubWideDirectoryAndDrift", func(t *testing.T) {
		runVerb(t, listTask, "add", "--role", "worker", "--cmd", "sleep 300")

		taskName := hubforge.TestShortname + ":" + listSlug + ":worker"

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
	}) {
		return
	}

	t.Run("TaskWorktreeRolesParentResumeAndRemove", func(t *testing.T) {
		worktree := namingTask
		// The pair is created from the prime, so its parent is the prime's orch strand.
		parent, err := agentname.Format("tst", "", agentname.RoleOrch)
		if err != nil {
			t.Fatalf("agentname.Format: %v", err)
		}

		work := t.TempDir()
		first, second := filepath.Join(work, "first.txt"), filepath.Join(work, "second.txt")
		runVerb(t, worktree, "add", "--role", "worker", "--cmd", envProbeCmd(first))
		runVerb(t, worktree, "add", "--role", "worker", "--cmd", envProbeCmd(second))

		fullFirst := "tst:" + namingSlug + ":worker"
		fullSecond := "tst:" + namingSlug + ":worker-2"
		names := statusNames(t, worktree)
		if !contains(names, fullFirst) || !contains(names, fullSecond) {
			t.Fatalf("status names = %v, want %q and %q", names, fullFirst, fullSecond)
		}

		if got, want := waitForProbe(t, first), fullFirst+"|"+parent; got != want {
			t.Errorf("first strand env = %q, want %q", got, want)
		}

		// Kill the server, then resume: the replayed process must see the same name and parent.
		if err := exec.Command(cfg.Tmux, "-L", reedengine.ServerName(h.Path), "kill-server").Run(); err != nil {
			t.Fatalf("tmux kill-server: %v", err)
		}
		if err := os.Remove(first); err != nil {
			t.Fatalf("remove probe: %v", err)
		}
		runVerb(t, worktree, "resume")
		if got, want := waitForProbe(t, first), fullFirst+"|"+parent; got != want {
			t.Errorf("resumed strand env = %q, want %q", got, want)
		}

		// A role segment and a full name each remove the right strand.
		runVerb(t, worktree, "remove", "--name", "worker-2")
		names = statusNames(t, worktree)
		if contains(names, fullSecond) || !contains(names, fullFirst) {
			t.Errorf("names after removing worker-2 = %v, want only %q", names, fullFirst)
		}
		runVerb(t, worktree, "remove", "--name", fullFirst)
		if names = statusNames(t, worktree); contains(names, fullFirst) {
			t.Errorf("names after removing the full name = %v, want %q gone", names, fullFirst)
		}
	})
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
