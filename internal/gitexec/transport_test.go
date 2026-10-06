// transport_test.go covers IsTransportFailure on hand-built *GitError values; it spawns no git.

package gitexec_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func TestIsTransportFailure(t *testing.T) {
	t.Parallel()

	phrases := []string{
		"fatal: unable to access 'x': Could not resolve host: example.com",
		"ssh: connect to host h port 22: Connection refused",
		"fatal: read error: Connection reset by peer",
		"ssh: connect to host h port 22: Connection timed out",
		"fatal: unable to access 'x': Operation timed out",
		"connect: Network is unreachable",
		"fatal: unable to access 'x': Failed to connect to h port 443",
	}
	for _, p := range phrases {
		for _, sub := range []string{"push", "fetch"} {
			err := &gitexec.GitError{Args: []string{sub, "origin"}, ExitCode: 128, Stderr: p}
			if !gitexec.IsTransportFailure(err) {
				t.Errorf("%s %q: want true", sub, p)
			}
			if !gitexec.IsTransportFailure(fmt.Errorf("wrapped: %w", err)) {
				t.Errorf("wrapped %s %q: want true", sub, p)
			}
		}
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"autoSetupRemote push", &gitexec.GitError{Args: []string{"-c", "push.autoSetupRemote=true", "push", "origin", "HEAD"}, Stderr: "fatal: Failed to connect to h"}, true},
		{"-C fetch", &gitexec.GitError{Args: []string{"-C", "/some/dir", "fetch"}, Stderr: "fatal: Could not resolve host: h"}, true},
		{"ls-remote", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, Stderr: "Connection refused"}, true},
		{"clone", &gitexec.GitError{Args: []string{"clone", "u"}, Stderr: "Connection refused"}, true},
		{"non-ff push", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "! [rejected] (non-fast-forward)"}, false},
		{"non-ff push with -c", &gitexec.GitError{Args: []string{"-c", "push.autoSetupRemote=true", "push"}, Stderr: "! [rejected] (non-fast-forward)"}, false},
		{"auth push", &gitexec.GitError{Args: []string{"push"}, Stderr: "fatal: Authentication failed"}, false},
		{"missing ref fetch", &gitexec.GitError{Args: []string{"fetch", "origin", "nope"}, Stderr: "fatal: couldn't find remote ref nope"}, false},
		{"pull with phrase", &gitexec.GitError{Args: []string{"pull"}, Stderr: "Connection refused"}, false},
		{"status with phrase", &gitexec.GitError{Args: []string{"status"}, Stderr: "Connection refused"}, false},
		{"plain error", errors.New("Connection refused"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := gitexec.IsTransportFailure(c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
