// pushretry_test.go pins the retryable-refusal classifier and the bounded push retry loop.
// It runs offline, against a fake push and a recording sleep.

package gitrepo

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func pushRejection(stderr string) error {
	return &gitexec.GitError{Args: []string{"push"}, ExitCode: 1, Stderr: stderr}
}

func TestRetryablePushRefusal_ClassifiesByTheReasonOnThePushedRefsLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const remoteNoise = "remote: some server noise\n"
	archiveTagLine := pushRejection(" ! [remote rejected] refs/tags/archive/s/abc -> refs/tags/archive/s/abc (missing necessary objects)\n")
	missingObjects := []string{"missing necessary objects"}
	for _, tc := range []struct {
		name  string
		err   error
		ref   string
		extra []string
		want  bool
	}{
		{"bare failed", pushRejection(" ! [remote rejected] feat -> feat (failed)\n"), "feat", nil, true},
		{"bare failed with remote lines around it", pushRejection(remoteNoise + " ! [remote rejected] feat -> feat (failed)\n" + remoteNoise + "error: failed to push some refs\n"), "feat", nil, true},
		{"failed in full branch form", pushRejection(" ! [remote rejected] refs/heads/feat -> refs/heads/feat (failed)\n"), "feat", nil, true},
		{"failed in full tag form", pushRejection(" ! [remote rejected] refs/tags/v1 -> refs/tags/v1 (failed)\n"), "v1", nil, true},
		{"failed in short form for a full tag ref", pushRejection(" ! [remote rejected] archive/s/abc -> archive/s/abc (failed)\n"), "refs/tags/archive/s/abc", nil, true},
		{"failed in short form for a full branch ref", pushRejection(" ! [remote rejected] feat -> feat (failed)\n"), "refs/heads/feat", nil, true},
		{"failed in full form for a full tag ref", pushRejection(" ! [remote rejected] refs/tags/v1 -> refs/tags/v1 (failed)\n"), "refs/tags/v1", nil, true},
		{"failed for a branch of a full tag ref's name", pushRejection(" ! [remote rejected] refs/heads/v1 -> refs/heads/v1 (failed)\n"), "refs/tags/v1", nil, false},
		{"remote cannot lock ref with a spaced reason", pushRejection(" ! [remote rejected] feat -> feat (cannot lock ref 'refs/heads/feat': is at abc but expected def)\n"), "feat", nil, true},
		{"remote cannot lock ref in full form", pushRejection(" ! [remote rejected] refs/heads/feat -> refs/heads/feat (cannot lock ref 'refs/heads/feat': reference already exists)\n"), "feat", nil, true},
		{"failed for another ref", pushRejection(" ! [remote rejected] other -> other (failed)\n"), "feat", nil, false},
		{"cannot lock ref for another ref", pushRejection(" ! [remote rejected] other -> other (cannot lock ref 'refs/heads/other': x)\n"), "feat", nil, false},
		{"local cannot lock ref", pushRejection("error: cannot lock ref 'refs/remotes/origin/feat': unable to resolve reference\n"), "feat", nil, false},
		{"hook declined", pushRejection(" ! [remote rejected] feat -> feat (pre-receive hook declined)\n"), "feat", nil, false},
		{"non-fast-forward", pushRejection(" ! [rejected]        feat -> feat (non-fast-forward)\n"), "feat", nil, false},
		{"empty ref matches nothing", pushRejection(" ! [remote rejected] feat -> feat (failed)\n"), "", nil, false},
		{"not a git error", errors.New("exec: git not found"), "feat", nil, false},
		{"an extra reason on the pushed ref's line", archiveTagLine, "refs/tags/archive/s/abc", missingObjects, true},
		{"an extra reason not passed", archiveTagLine, "refs/tags/archive/s/abc", nil, false},
		{"an extra reason on another ref's line", archiveTagLine, "refs/tags/archive/other/abc", missingObjects, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			line, got := retryablePushRefusal(tc.err, tc.ref, tc.extra)
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
	tiergate.Require(t, 0)
	t.Parallel()

	failed := pushRejection(" ! [remote rejected] feat -> feat (failed)\n")
	cannotLock := pushRejection(" ! [remote rejected] feat -> feat (cannot lock ref 'refs/heads/feat': busy)\n")
	notFastForward := pushRejection(" ! [rejected] feat -> feat (non-fast-forward)\n")
	archiveTagLine := pushRejection(" ! [remote rejected] refs/tags/archive/s/abc -> refs/tags/archive/s/abc (missing necessary objects)\n")
	missingObjects := []string{"missing necessary objects"}
	customDelays := []time.Duration{3 * time.Second, 5 * time.Second}
	for _, tc := range []struct {
		name          string
		results       []error
		delays        []time.Duration
		wantErr       error
		wantExhausted bool
		wantPushes    int
		wantSleeps    []time.Duration
		ref           string
		extra         []string
		plain         bool
	}{
		{"success first try", []error{nil}, PushBackoffDelays(), nil, false, 1, nil, "feat", nil, true},
		{"refusal then success", []error{failed, nil}, PushBackoffDelays(), nil, false, 2, []time.Duration{time.Second}, "feat", nil, true},
		{"cannot lock ref then success", []error{cannotLock, nil}, PushBackoffDelays(), nil, false, 2, []time.Duration{time.Second}, "feat", nil, true},
		{"persistent refusal stops at the bound", []error{failed, failed, failed, failed, failed}, PushBackoffDelays(), failed, true, pushMaxAttempts, PushBackoffDelays(), "feat", nil, true},
		{"persistent refusal sleeps the caller's delays", []error{failed, failed, failed, failed}, customDelays, failed, true, len(customDelays) + 1, customDelays, "feat", nil, true},
		{"empty delays make one attempt", []error{failed, failed}, nil, failed, true, 1, nil, "feat", nil, true},
		{"non-retryable error is attempted once and returned unchanged", []error{notFastForward, nil}, PushBackoffDelays(), notFastForward, false, 1, nil, "feat", nil, true},
		{"a caller-named reason is retried once then succeeds", []error{archiveTagLine, nil}, PushBackoffDelays(), nil, false, 2, []time.Duration{time.Second}, "refs/tags/archive/s/abc", missingObjects, false},
		{"a caller-named reason is not retried by the plain retry", []error{archiveTagLine, nil}, PushBackoffDelays(), archiveTagLine, false, 1, nil, "refs/tags/archive/s/abc", nil, true},
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

			recordSleep := func(delay time.Duration) { sleeps = append(sleeps, delay) }
			var err error
			if tc.plain {
				err = PushWithRetry(push, tc.ref, tc.delays, recordSleep)
			} else {
				err = PushWithRetryReasons(push, tc.ref, tc.delays, recordSleep, tc.extra)
			}

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
