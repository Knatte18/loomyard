//go:build integration

// darntreescope_integration_test.go drives darnTreeScope against a real fabric pair and real git, since its answer depends on the merge base the pair's history leaves and on git's rename detection.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestDarnTreeScope_ClassifiesAtEachCall steps one pair through the diffs a task branch can hold and pins the seam's answer after each, reading the hub-wide darn.yaml afresh at every call.
// The steps run in order over one pair, so each builds on the history the steps before left.
func TestDarnTreeScope_ClassifiesAtEachCall(t *testing.T) {
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seed := func(t *testing.T, treeScope, treeScopeVerify string) {
		t.Helper()
		seedDarnHubConfig(t, hub.Path, `"run all"`, "3", "tree_scope: {}", treeScope, `tree_scope_verify: ""`, treeScopeVerify)
	}
	const trees = `tree_scope: {"docs": "", "tools": "./tools/..."}`
	const scopedVerify = `tree_scope_verify: "run {targets}"`
	seed(t, trees, scopedVerify)

	prime := hub.PrimeWorktree()
	locate := func(t *testing.T, slug string) func() (string, bool, error) {
		t.Helper()
		location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%s) = %v; want nil", hub.PairCodeWorktree(slug), err)
		}
		return darnTreeScope(location)
	}
	assertAnswer := func(t *testing.T, seam func() (string, bool, error), wantCommand string, wantScoped bool) {
		t.Helper()
		command, scoped, err := seam()
		if err != nil || command != wantCommand || scoped != wantScoped {
			t.Errorf("darnTreeScope()() = %q, %v, %v; want %q, %v, nil", command, scoped, err, wantCommand, wantScoped)
		}
	}
	assertRefusal := func(t *testing.T, seam func() (string, bool, error), wants ...string) {
		t.Helper()
		command, scoped, err := seam()
		if err == nil {
			t.Fatalf("darnTreeScope()() = %q, %v, nil; want a refusal", command, scoped)
		}
		for _, want := range wants {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("darnTreeScope()() error = %q; want it to contain %q", err, want)
			}
		}
	}

	const slug = "scoped"
	hubforge.AddPair(t, hub, slug)
	worktree := hub.PairCodeWorktree(slug)
	seam := locate(t, slug)

	if !t.Run("a pair with no commit of its own is tree-scoped over no targets", func(t *testing.T) {
		assertAnswer(t, seam, "run ", true)
	}) {
		return
	}

	if !t.Run("a diff wholly under the trees is tree-scoped over the touched trees' targets", func(t *testing.T) {
		gitkit.CommitFile(t, worktree, "docs/ä.md", "docs\n", "docs change")
		gitkit.CommitFile(t, worktree, "tools/a.go", "package tools\n", "tools change")
		assertAnswer(t, seam, "run ./tools/...", true)
	}) {
		return
	}

	if !t.Run("a merge that brings in only the parent's production change keeps the diff tree-scoped", func(t *testing.T) {
		gitkit.CommitFile(t, prime, "internal/p.go", "package internal\n", "parent production change")
		gitkit.Git(t, worktree, "merge", "--no-edit", gitkit.CurrentBranch(t, prime))
		assertAnswer(t, seam, "run ./tools/...", true)
	}) {
		return
	}

	if !t.Run("a merge resolution that adds a production file makes the diff not tree-scoped", func(t *testing.T) {
		gitkit.CommitFile(t, prime, "internal/q.go", "package internal\n", "parent second production change")
		gitkit.Git(t, worktree, "merge", "--no-commit", "--no-ff", gitkit.CurrentBranch(t, prime))
		if err := os.WriteFile(filepath.Join(worktree, "internal", "r.go"), []byte("package internal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, worktree, "add", "--", "internal/r.go")
		gitkit.Git(t, worktree, "commit", "--no-edit")
		assertAnswer(t, seam, "", false)
	}) {
		return
	}

	if !t.Run("a rename out of production counts the old path and makes the diff not tree-scoped", func(t *testing.T) {
		gitkit.CommitFile(t, prime, "internal/x.go", "package internal\n", "parent file to move")
		const freshSlug = "renamed"
		hubforge.AddPair(t, hub, freshSlug)
		freshWorktree := hub.PairCodeWorktree(freshSlug)
		if err := os.MkdirAll(filepath.Join(freshWorktree, "tools"), 0o755); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, freshWorktree, "mv", "internal/x.go", "tools/x.go")
		gitkit.Git(t, freshWorktree, "commit", "-m", "move the file into tools")
		assertAnswer(t, locate(t, freshSlug), "", false)
	}) {
		return
	}

	if !t.Run("a tree key with a leading dot is refused with the editor way forward", func(t *testing.T) {
		seed(t, `tree_scope: {"./tools": ""}`, scopedVerify)
		assertRefusal(t, seam, "./tools", "lyx config darn's editor")
	}) {
		return
	}

	if !t.Run("a darn.yaml that does not unmarshal is refused with the verify way forward", func(t *testing.T) {
		seed(t, trees, scopedVerify)
		path := filepath.Join(fabricengine.BoardDir(hub.Path), "_lyx", "config", "darn.yaml")
		if err := os.WriteFile(path, []byte("verify: [\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		assertRefusal(t, seam, "lyx config darn --set verify=", `then "lyx loom resume"`)
	}) {
		return
	}

	t.Run("an emptied tree_scope_verify turns tree scope off", func(t *testing.T) {
		seed(t, trees, `tree_scope_verify: ""`)
		assertAnswer(t, seam, "", false)
	})
}
