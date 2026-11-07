// preflight_integration_test.go tests preflightshed.NewPreflight's outcome mapping against a hubforge fixture hub.
// It is in-package because nothing in hubforge's dependency set reaches this package:
// go list -deps ./internal/hubforge lists internal/preflight but not internal/preflightshed.
//
// This file tests only the outcome mapping.
// The checks that preflight.Check runs are covered by internal/preflight's own integration suite.
// Duplicating them here would couple this package's tests to another package's check set.

package preflightshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// setupPreflightWrapperFixture builds a fully-configured real hub with fabric and junction setup.
// It seeds no loom status.json, since the tests in this file never read one.
func setupPreflightWrapperFixture(t *testing.T) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	slug := filepath.Base(h.Location.WorktreePath())

	// fabricengine.ConfigTemplate() is already reconciled by fabriccli.CloneAndWire when NewHub built h;
	// the repo-wide fabric.yaml override below is the change this fixture makes.
	hubforge.SeedFabricConfig(t, h, "branch_prefix: \"\"\npathspec: _extra\n")

	if err := fabricengine.WireJunctions(h.Location, slug, []string{"_lym", lymdirs.DotLymDirName, "_extra"}); err != nil {
		t.Fatalf("WireJunctions: %v", err)
	}

	// The wired junctions materialize through the _lym junction into the paired-sibling worktree's own git repo.
	// fabricengine.WireJunctions leaves them untracked, and the cleanliness check would otherwise report them.
	// The fixture commits them, so a freshly-built fixture is clean on both sides.
	// The commit is --allow-empty: the records prime arrives clean.
	// .lym is excluded through the records repo's .git/info/exclude.
	// The _extra junction target materializes as an empty directory that git does not track.
	// The pair is then a no-op, and it must be kept.
	// Deleting it would silently drop a guarantee:
	// the fixture stays clean if a future change reintroduces untracked records content.
	gitkit.MustRun(t, h.PrimeRecords(), "git", "add", "-A")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "commit", "--allow-empty", "-m", "seed junctions")

	return h
}

// TestPreflight_Scenario runs one fixture hub through named steps in a fixed order.
// Preconditions that all pass yield shedengine.Done.
// A call on an already-cancelled context returns an error with no verdict.
// An untracked file in the prime worktree fails the cleanliness check and yields shedengine.Stuck with its way forward.
// Clearing the dirt lets the same row proceed.
// The steps share one hub, so the test is parallel as a whole and no step is.
// The last step relies on being last: it dirties the worktree the earlier steps need clean.
//
// The cancelled step is Tier 2: preflight.Check reaches lymcwd.Resolve's git spawn unconditionally,
// so every path that calls Check is Tier 2 whatever it returns.
// The producer has no injectable seam between entryErr and the Check call.
// The step cannot run without a real fixture.
func TestPreflight_Scenario(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := setupPreflightWrapperFixture(t)
	p := NewPreflight("Preflight", h.PrimeWorktree())

	if !t.Run("all preconditions pass", func(t *testing.T) {
		shedfake.RequireOutcome(t, p, shedengine.Done)
	}) {
		return
	}

	if !t.Run("cancelled context returns an error, not a verdict", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, _, err := p.Call(ctx)
		if err == nil {
			t.Fatalf("Call(cancelled) error = nil; want non-nil error")
		}
		if outcome == shedengine.Done || outcome == shedengine.Stuck {
			t.Errorf("Call(cancelled) outcome = %q; want no verdict alongside a cancellation error", outcome)
		}
	}) {
		return
	}

	t.Run("broken precondition maps to Stuck and the way forward clears it", func(t *testing.T) {
		untracked := filepath.Join(h.PrimeWorktree(), "untracked.txt")
		if err := os.WriteFile(untracked, []byte("new"), 0o644); err != nil {
			t.Fatalf("write untracked file: %v", err)
		}

		ptr := shedfake.RequireOutcome(t, p, shedengine.Stuck)
		report, _, cerr := preflight.Check(h.PrimeWorktree())
		if cerr != nil {
			t.Fatalf("preflight.Check error = %v; want nil", cerr)
		}
		if want := "preconditions not met: " + formatFailures(report) + wayForward(report); ptr.Reason != want {
			t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
		}
		if !strings.Contains(ptr.Reason, "way forward: commit or stash the code changes with git") {
			t.Errorf("Call() Reason = %q; want the worktree-clean way forward", ptr.Reason)
		}

		if err := os.Remove(untracked); err != nil {
			t.Fatalf("remove untracked file: %v", err)
		}
		outcome, _, err := p.Call(context.Background())
		if err != nil || outcome != shedengine.Done {
			t.Errorf("re-step Call() = (%q, %v); want Done once the worktree is clean", outcome, err)
		}
	})
}

// hubStampPath returns the hub's build stamp file.
func hubStampPath(h *hubforge.Hub) string {
	return hubreconcile.Geometry{BoardDir: h.BoardDir()}.StampPath()
}

// commitRetiredBatcherKey commits a batcher.yaml carrying the retired master_base key.
// The commit goes into the records worktree at recordsRoot.
func commitRetiredBatcherKey(t *testing.T, recordsRoot string) {
	t.Helper()

	module, _ := configreg.Lookup("batcher")
	content := strings.Replace(module.Template(), "orientation: 31400", "master_base: 52000", 1)
	gitkit.CommitFile(t, recordsRoot, configengine.ConfigFileRel("batcher"), content, "fixture: retired key")
}

// TestPreflight_StaleStampReconcilesBeforeTheCleanCheck verifies the reconcile step runs first.
// A stale hub's committed retired key is removed and committed before the clean check.
// The row still ends Done and the stamp is written.
func TestPreflight_StaleStampReconcilesBeforeTheCleanCheck(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := setupPreflightWrapperFixture(t)
	hubforge.AddPair(t, h, "pair-a")
	task := h.PairCodeWorktree("pair-a")
	commitRetiredBatcherKey(t, h.PairRecordsSibling("pair-a"))
	p := NewPreflight("Preflight", task)

	shedfake.RequireOutcome(t, p, shedengine.Done)

	if data, err := os.ReadFile(configengine.ConfigFile(task, "batcher")); err != nil || strings.Contains(string(data), "master_base") {
		t.Errorf("task batcher.yaml still carries the retired key (err %v)", err)
	}
	if _, err := os.Stat(hubStampPath(h)); err != nil {
		t.Errorf("stamp after a Done row: %v; want it written", err)
	}
}

// TestPreflight_ReconcileFailureWarnsAndGoesOnToTheCheck verifies a reconcile failure warns and does not stop the row.
// An unparseable config in another pair, or a held hub lock, lets the row reach the check.
// The row settles Done on an otherwise clean hub and writes no stamp.
func TestPreflight_ReconcileFailureWarnsAndGoesOnToTheCheck(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("unparseable config in another pair", func(t *testing.T) {
		t.Parallel()

		h := setupPreflightWrapperFixture(t)
		hubforge.AddPair(t, h, "pair-a")
		hubforge.AddPair(t, h, "pair-b")
		gitkit.CommitFile(t, h.PairRecordsSibling("pair-b"), configengine.ConfigFileRel("loom"), "a: [unclosed\n", "fixture: broken loom.yaml")
		p := NewPreflight("Preflight", h.PairCodeWorktree("pair-a"))

		shedfake.RequireOutcome(t, p, shedengine.Done)

		if _, err := os.Stat(hubStampPath(h)); err == nil {
			t.Errorf("stamp written after a failed reconcile")
		}
	})

	t.Run("held hub lock", func(t *testing.T) {
		t.Parallel()

		h := setupPreflightWrapperFixture(t)
		geometry := hubreconcile.Geometry{BoardDir: h.BoardDir()}
		if err := os.MkdirAll(filepath.Dir(geometry.LockPath()), 0o755); err != nil {
			t.Fatalf("mkdir lock dir: %v", err)
		}
		held, err := lock.AcquireWriteLock(geometry.LockPath())
		if err != nil {
			t.Fatalf("hold lock: %v", err)
		}
		defer func() { _ = held.Release() }()
		p := newPreflightWith("Preflight", h.PrimeWorktree(), hubreconcile.Options{LockWait: 50 * time.Millisecond})

		shedfake.RequireOutcome(t, p, shedengine.Done)

		if _, err := os.Stat(hubStampPath(h)); err == nil {
			t.Errorf("stamp written without the lock")
		}
	})
}
