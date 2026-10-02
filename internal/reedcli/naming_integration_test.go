//go:build integration && !windows

// naming_integration_test.go drives the reed verbs on a real hub to prove a strand's full name and parent reach its process, survive a server kill and `resume`, and address the right strand on `remove`.

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

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
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

func TestNaming_TaskWorktreeRolesParentResumeAndRemove(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	skipWithoutMultiplexer(t, h)

	const slug = "naming-task"
	const parent = "tst:hub"
	hubforge.AddPair(t, h, slug)
	worktree := h.PairWarpWorktree(slug)
	l, err := lyxcwd.ResolveWorktree(worktree)
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	seed := shedrun.Seed{Recipe: shedrun.RecipeNames()[0], Driver: shedrun.DriverGo, Parent: parent}
	if err := shedrun.WriteSeed(l, shedrun.SelfRunID, seed); err != nil {
		t.Fatalf("WriteSeed: %v", err)
	}
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	work := t.TempDir()
	first, second := filepath.Join(work, "first.txt"), filepath.Join(work, "second.txt")
	runVerb(t, worktree, "add", "--role", "worker", "--cmd", envProbeCmd(first))
	runVerb(t, worktree, "add", "--role", "worker", "--cmd", envProbeCmd(second))

	fullFirst := "tst:" + slug + ":worker"
	fullSecond := "tst:" + slug + ":worker-2"
	names := statusNames(t, worktree)
	if !contains(names, fullFirst) || !contains(names, fullSecond) {
		t.Fatalf("status names = %v, want %q and %q", names, fullFirst, fullSecond)
	}

	if got, want := waitForProbe(t, first), fullFirst+"|"+parent; got != want {
		t.Errorf("first strand env = %q, want %q", got, want)
	}

	// Kill the server, then resume: the replayed process must see the same name and parent.
	cfg, err := reedengine.LoadConfig(h.Location.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
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
}

func TestNaming_PrimeStrandHasCodeAndRoleAndNoParent(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	skipWithoutMultiplexer(t, h)
	worktree := h.PrimeWorktree()
	t.Cleanup(func() {
		var buf bytes.Buffer
		RunCLIIn(worktree, &buf, []string{"down"})
	})

	probe := filepath.Join(t.TempDir(), "prime.txt")
	env := runVerb(t, worktree, "add", "--role", "worker", "--cmd", envProbeCmd(probe))

	want := hubforge.TestCode + ":worker"
	if name, _ := env["name"].(string); name != want {
		t.Errorf("prime strand name = %q, want %q", name, want)
	}
	if got := waitForProbe(t, probe); got != want+"|" {
		t.Errorf("prime strand env = %q, want %q (no LYX_PARENT)", got, want+"|")
	}
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
