// pushretry_test.go pins what Add's push seam adds to gitrepo's retry: the push it runs and the sleep it hands over, and the outcome mapping of a coalesced push, offline against a fake runner and a recording sleep.

package fabricengine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

func pushRejection(stderr string) error {
	return &gitexec.GitError{Args: []string{"push"}, ExitCode: 1, Stderr: stderr}
}

func TestPushBranchWithRetry_RetriesOnlyTransientRefusalsUpToTheBound(t *testing.T) {
	t.Parallel()

	failed := pushRejection(" ! [remote rejected] feat -> feat (failed)\n")
	for _, tc := range []struct {
		name         string
		err          error
		wantExhaust  bool
		wantSleeps   []time.Duration
		wantPushRuns int
	}{
		{"success", nil, false, nil, 1},
		{"persistent refusal exhausts gitrepo's retry", failed, true, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var pushes int
			var sleeps []time.Duration
			seam := pushSeam{
				run: func(args []string, cwd string) (string, error) {
					if !reflect.DeepEqual(args, []string{"push", "-u", "origin", "feat"}) || cwd != "dir" {
						t.Errorf("push ran %v in %q", args, cwd)
					}
					pushes++
					return "", tc.err
				},
				sleep: func(delay time.Duration) { sleeps = append(sleeps, delay) },
			}

			err := seam.pushBranchWithRetry("dir", "feat")

			if pushes != tc.wantPushRuns {
				t.Errorf("pushes = %d; want %d", pushes, tc.wantPushRuns)
			}
			if !reflect.DeepEqual(sleeps, tc.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", sleeps, tc.wantSleeps)
			}
			if errors.Is(err, gitrepo.ErrPushRetriesExhausted) != tc.wantExhaust {
				t.Errorf("err = %v; ErrPushRetriesExhausted match want %v", err, tc.wantExhaust)
			}
		})
	}
}

func TestUnpushedOutcome_WarnsAndLeavesCommitsForARejectedOrExhaustedPush(t *testing.T) {
	t.Parallel()

	other := errors.New("boom")
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"success", nil, nil},
		{"diverged remote", gitrepo.ErrPushRejected, nil},
		{"exhausted retry", errors.Join(gitrepo.ErrPushRetriesExhausted, other), nil},
		{"any other error", other, other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := unpushedOutcome("path", tc.err); got != tc.want {
				t.Errorf("unpushedOutcome = %v; want %v", got, tc.want)
			}
		})
	}
}
