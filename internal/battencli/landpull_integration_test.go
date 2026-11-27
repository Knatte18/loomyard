// landpull_integration_test.go drives the Land row's prime pull over a real hub whose prime branch holds a landing.

package battencli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLandDeps_PullWithAnOwedLandingPush asserts the prime pull leaves an owed landing tip in place.
// An unrecorded commit below the tip is judged as on any ahead branch.
// An unreachable origin is a git transport failure whether or not the landing was pushed.
func TestLandDeps_PullWithAnOwedLandingPush(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// unrecordedBelow commits one unrecorded commit below the landing tip.
		unrecordedBelow bool
		// landed pushes the tip and drops its ledger entry.
		landed bool
		// originMoved puts a second clone's commit on origin, so the prime is diverged.
		originMoved bool
		unreachable bool
		// wantRefusal is a part of the refusal the pull answers with; empty with no wantClass means it passes.
		wantRefusal string
		wantClass   shedengine.TransientClass
	}{
		{name: "OwedTipReadsLevel"},
		{name: "LandedTipPulls", landed: true},
		{name: "OriginMovedPastAnOwedTipIsDiverged", originMoved: true, wantRefusal: "remote diverged and local"},
		{name: "UnrecordedCommitBelowIsAnAheadBranch", unrecordedBelow: true, wantRefusal: "has unpushed commits; aborting, no changes"},
		{name: "UnreachableWithThePushOwed", unreachable: true, wantClass: shedengine.TransientGitTransport},
		{name: "UnreachableAfterItLanded", landed: true, unreachable: true, wantClass: shedengine.TransientGitTransport},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			prime := h.PrimeWorktree()
			if tt.unrecordedBelow {
				gitkit.CommitFile(t, prime, "below.txt", "below\n", "unrecorded commit")
			}
			tip := gitkit.CommitFile(t, prime, "landed.txt", "landed\n", "landing")
			if tt.landed {
				gitkit.Git(t, prime, "push")
			} else {
				err := pushledger.Update(h.Location.HubPath, pushledger.LandingKey, time.Unix(1, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
					return pushledger.Entry{Landings: []pushledger.Landing{{Run: "a-run", Tip: tip, ParentBranch: "main", At: time.Unix(1, 0)}}}, true, nil
				})
				if err != nil {
					t.Fatalf("pushledger.Update() = %v; want nil", err)
				}
			}
			if tt.originMoved {
				clone := filepath.Join(t.TempDir(), "clone")
				gitkit.MustRun(t, filepath.Dir(clone), "git", "clone", "-q", "-b", gitkit.CurrentBranch(t, prime), h.CodeBare, clone)
				gitkit.Git(t, clone, "config", "user.email", "test@test.com")
				gitkit.Git(t, clone, "config", "user.name", "Test")
				gitkit.CommitFile(t, clone, "origin.txt", "origin\n", "another clone's commit")
				gitkit.Git(t, clone, "push", "-q")
			}
			if tt.unreachable {
				gitkit.Git(t, prime, "remote", "set-url", "origin", "http://127.0.0.1:1/x")
			}

			_, err := landDeps(h.Location, "a-run", nil).Pull(context.Background())

			switch {
			case tt.wantRefusal != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantRefusal) {
					t.Errorf("Pull() error = %v; want one containing %q", err, tt.wantRefusal)
				}
			case tt.wantClass == "" && err != nil:
				t.Errorf("Pull() error = %v; want nil", err)
			}
			if got := shedengine.TransientOf(err); got != tt.wantClass {
				t.Errorf("TransientOf(Pull() error %v) = %q; want %q", err, got, tt.wantClass)
			}
			if got := gitkit.RevParse(t, prime, "HEAD"); got != tip {
				t.Errorf("prime HEAD = %s; want the landing tip %s", got, tip)
			}
		})
	}
}
