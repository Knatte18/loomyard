// pairpush_onerepository_integration_test.go covers a one-repository pair's push, hand-over and settling.
// The hub is built by hubforge.NewOneRepoHub, so each pair's one branch carries its records and its code.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/takeoverkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// oneRepoPair is a pair of a one-repository hub whose branch was pushed once by Add.
type oneRepoPair struct {
	hub      *hubforge.Hub
	location *lymcwd.Location
	slug     string
	path     string
	branch   string
}

// newOneRepoPair adds the pair slug to a fresh one-repository hub.
func newOneRepoPair(t *testing.T, slug string) oneRepoPair {
	t.Helper()

	h := hubforge.NewOneRepoHub(t, ".")
	added := hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	location, err := lymcwd.ResolveWorktree(added.Path)
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", added.Path, err)
	}
	return oneRepoPair{hub: h, location: location, slug: slug, path: added.Path, branch: added.Branch}
}

// commitTransition commits one records file on the pair's branch as a transition of the run.
// The commit's Warp-SHA trailer names the commit the branch stood at before it.
func (p oneRepoPair) commitTransition(t *testing.T, name string) string {
	t.Helper()

	over := gitkit.RevParse(t, p.path, "HEAD")
	message := "transition " + name + "\n\n" + fabricengine.TransitionTrailerKey + ": " + p.slug + "\n" + fabricengine.WarpSHATrailerKey + ": " + over
	return gitkit.CommitFile(t, p.path, "_lym/"+name+".txt", name+"\n", message)
}

// push runs the pair's synchronous push as the machine here names, with no wait between retries.
func (p oneRepoPair) push(opts fabricengine.SyncOptions, here fabricengine.MachineNames) (fabricengine.PushResult, error) {
	opts.BackoffSleep = func(time.Duration) {}
	return fabricengine.PushPairAnchored(p.location, opts, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded, Here: here})
}

// plantTakeover pushes a commit holding the takeover entry of the run to origin's branch from a second clone.
// It returns the new origin tip.
func (p oneRepoPair) plantTakeover(t *testing.T, entry shedengine.Takeover) string {
	t.Helper()

	_, statusRel, err := shedrun.TreeRunFileRels(p.slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", p.slug, err)
	}
	other := cloneBareForTest(t, p.hub.CodeBare, p.branch)
	statusPath := filepath.Join(other, statusRel)
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", statusPath, err)
	}
	if err := os.WriteFile(statusPath, []byte(`{"state":"running"}`), 0o644); err != nil {
		t.Fatalf("write the status: %v", err)
	}
	if err := takeoverkit.WriteTakeover(statusPath, entry); err != nil {
		t.Fatalf("WriteTakeover: %v", err)
	}
	gitkit.Git(t, other, "add", "--", statusRel)
	gitkit.Git(t, other, "commit", "-m", "takeover of "+p.slug)
	gitkit.Git(t, other, "push", "origin", p.branch)
	return gitkit.RevParse(t, other, "HEAD")
}

// TestPairPush_OneRepository covers the push, hand-over and code-tip settling of a one-branch pair.
func TestPairPush_OneRepository(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}

	t.Run("PushesTheBranchOnce", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "push-once")
		tip := p.commitTransition(t, "first")

		res, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{})

		if err != nil {
			t.Fatalf("PushPairAnchored() error = %v; want nil", err)
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.branch); got != tip {
			t.Errorf("origin branch = %s; want the local tip %s", got, tip)
		}
		if got := countBranchPushed(res, "side=records repo="); got != 1 {
			t.Errorf("PushPairAnchored() record = %+v; want exactly one branch push as the records side, got %d", res.Mutated().Entries(), got)
		}
		if got := countBranchPushed(res, "side=code repo="); got != 0 {
			t.Errorf("PushPairAnchored() record = %+v; want no push as the code side, got %d", res.Mutated().Entries(), got)
		}
	})

	t.Run("RefusedPushLeavesOneOwedRecord", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "refused")
		p.commitTransition(t, "first")
		gitkit.Git(t, p.path, "remote", "set-url", "origin", refusedOriginURL)

		_, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{})

		if !fabricengine.IsPushOwed(err) {
			t.Fatalf("PushPairAnchored() error = %v; want an owed push", err)
		}
		record := requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabricengine.PushFailureTransport)
		if record.CodeWorktree != p.path || record.Branch != p.branch {
			t.Errorf("record = %+v; want the pair's worktree %s and branch %s", record, p.path, p.branch)
		}
		if want := "`lym fabric push` in " + p.path; !strings.Contains(fabricengine.FailedPushWayForward(record), want) {
			t.Errorf("way forward = %q; want it to name %q", fabricengine.FailedPushWayForward(record), want)
		}
		owed, err := fabricengine.FailedPushesOwedBy(p.hub.Location.HubPath, p.slug)
		if err != nil || len(owed) != 1 {
			t.Errorf("FailedPushesOwedBy() = %+v, %v; want the one record", owed, err)
		}
	})

	t.Run("TakeoverKeepsEveryTransitionOnOrigin", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "taken-over")
		first := p.commitTransition(t, "first")
		if _, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{}); err != nil {
			t.Fatalf("first push error = %v; want nil", err)
		}
		takeover := p.plantTakeover(t, entry)
		p.commitTransition(t, "second")

		_, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{Current: "neve"})

		var takeoverErr *fabricengine.TakeoverError
		if !errors.As(err, &takeoverErr) || takeoverErr.Slug != p.slug || takeoverErr.Entry != entry {
			t.Fatalf("push error = %v; want a *TakeoverError of %q naming %+v", err, p.slug, entry)
		}
		res, err := fabricengine.HandOverTips(p.hub.Location, fabricengine.HandOverRequest{Slug: p.slug, Pair: p.location, Entry: entry})
		if err != nil || res.TipPushed {
			t.Fatalf("HandOverTips() = %+v, %v; want no tip pushed and no error", res, err)
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.branch); got != takeover {
			t.Errorf("origin branch = %s; want the takeover commit %s left alone", got, takeover)
		}
		gitkit.Git(t, p.hub.CodeBare, "merge-base", "--is-ancestor", first, p.branch)
	})

	t.Run("NoCodeTipIsOwed", func(t *testing.T) {
		t.Parallel()
		p := newOneRepoPair(t, "no-owed-tip")
		first := p.commitTransition(t, "first")
		if _, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{}); err != nil {
			t.Fatalf("first push error = %v; want nil", err)
		}
		second := p.commitTransition(t, "second")
		if _, err := p.push(fabricengine.SyncOptions{}, fabricengine.MachineNames{}); err != nil {
			t.Fatalf("second push error = %v; want nil", err)
		}
		third := p.commitTransition(t, "third")

		reading, err := fabricengine.OriginCodeTip(p.hub.Location, p.slug)
		if err != nil || reading.Missing || reading.Tip != first {
			t.Fatalf("OriginCodeTip() = %+v, %v; want the tip %s held by origin", reading, err, first)
		}
		returned, err := fabricengine.ReturnToPreviousTransition(p.location, p.slug)
		if err != nil || returned {
			t.Errorf("ReturnToPreviousTransition() = %v, %v; want false, nil", returned, err)
		}
		if got := gitkit.RevParse(t, p.path, "HEAD"); got != third {
			t.Errorf("local tip = %s; want it left at %s, with nothing committed", got, third)
		}
		res, err := fabricengine.HandOverTips(p.hub.Location, fabricengine.HandOverRequest{Slug: p.slug, Pair: p.location, Entry: entry})
		if err != nil || res.TipPushed {
			t.Fatalf("HandOverTips() = %+v, %v; want no tip pushed and no error", res, err)
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.branch); got != second {
			t.Errorf("origin branch = %s; want it left at %s", got, second)
		}
	})
}
