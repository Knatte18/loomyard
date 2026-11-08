// pairbranch_integration_test.go covers a branch prefix set while pairs made under the empty prefix are live:
// a pair is read from its own worktrees, local branches and origin.
// Remove, discard and the pair's name lookups reach it by the branches it was made under.
// A new pair gets the prefixed branch.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// laterBranchPrefix is the branch_prefix the tests set after their pairs were made under the empty prefix.
const laterBranchPrefix = "hanf/"

// earlierBranchPrefix is a non-empty branch_prefix a pair was made under before the prefix changed to another value.
const earlierBranchPrefix = "earlier/"

// topologyWithPrefix returns a Topology over l's hub config with branch_prefix set to prefix.
// It models a prefix changed after the hub's pairs were made.
func topologyWithPrefix(t *testing.T, l *lymcwd.Location, prefix string) *fabricengine.Topology {
	t.Helper()

	cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(l))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.BranchPrefix = prefix
	return fabricengine.NewTopology(cfg)
}

// pairOnOriginOnly makes slug's pair under the empty prefix and removes it locally.
// Both its branches stay on origin.
// It returns the hub with a Topology whose branch_prefix is laterBranchPrefix.
func pairOnOriginOnly(t *testing.T, slug string) (*hubforge.Hub, *fabricengine.Topology) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, slug)
	recordsBranch := fabricengine.RecordsBranchName(slug)
	gitkit.MustRun(t, h.PairCodeWorktree(slug), "git", "push", "origin", slug)
	gitkit.MustRun(t, h.PrimeRecords(), "git", "push", "origin", recordsBranch)
	if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
		t.Fatalf("Remove(%q) under the empty prefix: %v", slug, err)
	}
	if parts, err := h.Topology.PairParts(h.Location, slug); err != nil || parts.Any() {
		t.Fatalf("PairParts(%q) = %+v, %v after the local removal; want nothing left locally", slug, parts, err)
	}
	if !gitkit.BranchExists(t, h.CodeBare, slug) || !gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
		t.Fatalf("origin lacks a branch of %q after the local removal; want both", slug)
	}
	return h, topologyWithPrefix(t, h.Location, laterBranchPrefix)
}

// TestBranchPrefixChange_PairWithNothingLeftLocally pins that a pair left with only origin branches:
// made under the empty prefix is still found and torn down by its bare-slug names once branch_prefix is set.
func TestBranchPrefixChange_PairWithNothingLeftLocally(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("RemoveDeletesTheBareSlugBranchesOnOrigin", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			slug string
			// launcherDirLeft leaves the pair's launcher directory behind as its only local remnant.
			launcherDirLeft bool
		}{
			{"NothingLeftLocally", "pn-remove", false},
			{"OnlyTheLauncherDirectoryLeft", "pn-launchers", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				h, top := pairOnOriginOnly(t, tt.slug)
				recordsBranch := fabricengine.RecordsBranchName(tt.slug)
				if tt.launcherDirLeft {
					if err := os.MkdirAll(h.PairLauncherDir(tt.slug), 0o755); err != nil {
						t.Fatalf("create the launcher directory of %q: %v", tt.slug, err)
					}
				}

				res, err := top.Remove(h.Location, tt.slug, false, true)
				if err != nil {
					t.Fatalf("Remove(%q, remote) error = %v", tt.slug, err)
				}
				if res.SiblingBranch != recordsBranch {
					t.Errorf("Remove(%q).SiblingBranch = %q; want the bare records name %q", tt.slug, res.SiblingBranch, recordsBranch)
				}
				if gitkit.BranchExists(t, h.CodeBare, tt.slug) {
					t.Errorf("task branch %q still on origin after Remove with remote; want it deleted", tt.slug)
				}
				if gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
					t.Errorf("records branch %q still on origin after Remove with remote; want it deleted", recordsBranch)
				}
			})
		}
	})

	t.Run("DiscardDeletesTheBareSlugTaskBranchOnOrigin", func(t *testing.T) {
		t.Parallel()

		const slug = "pn-discard"
		h, top := pairOnOriginOnly(t, slug)

		res, err := top.DiscardTaskBranch(h.Location, slug)
		if err != nil {
			t.Fatalf("DiscardTaskBranch(%q) error = %v", slug, err)
		}
		if !res.OriginDeleted || gitkit.BranchExists(t, h.CodeBare, slug) {
			t.Errorf("DiscardTaskBranch(%q) OriginDeleted = %v; want the bare-slug task branch deleted on origin", slug, res.OriginDeleted)
		}
	})

	t.Run("OriginNamesThePairByWhicheverBranchItHolds", func(t *testing.T) {
		t.Parallel()

		const slug = "pn-origin"
		tests := []struct {
			name string
			// dropTask and dropRecords delete that side's branch from origin.
			dropTask, dropRecords bool
			wantNotFound          bool
			wantBranch            string
		}{
			{name: "BothBranchesOnOrigin", wantBranch: slug},
			{name: "OnlyRecordsBranchOnOrigin", dropTask: true, wantBranch: slug},
			{name: "OnlyTaskBranchOnOrigin", dropRecords: true, wantBranch: slug},
			{name: "NeitherBranchOnOrigin", dropTask: true, dropRecords: true, wantNotFound: true, wantBranch: laterBranchPrefix + slug},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				h, top := pairOnOriginOnly(t, slug)
				l := h.Location
				if tt.dropTask {
					gitkit.Git(t, h.CodeBare, "branch", "-D", slug)
				}
				if tt.dropRecords {
					gitkit.Git(t, h.RecordsBare, "branch", "-D", fabricengine.RecordsBranchName(slug))
				}

				err := top.RemoveRefusal(l, slug, false)
				if gotNotFound := errors.Is(err, fabricengine.ErrPairNotFound); gotNotFound != tt.wantNotFound || (err != nil && !gotNotFound) {
					t.Errorf("RemoveRefusal(%q) = %v; want ErrPairNotFound = %v and no other error", slug, err, tt.wantNotFound)
				}
				if got, err := top.PairTaskBranchOrOrigin(l, slug); err != nil || got != tt.wantBranch {
					t.Errorf("PairTaskBranchOrOrigin(%q) = %q, %v; want %q", slug, got, err, tt.wantBranch)
				}
				if got, err := top.PairTaskBranch(l, slug); err != nil || got != laterBranchPrefix+slug {
					t.Errorf("PairTaskBranch(%q) = %q, %v; want the current prefix's %q, as it never asks origin", slug, got, err, laterBranchPrefix+slug)
				}
			})
		}
	})
}

// TestPairTaskBranch_ReadsThePairItself pins PairTaskBranch's read order over one pair:
// made under the empty prefix or an earlier one, with branch_prefix set afterwards.
func TestPairTaskBranch_ReadsThePairItself(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "pt-pair"
	recordsBranch := fabricengine.RecordsBranchName(slug)
	tests := []struct {
		name string
		// arrange changes the pair before PairTaskBranch reads it.
		arrange func(t *testing.T, h *hubforge.Hub)
		want    string
	}{
		{"TaskWorktreeOnAnUnrelatedBranchIsNotRead", func(t *testing.T, h *hubforge.Hub) {
			gitkit.MustRun(t, h.PairCodeWorktree(slug), "git", "checkout", "-b", "elsewhere")
		}, slug},
		{"DetachedTaskWorktreeIsNotRead", func(t *testing.T, h *hubforge.Hub) {
			gitkit.MustRun(t, h.PairCodeWorktree(slug), "git", "checkout", "--detach")
		}, slug},
		{"OnlyTheRecordsWorktreeStands", func(t *testing.T, h *hubforge.Hub) {
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
		}, slug},
		{"RecordsWorktreeStepAnswersBeforeLocalBranches", func(t *testing.T, h *hubforge.Hub) {
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", laterBranchPrefix+slug)
		}, slug},
		{"OnlyLocalBareBranchesLeft", func(t *testing.T, h *hubforge.Hub) {
			removePairWorktreesByHand(t, h, slug)
		}, slug},
		{"OnlyTheLocalRecordsBranchLeft", func(t *testing.T, h *hubforge.Hub) {
			removePairWorktreesByHand(t, h, slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "-D", slug)
		}, slug},
		{"PrefixedBranchWinsOverBareBranch", func(t *testing.T, h *hubforge.Hub) {
			removePairWorktreesByHand(t, h, slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", laterBranchPrefix+slug)
		}, laterBranchPrefix + slug},
		{"NothingLeftFallsThroughToThePrefixedName", func(t *testing.T, h *hubforge.Hub) {
			removePairWorktreesByHand(t, h, slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "-D", slug)
			gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "-D", recordsBranch)
		}, laterBranchPrefix + slug},
		{"TaskWorktreeOnAnEarlierPrefixBranchIsRead", func(t *testing.T, h *hubforge.Hub) {
			renamePairBranches(t, h, slug, earlierBranchPrefix+slug)
			gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(slug))
		}, earlierBranchPrefix + slug},
		{"RecordsWorktreeOnAnEarlierPrefixBranchIsRead", func(t *testing.T, h *hubforge.Hub) {
			renamePairBranches(t, h, slug, earlierBranchPrefix+slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
		}, earlierBranchPrefix + slug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, h, slug)
			tt.arrange(t, h)

			top := topologyWithPrefix(t, h.Location, laterBranchPrefix)
			if got, err := top.PairTaskBranch(h.Location, slug); err != nil || got != tt.want {
				t.Errorf("PairTaskBranch(%q) = %q, %v; want %q", slug, got, err, tt.want)
			}
		})
	}

	// A half-torn pair whose records repository cannot be resolved stays readable:
	// the records step is skipped and a local branch names the pair.
	// The anchor directory is deleted from the prime worktree,
	// so the worktree list RecordsRepoRoot resolves through cannot run.
	t.Run("UnresolvableRecordsRepositoryLeavesTheLocalBranchStep", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend"})
		hubforge.AddPair(t, h, slug)
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
		if err := os.RemoveAll(h.Location.AnchorPath()); err != nil {
			t.Fatalf("delete the anchor directory: %v", err)
		}
		if _, err := fabricengine.RecordsRepoRoot(h.Location); err == nil {
			t.Fatalf("RecordsRepoRoot() error = nil; want the records repository unresolvable")
		}

		top := topologyWithPrefix(t, h.Location, laterBranchPrefix)
		if got, err := top.PairTaskBranch(h.Location, slug); err != nil || got != slug {
			t.Errorf("PairTaskBranch(%q) = %q, %v; want %q", slug, got, err, slug)
		}
		if _, err := top.PairParts(h.Location, slug); err != nil {
			t.Errorf("PairParts(%q) error = %v; want none", slug, err)
		}
	})
}

// renamePairBranches renames slug's task branch to branch and its records branch to branch's records name.
// It renames them as a pair made under an earlier prefix carries its branches, while both worktrees stand.
func renamePairBranches(t *testing.T, h *hubforge.Hub, slug, branch string) {
	t.Helper()

	gitkit.MustRun(t, h.PairCodeWorktree(slug), "git", "branch", "-m", branch)
	gitkit.MustRun(t, h.PairRecordsSibling(slug), "git", "branch", "-m", fabricengine.RecordsBranchName(branch))
}
