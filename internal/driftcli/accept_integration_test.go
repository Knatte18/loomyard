//go:build !windows

// accept_integration_test.go drives the built `lym drift accept` in a hubforge hub pair: the order of its check, capture and record, the skip list its capture is handed, and what it keeps.

package driftcli_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAccept_CapturesBeforeItRecords covers the order check, capture, record: a refusal spends no capture, a capture that does not pass records nothing, and a passing capture keeps its output and records the acceptance.
// Every capture is handed the scope's skip list.
func TestAccept_CapturesBeforeItRecords(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const (
		acceptedTest = "t1"
		skippedTest  = "slow"
		keptOutput   = "new\n"
	)
	bin := lymbin.Build(t)
	tests := []struct {
		name string
		row  string
		// holdLock holds the accept lock while the verb runs.
		holdLock bool
		// capture is the end of the capture command, after it records that it ran and copies its skip file.
		capture  string
		wantErr  []string
		wantRan  bool
		wantKept bool
	}{
		{name: "a passing capture keeps the output and records the acceptance", row: "Drift-Verify", capture: "mkdir -p {baseline_dir} && echo new > {baseline_dir}/t1.txt", wantRan: true, wantKept: true},
		{name: "a capture that exits non-zero refuses and records nothing", row: "Drift-Verify", capture: ": {baseline_dir}; exit 1", wantErr: []string{"the capture of t1's output did not pass", "exit 1", "way forward"}, wantRan: true},
		{name: "a refused check runs no capture", row: "Drift-Await", capture: "mkdir -p {baseline_dir} && echo new > {baseline_dir}/t1.txt", wantErr: []string{"Drift-Verify", "lym loom status"}},
		{name: "a held accept lock refuses before any capture", row: "Drift-Verify", holdLock: true, capture: "mkdir -p {baseline_dir} && echo new > {baseline_dir}/t1.txt", wantErr: []string{"another accept holds", "lym drift accept again"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
			pair := h.PairCodeWorktree(pairSlug)
			loc, err := lymcwd.Resolve(pair)
			if err != nil {
				t.Fatal(err)
			}
			paths := driftengine.NewPaths(loc.AnchorPath())

			work := t.TempDir()
			ran, skipCopy := filepath.Join(work, "ran"), filepath.Join(work, "skip")
			command := `touch "$RAN"; cp {skip_file} "$SKIPCOPY"; ` + tt.capture
			driftYAML := fmt.Sprintf("capture:\n  command: %q\n  timeout_min: 1\n  env:\n    RAN: %q\n    SKIPCOPY: %q\nverify_max_runs: 2\n", command, ran, skipCopy)
			driftFile := configengine.ConfigFile(h.BoardDir(), "drift")
			if err := os.MkdirAll(filepath.Dir(driftFile), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(driftFile, []byte(driftYAML), 0o644); err != nil {
				t.Fatal(err)
			}

			if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeTapestry, Driver: shedrun.DriverGo}); err != nil {
				t.Fatal(err)
			}
			lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
				t.Fatal(err)
			}
			status := shedengine.Status{CurrentProducer: tt.row, State: shedengine.StateBlocked}
			if err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, status); err != nil {
				t.Fatal(err)
			}

			if err := os.MkdirAll(paths.BaselineDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(paths.BaselineDir, "t1.txt"), []byte("baseline\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			scope := driftengine.ScopeRecord{Scope: driftengine.ScopePartial, Skip: []string{skippedTest}, RecordedAt: "2026-01-01T00:00:00Z"}
			if err := driftengine.UpdateScope(paths, func(driftengine.ScopeRecord, bool) (driftengine.ScopeRecord, error) { return scope, nil }); err != nil {
				t.Fatal(err)
			}

			if tt.holdLock {
				held, taken, err := lock.TryAcquireWriteLock(paths.AcceptLock)
				if err != nil || !taken {
					t.Fatalf("TryAcquireWriteLock() = %v, %v; want the accept lock taken", taken, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			env := lymRun(t, bin, pair, "accept", acceptedTest, "--reason", "intended")

			if tt.wantErr != nil {
				if env.OK {
					t.Fatalf("lym drift accept = ok; want a refusal naming %q", tt.wantErr)
				}
				for _, fragment := range tt.wantErr {
					if !strings.Contains(env.Error, fragment) {
						t.Errorf("lym drift accept refused with %q; want it to name %q", env.Error, fragment)
					}
				}
			} else {
				if !env.OK {
					t.Fatalf("lym drift accept: %s", env.Error)
				}
				body, _ := env.Raw["accept"].(map[string]any)
				if log, _ := body["log"].(string); body["test"] != acceptedTest || log == "" {
					t.Errorf("lym drift accept result = %v; want test %s and the capture's log", body, acceptedTest)
				}
			}

			if _, err := os.Stat(ran); (err == nil) != tt.wantRan {
				t.Errorf("capture ran = %v (stat err %v); want %v", err == nil, err, tt.wantRan)
			}
			if tt.wantRan {
				got, err := os.ReadFile(skipCopy)
				if err != nil || strings.TrimSpace(string(got)) != skippedTest {
					t.Errorf("skip file the capture read = %q, %v; want the scope's skip list %q", got, err, skippedTest)
				}
			}

			rec, found, err := driftengine.ReadScope(paths)
			if err != nil || !found {
				t.Fatalf("ReadScope() = %v, %v; want the seeded record", found, err)
			}
			accepted := len(rec.Accepted) == 1 && rec.Accepted[0].Test == acceptedTest
			if accepted != tt.wantKept {
				t.Errorf("scope record accepted = %+v; want the acceptance recorded = %v", rec.Accepted, tt.wantKept)
			}
			kept, err := os.ReadFile(filepath.Join(paths.AcceptedTestsDir, "t1.txt"))
			if tt.wantKept {
				if err != nil || string(kept) != keptOutput {
					t.Errorf("kept output = %q, %v; want %q", kept, err, keptOutput)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("kept output = %q, %v; want none after a refusal", kept, err)
			}
		})
	}
}
