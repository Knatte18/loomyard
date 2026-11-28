// landingchain_integration_test.go covers the batch-landing verbs of landingchain.go over a hub with three task pairs.
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// chainHub is a hub whose parent pair is the prime, with the task pairs one, two and three off it.
type chainHub struct {
	hub    *hubforge.Hub
	parent *fabricengine.Fabric
	// base is the parent branch's tip, which every task branch forked from.
	base     string
	branches map[string]string
}

func newChainHub(t *testing.T) *chainHub {
	t.Helper()
	h := &chainHub{hub: hubforge.CopyHub(t, hubforge.Shape{Anchor: "."}), branches: map[string]string{}}
	for _, slug := range []string{"one", "two", "three"} {
		h.branches[slug] = hubforge.AddPair(t, h.hub, slug).Branch
	}
	h.parent = hubforge.OpenFabric(t, h.hub)
	h.base = gitkit.RevParse(t, h.hub.PrimeWorktree(), "HEAD")
	return h
}

// commit commits content to file on the task branch of slug and returns the commit.
func (h *chainHub) commit(t *testing.T, slug, file, content string) string {
	t.Helper()
	return gitkit.CommitFile(t, h.hub.PairCodeWorktree(slug), file, content, slug+" work")
}

func (h *chainHub) link(slug, head string, trueMerge bool) fabricengine.ChainLink {
	return fabricengine.ChainLink{
		Slug:      slug,
		TaskHead:  head,
		Message:   "land " + slug + "\n\nLym-Landing: " + slug,
		TrueMerge: trueMerge,
	}
}

// log returns format's rendering of commit in the parent's repository.
func (h *chainHub) log(t *testing.T, commit, format string) string {
	t.Helper()
	return gitkit.Git(t, h.hub.PrimeWorktree(), "log", "-1", "--format="+format, commit)
}

func TestBuildLandingChain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, tt := range []struct {
		name        string
		middleMerge bool
	}{
		{"ThreeSquashes", false},
		{"TheMiddleOneTrueMerges", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newChainHub(t)
			var links []fabricengine.ChainLink
			var heads []string
			for _, slug := range []string{"one", "two", "three"} {
				heads = append(heads, h.commit(t, slug, slug+".txt", slug+"\n"))
				links = append(links, h.link(slug, heads[len(heads)-1], tt.middleMerge && slug == "two"))
			}

			result, err := h.parent.BuildLandingChain(h.base, links)
			if err != nil {
				t.Fatalf("BuildLandingChain() = %v; want nil", err)
			}
			if len(result.Commits) != 3 || len(result.Conflicted) != 0 || len(result.Held) != 0 {
				t.Fatalf("BuildLandingChain() = %+v; want three commits and nothing left out", result)
			}
			if got := gitkit.RevParse(t, h.hub.PrimeWorktree(), "HEAD"); got != h.base {
				t.Errorf("parent HEAD = %s; want unmoved %s", got, h.base)
			}

			// The reference trees come from merging the branches in turn by plain git.
			gitkit.Git(t, h.hub.PrimeWorktree(), "checkout", "-b", "reference", h.base)
			previous := h.base
			for i, link := range links {
				gitkit.Git(t, h.hub.PrimeWorktree(), "merge", "--no-ff", "-m", "reference", heads[i])
				commit := result.Commits[i]
				if commit.Slug != link.Slug {
					t.Errorf("commit %d slug = %q; want %q", i, commit.Slug, link.Slug)
				}
				wantParents := previous
				if link.TrueMerge {
					wantParents += " " + heads[i]
				}
				if got := h.log(t, commit.Commit, "%P"); got != wantParents {
					t.Errorf("commit %d parents = %q; want %q", i, got, wantParents)
				}
				if got, want := h.log(t, commit.Commit, "%B"), "land "+link.Slug+"\n\nLym-Landing: "+link.Slug; got != want {
					t.Errorf("commit %d message = %q; want %q", i, got, want)
				}
				if got, want := h.log(t, commit.Commit, "%T"), h.log(t, "HEAD", "%T"); got != want {
					t.Errorf("commit %d tree = %s; want the plain merge's %s", i, got, want)
				}
				previous = commit.Commit
			}
		})
	}

	t.Run("AConflictingLinkIsLeftOut", func(t *testing.T) {
		t.Parallel()
		h := newChainHub(t)
		links := []fabricengine.ChainLink{
			h.link("one", h.commit(t, "one", "shared.txt", "one\n"), false),
			h.link("two", h.commit(t, "two", "shared.txt", "two\n"), false),
			h.link("three", h.commit(t, "three", "three.txt", "three\n"), false),
		}

		result, err := h.parent.BuildLandingChain(h.base, links)
		if err != nil {
			t.Fatalf("BuildLandingChain() = %v; want nil", err)
		}
		if !slices.Equal(result.Conflicted, []string{"two"}) || len(result.Commits) != 2 {
			t.Fatalf("BuildLandingChain() = %+v; want two conflicted and two commits", result)
		}
		if got, want := h.log(t, result.Commits[1].Commit, "%P"), result.Commits[0].Commit; got != want {
			t.Errorf("third commit parent = %s; want the first commit %s", got, want)
		}
	})

	t.Run("ALinkTheBaseHoldsAddsNoCommit", func(t *testing.T) {
		t.Parallel()
		h := newChainHub(t)

		result, err := h.parent.BuildLandingChain(h.base, []fabricengine.ChainLink{h.link("one", h.base, false)})

		if err != nil || !slices.Equal(result.Held, []string{"one"}) || len(result.Commits) != 0 {
			t.Errorf("BuildLandingChain() = %+v, %v; want one held and no commit", result, err)
		}
	})

	t.Run("ATaskHeadThatNamesNoCommitIsAnError", func(t *testing.T) {
		t.Parallel()
		h := newChainHub(t)
		links := []fabricengine.ChainLink{
			h.link("one", h.commit(t, "one", "one.txt", "one\n"), false),
			h.link("two", "no-such-head", false),
		}

		result, err := h.parent.BuildLandingChain(h.base, links)

		if err == nil || !strings.Contains(err.Error(), "two") || len(result.Commits) != 0 {
			t.Errorf("BuildLandingChain() = %+v, %v; want no commit and an error naming two", result, err)
		}
	})
}

func TestMergesClean(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newChainHub(t)
	parentBranch := gitkit.CurrentBranch(t, h.hub.PrimeWorktree())
	gitkit.CommitFile(t, h.hub.PrimeWorktree(), "shared.txt", "parent\n", "parent work")
	h.commit(t, "one", "one.txt", "one\n")
	h.commit(t, "two", "shared.txt", "two\n")

	tests := []struct {
		name    string
		source  string
		want    bool
		wantErr bool
	}{
		{"CleanPair", h.branches["one"], true, false},
		{"ConflictingBranch", h.branches["two"], false, false},
		{"UnresolvedRef", "no-such-branch", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := h.parent.MergesClean(parentBranch, tt.source)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("MergesClean(%s, %s) = %v, %v; want %v, error %v", parentBranch, tt.source, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestMoveCodeTo(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// arrange changes the parent checkout and returns the tip to move to, built on base.
		arrange   func(t *testing.T, h *chainHub, tip string) string
		wantMoved bool
		wantErr   bool
		// wantGuard judges a refusal's typed error.
		wantGuard func(*fabricengine.MergeGuardError) bool
	}{
		{name: "HeadAtBaseMoves", wantMoved: true},
		{name: "HeadPastBaseStays", arrange: func(t *testing.T, h *chainHub, tip string) string {
			gitkit.CommitFile(t, h.hub.PrimeWorktree(), "later.txt", "later\n", "later parent work")
			return tip
		}},
		{name: "ATipNotDescendingFromBaseErrors", wantErr: true, arrange: func(t *testing.T, h *chainHub, _ string) string {
			root := gitkit.Git(t, h.hub.PrimeWorktree(), "rev-list", "--max-parents=0", h.base)
			return gitkit.Git(t, h.hub.PrimeWorktree(), "commit-tree", "-m", "unrelated", root+"^{tree}")
		}},
		{name: "TrackedDirtIsRefused", wantErr: true,
			wantGuard: (*fabricengine.MergeGuardError).WorktreeDirty,
			arrange: func(t *testing.T, h *chainHub, tip string) string {
				if err := os.WriteFile(filepath.Join(h.hub.PrimeWorktree(), "README"), []byte("dirty\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return tip
			}},
		{name: "ADetachedHeadIsRefused", wantErr: true,
			wantGuard: (*fabricengine.MergeGuardError).DetachedHead,
			arrange: func(t *testing.T, h *chainHub, tip string) string {
				gitkit.Git(t, h.hub.PrimeWorktree(), "checkout", "--detach")
				return tip
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newChainHub(t)
			result, err := h.parent.BuildLandingChain(h.base, []fabricengine.ChainLink{
				h.link("one", h.commit(t, "one", "one.txt", "one\n"), false),
			})
			if err != nil || len(result.Commits) != 1 {
				t.Fatalf("BuildLandingChain() = %+v, %v; want one commit", result, err)
			}
			tip := result.Commits[0].Commit
			if tt.arrange != nil {
				tip = tt.arrange(t, h, tip)
			}
			head := gitkit.RevParse(t, h.hub.PrimeWorktree(), "HEAD")

			moved, err := h.parent.MoveCodeTo(h.base, tip)

			if moved != tt.wantMoved || (err != nil) != tt.wantErr {
				t.Errorf("MoveCodeTo() = %v, %v; want %v, error %v", moved, err, tt.wantMoved, tt.wantErr)
			}
			if tt.wantGuard != nil {
				var guard *fabricengine.MergeGuardError
				if !errors.As(err, &guard) || !tt.wantGuard(guard) {
					t.Errorf("MoveCodeTo() error = %v; want a *MergeGuardError of this reason", err)
				}
			}
			wantHead := head
			if tt.wantMoved {
				wantHead = tip
			}
			if got := gitkit.RevParse(t, h.hub.PrimeWorktree(), "HEAD"); got != wantHead {
				t.Errorf("parent HEAD = %s; want %s", got, wantHead)
			}
		})
	}
}

func TestNewestLandingOf(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := newChainHub(t)
	prime := h.hub.PrimeWorktree()
	parentBranch := gitkit.CurrentBranch(t, prime)
	gitkit.CommitFile(t, prime, "first.txt", "first\n", "first\n\nLym-Landing: one")
	newest := gitkit.CommitFile(t, prime, "second.txt", "second\n", "second\n\nLym-Landing: one")
	gitkit.CommitFile(t, prime, "later.txt", "later\n", "later work")
	gitkit.Git(t, prime, "checkout", "-q", "-b", "side")
	gitkit.CommitFile(t, prime, "side.txt", "side\n", "side\n\nLym-Landing: three")
	gitkit.Git(t, prime, "checkout", "-q", parentBranch)
	gitkit.Git(t, prime, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	gitkit.Git(t, prime, "push", "origin", parentBranch)

	tests := []struct {
		name      string
		slug      string
		want      string
		wantFound bool
	}{
		{"TheNewestOfTwoPastOrigin", "one", newest, true},
		{"ASlugNoTrailerNames", "two", "", false},
		{"ASlugOnlyASecondParentsCommitNames", "three", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found, err := fabricengine.NewestLandingOf(h.hub.Location, parentBranch, tt.slug)
			if err != nil || got != tt.want || found != tt.wantFound {
				t.Errorf("NewestLandingOf(%s) = %q, %v, %v; want %q, %v, nil", tt.slug, got, found, err, tt.want, tt.wantFound)
			}
		})
	}
}
