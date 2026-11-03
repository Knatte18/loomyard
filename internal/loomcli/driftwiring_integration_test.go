// driftwiring_integration_test.go checks that the Drift seams loomcli builds run the capture and the compare where a step's dir resolves in a hub anchored at a subdirectory: the anchored directory for no dir, and the worktree root joined with a relative dir.

package loomcli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/loomcli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDriftDeps_RunWhereTheStepsDirResolves covers the capture and the compare seams over a hub anchored at a subdirectory:
// each declared command records its working directory, which must be the anchored directory both for a step with no dir and for a step whose dir `backend` is joined onto the worktree root.
// A relative dir joined onto the anchored directory instead names a directory that does not exist, so the run is refused.
func TestDriftDeps_RunWhereTheStepsDirResolves(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	rows := []struct {
		name string
		// dirLine is the dir key of both steps, with its indentation and newline; empty sets none.
		dirLine string
	}{
		{"NoDirRunsInTheAnchor", ""},
		{"ARelativeDirJoinsTheWorktreeRoot", "  dir: backend\n"},
	}
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{hubforge.TemplatePairSlug}})
			anchor := filepath.Join(h.PairCodeWorktree(hubforge.TemplatePairSlug), "backend")
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
%s  timeout_min: 1
compare:
  command: 'pwd >> %q; echo "$DRIFT_MODULE $DRIFT_OWN" >> %q; : {baseline_dir}'
  env:
    DRIFT_OWN: from-step
%s  timeout_min: 1
`, cwds, envs, tt.dirLine, cwds, envs, tt.dirLine)
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

			statusPath, statusLockPath := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
				t.Fatal(err)
			}
			baseline := shedengine.Status{CurrentProducer: "Drift-Baseline", State: shedengine.StateRunning}
			if err := state.WriteJSON(statusPath, statusLockPath, baseline); err != nil {
				t.Fatal(err)
			}

			deps := loomcli.DriftDepsAt(loc, statusPath, statusLockPath)
			record, err := deps.RunCapture(context.Background())
			if err != nil || record.State != driftengine.CapturePassed {
				t.Fatalf("RunCapture() = %+v, %v; want a passed capture", record, err)
			}
			result, err := deps.Compare(context.Background())
			if err != nil || !result.Passed {
				t.Fatalf("Compare() = %+v, %v; want a passed compare", result, err)
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
			if len(got) != 2 {
				t.Fatalf("recorded working directories = %q; want one each from the capture and the compare", got)
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
			if len(gotEnvs) != 2 {
				t.Fatalf("recorded environments = %q; want one each from the capture and the compare", gotEnvs)
			}
			for i, line := range gotEnvs {
				if line != "from-env-module from-step" {
					t.Errorf("command %d ran with DRIFT_MODULE and DRIFT_OWN %q; want the env module's DRIFT_MODULE and the step's own DRIFT_OWN", i, line)
				}
			}
		})
	}
}

// TestDriftDeps_AcceptedSeamsKeepAcceptedTestsInTheRun covers the CaptureAccepted and CompareAccepted seams against Compare:
// with skip s1 and accepted test t1, Compare leaves out both, the two accepted seams leave out s1 alone, and CompareAccepted renders {baseline_dir} as the accepted profile directory.
func TestDriftDeps_AcceptedSeamsKeepAcceptedTestsInTheRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{hubforge.TemplatePairSlug}})
	anchor := filepath.Join(h.PairCodeWorktree(hubforge.TemplatePairSlug), "backend")
	loc, err := lymcwd.Resolve(anchor)
	if err != nil {
		t.Fatal(err)
	}

	seen := t.TempDir()
	driftYAML := fmt.Sprintf(`capture:
  command: 'mkdir -p {baseline_dir} && cat {skip_file} > {baseline_dir}/skip.txt'
  timeout_min: 1
compare:
  command: |
    printf '%%s\n' {baseline_dir} > %q
    cat {skip_file} > %q
  timeout_min: 1
`, filepath.Join(seen, "dir.txt"), filepath.Join(seen, "skip.txt"))
	driftFile := configengine.ConfigFile(h.BoardDir(), "drift")
	if err := os.MkdirAll(filepath.Dir(driftFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driftFile, []byte(driftYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := driftengine.NewPaths(loc.AnchorPath())
	err = driftengine.UpdateScope(paths, func(driftengine.ScopeRecord, bool) (driftengine.ScopeRecord, error) {
		return driftengine.ScopeRecord{
			Scope:    driftengine.ScopePartial,
			Skip:     []string{"s1"},
			Accepted: []driftengine.Acceptance{{Test: "t1"}},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	readSeen := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(seen, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	deps := loomcli.DriftDepsAt(loc, "", "")

	if result, err := deps.Compare(context.Background()); err != nil || !result.Passed {
		t.Fatalf("Compare() = %+v, %v; want a passed compare", result, err)
	}
	if got, want := readSeen("skip.txt"), "s1\nt1\n"; got != want {
		t.Errorf("Compare() skip file = %q; want %q, the skip and the accepted test both left out", got, want)
	}
	if got, want := strings.TrimSpace(readSeen("dir.txt")), paths.BaselineDir; got != want {
		t.Errorf("Compare() rendered {baseline_dir} as %q; want the baseline directory %q", got, want)
	}

	if result, err := deps.CompareAccepted(context.Background()); err != nil || !result.Passed {
		t.Fatalf("CompareAccepted() = %+v, %v; want a passed compare", result, err)
	}
	if got, want := readSeen("skip.txt"), "s1\n"; got != want {
		t.Errorf("CompareAccepted() skip file = %q; want %q, the accepted test left in", got, want)
	}
	if got, want := strings.TrimSpace(readSeen("dir.txt")), paths.AcceptedDir; got != want {
		t.Errorf("CompareAccepted() rendered {baseline_dir} as %q; want the accepted profile directory %q", got, want)
	}

	captured, err := deps.CaptureAccepted(context.Background())
	if err != nil || !captured.Passed {
		t.Fatalf("CaptureAccepted() = %+v, %v; want a passed capture", captured, err)
	}
	data, err := os.ReadFile(filepath.Join(paths.AcceptedDir, "skip.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "s1\n"; got != want {
		t.Errorf("CaptureAccepted() skip file = %q; want %q, the accepted test left in", got, want)
	}
}
