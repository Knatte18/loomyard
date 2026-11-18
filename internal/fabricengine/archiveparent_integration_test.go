// archiveparent_integration_test.go covers ArchiveParentMerge over a pair fixture whose parent branch is the prime's.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestArchiveParentMerge drives the archive of a parent branch the run's own merge put ahead of origin.
func TestArchiveParentMerge(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "archive-parent"
	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}

	t.Run("ARejectedPushOverAParentHoldingTheWorkLeavesTheMergeUnderAPushedTag", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		primeDir := d.hub.Location.WorktreePath()
		primeBranch := gitkit.CurrentBranch(t, primeDir)
		gitkit.Git(t, primeDir, "push", "--set-upstream", "origin", primeBranch)
		other := filepath.Join(t.TempDir(), "other")
		gitkit.Git(t, filepath.Dir(other), "clone", "--branch", primeBranch, d.hub.CodeBare, other)
		adopters := gitkit.CommitFile(t, other, "adopted.txt", "the task's work\n", "the adopter's merge")
		gitkit.Git(t, other, "push", "origin", primeBranch)
		gitkit.Git(t, primeDir, "merge", "--no-ff", "-m", "merge the run", d.warpBranch)
		merge := gitkit.RevParse(t, primeDir, "HEAD")

		tag, err := fabricengine.ArchiveParentMerge(d.hub.Location, slug)

		if err != nil {
			t.Fatalf("ArchiveParentMerge() error = %v", err)
		}
		if want := fabricengine.OwnerArchiveTagName(slug, merge); tag != want {
			t.Errorf("ArchiveParentMerge() tag = %q; want %q", tag, want)
		}
		if got := tagTargetAt(t, d.hub.CodeBare, tag); got != merge {
			t.Errorf("origin tag %s points at %s; want the local merge %s", tag, got, merge)
		}
		if got := gitkit.RevParse(t, primeDir, "HEAD"); got != adopters {
			t.Errorf("local parent branch = %s; want it level with origin at %s", got, adopters)
		}
		if got := bareBranchTip(t, d.hub.CodeBare, primeBranch); got != adopters {
			t.Errorf("origin parent branch = %s; want it unchanged at %s, holding the task's work once", got, adopters)
		}
	})

	t.Run("ALevelBranchArchivesNothing", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		primeDir := d.hub.Location.WorktreePath()
		primeBranch := gitkit.CurrentBranch(t, primeDir)
		gitkit.Git(t, primeDir, "push", "--set-upstream", "origin", primeBranch)
		before := gitkit.RevParse(t, primeDir, "HEAD")

		tag, err := fabricengine.ArchiveParentMerge(d.hub.Location, slug)

		if err != nil {
			t.Fatalf("ArchiveParentMerge() error = %v", err)
		}
		if tag != "" {
			t.Errorf("ArchiveParentMerge() tag = %q; want none", tag)
		}
		if got := gitkit.Git(t, d.hub.CodeBare, "tag", "--list", "archive-owner/*"); got != "" {
			t.Errorf("origin archive tags = %q; want none", got)
		}
		if got := gitkit.RevParse(t, primeDir, "HEAD"); got != before {
			t.Errorf("local parent branch = %s; want it left at %s", got, before)
		}
	})
}
