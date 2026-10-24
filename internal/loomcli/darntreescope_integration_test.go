//go:build tier1

// darntreescope_integration_test.go drives darnTreeScope against a real fabric pair and real git, since its answer depends on the merge base the pair's history leaves and on git's rename detection.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestDarnTreeScope_ClassifiesAtEachCall steps one pair through the diffs a task branch can hold and pins the seam's answer after each, reading the hub-wide darn.yaml afresh at every call.
// The steps run in order over one pair, so each builds on the history the steps before left.
func TestDarnTreeScope_ClassifiesAtEachCall(t *testing.T) {
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	seed := func(t *testing.T, treeScope string) {
		t.Helper()
		seedDarnHubConfig(t, hub.Path, "3", "tree_scope: {}", treeScope)
	}
	const trees = `tree_scope: {"docs": "", "tools": "./tools/..."}`
	// The non-scoped rows render their own key's call over the whole tree.
	const landingCall = "lyx gate test --wait --gate finalize"
	// scopedCall is the call a tree-scoped site runs: its row key's call followed by the touched trees' targets.
	scopedCall := func(gateKey, targets string) string {
		return "lyx gate test --wait --gate " + gateKey + " " + targets
	}
	writeHubGateConfig(t, hub.Path, strings.Replace(gateslot.ConfigTemplate(), "tiers: {}", `tiers:
  tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json}`, 1))
	seed(t, trees)

	prime := hub.PrimeWorktree()
	resolve := func(t *testing.T, slug string) *lyxcwd.Location {
		t.Helper()
		location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%s) = %v; want nil", hub.PairCodeWorktree(slug), err)
		}
		return location
	}
	locate := func(t *testing.T, slug string) func(gateKey string) (string, bool, error) {
		t.Helper()
		return darnTreeScope(resolve(t, slug))
	}
	// landing builds the pair's landing seams over the darn verify source, with the values the seams under test do not read left minimal.
	landing := func(t *testing.T, slug string) landingshed.Deps {
		t.Helper()
		location := resolve(t, slug)
		return landingDeps(
			location,
			shedrun.RecipeDarn,
			websterengine.Geometry{WebsterDir: "/webster", StencilsDir: "/stencils"},
			"task/"+slug,
			"main",
			true,
			modelspec.Registry{"claude/sonnet-5": {}},
			&shuttleengine.Runner{},
			landingshed.Config{},
			"parent-session",
			nil,
			func(string, time.Time) error { return nil },
			func() (string, error) { return "", nil },
			darnVerifySource(location),
			parentLockDeps(location, shedrun.SelfRunID, "", ""),
		)
	}
	assertLanding := func(t *testing.T, deps landingshed.Deps, wantCommand string) {
		t.Helper()
		command, err := deps.VerifyCommand("finalize")
		if err != nil || command != wantCommand {
			t.Errorf("VerifyCommand(finalize) = %q, %v; want %q, nil", command, err, wantCommand)
		}
	}
	// assertAnswer pins the seam's answer for the darn-write key: wantTargets is the targets its call holds, and an empty one means no call at all.
	assertAnswer := func(t *testing.T, seam func(gateKey string) (string, bool, error), wantTargets string, wantScoped bool) {
		t.Helper()
		wantCommand := ""
		if wantTargets != "" {
			wantCommand = scopedCall("darn-write", wantTargets)
		}
		command, scoped, err := seam("darn-write")
		if err != nil || command != wantCommand || scoped != wantScoped {
			t.Errorf("darnTreeScope()() = %q, %v, %v; want %q, %v, nil", command, scoped, err, wantCommand, wantScoped)
		}
	}
	assertRefusal := func(t *testing.T, seam func(gateKey string) (string, bool, error), wants ...string) {
		t.Helper()
		command, scoped, err := seam("darn-write")
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
	deps := landing(t, slug)

	if !t.Run("a pair with no commit of its own is tree-scoped over no targets and runs nothing", func(t *testing.T) {
		assertAnswer(t, seam, "", true)
		assertLanding(t, deps, "")
	}) {
		return
	}

	if !t.Run("a docs-only diff under a tree with an empty target is tree-scoped and runs nothing", func(t *testing.T) {
		gitkit.CommitFile(t, worktree, "docs/ä.md", "docs\n", "docs change")
		assertAnswer(t, seam, "", true)
		assertLanding(t, deps, "")
	}) {
		return
	}

	if !t.Run("a diff wholly under the trees is tree-scoped over the touched trees' targets, each row key's call", func(t *testing.T) {
		gitkit.CommitFile(t, worktree, "tools/a.go", "package tools\n", "tools change")
		for _, gateKey := range []string{"darn-write", "code-burler-fix", "finalize"} {
			command, scoped, err := seam(gateKey)
			if want := scopedCall(gateKey, "./tools/..."); err != nil || !scoped || command != want {
				t.Errorf("darnTreeScope()(%s) = %q, %v, %v; want %q, true, nil", gateKey, command, scoped, err, want)
			}
		}
		assertLanding(t, deps, scopedCall("finalize", "./tools/..."))
	}) {
		return
	}

	if !t.Run("a merge that brings in only the parent's production change keeps the diff tree-scoped", func(t *testing.T) {
		gitkit.CommitFile(t, prime, "internal/p.go", "package internal\n", "parent production change")
		gitkit.Git(t, worktree, "merge", "--no-edit", gitkit.CurrentBranch(t, prime))
		assertAnswer(t, seam, "./tools/...", true)
		assertLanding(t, deps, scopedCall("finalize", "./tools/..."))
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
		assertLanding(t, deps, landingCall)
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
		assertLanding(t, landing(t, freshSlug), landingCall)
	}) {
		return
	}

	if !t.Run("a tree key with a leading dot is refused with the editor way forward", func(t *testing.T) {
		seed(t, `tree_scope: {"./tools": ""}`)
		assertRefusal(t, seam, "./tools", "lyx config darn's editor")
		wantRefusal := []string{"./tools", "lyx config darn's editor", "then re-run the lyx command that refused"}
		_, commandErr := deps.VerifyCommand("finalize")
		if commandErr == nil {
			t.Error("VerifyCommand(finalize) error = nil; want a refusal")
		} else {
			for _, want := range wantRefusal {
				if !strings.Contains(commandErr.Error(), want) {
					t.Errorf("VerifyCommand(finalize) error = %q; want it to contain %q", commandErr, want)
				}
			}
		}
	}) {
		return
	}

	if !t.Run("a darn.yaml that does not unmarshal is refused with the hub config way forward", func(t *testing.T) {
		seed(t, trees)
		path := filepath.Join(fabricengine.BoardDir(hub.Path), "_lyx", "config", "darn.yaml")
		if err := os.WriteFile(path, []byte("writer: [\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		assertRefusal(t, seam, hubConfigWayForward)
	}) {
		return
	}

	t.Run("an emptied tree_scope turns tree scope off", func(t *testing.T) {
		seed(t, `tree_scope: {}`)
		assertAnswer(t, seam, "", false)
		assertLanding(t, deps, landingCall)
	})
}
