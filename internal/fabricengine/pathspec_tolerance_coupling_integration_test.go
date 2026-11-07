// pathspec_tolerance_coupling_integration_test.go pins the one coupling that commitWeftLocked's tolerance rests on.
// The tolerance accepts a pathspec that resolved to nothing, recognised by git's "did not match any files" message.
// The match runs on the error that comes back out of gitrepo.StageAndCommit.
// It works only because *gitexec.GitError.Error() renders git's trimmed stderr into its message,
// and every wrapper in the chain keeps that message with %w.
// Nothing else in this package depends on that rendering.
// A change that folds stderr out of Error, or swaps a %w for a %v, would turn the tolerance into a hard failure.
// No existing test would catch it.
// The behavioural tests reach the tolerance only through pathspecs the pre-filter already removes.
//
// This test drives a real `git add` against a pathspec matching nothing.
// It goes straight through gitrepo.StageAndCommit and asserts that the marker text survives to the caller.

package fabricengine_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pathspecMissMarker is the substring `commitWeftLocked` matches to recognise a pathspec that matched no files.
// It is git's own wording, copied here so that a change to git's message fails this test loudly.
// The error chain carrying the message must keep it intact, so a change there fails here as well.
const pathspecMissMarker = "did not match any files"

// TestStageAndCommit_PathspecMissMarkerSurvivesTheErrorChain asserts the marker text survives the error chain.
// A `git add` against a pathspec matching nothing fails,
// and the error from gitrepo.StageAndCommit still carries git's "did not match any files" text.
// That text is what the `commitWeftLocked` tolerance matches on.
//
//testtiming:keep git's "did not match any files" text surviving gitrepo.StageAndCommit's error chain, which the pathspec tolerance matches on; coverage of its blocks by other tests does not show an assertion of this
func TestStageAndCommit_PathspecMissMarkerSurvivesTheErrorChain(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// A real hub gives a real weft checkout with real history.
	// So `git add` fails for the reason under test, and not because the repo is unusable.
	weftPath := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."}).PrimeRecords()
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

	// The marker reaches the caller only through *gitexec.GitError's stderr rendering.
	// Assert that shape too.
	// Otherwise a change that preserves the text by another route passes this test for the wrong reason.
	var gitErr *gitexec.GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("StageAndCommit error = %v; want it to wrap a *gitexec.GitError — that wrapper is what carries git's stderr, and the marker with it", err)
	}
	if !strings.Contains(gitErr.Stderr, pathspecMissMarker) {
		t.Errorf("*gitexec.GitError.Stderr = %q; want it to carry the %q marker", gitErr.Stderr, pathspecMissMarker)
	}
}
