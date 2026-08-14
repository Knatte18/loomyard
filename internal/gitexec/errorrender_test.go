// errorrender_test.go covers GitError.Error's rendering rules: the
// trailing-stderr segment's presence and trimming, and per-arg quoting.
// It spawns no git, so it carries no //go:build tag.

package gitexec_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

func TestGitError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *gitexec.GitError
		want string
	}{
		{
			name: "StderrPresent",
			err:  &gitexec.GitError{Args: []string{"status"}, ExitCode: 1, Stderr: "fatal: not a git repository"},
			want: `git status: exit 1: fatal: not a git repository`,
		},
		{
			name: "StderrEmpty",
			err:  &gitexec.GitError{Args: []string{"status"}, ExitCode: 1, Stderr: ""},
			want: `git status: exit 1`,
		},
		{
			name: "StderrWhitespaceOnly",
			err:  &gitexec.GitError{Args: []string{"status"}, ExitCode: 1, Stderr: "   \n\t  "},
			want: `git status: exit 1`,
		},
		{
			name: "StderrSurroundingWhitespaceTrimmed",
			err:  &gitexec.GitError{Args: []string{"status"}, ExitCode: 1, Stderr: "  fatal: boom  \n"},
			want: `git status: exit 1: fatal: boom`,
		},
		{
			name: "ArgWithSpaceQuoted",
			err:  &gitexec.GitError{Args: []string{"commit", "-m", "a message"}, ExitCode: 1, Stderr: ""},
			want: `git commit -m "a message": exit 1`,
		},
		{
			name: "EmptyArgQuoted",
			err:  &gitexec.GitError{Args: []string{"commit", "-m", ""}, ExitCode: 1, Stderr: ""},
			want: `git commit -m "": exit 1`,
		},
		{
			name: "OrdinaryArgUnquoted",
			err:  &gitexec.GitError{Args: []string{"rev-parse", "HEAD"}, ExitCode: 128, Stderr: ""},
			want: `git rev-parse HEAD: exit 128`,
		},
		{
			name: "MixedVector",
			err:  &gitexec.GitError{Args: []string{"commit", "-m", "a message", "--amend"}, ExitCode: 1, Stderr: "  fatal: boom  "},
			want: `git commit -m "a message" --amend: exit 1: fatal: boom`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("GitError.Error() = %q; want %q", got, tt.want)
			}
		})
	}
}

// TestGitError_ErrorOmitsDir pins the deliberate omission GitError's doc comment records: Dir is
// carried for a caller that wants to name the directory itself, and is never rendered by Error.
// The omission is load-bearing rather than an oversight — nearly every fabricengine and gitrepo
// wrapper already names the repo or worktree it was operating on, so rendering Dir here would put
// the same path twice into the part of the message an operator reads first.
// Without this test a future "make the error more informative" change would fold Dir into Error and
// silently reintroduce that duplication at every one of those wrapping call sites at once.
func TestGitError_ErrorOmitsDir(t *testing.T) {
	const dir = "/tmp/some/worktree/path"
	err := &gitexec.GitError{
		Args:     []string{"status", "--porcelain"},
		Dir:      dir,
		ExitCode: 128,
		Stderr:   "fatal: not a git repository",
	}

	got := err.Error()
	if strings.Contains(got, dir) {
		t.Errorf("GitError.Error() = %q; it must not render Dir (%q) — see GitError's doc comment for why the caller's own wrapper owns the \"where\"", got, dir)
	}

	const want = `git status --porcelain: exit 128: fatal: not a git repository`
	if got != want {
		t.Errorf("GitError.Error() = %q; want %q", got, want)
	}
}
