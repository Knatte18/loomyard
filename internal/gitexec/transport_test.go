// transport_test.go covers IsTransportFailure on hand-built *GitError values; it spawns no git.
// Its local-path rows use a temporary directory.

package gitexec_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestIsTransportFailure(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	phrases := []string{
		"fatal: unable to access 'x': Could not resolve host: example.com",
		"ssh: connect to host h port 22: Connection refused",
		"fatal: read error: Connection reset by peer",
		"ssh: connect to host h port 22: Connection timed out",
		"fatal: unable to access 'x': Operation timed out",
		"connect: Network is unreachable",
		"fatal: unable to access 'x': Failed to connect to h port 443",
		"fatal: unable to access 'x': Operation too slow. Less than 1000 bytes/sec transferred the last 60 seconds",
		"fatal: unable to access 'x': HTTP/2 stream 1 was closed cleanly, but before getting all response header fields",
		"error: RPC failed; curl 92 HTTP/2 stream 5 was not closed cleanly: INTERNAL_ERROR (err 2)",
		"fatal: unable to access 'x': HTTP/2 stream 1 was reset",
		"error: RPC failed; curl 56 Recv failure: Connection was aborted",
		"fatal: unable to access 'x': Send failure: Broken pipe",
		"fatal: unable to access 'x': Empty reply from server",
		"fetch-pack: unexpected disconnect while reading sideband packet",
		"fatal: early EOF",
		"fatal: unable to access 'x': gnutls_handshake() failed: Error in the pull function.",
		"error: RPC failed; curl 56 GnuTLS recv error (-110): The TLS connection was non-properly terminated.",
		"fatal: unable to access 'x': The requested URL returned error: 502",
		"fatal: unable to access 'x': The requested URL returned error: 503",
		"fatal: unable to access 'x': The requested URL returned error: 504",
	}
	for _, p := range phrases {
		for _, sub := range []string{"push", "fetch", "ls-remote"} {
			err := &gitexec.GitError{Args: []string{sub, "origin"}, ExitCode: 128, Stderr: p}
			if !gitexec.IsTransportFailure(err) {
				t.Errorf("%s %q: want true", sub, p)
			}
			if !gitexec.IsTransportFailure(fmt.Errorf("wrapped: %w", err)) {
				t.Errorf("wrapped %s %q: want true", sub, p)
			}
		}
	}

	existing := t.TempDir()
	missing := filepath.Join(existing, "removed.git")
	dangling := filepath.Join(existing, "dangling.git")
	if err := os.Symlink(missing, dangling); err != nil {
		t.Fatal(err)
	}
	apostrophe := filepath.Join(existing, "it's", "plain.git")
	if err := os.MkdirAll(apostrophe, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"push to a missing local path", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "fatal: '" + missing + "' does not appear to be a git repository\nfatal: Could not read from remote repository."}, true},
		{"ls-remote of a missing file URL", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, Stderr: "fatal: 'file://" + missing + "' does not appear to be a git repository"}, true},
		{"fetch of a missing relative path", &gitexec.GitError{Args: []string{"fetch", "origin"}, Dir: existing, Stderr: "fatal: './removed.git' does not appear to be a git repository"}, true},
		{"push to a link whose target is missing", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "fatal: '" + dangling + "' does not appear to be a git repository"}, true},
		{"push to an unconfigured remote", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "fatal: 'origin' does not appear to be a git repository"}, false},
		{"ls-remote of a file URL that holds no repository", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, Stderr: "fatal: 'file://" + existing + "' does not appear to be a git repository"}, false},
		{"fetch of an existing relative path", &gitexec.GitError{Args: []string{"fetch", "origin"}, Dir: filepath.Dir(existing), Stderr: "fatal: './" + filepath.Base(existing) + "' does not appear to be a git repository"}, false},
		{"push to a path holding an apostrophe", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "fatal: '" + apostrophe + "' does not appear to be a git repository"}, false},
		{"clone of a missing local path", &gitexec.GitError{Args: []string{"clone", missing}, Stderr: "fatal: repository '" + missing + "' does not exist"}, true},
		{"push to a directory that holds no repository", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "fatal: '" + existing + "' does not appear to be a git repository"}, false},
		{"local command naming a missing path", &gitexec.GitError{Args: []string{"status"}, Stderr: "fatal: '" + missing + "' does not appear to be a git repository"}, false},
		{"autoSetupRemote push", &gitexec.GitError{Args: []string{"-c", "push.autoSetupRemote=true", "push", "origin", "HEAD"}, Stderr: "fatal: Failed to connect to h"}, true},
		{"-C fetch", &gitexec.GitError{Args: []string{"-C", "/some/dir", "fetch"}, Stderr: "fatal: Could not resolve host: h"}, true},
		{"ls-remote", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, Stderr: "Connection refused"}, true},
		{"clone", &gitexec.GitError{Args: []string{"clone", "u"}, Stderr: "Connection refused"}, true},
		{"non-ff push", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "! [rejected] (non-fast-forward)"}, false},
		{"non-ff push with -c", &gitexec.GitError{Args: []string{"-c", "push.autoSetupRemote=true", "push"}, Stderr: "! [rejected] (non-fast-forward)"}, false},
		{"auth push", &gitexec.GitError{Args: []string{"push"}, Stderr: "fatal: Authentication failed"}, false},
		{"missing ref fetch", &gitexec.GitError{Args: []string{"fetch", "origin", "nope"}, Stderr: "fatal: couldn't find remote ref nope"}, false},
		{"forbidden fetch", &gitexec.GitError{Args: []string{"fetch", "origin"}, Stderr: "fatal: unable to access 'x': The requested URL returned error: 403"}, false},
		{"missing repository ls-remote", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, Stderr: "fatal: unable to access 'x': The requested URL returned error: 404"}, false},
		{"too-large push with a sideband disconnect", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "error: RPC failed; HTTP 413 curl 22 The requested URL returned error: 413\nsend-pack: unexpected disconnect while reading sideband packet"}, false},
		{"too-large push with a send failure", &gitexec.GitError{Args: []string{"push", "origin"}, Stderr: "error: RPC failed; HTTP 413\nfatal: unable to access 'x': Send failure: Broken pipe"}, false},
		{"rate-limited fetch with a receive failure", &gitexec.GitError{Args: []string{"fetch", "origin"}, Stderr: "fatal: unable to access 'x': The requested URL returned error: 429\nerror: RPC failed; curl 56 Recv failure"}, true},
		{"timed-out request with a disconnect", &gitexec.GitError{Args: []string{"fetch", "origin"}, Stderr: "error: RPC failed; HTTP 408\nfetch-pack: unexpected disconnect while reading sideband packet"}, true},
		{"pull with phrase", &gitexec.GitError{Args: []string{"pull"}, Stderr: "Connection refused"}, true},
		{"timed-out remote command without a phrase", &gitexec.GitError{Args: []string{"ls-remote", "origin"}, ExitCode: -1, Timeout: time.Minute}, true},
		{"timed-out local command", &gitexec.GitError{Args: []string{"status"}, ExitCode: -1, Timeout: time.Minute}, false},
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
