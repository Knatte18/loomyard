//go:build integration

// dirtiness_message_integration_test.go pins the composed shape of worktreeDirty's error against
// real git, rather than its wording.
//
// The gitexec split (the checked-call migration) changed what the underlying error renders: a
// non-zero git exit now arrives as a *gitexec.GitError whose own Error already spells out
// `git status --porcelain …: exit <code>: <stderr>`. Every wrapper written before that split
// supplied the git command itself, because the error it wrapped was a bare exit code and would
// otherwise have been undiagnosable. Left unchanged, those wrappers emit the command twice, which
// pushes git's own actionable stderr — the only part an operator can act on — behind a duplicate of
// what they have already been told.
//
// This test drives the real failure (a git status against a directory that is not a repository at
// all) and asserts the composed message names the git command exactly once while still carrying
// git's stderr. It is deliberately shape-based, not a golden string: the wrapper's prose is free to
// change, the duplication is not.

package fabricengine

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// TestWorktreeDirty_ErrorNamesGitCommandOnce reproduces a real non-zero git exit and asserts the
// error worktreeDirty composes from it does not repeat the git command the wrapped
// *gitexec.GitError already renders.
func TestWorktreeDirty_ErrorNamesGitCommandOnce(t *testing.T) {
	t.Parallel()

	// A fresh temp dir is not a git repository, so `git status --porcelain` exits non-zero with a
	// real "not a git repository" stderr — the exact substrate failure the CLI hits when a verb is
	// pointed at a path that is not a checkout.
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

	// The wrapped GitError renders the command once. Any second occurrence is a wrapper that
	// predates the gitexec split still supplying what it no longer needs to.
	const gitCommand = "git status --porcelain"
	if occurrences := strings.Count(msg, gitCommand); occurrences != 1 {
		t.Errorf("worktreeDirty error names %q %d time(s); want exactly 1 — the wrapper must name what the probe was for, and leave the command to *gitexec.GitError:\n%s", gitCommand, occurrences, msg)
	}

	// Guard the other direction: stripping the duplication must not strip git's own diagnosis with
	// it, which is the whole reason the checked entry point carries stderr at all.
	if !strings.Contains(msg, "not a git repository") {
		t.Errorf("worktreeDirty error dropped git's own stderr; want it carried through the wrap:\n%s", msg)
	}

	// The wrapper still owns the "where": *gitexec.GitError deliberately does not render Dir.
	if !strings.Contains(msg, notARepo) {
		t.Errorf("worktreeDirty error does not name the probed directory %q; the wrapper owns the \"where\" because GitError does not render Dir:\n%s", notARepo, msg)
	}
}
