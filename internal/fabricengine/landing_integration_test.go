// landing_integration_test.go covers the git steps on a branch runs landed on, over hubforge hubs with bare origins.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landOn commits a landing of the run in the checkout at dir and returns the commit.
func landOn(t *testing.T, dir, run string) string {
	t.Helper()
	return gitkit.CommitFile(t, dir, run+".txt", run+"\n", "land "+run+"\n\n"+fabriclayout.LandingTrailerKey+": "+run)
}

// hubWithPushedPrimeCode returns a hub whose prime code branch is on origin with an upstream, and that branch.
func hubWithPushedPrimeCode(t *testing.T) (*hubforge.Hub, string) {
	t.Helper()
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	gitkit.Git(t, h.PrimeWorktree(), "push", "-q", "-u", "origin", "HEAD")
	return h, gitkit.CurrentBranch(t, h.PrimeWorktree())
}

func TestPushPrimeCodeTip(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, branch := hubWithPushedPrimeCode(t)
	originTip := func() string { return fabricengine.BareBranchSHAForTest(t, h.CodeBare, branch) }
	opts := fabricengine.SyncOptions{OwedUnrecorded: true, BackoffSleep: func(time.Duration) {}}

	tip := landOn(t, h.PrimeWorktree(), "run-a")
	gitkit.CommitFile(t, h.PrimeWorktree(), "local.txt", "local\n", "a commit no landing recorded")
	if err := fabricengine.PushPrimeCodeTip(h.Location, tip, opts); err != nil {
		t.Fatalf("PushPrimeCodeTip() = %v; want nil", err)
	}
	if got := originTip(); got != tip {
		t.Errorf("origin tip = %s; want the landing tip %s and nothing past it", got, tip)
	}

	next := landOn(t, h.PrimeWorktree(), "run-b")
	originURL := gitkit.Git(t, h.PrimeWorktree(), "config", "--get", "remote.origin.url")
	// git refuses this URL with `connection refused`, which reads as a transport failure.
	gitkit.Git(t, h.PrimeWorktree(), "remote", "set-url", "origin", "http://127.0.0.1:1/x")
	if err := fabricengine.PushPrimeCodeTip(h.Location, next, opts); !fabricengine.IsPushOwed(err) {
		t.Fatalf("PushPrimeCodeTip() over an unreachable origin = %v; want an owed push", err)
	}
	requireNoFailedPush(t, h.Path)
	gitkit.Git(t, h.PrimeWorktree(), "remote", "set-url", "origin", strings.TrimSpace(originURL))

	other := cloneBareForTest(t, h.CodeBare, branch)
	gitkit.CommitFile(t, other, "other.txt", "other\n", "another clone's commit")
	gitkit.MustRun(t, other, "git", "push")
	err := fabricengine.PushPrimeCodeTip(h.Location, next, opts)
	var failed *fabricengine.PushFailedError
	if !errors.As(err, &failed) || failed.Record.Class != fabriclayout.PushFailureNotFastForward || fabricengine.IsPushOwed(err) {
		t.Fatalf("PushPrimeCodeTip() against a moved origin = %v; want a refused not-fast-forward push", err)
	}
	wayForward := fabricengine.FailedPushWayForward(failed.Record)
	for _, want := range []string{next, "git merge origin/" + branch, "`git push` in " + h.PrimeWorktree()} {
		if !strings.Contains(wayForward, want) {
			t.Errorf("way forward = %q; want it to hold %q", wayForward, want)
		}
	}
	requireOneFailedPush(t, h.Path, fabriclayout.RepoCode, fabriclayout.PushFailureNotFastForward)
	if stands, err := fabricengine.PrimeCodeRefusalStands(h.Location); err != nil || !stands {
		t.Errorf("PrimeCodeRefusalStands() = %v, %v; want true", stands, err)
	}
	if err := fabricengine.ClearPrimeCodeRefusal(h.Location); err != nil {
		t.Fatalf("ClearPrimeCodeRefusal() = %v; want nil", err)
	}
	if stands, err := fabricengine.PrimeCodeRefusalStands(h.Location); err != nil || stands {
		t.Errorf("PrimeCodeRefusalStands() after the clear = %v, %v; want false", stands, err)
	}

	// A protected branch refuses every push, so the record names the pull request and not a retry.
	protected, protectedBranch := hubWithPushedPrimeCode(t)
	installPreReceive(t, protected.CodeBare, protectedRefusingHook(protectedBranch))
	err = fabricengine.PushPrimeCodeTip(protected.Location, landOn(t, protected.PrimeWorktree(), "run-c"), opts)
	if !errors.As(err, &failed) {
		t.Fatalf("PushPrimeCodeTip() onto a protected branch = %v; want a recorded failure", err)
	}
	wayForward = fabricengine.FailedPushWayForward(failed.Record)
	for _, want := range []string{"beam.pull_request", "trail", "pull request by hand", "git reset --keep origin/" + protectedBranch} {
		if !strings.Contains(wayForward, want) {
			t.Errorf("way forward = %q; want it to hold %q", wayForward, want)
		}
	}
}

func TestParentAheadReads(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, branch := hubWithPushedPrimeCode(t)
	prime := h.PrimeWorktree()
	level := gitkit.RevParse(t, prime, "HEAD")

	first := landOn(t, prime, "run-a")
	if only, err := fabricengine.AheadByOnly(h.Location, []string{first}); err != nil || !only {
		t.Errorf("AheadByOnly() over one owed tip = %v, %v; want true", only, err)
	}
	plain := gitkit.CommitFile(t, prime, "plain.txt", "plain\n", "a plain commit")
	second := landOn(t, prime, "run-b")

	landings, err := fabricengine.LandingCommits(h.Location)
	if want := map[string]string{"run-a": first, "run-b": second}; err != nil || !maps.Equal(landings, want) {
		t.Errorf("LandingCommits() = %v, %v; want %v", landings, err, want)
	}
	if only, err := fabricengine.AheadByOnly(h.Location, []string{first, second}); err != nil || only {
		t.Errorf("AheadByOnly() with the unrecorded commit %s = %v, %v; want false", plain, only, err)
	}

	if err := fabricengine.ResetParentToOrigin(h.Location); err != nil {
		t.Fatalf("ResetParentToOrigin() = %v; want nil", err)
	}
	if got := gitkit.RevParse(t, prime, "refs/heads/"+branch); got != level {
		t.Errorf("local branch = %s; want origin's tip %s", got, level)
	}

	// A tracked change on a path the reset rewrites, and a branch with no upstream, each refuse with nothing moved.
	ahead := gitkit.CommitFile(t, prime, "edit.txt", "landed\n", "a commit to drop")
	if err := os.WriteFile(filepath.Join(prime, "edit.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("edit edit.txt: %v", err)
	}
	if err := fabricengine.ResetParentToOrigin(h.Location); err == nil || gitkit.RevParse(t, prime, "HEAD") != ahead {
		t.Errorf("ResetParentToOrigin() over a dirty touched path = %v; want a refusal with HEAD at %s", err, ahead)
	}
	gitkit.Git(t, prime, "checkout", "--", "edit.txt")
	gitkit.Git(t, prime, "branch", "--unset-upstream")
	if err := fabricengine.ResetParentToOrigin(h.Location); err == nil || gitkit.RevParse(t, prime, "HEAD") != ahead {
		t.Errorf("ResetParentToOrigin() with no upstream = %v; want a refusal with HEAD at %s", err, ahead)
	}
}
