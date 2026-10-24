//go:build tier1

// taskbranchdiscard_integration_test.go covers Topology.DiscardTaskBranch and the deletion half it runs under archive tags, over hubforge hubs.
// Every hub has an empty branch_prefix, so a pair's task branch is the bare slug, and h.CodeBare is the warp origin.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// discardHub returns a hub holding slug's pair, pushed, with one unpushed commit on the task branch and the pair's worktrees removed by hand, so both branches survive.
// It returns the hub and the task branch's local tip.
func discardHub(t *testing.T, slug string) (*hubforge.Hub, string) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	warpBranchCommit(t, h.Location, slug, "local-work.txt")
	removePairWorktreesByHand(t, h, slug)
	return h, gitkit.RevParse(t, h.PrimeWorktree(), slug)
}

// advanceOrigin commits one file on top of branch's origin tip through a scratch clone, pushes it and returns the new origin tip.
func advanceOrigin(t *testing.T, h *hubforge.Hub, branch string) string {
	t.Helper()

	scratch := t.TempDir()
	gitkit.MustRun(t, scratch, "git", "clone", "--branch", branch, h.CodeBare, "clone")
	clone := filepath.Join(scratch, "clone")
	gitkit.CommitFile(t, clone, "origin-work.txt", "origin", "origin work")
	gitkit.MustRun(t, clone, "git", "push", "origin", branch)
	return gitkit.RevParse(t, h.CodeBare, branch)
}

// archiveTagsOnOrigin lists the archive tags of slug on h's warp origin.
func archiveTagsOnOrigin(t *testing.T, h *hubforge.Hub, slug string) []string {
	t.Helper()

	return strings.Fields(gitkit.Git(t, h.CodeBare, "tag", "--list", fabricengine.ArchiveTagPrefix+slug+"/*"))
}

// TestDiscardTaskBranch pins which tips DiscardTaskBranch archives, that every branch is gone afterwards, that a missing branch is a zero result, and that a refused archive push deletes nothing.
func TestDiscardTaskBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup returns the hub and the tips that must each hold an archive tag.
		setup func(t *testing.T, slug string) (*hubforge.Hub, []string)
	}{
		{"DivergedTipsAreBothArchived", func(t *testing.T, slug string) (*hubforge.Hub, []string) {
			h, localTip := discardHub(t, slug)
			originTip := advanceOrigin(t, h, slug)
			return h, []string{localTip, originTip}
		}},
		{"LocalTipContainedInOriginArchivesOriginTip", func(t *testing.T, slug string) (*hubforge.Hub, []string) {
			h, _ := discardHub(t, slug)
			gitkit.MustRun(t, h.PrimeWorktree(), "git", "push", "origin", slug)
			return h, []string{advanceOrigin(t, h, slug)}
		}},
		{"OriginTipContainedInLocalArchivesLocalTip", func(t *testing.T, slug string) (*hubforge.Hub, []string) {
			h, localTip := discardHub(t, slug)
			return h, []string{localTip}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			const slug = "discard"
			h, wantTipped := tt.setup(t, slug)

			res, err := h.Topology.DiscardTaskBranch(h.Location, slug)
			if err != nil {
				t.Fatalf("DiscardTaskBranch: %v", err)
			}
			if !res.LocalDeleted || !res.OriginDeleted {
				t.Errorf("LocalDeleted = %v, OriginDeleted = %v; want both true", res.LocalDeleted, res.OriginDeleted)
			}
			if gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
				t.Errorf("task branch %q still local", slug)
			}
			if gitkit.BranchExists(t, h.CodeBare, slug) {
				t.Errorf("task branch %q still on origin", slug)
			}
			var wantTags []string
			for _, tip := range wantTipped {
				wantTags = append(wantTags, fabricengine.ArchiveTagPrefix+slug+"/"+tip[:12])
			}
			slices.Sort(wantTags)
			gotTags := slices.Clone(res.ArchiveTags)
			slices.Sort(gotTags)
			if !slices.Equal(gotTags, wantTags) {
				t.Errorf("ArchiveTags = %v; want %v", gotTags, wantTags)
			}
			if onOrigin := archiveTagsOnOrigin(t, h, slug); !slices.Equal(onOrigin, wantTags) {
				t.Errorf("archive tags on origin = %v; want %v", onOrigin, wantTags)
			}
		})
	}

	t.Run("NoBranchAnywhereIsAZeroResult", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

		res, err := h.Topology.DiscardTaskBranch(h.Location, "never-added")
		if err != nil {
			t.Fatalf("DiscardTaskBranch: %v", err)
		}
		if len(res.ArchiveTags) != 0 || res.LocalDeleted || res.OriginDeleted || res.Mutations.Len() != 0 {
			t.Errorf("DiscardTaskBranch = %+v; want a zero result", res)
		}
	})

	t.Run("RefusedArchivePushDeletesNothing", func(t *testing.T) {
		t.Parallel()
		const slug = "discard-hook"
		h, _ := discardHub(t, slug)
		hook := "#!/bin/sh\nwhile read old new ref; do\n  case \"$ref\" in refs/tags/archive/*) echo \"archive tags refused\" >&2; exit 1;; esac\ndone\n"
		if err := os.WriteFile(filepath.Join(h.CodeBare, "hooks", "pre-receive"), []byte(hook), 0o755); err != nil {
			t.Fatalf("install pre-receive hook: %v", err)
		}

		_, err := h.Topology.DiscardTaskBranch(h.Location, slug)
		if !errors.Is(err, fabricengine.ErrArchiveFailed) {
			t.Fatalf("DiscardTaskBranch error = %v; want one wrapping ErrArchiveFailed", err)
		}
		if !gitkit.BranchExists(t, h.PrimeWorktree(), slug) {
			t.Errorf("task branch %q gone locally; want it kept", slug)
		}
		if !gitkit.BranchExists(t, h.CodeBare, slug) {
			t.Errorf("task branch %q gone from origin; want it kept", slug)
		}
	})
}

// TestDeleteArchivedTaskBranch pins the gates of the deletion half: a local tag that does not hold the tip refuses the local delete, the remote gate refuses the local-only archived-tip kind, and a stale lease fails and keeps the origin tip.
func TestDeleteArchivedTaskBranch(t *testing.T) {
	t.Parallel()

	t.Run("LocalTagNotHoldingTheTipRefusesTheLocalDelete", func(t *testing.T) {
		t.Parallel()
		const slug = "dat-local"
		h, tip := discardHub(t, slug)
		base := gitkit.RevParse(t, h.PrimeWorktree(), slug+"~1")
		const tag = "archive/dat-local/base"
		gitkit.MustRun(t, h.PrimeWorktree(), "git", "tag", tag, base)

		localDeleted, _, err := fabricengine.DeleteArchivedTaskBranchForTest(h.PrimeWorktree(), slug, tag, "", "")
		if !RefusedByGate(err, fabricengine.CheckDirtiness) {
			t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
		}
		if localDeleted {
			t.Errorf("localDeleted = true; want false")
		}
		if got := gitkit.RevParse(t, h.PrimeWorktree(), slug); got != tip {
			t.Errorf("task branch = %q; want it kept at %q", got, tip)
		}
	})

	t.Run("RemoteGateRefusesTheArchivedTipKind", func(t *testing.T) {
		t.Parallel()
		const slug = "dat-kind"
		h, _ := discardHub(t, slug)
		originTip := gitkit.RevParse(t, h.CodeBare, slug)

		err := fabricengine.DeleteRemoteWithArchivedTipForTest(h.PrimeWorktree(), slug, "archive/dat-kind/x", originTip)
		if !RefusedByGate(err, fabricengine.CheckDirtiness) {
			t.Fatalf("error = %v; want a CheckDirtiness refusal", err)
		}
		if got := gitkit.RevParse(t, h.CodeBare, slug); got != originTip {
			t.Errorf("origin task branch = %q; want unchanged %q", got, originTip)
		}
	})

	t.Run("StaleLeaseFailsAndKeepsTheOriginTip", func(t *testing.T) {
		t.Parallel()
		const slug = "dat-stale"
		h, _ := discardHub(t, slug)
		staleTip := gitkit.RevParse(t, h.CodeBare, slug)
		advanced := advanceOrigin(t, h, slug)

		_, originDeleted, err := fabricengine.DeleteArchivedTaskBranchForTest(h.PrimeWorktree(), slug, "", "archive/dat-stale/x", staleTip)
		if err == nil {
			t.Fatal("error = nil; want a stale-lease failure")
		}
		if !strings.Contains(err.Error(), slug) {
			t.Errorf("error %q does not name the branch %q", err, slug)
		}
		if originDeleted {
			t.Errorf("originDeleted = true; want false")
		}
		if got := gitkit.RevParse(t, h.CodeBare, slug); got != advanced {
			t.Errorf("origin task branch = %q; want the advanced tip %q", got, advanced)
		}
	})
}
