//go:build integration

// geometry_integration_test.go drives ReedGeometry and WebsterGeometry against one real hub from hubforge:
// the prime's reed geometry carries the hub's shortname with no slug or parent, a task worktree carries its slug and the parent its origin record names, or none when the origin names none, and a hub with no .lyx-shortname yields an empty shortname and no error.

package hubgeom_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestGeometryOverRealHub builds one hub and runs each geometry case as a step over it, in the order below.
// The steps run serially, and the last removes the hub's shortname record, so it must stay last.
// The top-level test calls t.Parallel and no step does, because the steps share the one hub fixture.
func TestGeometryOverRealHub(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	if !t.Run("reed prime carries the shortname only", func(t *testing.T) {
		geom, err := hubgeom.ReedGeometry(h.Location)
		if err != nil {
			t.Fatalf("ReedGeometry(prime): %v", err)
		}
		if geom.NameShortname != hubforge.TestShortname {
			t.Errorf("NameShortname = %q; want %q", geom.NameShortname, hubforge.TestShortname)
		}
		if geom.NameSlug != "" || geom.ParentName != "" {
			t.Errorf("prime NameSlug/ParentName = %q/%q; want both empty", geom.NameSlug, geom.ParentName)
		}
	}) {
		return
	}

	if !t.Run("webster parent name comes from the origin record", func(t *testing.T) {
		const slug = "webster-parent-task"
		hubforge.AddPair(t, h, slug)

		l, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		if got, want := hubgeom.WebsterGeometry(l).ParentName, hubforge.TestShortname+":orch"; got != want {
			t.Errorf("ParentName = %q; want %q (the prime's orch, from the origin record)", got, want)
		}

		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor = %v, found %v", err, found)
		}
		origin.ParentWorktree = ""
		if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin without parent_worktree: %v", err)
		}
		if got := hubgeom.WebsterGeometry(l).ParentName; got != "" {
			t.Errorf("ParentName without a parent worktree = %q; want empty", got)
		}
	}) {
		return
	}

	if !t.Run("reed task worktree carries its slug and the origin's parent", func(t *testing.T) {
		const slug = "naming-task"
		hubforge.AddPair(t, h, slug)

		l, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		before, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) before seeding: %v", err)
		}
		if before.NameShortname != hubforge.TestShortname || before.NameSlug != slug {
			t.Errorf("task NameShortname/NameSlug = %q/%q; want %q/%q", before.NameShortname, before.NameSlug, hubforge.TestShortname, slug)
		}
		if want := hubforge.TestShortname + ":orch"; before.ParentName != want {
			t.Errorf("ParentName before seeding = %q; want %q (the prime's orch, from the origin record)", before.ParentName, want)
		}

		origin, found, err := fabricengine.ReadOriginFor(l, slug)
		if err != nil || !found {
			t.Fatalf("ReadOriginFor = %v, found %v", err, found)
		}
		origin.ParentWorktree = "other-task"
		if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin with another parent_worktree: %v", err)
		}
		const parent = hubforge.TestShortname + ":other-task:orch"
		after, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) after replanting the parent: %v", err)
		}
		if after.ParentName != parent {
			t.Errorf("ParentName after replanting = %q; want %q", after.ParentName, parent)
		}

		origin.ParentWorktree = ""
		if err := fabricengine.WriteOrigin(&fabricengine.Mutations{}, l, slug, origin); err != nil {
			t.Fatalf("WriteOrigin without parent_worktree: %v", err)
		}
		none, err := hubgeom.ReedGeometry(l)
		if err != nil {
			t.Fatalf("ReedGeometry(task) without a parent worktree: %v", err)
		}
		if none.ParentName != "" {
			t.Errorf("ParentName without a parent worktree = %q; want empty", none.ParentName)
		}
	}) {
		return
	}

	if !t.Run("reconcile restores the lock directory of a recreated sibling worktree", func(t *testing.T) {
		const slug = "recreated-task"
		hubforge.AddPair(t, h, slug)
		if err := os.RemoveAll(h.PairRecordsSibling(slug)); err != nil {
			t.Fatalf("hand-delete sibling worktree: %v", err)
		}

		if pair := reconcilePairFor(t, h, slug); pair.Error != "" {
			t.Fatalf("reconcile pair error = %q; want none", pair.Error)
		}
		requireLockDirAndOrchParent(t, h, slug)
	}) {
		return
	}

	if !t.Run("reconcile heals a pair whose lock directory was deleted", func(t *testing.T) {
		const slug = "lockless-task"
		hubforge.AddPair(t, h, slug)
		lockDir := fabricengine.RecordsLockDirPath(h.PairRecordsSibling(slug))
		if info, err := os.Stat(lockDir); err != nil || !info.IsDir() {
			t.Fatalf("freshly added pair has no lock directory: %v", err)
		}
		if err := os.RemoveAll(lockDir); err != nil {
			t.Fatalf("delete lock directory: %v", err)
		}

		l, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		_, err = hubgeom.ResolveParent(l)
		if err == nil || !strings.Contains(err.Error(), filepath.Base(lockDir)) || !strings.Contains(err.Error(), "lyx fabric reconcile") {
			t.Fatalf("ResolveParent without the lock directory = %v; want an error naming the lock directory and `lyx fabric reconcile`", err)
		}

		pair := reconcilePairFor(t, h, slug)
		if pair.Action != fabricengine.ReconcileActionAlreadyHealthy || pair.Error != "" {
			t.Errorf("reconcile pair action/error = %q/%q; want %q and no error", pair.Action, pair.Error, fabricengine.ReconcileActionAlreadyHealthy)
		}
		if !strings.Contains(pair.Detail, lockDir) {
			t.Errorf("reconcile pair detail = %q; want it to name the restored lock directory %s", pair.Detail, lockDir)
		}
		requireLockDirAndOrchParent(t, h, slug)
	}) {
		return
	}

	// This step removes the shortname record every earlier step read, so it runs last.
	t.Run("hub with no shortname yields an empty shortname", func(t *testing.T) {
		if err := os.Remove(filepath.Join(h.BoardDir(), fabricengine.ShortnameFileName)); err != nil {
			t.Fatalf("remove shortname record: %v", err)
		}

		geom, err := hubgeom.ReedGeometry(h.Location)
		if err != nil {
			t.Fatalf("ReedGeometry on a hub with no shortname: %v", err)
		}
		if geom.NameShortname != "" {
			t.Errorf("NameShortname = %q; want empty", geom.NameShortname)
		}
	})
}

// reconcilePairFor runs `fabric reconcile` from the hub's prime through the CLI and returns slug's pair report.
func reconcilePairFor(t *testing.T, h *hubforge.Hub, slug string) fabricengine.ReconcilePairResult {
	t.Helper()

	var out bytes.Buffer
	if code := fabriccli.RunCLIIn(h.Location.WorktreePath(), &out, []string{"reconcile"}); code != 0 {
		t.Fatalf("reconcile = %d; want 0\noutput: %s", code, out.String())
	}
	var envelope struct {
		Pairs []fabricengine.ReconcilePairResult `json:"pairs"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode reconcile envelope: %v\noutput: %s", err, out.String())
	}
	for _, pair := range envelope.Pairs {
		if filepath.Base(pair.CodeWorktree) == slug {
			return pair
		}
	}
	t.Fatalf("reconcile envelope has no pair for %q\noutput: %s", slug, out.String())
	return fabricengine.ReconcilePairResult{}
}

// requireLockDirAndOrchParent asserts slug's sibling worktree has its lock directory and its parent resolves to the prime's orch.
func requireLockDirAndOrchParent(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if info, err := os.Stat(fabricengine.RecordsLockDirPath(h.PairRecordsSibling(slug))); err != nil || !info.IsDir() {
		t.Errorf("lock directory missing in %s: %v", h.PairRecordsSibling(slug), err)
	}
	l, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	parent, err := hubgeom.ResolveParent(l)
	if err != nil {
		t.Fatalf("ResolveParent: %v", err)
	}
	if want := hubforge.TestShortname + ":orch"; parent.Name != want {
		t.Errorf("parent name = %q; want %q", parent.Name, want)
	}
}
