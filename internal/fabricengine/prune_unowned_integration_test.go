//go:build integration

// prune_unowned_integration_test.go pins Prune's ownership gate.
// Prune's orphan pass enumerates on directory NAME alone — every hub child whose name ends in the
// weft suffix — so the set it reports has never been the set it owns.
// Before the gate, `git worktree remove --force` refusing such a path was read as licence to
// os.RemoveAll it, and `prune --apply` destroyed an ordinary operator directory (and a wholly
// unrelated git clone) reporting removed:true, ok:true, exit 0, with no --force and no warning.
//
// Package fabricengine_test to reuse newFabricFixture from
// reconcile_stale_registration_test.go; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// TestPrune_RefusesHubDirectoryItDoesNotOwn parks an ordinary operator directory named
// "<slug>-weft" in the hub and asserts prune reports it Unowned and never deletes it — not on a dry
// run, not on --apply, and not even on --apply --force, since force answers "discard this work",
// never "this directory is mine".
func TestPrune_RefusesHubDirectoryItDoesNotOwn(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	// An ordinary operator directory that is not a git checkout at all and was never fabric's.
	// WeftWarpSlug("notes-weft") yields ("notes", true), so prune's orphan pass enumerates it.
	unowned := filepath.Join(l.HubPath, "notes-weft")
	if err := os.MkdirAll(unowned, 0o755); err != nil {
		t.Fatalf("create unowned hub directory: %v", err)
	}
	precious := filepath.Join(unowned, "precious.md")
	const sentinel = "MY-PRECIOUS-USER-NOTES"
	if err := os.WriteFile(precious, []byte(sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", precious, err)
	}

	for _, mode := range []struct {
		name         string
		apply, force bool
	}{
		{name: "dry-run", apply: false, force: false},
		{name: "apply", apply: true, force: false},
		{name: "apply+force", apply: true, force: true},
	} {
		result, err := topology.Prune(l, mode.apply, mode.force)
		if err != nil {
			t.Fatalf("%s: Prune error = %v", mode.name, err)
		}

		entry := findPruneEntryByWeftPath(t, result.Entries, unowned)
		if !entry.Unowned {
			t.Errorf("%s: Unowned = false for %s; want true — it is not a linked worktree of the weft repo", mode.name, unowned)
		}
		if entry.Removed {
			t.Errorf("%s: Removed = true for an unowned directory; want false", mode.name)
		}
		if !strings.Contains(entry.Error, "not a linked worktree") {
			t.Errorf("%s: Error = %q; want it to say the path is not a linked worktree of the weft repo", mode.name, entry.Error)
		}

		content, readErr := os.ReadFile(precious)
		if readErr != nil {
			t.Fatalf("%s: prune destroyed an operator directory it does not own: %v", mode.name, readErr)
		}
		if !strings.Contains(string(content), sentinel) {
			t.Fatalf("%s: operator content lost: %q no longer contains %q", mode.name, content, sentinel)
		}
	}
}

// TestPrune_RefusesUnrelatedGitCloneInHub is the second shape of the same defect: the parked
// directory is a complete, independently-initialised git repository with a committed file and a
// clean working tree. The clean tree means the dirty-tracked gate has nothing to say even in
// principle, so only the ownership gate stands between `prune --apply` and the whole clone.
func TestPrune_RefusesUnrelatedGitCloneInHub(t *testing.T) {
	t.Parallel()

	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	clone := filepath.Join(l.HubPath, "proj-weft")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatalf("create unrelated clone directory: %v", err)
	}
	lyxtest.MustRun(t, clone, "git", "init", "-b", "main", ".")

	code := filepath.Join(clone, "code.txt")
	const sentinel = "UNRELATED-PROJECT-SOURCE"
	if err := os.WriteFile(code, []byte(sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", code, err)
	}
	lyxtest.MustRun(t, clone, "git", "add", "code.txt")
	lyxtest.MustRun(t, clone, "git", "commit", "-m", "unrelated project commit")

	result, err := topology.Prune(l, true, true)
	if err != nil {
		t.Fatalf("Prune(apply, force) error = %v", err)
	}

	entry := findPruneEntryByWeftPath(t, result.Entries, clone)
	if !entry.Unowned {
		t.Errorf("Unowned = false for an unrelated git clone at %s; want true", clone)
	}
	if entry.Removed {
		t.Errorf("Removed = true for an unrelated git clone; want false")
	}

	content, readErr := os.ReadFile(code)
	if readErr != nil {
		t.Fatalf("prune --apply --force destroyed an unrelated git clone: %v", readErr)
	}
	if !strings.Contains(string(content), sentinel) {
		t.Fatalf("unrelated project source lost: %q no longer contains %q", content, sentinel)
	}
	if _, statErr := os.Stat(filepath.Join(clone, ".git")); statErr != nil {
		t.Fatalf("prune --apply --force destroyed the unrelated clone's gitdir: %v", statErr)
	}
}

// TestPrune_StillRemovesAStaleWeftWorktreeItOwns is the ownership gate's counter-test: a genuine
// stale pair — a weft worktree the hub's weft repo really does register — must still be removed by
// --apply, so the gate refuses only what fabric does not own rather than disabling prune outright.
func TestPrune_StillRemovesAStaleWeftWorktreeItOwns(t *testing.T) {
	t.Parallel()

	const slug = "prune-owned"
	fixture := newFabricFixture(t)
	l := fixture.Layout
	topology := fabricengine.NewTopology(fabricengine.Config{})

	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("setup Add: %v", err)
	}

	weftPath := fabricengine.WeftWorktreePath(l, slug)

	// Make the pair stale: the warp worktree directory disappears, the registration stays.
	if err := os.RemoveAll(fabricengine.WorktreePath(l, slug)); err != nil {
		t.Fatalf("hand-delete warp worktree: %v", err)
	}

	dry, err := topology.Prune(l, false, false)
	if err != nil {
		t.Fatalf("Prune(dry) error = %v", err)
	}
	if dryEntry := findPruneEntryByWeftPath(t, dry.Entries, weftPath); dryEntry.Unowned {
		t.Fatalf("dry run Unowned = true for a genuine stale pair at %s; the gate must refuse only what fabric does not own", weftPath)
	}

	applied, err := topology.Prune(l, true, false)
	if err != nil {
		t.Fatalf("Prune(apply) error = %v", err)
	}
	entry := findPruneEntryByWeftPath(t, applied.Entries, weftPath)
	if !entry.Removed {
		t.Errorf("Removed = false for a genuine stale pair; want true (Error = %q)", entry.Error)
	}
	if _, statErr := os.Stat(weftPath); !os.IsNotExist(statErr) {
		t.Errorf("stale weft worktree still present at %s after --apply", weftPath)
	}
}
