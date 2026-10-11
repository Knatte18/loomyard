// pushretry_test.go pins the retryable-refusal classifier and the bounded push retry loop, offline against a fake push and a recording sleep.

package gitrepo

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

func TestRetryablePushRefusal_ClassifiesByTheReasonOnThePushedRefsLine(t *testing.T) {
	t.Parallel()

	const remoteNoise = "remote: some server noise\n"
	for _, tc := range []struct {
		name string
		err  error
		ref  string
		want bool
	}{
		{"bare failed", pushRejection(" ! [remote rejected] feat -> feat (failed)\n"), "feat", true},
		{"bare failed with remote lines around it", pushRejection(remoteNoise + " ! [remote rejected] feat -> feat (failed)\n" + remoteNoise + "error: failed to push some refs\n"), "feat", true},
		{"failed in full branch form", pushRejection(" ! [remote rejected] refs/heads/feat -> refs/heads/feat (failed)\n"), "feat", true},
		{"failed in full tag form", pushRejection(" ! [remote rejected] refs/tags/v1 -> refs/tags/v1 (failed)\n"), "v1", true},
		{"remote cannot lock ref with a spaced reason", pushRejection(" ! [remote rejected] feat -> feat (cannot lock ref 'refs/heads/feat': is at abc but expected def)\n"), "feat", true},
		{"remote cannot lock ref in full form", pushRejection(" ! [remote rejected] refs/heads/feat -> refs/heads/feat (cannot lock ref 'refs/heads/feat': reference already exists)\n"), "feat", true},
		{"failed for another ref", pushRejection(" ! [remote rejected] other -> other (failed)\n"), "feat", false},
		{"cannot lock ref for another ref", pushRejection(" ! [remote rejected] other -> other (cannot lock ref 'refs/heads/other': x)\n"), "feat", false},
		{"local cannot lock ref", pushRejection("error: cannot lock ref 'refs/remotes/origin/feat': unable to resolve reference\n"), "feat", false},
		{"hook declined", pushRejection(" ! [remote rejected] feat -> feat (pre-receive hook declined)\n"), "feat", false},
		{"non-fast-forward", pushRejection(" ! [rejected]        feat -> feat (non-fast-forward)\n"), "feat", false},
		{"empty ref matches nothing", pushRejection(" ! [remote rejected] feat -> feat (failed)\n"), "", false},
		{"not a git error", errors.New("exec: git not found"), "feat", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			line, got := retryablePushRefusal(tc.err, tc.ref)
			if got != tc.want {
				t.Errorf("retryable = %v; want %v", got, tc.want)
			}
			if got && line == "" {
				t.Errorf("a retryable refusal returned no stderr line")
			}
		})
	}
}

func TestPushWithRetry_RetriesOnlyRetryableRefusalsUpToTheBound(t *testing.T) {
	t.Parallel()

	failed := pushRejection(" ! [remote rejected] feat -> feat (failed)\n")
	cannotLock := pushRejection(" ! [remote rejected] feat -> feat (cannot lock ref 'refs/heads/feat': busy)\n")
	notFastForward := pushRejection(" ! [rejected] feat -> feat (non-fast-forward)\n")
	for _, tc := range []struct {
		name          string
		results       []error
		wantErr       error
		wantExhausted bool
		wantPushes    int
		wantSleeps    []time.Duration
	}{
		{"success first try", []error{nil}, nil, false, 1, nil},
		{"refusal then success", []error{failed, nil}, nil, false, 2, []time.Duration{time.Second}},
		{"cannot lock ref then success", []error{cannotLock, nil}, nil, false, 2, []time.Duration{time.Second}},
		{"persistent refusal stops at the bound", []error{failed, failed, failed, failed, failed}, failed, true, pushMaxAttempts, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
		{"non-retryable error is attempted once and returned unchanged", []error{notFastForward, nil}, notFastForward, false, 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var pushes int
			var sleeps []time.Duration
			push := func() error {
				err := tc.results[pushes]
				pushes++
				return err
			}

			err := PushWithRetry(push, "feat", func(delay time.Duration) { sleeps = append(sleeps, delay) })

			if pushes != tc.wantPushes {
				t.Errorf("pushes = %d; want %d", pushes, tc.wantPushes)
			}
			if !reflect.DeepEqual(sleeps, tc.wantSleeps) {
				t.Errorf("sleeps = %v; want %v", sleeps, tc.wantSleeps)
			}
			if errors.Is(err, ErrPushRetriesExhausted) != tc.wantExhausted {
				t.Errorf("err = %v; ErrPushRetriesExhausted match want %v", err, tc.wantExhausted)
			}
			if errors.Is(err, ErrPushRejected) {
				t.Errorf("err = %v; must never match ErrPushRejected", err)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("err = %v; want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v; want it to wrap the last push error %v", err, tc.wantErr)
			}
		})
	}
}
