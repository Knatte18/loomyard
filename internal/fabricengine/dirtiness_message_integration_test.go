// dirtiness_message_integration_test.go pins the composed shape of worktreeDirty's error against real git.
//
// A non-zero git exit already arrives as a *gitexec.GitError that names the git command,
// so the wrapper must not name it again: the composed error names the command once and keeps git's stderr.
//
// The test drives a real git status against a directory that is not a repository.
// Its assertions check the shape of the message, not a golden string, so the wrapper's wording can change.

package fabricengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWorktreeDirty_ErrorNamesGitCommandOnce reproduces a real non-zero git exit.
// It asserts that worktreeDirty's composed error does not repeat the git command.
// The *gitexec.GitError it wraps already renders that command.
//
//testtiming:keep worktreeDirty's error rendering git's command once rather than twice; coverage of its blocks by other tests does not show an assertion of this
func TestWorktreeDirty_ErrorNamesGitCommandOnce(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// A fresh temp dir is not a git repository,
	// so `git status --porcelain` exits non-zero with a real "not a git repository" stderr.
	// That is the failure the CLI hits when a verb is pointed at a path that is not a checkout.
	notARepo := t.TempDir()

	dirty, _, err := worktreeDirty(scopeAll, notARepo)
	if err == nil {
		t.Fatalf("worktreeDirty(scopeAll, %q) = (%v, nil); want a non-nil error against a non-repository path", notARepo, dirty)
	}

	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("worktreeDirty error = %v; want it to wrap a *gitexec.GitError so callers can still recover the exit code and stderr", err)
	}

	msg := err.Error()

	// The wrapped GitError renders the command once.
	// A second occurrence means the wrapper repeats the command.
	const gitCommand = "git status --porcelain"
	if occurrences := strings.Count(msg, gitCommand); occurrences != 1 {
		t.Errorf("worktreeDirty error names %q %d time(s); want exactly 1 — the wrapper must name what the probe was for, and leave the command to *gitexec.GitError:\n%s", gitCommand, occurrences, msg)
	}

	// The test also guards the other direction: removing the duplicate must not remove git's stderr.
	// The checked entry point carries that stderr for diagnosis.
	if !strings.Contains(msg, "not a git repository") {
		t.Errorf("worktreeDirty error dropped git's own stderr; want it carried through the wrap:\n%s", msg)
	}

	// The wrapper names the directory, because *gitexec.GitError does not render Dir.
	if !strings.Contains(msg, notARepo) {
		t.Errorf("worktreeDirty error does not name the probed directory %q; the wrapper owns the \"where\" because GitError does not render Dir:\n%s", notARepo, msg)
	}
}
