// errorrender_test.go covers GitError.Error's rendering rules: the
// trailing-stderr segment's presence and trimming, and per-arg quoting.
// It spawns no git, so it carries no //go:build tag.

package gitexec_test

import (
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
