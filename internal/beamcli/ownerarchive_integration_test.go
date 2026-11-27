// ownerarchive_integration_test.go pins which pairs the owner archive covers and when `lym beam start` skips it.
// It runs over hubforge hubs whose pairs hold tips origin lacks.

package beamcli

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// archiveOwnerTagsOnCodeOrigin lists the owner archive tags on the hub's code origin, sorted.
func archiveOwnerTagsOnCodeOrigin(t *testing.T, h *hubforge.Hub) []string {
	t.Helper()

	out := gitkit.Git(t, h.CodeBare, "tag", "--list", fabricengine.OwnerArchiveTagPrefix+"*")
	if out == "" {
		return nil
	}
	tags := strings.Fields(out)
	slices.Sort(tags)
	return tags
}

func TestArchiveOwnerTips_CoversTheWorktreesOwnPairAndTheRunsUnderIt(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// worktree is the archived worktree; empty names the prime.
		worktree string
		// taggedSlugs are the pairs whose tips the archive tags.
		taggedSlugs []string
	}{
		{name: "ThePrimeArchivesItsOwnPairNamedRootAndEachRunUnderIt", taggedSlugs: []string{"r", "root"}},
		{name: "ABeamArchivesOnlyItsOwnPair", worktree: "b", taggedSlugs: []string{"b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
			hubforge.AddPairWith(t, h, "r", fabricengine.AddOptions{SkipPush: true})
			prime := h.Location
			seedBattenRun(t, prime, "r")
			tips := map[string]string{
				"root": gitkit.CommitFile(t, h.PrimeWorktree(), "owner-archive.txt", "local", "local code commit"),
				"b":    gitkit.Git(t, h.PairCodeWorktree("b"), "rev-parse", "HEAD"),
				"r":    gitkit.Git(t, h.PairCodeWorktree("r"), "rev-parse", "HEAD"),
			}
			var want []string
			for _, slug := range tt.taggedSlugs {
				want = append(want, fabricengine.OwnerArchiveTagName(slug, tips[slug]))
			}
			slices.Sort(want)
			worktree := tt.worktree
			if worktree == "" {
				worktree = prime.WorktreeName
			}

			if err := archiveOwnerTips(prime, worktree); err != nil {
				t.Fatalf("archiveOwnerTips() = %v; want nil", err)
			}

			if got := archiveOwnerTagsOnCodeOrigin(t, h); !slices.Equal(got, want) {
				t.Errorf("owner archive tags on the code origin = %v; want %v", got, want)
			}
		})
	}
}

func TestOwnerArchiveBeforeRefusal_SkipsWhileAnAdoptCoversTheWorktree(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.Location
	tip := gitkit.CommitFile(t, h.PrimeWorktree(), "owner-archive.txt", "local", "local code commit")
	archive := ownerArchiveBeforeRefusal(primeTarget(prime))

	if err := adoptrecord.Write(h.Path, adoptrecord.Record{State: adoptrecord.StateRunning, Subjects: []string{"root"}}); err != nil {
		t.Fatalf("adoptrecord.Write() = %v; want nil", err)
	}
	if err := archive(); err != nil {
		t.Fatalf("archive() under an adopt = %v; want nil", err)
	}
	if got := archiveOwnerTagsOnCodeOrigin(t, h); len(got) != 0 {
		t.Errorf("owner archive tags under an adopt = %v; want none", got)
	}

	if err := adoptrecord.Write(h.Path, adoptrecord.Record{State: adoptrecord.StateDone, Subjects: []string{"root"}}); err != nil {
		t.Fatalf("adoptrecord.Write() = %v; want nil", err)
	}
	if err := archive(); err != nil {
		t.Fatalf("archive() with the adopt done = %v; want nil", err)
	}
	if got, want := archiveOwnerTagsOnCodeOrigin(t, h), []string{fabricengine.OwnerArchiveTagName("root", tip)}; !slices.Equal(got, want) {
		t.Errorf("owner archive tags with the adopt done = %v; want %v", got, want)
	}
}
