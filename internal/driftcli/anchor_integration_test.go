//go:build !windows

// anchor_integration_test.go drives the built `lym drift` verbs in a hub whose anchor is a subdirectory, and checks that the detached capture child and every drift command run in the anchored directory, not the worktree root.

package driftcli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDrift_RunsInTheAnchoredDirectory covers the main capture's child, the compare and the side capture of a hub anchored at a subdirectory:
// each declared command records its working directory, and every one must be the anchored directory.
// A capture child started anywhere but the anchored directory cannot resolve its worktree, never records its start, and marks its error as lasting.
func TestDrift_RunsInTheAnchoredDirectory(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{pairSlug}})
	pair := h.PairCodeWorktree(pairSlug)
	anchor := filepath.Join(pair, "backend")
	loc, err := lymcwd.Resolve(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if loc.AnchorPath() != anchor || loc.AnchorPath() == loc.WorktreePath() {
		t.Fatalf("anchored path = %q, worktree = %q; want an anchor below the worktree root", loc.AnchorPath(), loc.WorktreePath())
	}

	cwds := filepath.Join(t.TempDir(), "cwds")
	envs := filepath.Join(filepath.Dir(cwds), "envs")
	driftYAML := fmt.Sprintf(`capture:
  command: |
    pwd >> %q
    echo "$DRIFT_MODULE $DRIFT_OWN" >> %q
    mkdir -p {baseline_dir} && echo ok > {baseline_dir}/profile.txt
  env:
    DRIFT_OWN: from-step
  timeout_min: 1
compare:
  command: 'pwd >> %q; echo "$DRIFT_MODULE $DRIFT_OWN" >> %q; : {baseline_dir}'
  env:
    DRIFT_OWN: from-step
  timeout_min: 1
verify_max_runs: 3
`, cwds, envs, cwds, envs)
	driftFile := configengine.ConfigFile(h.BoardDir(), "drift")
	if err := os.MkdirAll(filepath.Dir(driftFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driftFile, []byte(driftYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "env"), []byte("vars:\n  DRIFT_MODULE: from-env-module\n  DRIFT_OWN: from-env-module\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}
	writeStatus := func(row string, status shedengine.State) {
		t.Helper()
		lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, shedengine.Status{CurrentProducer: row, State: status}); err != nil {
			t.Fatal(err)
		}
	}

	writeStatus("Drift-Await", shedengine.StateBlocked)
	if env := lymRun(t, bin, pair, "capture-child"); env.OK || !strings.Contains(env.Error, "does not equal the anchored directory") || env.Raw[driftengine.LastingFaultKey] != true {
		t.Errorf("lym drift capture-child at the worktree root = %+v; want its cwd refused and marked %s", env, driftengine.LastingFaultKey)
	}
	if env := lymRun(t, bin, anchor, "capture"); !env.OK {
		t.Fatalf("lym drift capture: %s", env.Error)
	}
	waitFor(t, "the capture record to read passed", func() bool { return captureState(statusBody(t, bin, anchor)) == driftengine.CapturePassed })

	writeStatus("Drift-Verify", shedengine.StateRunning)
	for _, args := range [][]string{{"compare"}, {"capture", "--side"}} {
		if env := lymRun(t, bin, anchor, args...); !env.OK {
			t.Fatalf("lym drift %v: %s", args, env.Error)
		}
	}

	want, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cwds)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(got) != 3 {
		t.Fatalf("recorded working directories = %q; want one each from the main capture, the compare and the side capture", got)
	}
	for i, dir := range got {
		if dir != want {
			t.Errorf("command %d ran in %q; want the anchored directory %q", i, dir, want)
		}
	}

	envData, err := os.ReadFile(envs)
	if err != nil {
		t.Fatal(err)
	}
	gotEnvs := strings.Split(strings.TrimSpace(string(envData)), "\n")
	if len(gotEnvs) != 3 {
		t.Fatalf("recorded environments = %q; want one each from the main capture, the compare and the side capture", gotEnvs)
	}
	for i, line := range gotEnvs {
		if line != "from-env-module from-step" {
			t.Errorf("command %d ran with DRIFT_MODULE and DRIFT_OWN %q; want the env module's DRIFT_MODULE and the step's own DRIFT_OWN", i, line)
		}
	}
}
