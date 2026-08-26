//go:build integration

// cloneconfigcommit_integration_test.go proves both halves of the reported symptom: clone commits
// the per-worktree module configs configsync.ReconcileAll just materialised, on the weft primary
// branch, and a pair forked afterwards inherits them through the branch fork rather than needing its
// own reconcile pass. It also proves the commit is anchor-scoped, single, and shaped as the
// documented one-KindFileWritten-per-module-then-one-KindCommitCreated mutation record.

package fabriccli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// nonFabricModuleNames returns configreg.Modules() filtered to every entry whose Name is not
// "fabric" -- the set CloneAndWire's per-worktree reconcile loop covers, derived fresh every call so
// a tenth registered module never silently makes these tests wrong.
func nonFabricModuleNames() []string {
	var names []string
	for _, m := range configreg.Modules() {
		if m.Name == "fabric" {
			continue
		}
		names = append(names, m.Name)
	}
	return names
}

// gitLsFiles runs `git ls-files` in dir and returns the slash-separated paths it reports.
func gitLsFiles(t *testing.T, dir string) []string {
	t.Helper()

	cmd := exec.Command("git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", dir, err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// containsPath reports whether want is present in got.
func containsPath(got []string, want string) bool {
	for _, g := range got {
		if g == want {
			return true
		}
	}
	return false
}

// TestCloneConfigCommit_WeftPrimeCleanAfterClone asserts a freshly-built hub's weft prime worktree
// is clean, and that git ls-files reports every non-fabric module's config file, proving the
// symptom's "reported dirty, untracked configs" half is fixed.
func TestCloneConfigCommit_WeftPrimeCleanAfterClone(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	if status := gitkit.GitStatusPorcelain(t, h.PrimeWeft()); status != "" {
		t.Errorf("weft prime status --porcelain = %q; want empty (clean)", status)
	}

	tracked := gitLsFiles(t, h.PrimeWeft())
	for _, name := range nonFabricModuleNames() {
		want := configengine.ConfigFileRel(name)
		if !containsPath(tracked, want) {
			t.Errorf("git ls-files in weft prime does not contain %q; got %v", want, tracked)
		}
	}
}

// TestCloneConfigCommit_PairInheritsConfigs asserts a pair created off a freshly-built hub has its
// anchored loom.yaml config on disk -- the direct end-to-end proof of the reported symptom, stopping
// short of spawning loom itself.
func TestCloneConfigCommit_PairInheritsConfigs(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	hubforge.AddPair(t, h, "pair-inherits-configs")

	anchoredWeftBase := filepath.Join(h.PairWeftSibling("pair-inherits-configs"), h.Anchor)
	loomConfigPath := configengine.ConfigFile(anchoredWeftBase, "loom")
	if _, err := os.Stat(loomConfigPath); err != nil {
		t.Errorf("pair weft sibling loom config %s: %v; want it present on disk (inherited from the clone commit)", loomConfigPath, err)
	}
}

// TestCloneConfigCommit_AnchorScoped repeats the clean-and-tracked assertion at a non-"." anchor and
// asserts the committed paths git reports are prefixed with the anchor, proving the commit was
// anchor-scoped rather than run at the weft base, which at a non-"." anchor is a subdirectory of the
// worktree root.
func TestCloneConfigCommit_AnchorScoped(t *testing.T) {
	h := hubforge.NewHub(t, "backend")

	if status := gitkit.GitStatusPorcelain(t, h.PrimeWeft()); status != "" {
		t.Errorf("weft prime status --porcelain = %q; want empty (clean)", status)
	}

	tracked := gitLsFiles(t, h.PrimeWeft())
	for _, name := range nonFabricModuleNames() {
		want := "backend/" + configengine.ConfigFileRel(name)
		if !containsPath(tracked, want) {
			t.Errorf("git ls-files in weft prime does not contain %q; got %v", want, tracked)
		}
	}
}

// TestCloneConfigCommit_OneCommitNotOnePerModule asserts the weft primary branch of a freshly-built
// hub carries the "fabric clone: record module configs" commit subject exactly once, and that a
// second configsync.ReconcileAll over the same weft base reports Applied false for every module --
// proving the commit is a single batched commit, not one per module, and that it observably landed.
func TestCloneConfigCommit_OneCommitNotOnePerModule(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	cmd := exec.Command("git", "log", "--oneline")
	cmd.Dir = h.PrimeWeft()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log --oneline in %s: %v", h.PrimeWeft(), err)
	}

	const wantSubject = "fabric clone: record module configs"
	count := strings.Count(string(out), wantSubject)
	if count != 1 {
		t.Errorf("weft primary git log --oneline contains %q %d times; want exactly 1\nlog:\n%s", wantSubject, count, out)
	}

	results, err := configsync.ReconcileAll(h.WeftBase, true)
	if err != nil {
		t.Fatalf("second ReconcileAll: %v", err)
	}
	for _, r := range results {
		if r.Applied {
			t.Errorf("second ReconcileAll: module %s reported Applied true; want false (nothing left to reconcile)", r.Module)
		}
	}
}

// TestCloneConfigCommit_MutationRecordShape reads the mutation record CloneAndWire produced for a
// freshly-built hub through hubforge.Hub's Mutations field, and asserts it contains one
// KindFileWritten entry per non-fabric module, followed by a KindCommitCreated entry whose Target is
// the weft worktree, with the commit entry last -- array order is part of the vocabulary.
func TestCloneConfigCommit_MutationRecordShape(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	entries := h.Mutations.Entries()
	moduleNames := nonFabricModuleNames()

	// The commit entry must be last, and it must be immediately preceded by exactly one
	// KindFileWritten entry per module in the derived set -- the ReconcileAll loop appends those
	// entries right before CommitAnchoredPaths is called, so their positions are load-bearing, not
	// merely their presence.
	wantLen := len(moduleNames) + 1
	if len(entries) < wantLen {
		t.Fatalf("mutation record has %d entries; want at least %d (one KindFileWritten per module plus a trailing KindCommitCreated)\nentries: %+v", len(entries), wantLen, entries)
	}
	tail := entries[len(entries)-wantLen:]

	for i, name := range moduleNames {
		e := tail[i]
		if e.Kind != fabricengine.KindFileWritten {
			t.Errorf("tail entry %d Kind = %q; want %q (module %s)", i, e.Kind, fabricengine.KindFileWritten, name)
		}
		wantSuffix := "/" + configengine.ConfigFileRel(name)
		if !strings.HasSuffix(e.Target, wantSuffix) {
			t.Errorf("tail entry %d Target = %q; want a suffix of %q (module %s)", i, e.Target, wantSuffix, name)
		}
	}

	last := tail[len(tail)-1]
	if last.Kind != fabricengine.KindCommitCreated {
		t.Errorf("last mutation record entry Kind = %q; want %q", last.Kind, fabricengine.KindCommitCreated)
	}
	wantTarget := hubRelativeWeftTarget(t, h)
	if last.Target != wantTarget {
		t.Errorf("last entry Target = %q; want %q (the weft worktree)", last.Target, wantTarget)
	}
}

// hubRelativeWeftTarget returns the hub-relative, slash-separated path fabricengine.Mutations.Append
// would have recorded for h's weft prime worktree, matching the conversion CommitWeftPaths's own
// recording site performs.
func hubRelativeWeftTarget(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	rel, err := filepath.Rel(h.Path, h.PrimeWeft())
	if err != nil {
		t.Fatalf("filepath.Rel(%s, %s): %v", h.Path, h.PrimeWeft(), err)
	}
	return filepath.ToSlash(rel)
}
