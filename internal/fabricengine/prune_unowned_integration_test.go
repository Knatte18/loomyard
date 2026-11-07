// prune_unowned_integration_test.go pins Prune's ownership gate.
// Prune's orphan pass enumerates every hub child whose name ends in the weft suffix, by name alone.
// The gate stops `prune --apply` from deleting such a directory unless fabric owns it.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestPrune_RefusesHubDirectoryItDoesNotOwn parks an ordinary operator directory named "<slug>-weft" in the hub.
// It asserts prune reports it Unowned and never deletes it, on a dry run, on --apply, and on --apply --force.
// Force answers "discard this work", never "this directory is mine", so it does not lift the refusal.
func TestPrune_RefusesHubDirectoryItDoesNotOwn(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	// An ordinary operator directory that is not a git checkout at all and was never fabric's.
	// `WeftWarpSlug("notes-weft")` yields ("notes", true), so prune's orphan pass enumerates it.
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

// TestPrune_RefusesUnrelatedGitCloneInHub parks a complete, independently initialised git repository in the hub.
// The repository has a committed file and a clean working tree.
// A clean tree leaves the dirty-worktree check nothing to refuse.
// Only the ownership gate stands between `prune --apply` and the whole clone.
func TestPrune_RefusesUnrelatedGitCloneInHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	clone := filepath.Join(l.HubPath, "proj-weft")
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatalf("create unrelated clone directory: %v", err)
	}
	gitkit.Git(t, clone, "init", "-b", "main", ".")

	code := filepath.Join(clone, "code.txt")
	const sentinel = "UNRELATED-PROJECT-SOURCE"
	gitkit.CommitFile(t, clone, "code.txt", sentinel+"\n", "unrelated project commit")

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

// TestPrune_ReportsANonWorktreeDirectoryAtAPairPath parks a directory of build output at a pair slug's path.
// A late writer leaves such a directory after a removal.
// It asserts every mode reports the directory Unowned with the way to clear it and never deletes it.
func TestPrune_ReportsANonWorktreeDirectoryAtAPairPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	stray := fabricengine.WorktreePath(l, "leftover-task")
	buildOutput := filepath.Join(stray, "src", "App", "obj", "project.assets.json")
	if err := os.MkdirAll(filepath.Dir(buildOutput), 0o755); err != nil {
		t.Fatalf("create %s: %v", stray, err)
	}
	if err := os.WriteFile(buildOutput, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", buildOutput, err)
	}

	for _, mode := range []struct {
		name         string
		apply, force bool
	}{
		{name: "dry-run", apply: false, force: false},
		{name: "apply", apply: true, force: false},
		{name: "apply+force", apply: true, force: true},
	} {
		result, err := h.Topology.Prune(l, mode.apply, mode.force)
		if err != nil {
			t.Fatalf("%s: Prune error = %v", mode.name, err)
		}

		var entry *fabricengine.PruneEntry
		for i := range result.Entries {
			if filepath.Clean(filepath.FromSlash(result.Entries[i].CodeWorktree)) == filepath.Clean(stray) {
				entry = &result.Entries[i]
			}
		}
		if entry == nil {
			t.Fatalf("%s: Prune entries = %+v; want one for %s", mode.name, result.Entries, stray)
		}
		if !entry.Unowned || entry.Removed {
			t.Errorf("%s: Unowned, Removed = %v, %v for %s; want true, false", mode.name, entry.Unowned, entry.Removed, stray)
		}
		if !strings.Contains(entry.Error, "save anything worth keeping from it, then delete it") {
			t.Errorf("%s: Error = %q; want it to name the way to clear the directory", mode.name, entry.Error)
		}
		if _, statErr := os.Stat(buildOutput); statErr != nil {
			t.Fatalf("%s: prune deleted a directory it does not own: %v", mode.name, statErr)
		}
	}
}

// TestPrune_IgnoresDirectoriesThatAreNoPairPath asserts the pass over pair paths reports no entry for a live pair.
// It also asserts the pass ignores a hidden directory, a weft-suffixed directory and a reserved hub name.
func TestPrune_IgnoresDirectoriesThatAreNoPairPath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	hubforge.AddPairWith(t, h, "live-pair", fabricengine.AddOptions{SkipPush: true})

	livePair := fabricengine.WorktreePath(l, "live-pair")
	parked := []string{
		filepath.Join(l.HubPath, ".hidden"),
		filepath.Join(l.HubPath, "notes-weft"),
		filepath.Join(l.HubPath, "_portals"),
	}
	for _, path := range parked {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
	}
	notReported := append([]string{livePair}, parked...)

	result, err := h.Topology.Prune(l, false, false)
	if err != nil {
		t.Fatalf("Prune error = %v", err)
	}
	for _, entry := range result.Entries {
		got := filepath.Clean(filepath.FromSlash(entry.CodeWorktree))
		for _, path := range notReported {
			if got == filepath.Clean(path) {
				t.Errorf("Prune reported %+v; want no entry for %s", entry, path)
			}
		}
	}
}

// TestPrune_StillRemovesAStaleWeftWorktreeItOwns is the ownership gate's counter-test.
// A stale pair whose weft worktree the hub's weft repo registers must still be removed by --apply.
// The gate refuses only what fabric does not own, rather than disabling prune outright.
func TestPrune_StillRemovesAStaleWeftWorktreeItOwns(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "prune-owned"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	topology := h.Topology

	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{SkipPush: true})

	weftPath := fabricengine.RecordsWorktreePath(l, slug)

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
