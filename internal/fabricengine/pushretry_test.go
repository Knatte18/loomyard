// pushretry_test.go pins the transient-refusal classifier and the bounded retry loop of Add's branch pushes, offline against a fake runner and a recording sleep.

package fabricengine

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func pushRejection(stderr string) error {
	return &gitexec.GitError{Args: []string{"push"}, ExitCode: 1, Stderr: stderr}
}

func TestTransientPushRefusal_ClassifiesByTheReasonOnThePushedRefsLine(t *testing.T) {
	t.Parallel()

	const failedLine = " ! [remote rejected] feat -> feat (failed)\n"
	const remoteNoise = "remote: some server noise\n"
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"bare failed", pushRejection(failedLine), true},
		{"bare failed with remote lines around it", pushRejection(remoteNoise + failedLine + remoteNoise + "error: failed to push some refs\n"), true},
		{"full ref names", pushRejection(" ! [remote rejected] refs/heads/feat -> refs/heads/feat (failed)\n"), true},
		{"hook declined", pushRejection(" ! [remote rejected] feat -> feat (pre-receive hook declined)\n"), false},
		{"hook declined with remote lines", pushRejection(remoteNoise + " ! [remote rejected] feat -> feat (pre-receive hook declined)\n"), false},
		{"non-fast-forward", pushRejection(" ! [rejected]        feat -> feat (non-fast-forward)\n"), false},
		{"other reason", pushRejection(" ! [remote rejected] feat -> feat (unpacker error)\n"), false},
		{"failed for a different ref", pushRejection(" ! [remote rejected] other -> other (failed)\n"), false},
		{"not a git error", errors.New("exec: git not found"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			line, got := transientPushRefusal(tc.err, "feat")
			if got != tc.want {
				t.Errorf("transient = %v; want %v", got, tc.want)
			}
			if got && line == "" {
				t.Errorf("a transient refusal returned no stderr line")
			}
		})
	}
}

func TestPushBranchWithRetry_RetriesOnlyTransientRefusalsUpToTheBound(t *testing.T) {
	t.Parallel()

	failed := pushRejection(" ! [remote rejected] feat -> feat (failed)\n")
	notFastForward := pushRejection(" ! [rejected] feat -> feat (non-fast-forward)\n")
	hook := pushRejection(" ! [remote rejected] feat -> feat (pre-receive hook declined)\n")
	for _, tc := range []struct {
		name       string
		results    []error
		wantErr    error
		wantPushes int
		wantSleeps []time.Duration
	}{
		{"success first try", []error{nil}, nil, 1, nil},
		{"refusal then success", []error{failed, nil}, nil, 2, []time.Duration{time.Second}},
		{"persistent failed stops at the bound", []error{failed, failed, failed, failed, failed}, failed, 4, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
		{"hook declined is attempted once", []error{hook, nil}, hook, 1, nil},
		{"non-failed rejection is attempted once", []error{notFastForward, nil}, notFastForward, 1, nil},
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
					err := tc.results[pushes]
					pushes++
					return "", err
				},
				sleep: func(delay time.Duration) { sleeps = append(sleeps, delay) },
			}

			err := seam.pushBranchWithRetry("dir", "feat")

			if pushes != tc.wantPushes {
				t.Errorf("pushes = %d; want %d", pushes, tc.wantPushes)
			}
			if !reflect.DeepEqual(sleeps, tc.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", sleeps, tc.wantSleeps)
			}
			if err != tc.wantErr {
				t.Errorf("err = %v; want the last push error %v", err, tc.wantErr)
			}
		})
	}
}
