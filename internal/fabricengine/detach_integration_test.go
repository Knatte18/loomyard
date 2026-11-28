// detach_integration_test.go covers the detached checkout of a task pair: its restore, its checkouts,
// and the reads and the push that judge a marked worktree by its task branch.
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
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// markDetach writes the detach marker of l naming branch.
func markDetach(t *testing.T, l *lymcwd.Location, branch string) {
	t.Helper()
	if err := fabricengine.WriteDetachMarker(l, branch); err != nil {
		t.Fatalf("WriteDetachMarker() = %v; want nil", err)
	}
}

// writeUntracked writes content to the worktree-relative file rel of dir without staging it.
func writeUntracked(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// requireHead fails the test unless the checkout at dir is on branch, or detached when branch is empty.
func requireHead(t *testing.T, dir, branch string) {
	t.Helper()
	got := gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		branch = "HEAD"
	}
	if got != branch {
		t.Errorf("HEAD of %s = %s; want %s", dir, got, branch)
	}
}

func requireMarker(t *testing.T, l *lymcwd.Location, stands bool) {
	t.Helper()
	if _, got, err := fabricengine.ReadDetachMarker(l); err != nil || got != stands {
		t.Errorf("ReadDetachMarker() stands = %v, %v; want %v, nil", got, err, stands)
	}
}

func TestRestoreDetached(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name  string
		check func(t *testing.T, p *pairFixture)
	}{
		{"CleanAtAnEarlierCommitReturnsToTheBranch", func(t *testing.T, p *pairFixture) {
			earlier := gitkit.RevParse(t, p.warpPath, "HEAD")
			tip := gitkit.CommitFile(t, p.warpPath, "work.txt", "work\n", "work")
			gitkit.Git(t, p.warpPath, "checkout", "--detach", earlier)
			markDetach(t, p.location, p.warpBranch)

			state, err := fabricengine.RestoreDetached(p.location)

			if err != nil || !state.Restored || state.Commit != earlier {
				t.Errorf("RestoreDetached() = %+v, %v; want restored from %s", state, err, earlier)
			}
			requireHead(t, p.warpPath, p.warpBranch)
			if got := gitkit.RevParse(t, p.warpPath, "HEAD"); got != tip {
				t.Errorf("branch tip = %s; want unchanged %s", got, tip)
			}
			requireMarker(t, p.location, false)
		}},
		{"IgnoredOutputInADeletedDirectoryIsRemoved", func(t *testing.T, p *pairFixture) {
			gitkit.CommitFile(t, p.warpPath, "tool/.gitignore", "out.bin\n", "ignore build output")
			withTool := gitkit.CommitFile(t, p.warpPath, "tool/main.txt", "main\n", "tool")
			gitkit.Git(t, p.warpPath, "rm", "-r", "tool")
			gitkit.Git(t, p.warpPath, "commit", "-m", "drop tool")
			gitkit.Git(t, p.warpPath, "checkout", "--detach", withTool)
			writeUntracked(t, p.warpPath, "tool/out.bin", "built\n")
			markDetach(t, p.location, p.warpBranch)

			state, err := fabricengine.RestoreDetached(p.location)

			if err != nil || !state.Restored {
				t.Fatalf("RestoreDetached() = %+v, %v; want restored", state, err)
			}
			if got := gitkit.Git(t, p.warpPath, "status", "--porcelain"); got != "" {
				t.Errorf("status after restore = %q; want clean", got)
			}
		}},
		{"DirtyStaysDetachedWithItsMarker", func(t *testing.T, p *pairFixture) {
			gitkit.Git(t, p.warpPath, "checkout", "--detach")
			writeUntracked(t, p.warpPath, "stray.txt", "stray\n")
			markDetach(t, p.location, p.warpBranch)

			state, err := fabricengine.RestoreDetached(p.location)

			if err != nil || state.Restored || !slices.Equal(state.Dirty, []string{"stray.txt"}) {
				t.Errorf("RestoreDetached() = %+v, %v; want the dirty path stray.txt, not restored", state, err)
			}
			requireHead(t, p.warpPath, "")
			requireMarker(t, p.location, true)
		}},
		{"DetachedWithoutAMarkerStaysDetached", func(t *testing.T, p *pairFixture) {
			head := gitkit.RevParse(t, p.warpPath, "HEAD")
			gitkit.Git(t, p.warpPath, "checkout", "--detach")

			state, err := fabricengine.RestoreDetached(p.location)

			if err != nil || state.Restored || !state.Detached || state.Commit != head {
				t.Errorf("RestoreDetached() = %+v, %v; want detached at %s, not restored", state, err, head)
			}
			requireHead(t, p.warpPath, "")
		}},
		{"OnItsBranchLosesAStaleMarker", func(t *testing.T, p *pairFixture) {
			markDetach(t, p.location, p.warpBranch)

			if state, err := fabricengine.RestoreDetached(p.location); err != nil || state.Restored {
				t.Errorf("RestoreDetached() = %+v, %v; want nothing restored", state, err)
			}
			requireMarker(t, p.location, false)
		}},
		{"AMarkerNamingADeletedBranchErrors", func(t *testing.T, p *pairFixture) {
			gitkit.Git(t, p.warpPath, "checkout", "--detach")
			markDetach(t, p.location, "gone-branch")

			_, err := fabricengine.RestoreDetached(p.location)

			marker := fabricengine.DetachMarkerPath(p.location)
			if err == nil || !strings.Contains(err.Error(), marker) || !strings.Contains(err.Error(), "lym loom resume") {
				t.Errorf("RestoreDetached() = %v; want an error naming %s and lym loom resume", err, marker)
			}
			requireHead(t, p.warpPath, "")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.check(t, newPairFixture(t, "restore"))
		})
	}
}

func TestCheckoutDetached(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	openPair := func(t *testing.T, p *pairFixture) *fabricengine.Fabric {
		t.Helper()
		f, err := fabricengine.Open(p.location)
		if err != nil {
			t.Fatalf("Open() = %v; want nil", err)
		}
		return f
	}

	t.Run("ACleanCheckoutDetaches", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, "cleandetach")
		earlier := gitkit.RevParse(t, p.warpPath, "HEAD")
		gitkit.CommitFile(t, p.warpPath, "work.txt", "work\n", "work")

		if err := openPair(t, p).CheckoutDetached(earlier); err != nil {
			t.Fatalf("CheckoutDetached() = %v; want nil", err)
		}
		requireHead(t, p.warpPath, "")
		if got := gitkit.RevParse(t, p.warpPath, "HEAD"); got != earlier {
			t.Errorf("HEAD = %s; want %s", got, earlier)
		}
	})

	t.Run("ADirtyCheckoutIsRefused", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, "dirtydetach")
		f := openPair(t, p)
		writeUntracked(t, p.warpPath, "stray.txt", "stray\n")

		err := f.CheckoutDetached("HEAD")
		if err == nil || !strings.Contains(err.Error(), "stray.txt") {
			t.Errorf("CheckoutDetached() = %v; want a refusal naming stray.txt", err)
		}
		requireHead(t, p.warpPath, p.warpBranch)

		gitkit.Git(t, p.warpPath, "checkout", "--detach")
		err = f.CheckoutBranch(p.warpBranch)
		if err == nil || !strings.Contains(err.Error(), "stray.txt") {
			t.Errorf("CheckoutBranch() = %v; want a refusal naming stray.txt", err)
		}
		requireHead(t, p.warpPath, "")
	})

	t.Run("AMarkedPairOfASubpathAnchorReadsCleanAndByItsBranch", func(t *testing.T) {
		t.Parallel()
		hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{hubforge.TemplatePairSlug}})
		path := hub.PairCodeWorktree(hubforge.TemplatePairSlug)
		location, err := lymcwd.ResolveWorktree(path)
		if err != nil {
			t.Fatalf("ResolveWorktree() = %v; want nil", err)
		}
		branch := gitkit.CurrentBranch(t, path)
		f, err := fabricengine.Open(location)
		if err != nil {
			t.Fatalf("Open() = %v; want nil", err)
		}
		markDetach(t, location, branch)

		if err := f.CheckoutDetached("HEAD"); err != nil {
			t.Fatalf("CheckoutDetached() = %v; want nil", err)
		}
		if got := gitkit.Git(t, path, "status", "--porcelain"); got != "" {
			t.Errorf("status of the marked checkout = %q; want clean", got)
		}
		gitkit.CommitFile(t, path, "chain.txt", "chain\n", "commit origin lacks")
		if ahead, err := fabricengine.RunCommitsAhead(location); err != nil || ahead {
			t.Errorf("RunCommitsAhead() = %v, %v; want the level task branch, false, nil", ahead, err)
		}
	})
}

func TestParentHoldsTaskWork_ReadsTheBranchWhileDetached(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	p := newPairFixture(t, "holdsdetached")
	held := gitkit.RevParse(t, p.warpPath, "HEAD")
	gitkit.CommitFile(t, p.warpPath, "work.txt", "work\n", "work the parent lacks")
	gitkit.Git(t, p.warpPath, "checkout", "--detach", held)
	markDetach(t, p.location, p.warpBranch)

	got, err := fabricengine.ParentHoldsTaskWork(p.hub.Location, p.location, "holdsdetached")
	if err != nil || got {
		t.Errorf("ParentHoldsTaskWork() = %v, %v; want false, nil", got, err)
	}

	markDetach(t, p.location, "gone-branch")
	_, err = fabricengine.ParentHoldsTaskWork(p.hub.Location, p.location, "holdsdetached")
	if marker := fabricengine.DetachMarkerPath(p.location); err == nil || !strings.Contains(err.Error(), marker) {
		t.Errorf("ParentHoldsTaskWork() = %v; want an error naming %s", err, marker)
	}
}

func TestPushPairAnchored_HoldsOffTheCodeSideOfADetachedPair(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	p := newPairFixture(t, "heldoff")
	before := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
	codeTip := gitkit.CommitFile(t, p.warpPath, "warp.txt", "warp\n", "code")
	recordsTip := gitkit.CommitFile(t, p.weftPath, "records.txt", "records\n", "records")
	gitkit.Git(t, p.warpPath, "checkout", "--detach")
	markDetach(t, p.location, p.warpBranch)
	push := func() error {
		_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(nil), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded})
		return err
	}

	err := push()

	var failed *fabricengine.PushFailedError
	marker := fabricengine.DetachMarkerPath(p.location)
	if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureDeferred || !strings.Contains(err.Error(), marker) {
		t.Errorf("PushPairAnchored() = %v; want a deferred push naming %s", err, marker)
	}
	if !fabricengine.IsPushOwed(err) {
		t.Errorf("IsPushOwed(%v) = false; want true", err)
	}
	if got := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch); got != recordsTip {
		t.Errorf("origin records tip = %s; want pushed %s", got, recordsTip)
	}
	if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != before {
		t.Errorf("origin code tip = %s; want unmoved %s", got, before)
	}

	if err := fabricengine.RemoveDetachMarker(p.location); err != nil {
		t.Fatalf("RemoveDetachMarker() = %v; want nil", err)
	}
	gitkit.Git(t, p.warpPath, "checkout", p.warpBranch)
	if err := push(); err != nil {
		t.Fatalf("PushPairAnchored() after the restore = %v; want nil", err)
	}
	if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != codeTip {
		t.Errorf("origin code tip = %s; want pushed %s", got, codeTip)
	}
}
