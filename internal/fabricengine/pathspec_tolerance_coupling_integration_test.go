//go:build integration

// pathspec_tolerance_coupling_integration_test.go pins the one coupling commitWeftLocked's
// "did not match any files" tolerance rests on, which no behavioural test of that tolerance covers.
//
// commitWeftLocked recognises a pathspec that resolved to nothing by matching git's own message text
// on the error coming back out of gitrepo.StageAndCommit. That match only works because
// *gitexec.GitError.Error() renders git's trimmed stderr into its message and every wrapper in the
// chain preserves it with %w. Nothing else in this package depends on that rendering, so a change to
// it — folding stderr out of Error, or swapping a %w for a %v somewhere in the chain — would turn a
// documented tolerance back into a hard failure with every existing test still green, because the
// behavioural tests exercise the tolerance through pathspecs the pre-filter already removes.
//
// This test therefore drives the raw substrate coupling directly: a real `git add` against a
// pathspec matching nothing, straight through gitrepo.StageAndCommit, asserting the marker text
// survives to the caller.

package fabricengine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// pathspecMissMarker is the substring commitWeftLocked (weftgit.go) matches to recognise a pathspec
// that matched no files. It is git's own wording, reproduced here so a change to git's message or to
// the error chain that carries it fails loudly at this test rather than silently at the tolerance.
const pathspecMissMarker = "did not match any files"

// TestStageAndCommit_PathspecMissMarkerSurvivesTheErrorChain asserts that a `git add` against a
// pathspec matching nothing produces an error whose message still carries git's own
// "did not match any files" text by the time gitrepo.StageAndCommit returns it — the text
// commitWeftLocked's tolerance matches on.
func TestStageAndCommit_PathspecMissMarkerSurvivesTheErrorChain(t *testing.T) {
	t.Parallel()

	// A real hub gives a real weft checkout with real history, so `git add` fails for the reason
	// under test (the pathspec matches nothing) rather than because the repo is unusable.
	weftPath := hubforge.NewHub(t, ".").PrimeWeft()
	repo := gitrepo.New(weftPath)

	const missingPathspec = "no-such-directory-at-all"
	sha, committed, err := repo.StageAndCommit("staging a pathspec that matches nothing", []string{missingPathspec})
	if err == nil {
		t.Fatalf("StageAndCommit(%q) = (%q, %v, nil); want a non-nil error — gitrepo.StageAndCommit has no tolerance of its own, the tolerance is commitWeftLocked's", missingPathspec, sha, committed)
	}

	msg := err.Error()
	if !strings.Contains(msg, pathspecMissMarker) {
		t.Fatalf("StageAndCommit error lost the %q marker commitWeftLocked's tolerance matches on; the tolerance is now unreachable and a no-op weft commit will hard-fail instead:\n%s", pathspecMissMarker, msg)
	}

	// The marker reaches the caller only via *gitexec.GitError's stderr rendering. Assert the shape
	// too, so a future change that happens to preserve the text by some other route does not leave
	// this test passing for the wrong reason.
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("StageAndCommit error = %v; want it to wrap a *gitexec.GitError — that wrapper is what carries git's stderr, and the marker with it", err)
	}
	if !strings.Contains(gitErr.Stderr, pathspecMissMarker) {
		t.Errorf("*gitexec.GitError.Stderr = %q; want it to carry the %q marker", gitErr.Stderr, pathspecMissMarker)
	}
}
