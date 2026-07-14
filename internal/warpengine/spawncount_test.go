//go:build integration

// spawncount_test.go guards the spawn-count win landed by hostLayoutFor: it proves
// that Status and Reconcile no longer spawn one `git rev-parse --show-toplevel` per
// paired host worktree. Pre-change, Status/Reconcile called hubgeometry.Resolve
// once per enumerated worktree with a present weft sibling, so `--show-toplevel`
// spawns scaled linearly with the paired-worktree count; this guard fails again if
// that regression returns.

package warpengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// buildNPairFixture builds a paired-Add fixture with a total of n host worktrees:
// the prime pair from setupStatusFixture (which counts as one) plus n-1 additional
// full host+weft pairs added via w.Add(..., AddOptions{SkipGit: true}), matching
// reconcile_test.go's setup pattern that suppresses all weft-side git/push during
// setup. Each added pair creates a real weft sibling, which is mandatory for the
// Status measurement below: Status only reaches hostLayoutFor after passing the
// os.Stat(weftPath) weft-exists gate (status.go), so a bare host worktree with no
// weft sibling would hit `continue` first and never reach the resolver, making the
// spawn-count assertion vacuous even with the per-iteration Resolve regression
// present. Reconcile calls the resolver before its weft check and would scale with
// N regardless, but the same paired fixture is used for both measurements.
func buildNPairFixture(t *testing.T, n int) (lyxtest.PairedFixture, *Worktree) {
	t.Helper()

	f := setupStatusFixture(t)
	w := New(Config{BranchPrefix: ""})

	for i := 1; i < n; i++ {
		slug := fmt.Sprintf("pair-%d", i)
		if _, err := w.Add(f.Layout, slug, AddOptions{SkipGit: true}); err != nil {
			t.Fatalf("Add(%q): %v", slug, err)
		}
	}

	return f, w
}

// countShowToplevelSpawns points GIT_TRACE2_EVENT at a fresh t.TempDir() trace
// directory immediately before calling fn, then restores the prior
// GIT_TRACE2_EVENT value immediately after. It returns the number of trace files
// whose content contains "--show-toplevel": git's trace2 event target writes one
// file per process, and `rev-parse --show-toplevel` is exactly what
// hubgeometry.Resolve spawns, so this count is the number of Resolve spawns fn
// triggered. The caller's test must be non-parallel, since this toggles a
// process-global environment variable that every git subprocess inherits.
func countShowToplevelSpawns(t *testing.T, fn func()) int {
	t.Helper()

	traceDir := t.TempDir()
	prior, hadPrior := os.LookupEnv("GIT_TRACE2_EVENT")
	if err := os.Setenv("GIT_TRACE2_EVENT", traceDir); err != nil {
		t.Fatalf("Setenv GIT_TRACE2_EVENT: %v", err)
	}
	defer func() {
		if hadPrior {
			os.Setenv("GIT_TRACE2_EVENT", prior)
		} else {
			os.Unsetenv("GIT_TRACE2_EVENT")
		}
	}()

	fn()

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("ReadDir trace dir: %v", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(traceDir, entry.Name()))
		if err != nil {
			t.Fatalf("read trace file %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), "--show-toplevel") {
			count++
		}
	}
	return count
}

// TestResolveSpawnsDoNotScale guards the spawn-count win: pre-change, Status and
// Reconcile called hubgeometry.Resolve once per enumerated worktree with a present
// weft sibling, so `--show-toplevel` spawns scaled with the paired-worktree count.
// Post-change, hostLayoutFor's SiblingLayout fast path makes both 0 regardless of
// N. This test is deliberately non-parallel (no t.Parallel() anywhere in it) so it
// has an exclusive execution window and no sibling test's git spawns pollute the
// GIT_TRACE2_EVENT trace during either measurement.
func TestResolveSpawnsDoNotScale(t *testing.T) {
	t.Run("Status", func(t *testing.T) {
		f2, w2 := buildNPairFixture(t, 2)
		n2 := countShowToplevelSpawns(t, func() {
			if _, err := w2.Status(f2.Layout); err != nil {
				t.Fatalf("Status() [N=2]: %v", err)
			}
		})

		f4, w4 := buildNPairFixture(t, 4)
		n4 := countShowToplevelSpawns(t, func() {
			if _, err := w4.Status(f4.Layout); err != nil {
				t.Fatalf("Status() [N=4]: %v", err)
			}
		})

		if n4 > n2 {
			t.Errorf("Status() --show-toplevel spawns: N=2 got %d, N=4 got %d; want N=4 <= N=2 (spawn count must not scale with worktree count)", n2, n4)
		}
	})

	t.Run("Reconcile", func(t *testing.T) {
		f2, w2 := buildNPairFixture(t, 2)
		n2 := countShowToplevelSpawns(t, func() {
			if _, err := w2.Reconcile(f2.Layout); err != nil {
				t.Fatalf("Reconcile() [N=2]: %v", err)
			}
		})

		f4, w4 := buildNPairFixture(t, 4)
		n4 := countShowToplevelSpawns(t, func() {
			if _, err := w4.Reconcile(f4.Layout); err != nil {
				t.Fatalf("Reconcile() [N=4]: %v", err)
			}
		})

		if n4 > n2 {
			t.Errorf("Reconcile() --show-toplevel spawns: N=2 got %d, N=4 got %d; want N=4 <= N=2 (spawn count must not scale with worktree count)", n2, n4)
		}
	})
}
