//go:build !windows

// check_integration_test.go drives the built `lym drift check` in a hubforge hub pair.
// It covers the cells a changed capture leaves unexpected, the skip before Drift-Expect, and the missing-baseline fault.

package driftcli_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

// checkDriftYAML is a hub drift.yaml whose capture command writes one changed value into the directory it is told.
const checkDriftYAML = `capture:
  command: |
    mkdir -p {baseline_dir} && echo "value: new" > {baseline_dir}/profile.txt
  timeout_min: 1
`

// snapshotTree returns every regular file under dir by its relative path and content.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// entryCount returns how many entries dir holds, and zero while it does not exist.
func entryCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return len(entries)
}

// TestCheck_PrintsTheFindings covers the findings a check prints when a capture changes an unexpected value.
// The scenarios run in order over one hub, since each reseeds the run status and scope record.
func TestCheck_PrintsTheFindings(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	pair := h.PairCodeWorktree(pairSlug)
	loc, err := lymcwd.Resolve(pair)
	if err != nil {
		t.Fatal(err)
	}
	paths := driftengine.NewPaths(loc.AnchorPath())

	driftFile := configengine.ConfigFile(h.BoardDir(), "drift")
	if err := os.MkdirAll(filepath.Dir(driftFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driftFile, []byte(checkDriftYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: shedrun.RecipeDarn, Driver: shedrun.DriverGo}); err != nil {
		t.Fatal(err)
	}
	if err := driftengine.WriteCapture(paths, driftengine.CaptureRecord{State: driftengine.CapturePassed, Head: "baseline-head"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.BaselineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.BaselineDir, "profile.txt"), []byte("value: old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	seed := func(row string, runState shedengine.State, awaitedAt string) {
		t.Helper()
		lockPath := shedrun.StatusLock(loc, shedrun.SelfRunID)
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(shedrun.StatusFile(loc, shedrun.SelfRunID), lockPath, shedengine.Status{CurrentProducer: row, State: runState}); err != nil {
			t.Fatal(err)
		}
		err := driftengine.UpdateScope(paths, func(cur driftengine.ScopeRecord, _ bool) (driftengine.ScopeRecord, error) {
			cur.Scope, cur.Diff, cur.AwaitedAt = driftengine.ScopeFull, driftengine.DiffLines, awaitedAt
			return cur, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// check runs the verb, requires it to succeed and returns its body, failing the test when it changed a tracked file.
	check := func(t *testing.T) map[string]any {
		t.Helper()
		before := snapshotTree(t, filepath.Dir(paths.Scope))
		env := lymRun(t, bin, pair, "check")
		if !env.OK {
			t.Fatalf("lym drift check: %s", env.Error)
		}
		if after := snapshotTree(t, filepath.Dir(paths.Scope)); !reflect.DeepEqual(after, before) {
			t.Errorf("tracked drift files after the check = %v; want them unchanged from %v", after, before)
		}
		body, _ := env.Raw["check"].(map[string]any)
		return body
	}

	t.Run("a capture writing a changed value prints the cell with its case, old value and new value", func(t *testing.T) {
		seed("Drift-Verify", shedengine.StateRunning, time.Now().UTC().Format(time.RFC3339))
		captures := entryCount(t, paths.CurrentDir)

		body := check(t)

		if body["would_pass"] != false {
			t.Errorf("would_pass = %v; want false for a change nothing expects", body["would_pass"])
		}
		unexpected, _ := body["unexpected"].([]any)
		if len(unexpected) != 1 {
			t.Fatalf("unexpected = %v; want the one changed cell", body["unexpected"])
		}
		cell, _ := unexpected[0].(map[string]any)
		if cell["kind"] != "changed" || cell["case"] != "profile.txt" || cell["old"] != "value: old" || cell["new"] != "value: new" {
			t.Errorf("unexpected cell = %v; want a changed cell of profile.txt from %q to %q", cell, "value: old", "value: new")
		}
		findings, _ := body["findings"].(string)
		for _, want := range []string{`"profile.txt"`, `old "value: old"`, `new "value: new"`} {
			if !strings.Contains(findings, want) {
				t.Errorf("findings = %q; want it to name %s", findings, want)
			}
		}
		if entryCount(t, paths.CurrentDir) == captures {
			t.Errorf("current capture directory holds %d entries after the check; want a scratch capture added", captures)
		}
	})

	t.Run("a run before Drift-Expect prints that nothing is sealed yet and runs no capture", func(t *testing.T) {
		seed("Drift-Await", shedengine.StateBlocked, "")
		captures := entryCount(t, paths.CurrentDir)

		body := check(t)

		if skipped, _ := body["skipped"].(string); !strings.Contains(skipped, "no expectation is sealed yet") {
			t.Errorf("skipped = %q; want it to say no expectation is sealed yet", skipped)
		}
		if body["would_pass"] != true {
			t.Errorf("would_pass = %v; want true for a check that checked nothing", body["would_pass"])
		}
		if got := entryCount(t, paths.CurrentDir); got != captures {
			t.Errorf("current capture directory holds %d entries; want the %d it held before the check", got, captures)
		}
	})

	t.Run("a missing baseline directory is reported with lym drift defer and runs no capture", func(t *testing.T) {
		seed("Drift-Verify", shedengine.StateRunning, time.Now().UTC().Format(time.RFC3339))
		if err := os.RemoveAll(paths.BaselineDir); err != nil {
			t.Fatal(err)
		}
		captures := entryCount(t, paths.CurrentDir)

		body := check(t)

		faults, _ := body["faults"].(map[string]any)
		if baseline, _ := faults["baseline"].(string); !strings.Contains(baseline, "lym drift defer") {
			t.Errorf("faults = %v; want a baseline fault naming lym drift defer", faults)
		}
		if got := entryCount(t, paths.CurrentDir); got != captures {
			t.Errorf("current capture directory holds %d entries; want the %d it held before the check", got, captures)
		}
	})
}
