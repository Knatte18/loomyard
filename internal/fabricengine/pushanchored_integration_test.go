//go:build integration

// pushanchored_integration_test.go covers PushAnchored against a real hubforge pair's weft sibling:
// SkipGit and SkipPush each short-circuit to an empty result and push nothing; a weft carrying an
// unpushed commit is genuinely pushed and the mutation record carries exactly one KindBranchPushed
// entry; and a diverged weft remote surfaces gitrepo.ErrPushRejected unwrapped, distinguishable via
// errors.Is from a push error of a different kind. Reuses coalesce_integration_test.go's commitPlain
// fixture helper and gitsha_integration_test.go's BareBranchSHAForTest re-export.

package fabricengine_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// cloneBareForTest clones bareRepo into a fresh subdirectory of t.TempDir(), configuring a commit
// identity and checking out branch explicitly, and returns the clone's path — the second-clone
// fixture TestPushAnchored's diverged-remote scenario needs to advance the bare remote out from
// under the primary weft sibling.
// The explicit checkout is load-bearing, not cosmetic: bareRepo's own HEAD symref still points at
// git's default ("master"), which was never the branch actually pushed, so a plain `git clone`
// leaves the clone on a dangling, uncommitted "master" rather than on branch — committing there
// would silently diverge on the wrong ref.
func cloneBareForTest(t *testing.T, bareRepo, branch string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clone")
	gitkit.MustRun(t, filepath.Dir(dir), "git", "clone", bareRepo, dir)
	gitkit.MustRun(t, dir, "git", "config", "user.email", "test@test.com")
	gitkit.MustRun(t, dir, "git", "config", "user.name", "Test")
	gitkit.MustRun(t, dir, "git", "checkout", "-B", branch, "origin/"+branch)
	return dir
}

// TestPushAnchored_SkipGitOrSkipPush_PushesNothing asserts SkipGit and SkipPush each short-circuit
// to an empty result and a nil error, leaving the weft bare remote unadvanced.
func TestPushAnchored_SkipGitOrSkipPush_PushesNothing(t *testing.T) {
	tests := []struct {
		name string
		opts fabricengine.SyncOptions
	}{
		{name: "SkipGit", opts: fabricengine.SyncOptions{SkipGit: true}},
		{name: "SkipPush", opts: fabricengine.SyncOptions{SkipPush: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hubforge.NewHub(t, ".")
			// The weft bare is genuinely empty at clone time (hubforge's own doc comment), so it
			// carries no branch to read a "before" SHA from yet; a priming push establishes one.
			if _, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}); err != nil {
				t.Fatalf("PushAnchored() priming push error = %v; want nil", err)
			}
			bareHeadBefore := fabricengine.BareBranchSHAForTest(t, h.WeftBare, fabricengine.WeftBranchName("main"))
			commitPlain(t, h.PrimeWeft(), "weft-file.txt", "weft change never pushed")

			res, err := fabricengine.PushAnchored(h.Location, tt.opts)
			if err != nil {
				t.Fatalf("PushAnchored() error = %v; want nil", err)
			}
			if res.Mutated().Len() != 0 {
				t.Errorf("PushAnchored() record = %+v; want empty", res.Mutated().Entries())
			}

			if got := fabricengine.BareBranchSHAForTest(t, h.WeftBare, fabricengine.WeftBranchName("main")); got != bareHeadBefore {
				t.Errorf("weft bare = %q; want it unadvanced at %q", got, bareHeadBefore)
			}
		})
	}
}

// TestPushAnchored_PushesAndRecordsBranchPush covers the successful push path: a weft sibling
// carrying a commit ahead of its bare remote pushes it, the bare remote advances to the local HEAD,
// and the returned record contains exactly one KindBranchPushed entry.
func TestPushAnchored_PushesAndRecordsBranchPush(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	weftSHA := commitPlain(t, h.PrimeWeft(), "weft-file.txt", "weft change")

	res, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("PushAnchored() error = %v; want nil", err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, h.WeftBare, fabricengine.WeftBranchName("main")); got != weftSHA {
		t.Errorf("weft bare = %q; want it advanced to local HEAD %q", got, weftSHA)
	}

	entries := res.Mutated().Entries()
	found := 0
	for _, entry := range entries {
		if entry.Kind == fabricengine.KindBranchPushed {
			found++
		}
	}
	if found != 1 {
		t.Errorf("PushAnchored() record = %+v; want exactly one KindBranchPushed entry, got %d", entries, found)
	}
}

// TestPushAnchored_DivergedWeftRemote_ReturnsErrPushRejectedUnwrapped covers the rejection path a
// diverged weft remote produces: a second clone of the weft bare pushes a commit this weft sibling
// lacks, so this weft's next PushAnchored push is a genuine non-fast-forward rejection. The returned
// error must satisfy errors.Is(err, gitrepo.ErrPushRejected) — the unwrapped-sentinel property
// batch 7's closure depends on to warn-and-continue on exactly this condition.
func TestPushAnchored_DivergedWeftRemote_ReturnsErrPushRejectedUnwrapped(t *testing.T) {
	h := hubforge.NewHub(t, ".")

	// A priming push establishes real content and upstream tracking on the weft bare — the weft
	// bare is genuinely empty at clone time (hubforge's own doc comment) — before the second clone
	// below diverges from it.
	if _, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushAnchored() priming push error = %v; want nil", err)
	}

	weftClone2 := cloneBareForTest(t, h.WeftBare, fabricengine.WeftBranchName("main"))
	commitPlain(t, weftClone2, "other.txt", "from second weft clone")
	gitkit.MustRun(t, weftClone2, "git", "push")

	commitPlain(t, h.PrimeWeft(), "weft-file.txt", "weft change that will be rejected")

	_, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{})
	if err == nil {
		t.Fatal("PushAnchored() against a diverged weft remote error = nil; want gitrepo.ErrPushRejected")
	}
	if !errors.Is(err, gitrepo.ErrPushRejected) {
		t.Errorf("PushAnchored() error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPushRejected)", err)
	}
}

// TestPushAnchored_OtherPushErrorKind_DoesNotMatchErrPushRejected covers the negative half of the
// same sentinel property: a push error that is NOT a remote-divergence rejection — here, the weft
// sibling's origin remote removed entirely — must not satisfy errors.Is(err, gitrepo.ErrPushRejected).
func TestPushAnchored_OtherPushErrorKind_DoesNotMatchErrPushRejected(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	commitPlain(t, h.PrimeWeft(), "weft-file.txt", "weft change")
	gitkit.MustRun(t, h.PrimeWeft(), "git", "remote", "remove", "origin")

	_, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{})
	if err == nil {
		t.Fatal("PushAnchored() with no remote configured error = nil; want a non-nil push error")
	}
	if errors.Is(err, gitrepo.ErrPushRejected) {
		t.Errorf("PushAnchored() error = %v; want it NOT to satisfy errors.Is(err, gitrepo.ErrPushRejected) — this is a different error kind", err)
	}
}
