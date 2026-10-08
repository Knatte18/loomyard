//go:build integration

// warpforward_integration_test.go is the Tier-2 real-git coverage for the
// warp-only Fabric methods added in warpforward.go: CurrentBranch, IsAncestor,
// and ResetHard. Each test drives a real paired
// Fabric built from a hubforge hub's warp worktree and asserts the
// resulting git state directly — no fake, no mock — since the whole point of
// this file is proving the thin delegation actually reaches real git.
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
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestFabricWarp_IsAncestorOrdersWarpCommits proves IsAncestor reaches the warp checkout's history:
// an older warp commit is an ancestor of a later one, and not the other way round.
func TestFabricWarp_IsAncestorOrdersWarpCommits(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	olderSHA := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")
	laterSHA := gitkit.CommitFile(t, h.PrimeWorktree(), "ancestry.txt", "v1", "ancestry commit")

	if got, err := f.IsAncestor(olderSHA, laterSHA); err != nil || !got {
		t.Errorf("IsAncestor(older, later) = %v, %v; want true, nil", got, err)
	}
	if got, err := f.IsAncestor(laterSHA, olderSHA); err != nil || got {
		t.Errorf("IsAncestor(later, older) = %v, %v; want false, nil", got, err)
	}
}

// TestFabricWarp_ResetHardDiscardsCommitsOnCleanWorktree proves ResetHard discards a later commit,
// landing HEAD exactly at the older sha, when the warp checkout has no uncommitted changes.
// This is the half of ResetHard's contract that is unaffected by the gate: a clean tracked
// worktree is never dirty, so dirtyScopeTracked never refuses it.
//
//testtiming:keep ResetHard discarding a later commit and landing HEAD at the older sha on a clean worktree; coverage of its blocks by other tests does not show an assertion of this
func TestFabricWarp_ResetHardDiscardsCommitsOnCleanWorktree(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	olderSHA := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")

	// A committed change past olderSHA, with no uncommitted change on top —
	// ResetHard must still discard the committed history.
	laterPath := filepath.Join(h.PrimeWorktree(), "reset-hard-later.txt")
	gitkit.CommitFile(t, h.PrimeWorktree(), "reset-hard-later.txt", "committed", "later commit past olderSHA")

	if err := f.ResetHard(fabricengine.NewMutations(""), olderSHA); err != nil {
		t.Fatalf("ResetHard(%q): %v", olderSHA, err)
	}

	if got := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); got != olderSHA {
		t.Errorf("HEAD SHA after ResetHard = %q; want %q", got, olderSHA)
	}
	if _, err := os.Stat(laterPath); !os.IsNotExist(err) {
		t.Errorf("file %s still present after ResetHard; want discarded (err=%v)", laterPath, err)
	}
}

// TestFabricWarp_ResetHardRefusesDirtyWarpCheckout proves ResetHard refuses to run, leaving both
// the later commit and the uncommitted working-tree change on disk, when the warp checkout has
// uncommitted tracked changes. This is card 11's deliberate hardening of ResetHard's contract:
// it no longer unconditionally discards, matching Pull's own pre-existing ErrWarpDirty check but
// enforced at the ResetHard call site itself rather than only by callers who wrap it in Pull.
//
//testtiming:keep ResetHard refusing a dirty tracked checkout and leaving the commit and the change on disk; coverage of its blocks by other tests does not show an assertion of this
func TestFabricWarp_ResetHardRefusesDirtyWarpCheckout(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	olderSHA := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")

	// A committed change past olderSHA, then an uncommitted change on top —
	// ResetHard must refuse rather than discard either one.
	laterPath := filepath.Join(h.PrimeWorktree(), "reset-hard-later.txt")
	gitkit.CommitFile(t, h.PrimeWorktree(), "reset-hard-later.txt", "committed", "later commit past olderSHA")
	const uncommittedContent = "uncommitted edit"
	if err := os.WriteFile(laterPath, []byte(uncommittedContent), 0o644); err != nil {
		t.Fatalf("write uncommitted change: %v", err)
	}

	err = f.ResetHard(fabricengine.NewMutations(""), olderSHA)
	if err == nil {
		t.Fatalf("ResetHard(%q) on dirty warp checkout error = nil; want a refusal", olderSHA)
	}
	if !strings.Contains(err.Error(), "dirtiness check failed") {
		t.Errorf("ResetHard(%q) error = %q; want a dirtiness-gate refusal", olderSHA, err)
	}

	if got := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); got == olderSHA {
		t.Errorf("HEAD SHA after refused ResetHard = %q; want the later commit to remain (refusal must not discard history)", got)
	}
	gotContent, err := os.ReadFile(laterPath)
	if err != nil {
		t.Fatalf("read %s after refused ResetHard: %v", laterPath, err)
	}
	if string(gotContent) != uncommittedContent {
		t.Errorf("content of %s after refused ResetHard = %q; want uncommitted change left on disk (%q)", laterPath, gotContent, uncommittedContent)
	}
}

// TestFabricWarp_CurrentBranchErrorsOnDetachedHead proves CurrentBranch returns a non-nil error
// when warp's HEAD is already detached, matching gitrepo.Repo.CurrentBranch's documented
// detached-HEAD rejection.
func TestFabricWarp_CurrentBranchErrorsOnDetachedHead(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}

	gitkit.MustRun(t, h.PrimeWorktree(), "git", "checkout", "--detach")

	if _, err := f.CurrentBranch(); err == nil {
		t.Fatalf("CurrentBranch() on detached HEAD error = nil; want non-nil")
	}
}

// TestStencilSource_BuildAncestryFollowsTheRevision pins StencilSource's three source shapes and the ancestry its Build reports.
// The ancestry is memoized, so a second call returns the first answer.
func TestStencilSource_BuildAncestryFollowsTheRevision(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	prime := h.PrimeWorktree()
	headSHA := gitkit.RevParse(t, prime, "HEAD")
	absentSHA := strings.Repeat("a", len(headSHA))
	sourceDir := filepath.Join(prime, "contracts", "stencils")

	tests := []struct {
		name         string
		sourceDir    string
		revision     string
		wantDir      string
		wantNoBuild  bool
		wantAncestry stencilstore.BuildAncestry
	}{
		{name: "empty source dir is the zero source", revision: headSHA, wantNoBuild: true},
		{name: "empty revision has no build func", sourceDir: sourceDir, wantDir: sourceDir, wantNoBuild: true},
		{name: "head sha is in head", sourceDir: sourceDir, revision: headSHA, wantDir: sourceDir, wantAncestry: stencilstore.BuildInHead},
		{name: "absent sha is not in head", sourceDir: sourceDir, revision: absentSHA, wantDir: sourceDir, wantAncestry: stencilstore.BuildNotInHead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := fabricengine.StencilSource(prime, tt.sourceDir, tt.revision)
			if source.Dir != tt.wantDir {
				t.Errorf("Source.Dir = %q; want %q", source.Dir, tt.wantDir)
			}
			if tt.wantNoBuild {
				if source.Build != nil {
					t.Errorf("Source.Build != nil; want nil")
				}
				return
			}
			if got := source.Build(); got != tt.wantAncestry {
				t.Errorf("first Build() = %v; want %v", got, tt.wantAncestry)
			}
			if got := source.Build(); got != tt.wantAncestry {
				t.Errorf("second Build() = %v; want the memoized %v", got, tt.wantAncestry)
			}
		})
	}
}
