//go:build tier1

// startretry_test.go covers the exec core's retry of a git the host could not start for EAGAIN.

package gitexec_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/proc"
)

// TestRun_StartRefusedForEAGAIN pins the start retry: a start refused for EAGAIN is tried again up to four times in all, while a git that ran and exited nonzero, and any other start failure, is started once.
// A refusal is noted in proc's spawn-refusal record.
// It replaces the package's start seam, which is process-global state, so neither the test nor its rows run in parallel.
func TestRun_StartRefusedForEAGAIN(t *testing.T) {
	refused := &os.PathError{Op: "fork/exec", Path: "/usr/bin/git", Err: syscall.EAGAIN}
	missing := &os.PathError{Op: "fork/exec", Path: "/usr/bin/git", Err: syscall.ENOENT}
	dir := t.TempDir()
	const firstDelay = 100 * time.Millisecond

	tests := []struct {
		name       string
		args       []string
		failures   []error
		wantStarts int
		wantWaits  []time.Duration
		wantStdout string
		wantErr    error
		wantGitErr bool
	}{
		{name: "RefusedThenStarted", args: []string{"--version"}, failures: []error{refused, refused}, wantStarts: 3, wantWaits: []time.Duration{firstDelay, 2 * firstDelay}, wantStdout: "git version"},
		{name: "RefusedEveryTry", args: []string{"--version"}, failures: []error{refused, refused, refused, refused, refused}, wantStarts: 4, wantWaits: []time.Duration{firstDelay, 2 * firstDelay, 4 * firstDelay}, wantErr: syscall.EAGAIN},
		{name: "RemoteRefusedEveryTry", args: []string{"ls-remote", dir}, failures: []error{refused, refused, refused, refused, refused}, wantStarts: 4, wantWaits: []time.Duration{firstDelay, 2 * firstDelay, 4 * firstDelay}, wantErr: syscall.EAGAIN},
		{name: "OtherStartFailure", args: []string{"--version"}, failures: []error{missing}, wantStarts: 1, wantErr: syscall.ENOENT},
		{name: "GitExitsNonzero", args: []string{"rev-parse", "--verify", "no-such-ref"}, wantStarts: 1, wantGitErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			starts, waits := gitexec.SetStartFailuresForTest(t, firstDelay, tt.failures...)
			stdout, err := gitexec.Run(tt.args, dir)
			if got := starts(); got != tt.wantStarts {
				t.Errorf("Run(%v) started git %d times; want %d", tt.args, got, tt.wantStarts)
			}
			if got := waits(); !slices.Equal(got, tt.wantWaits) {
				t.Errorf("Run(%v) waited %v between starts; want %v", tt.args, got, tt.wantWaits)
			}
			if !strings.HasPrefix(stdout, tt.wantStdout) {
				t.Errorf("Run(%v) stdout = %q; want prefix %q", tt.args, stdout, tt.wantStdout)
			}
			var gitErr *gitexec.GitError
			switch {
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Errorf("Run(%v) error = %v; want one wrapping %v", tt.args, err, tt.wantErr)
			case tt.wantErr != nil && errors.As(err, &gitErr):
				t.Errorf("Run(%v) returned a *GitError for a git that never ran: %v", tt.args, err)
			case tt.wantGitErr && !errors.As(err, &gitErr):
				t.Errorf("Run(%v) error = %v; want a *GitError", tt.args, err)
			case tt.wantErr == nil && !tt.wantGitErr && err != nil:
				t.Errorf("Run(%v) error = %v; want nil", tt.args, err)
			}
		})
	}

	if _, noted := proc.LastSpawnAgain(); !noted {
		t.Errorf("proc.LastSpawnAgain() noted nothing after refused starts")
	}
}
