//go:build integration

// pushanchored_integration_test.go covers PushAnchored and PushPairAnchored against real hubforge pairs, the push lock both take, and PushAnchored against a hub's weft sibling:
// SkipGit and SkipPush each short-circuit to an empty result and push nothing; a weft carrying an
// unpushed commit is genuinely pushed and the mutation record carries exactly one KindBranchPushed
// entry; and a diverged weft remote surfaces gitrepo.ErrPushRejected unwrapped, distinguishable via
// errors.Is from a push error of a different kind.
// Reuses gitsha_integration_test.go's BareBranchSHAForTest re-export.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
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
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.MustRun(t, dir, "git", "checkout", "-B", branch, "origin/"+branch)
	return dir
}

// TestPushAnchored_SkipGitOrSkipPush_PushesNothing asserts SkipGit and SkipPush each short-circuit
// to an empty result and a nil error, leaving the weft bare remote unadvanced.
func TestPushAnchored_SkipGitOrSkipPush_PushesNothing(t *testing.T) {
	t.Parallel()

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
			if _, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded); err != nil {
				t.Fatalf("PushAnchored() priming push error = %v; want nil", err)
			}
			bareHeadBefore := fabricengine.BareBranchSHAForTest(t, h.RecordsBare, fabricengine.RecordsBranchName("main"))
			gitkit.CommitFile(t, h.PrimeRecords(), "weft-file.txt", "weft change never pushed", "weft change never pushed")

			res, err := fabricengine.PushAnchored(h.Location, tt.opts, fabricengine.LockWaitUnbounded)
			if err != nil {
				t.Fatalf("PushAnchored() error = %v; want nil", err)
			}
			if res.Mutated().Len() != 0 {
				t.Errorf("PushAnchored() record = %+v; want empty", res.Mutated().Entries())
			}

			if got := fabricengine.BareBranchSHAForTest(t, h.RecordsBare, fabricengine.RecordsBranchName("main")); got != bareHeadBefore {
				t.Errorf("weft bare = %q; want it unadvanced at %q", got, bareHeadBefore)
			}
		})
	}
}

// TestPushAnchored_PushesAndRecordsBranchPush covers the successful push path: a weft sibling
// carrying a commit ahead of its bare remote pushes it, the bare remote advances to the local HEAD,
// and the returned record contains exactly one KindBranchPushed entry.
//
//testtiming:keep a successful weft push advancing the bare remote and recording exactly one KindBranchPushed entry; coverage of its blocks by other tests does not show an assertion of this
func TestPushAnchored_PushesAndRecordsBranchPush(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	weftSHA := gitkit.CommitFile(t, h.PrimeRecords(), "weft-file.txt", "weft change", "weft change")
	warpBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
	warpBareBefore := fabricengine.BareBranchSHAForTest(t, h.CodeBare, warpBranch)
	gitkit.CommitFile(t, h.PrimeWorktree(), "warp-file.txt", "warp change never pushed", "warp change never pushed")

	res, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if err != nil {
		t.Fatalf("PushAnchored() error = %v; want nil", err)
	}

	if got := fabricengine.BareBranchSHAForTest(t, h.CodeBare, warpBranch); got != warpBareBefore {
		t.Errorf("warp bare = %q; want it unmoved at %q (PushAnchored pushes the records side only)", got, warpBareBefore)
	}

	if got := fabricengine.BareBranchSHAForTest(t, h.RecordsBare, fabricengine.RecordsBranchName("main")); got != weftSHA {
		t.Errorf("weft bare = %q; want it advanced to local HEAD %q", got, weftSHA)
	}

	entries := res.Mutated().Entries()
	found := 0
	var pushed fabricengine.Mutation
	for _, entry := range entries {
		if entry.Kind == fabricengine.KindBranchPushed {
			found++
			pushed = entry
		}
	}
	if found != 1 {
		t.Fatalf("PushAnchored() record = %+v; want exactly one KindBranchPushed entry, got %d", entries, found)
	}
	if !strings.HasPrefix(pushed.Detail, "side=records repo=") || !strings.Contains(pushed.Detail, " remote=") {
		t.Errorf("KindBranchPushed Detail = %q; want prefix %q and a %q part", pushed.Detail, "side=records repo=", " remote=")
	}
}

// TestPushAnchored_DivergedWeftRemote_ReturnsErrPushRejectedUnwrapped covers the rejection path a
// diverged weft remote produces: a second clone of the weft bare pushes a commit this weft sibling
// lacks, so this weft's next PushAnchored push is a genuine non-fast-forward rejection. The returned
// error must satisfy errors.Is(err, gitrepo.ErrPushRejected) — the unwrapped-sentinel property
// batch 7's closure depends on to warn-and-continue on exactly this condition.
//
//testtiming:keep a diverged weft remote returning gitrepo.ErrPushRejected unwrapped; coverage of its blocks by other tests does not show an assertion of this
func TestPushAnchored_DivergedWeftRemote_ReturnsErrPushRejectedUnwrapped(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")

	// A priming push establishes real content and upstream tracking on the weft bare — the weft
	// bare is genuinely empty at clone time (hubforge's own doc comment) — before the second clone
	// below diverges from it.
	if _, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded); err != nil {
		t.Fatalf("PushAnchored() priming push error = %v; want nil", err)
	}

	weftClone2 := cloneBareForTest(t, h.RecordsBare, fabricengine.RecordsBranchName("main"))
	gitkit.CommitFile(t, weftClone2, "other.txt", "from second weft clone", "from second weft clone")
	gitkit.MustRun(t, weftClone2, "git", "push")

	gitkit.CommitFile(t, h.PrimeRecords(), "weft-file.txt", "weft change that will be rejected", "weft change that will be rejected")

	_, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
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
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	gitkit.CommitFile(t, h.PrimeRecords(), "weft-file.txt", "weft change", "weft change")
	gitkit.MustRun(t, h.PrimeRecords(), "git", "remote", "remove", "origin")

	_, err := fabricengine.PushAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if err == nil {
		t.Fatal("PushAnchored() with no remote configured error = nil; want a non-nil push error")
	}
	if errors.Is(err, gitrepo.ErrPushRejected) {
		t.Errorf("PushAnchored() error = %v; want it NOT to satisfy errors.Is(err, gitrepo.ErrPushRejected) — this is a different error kind", err)
	}
}

// pushPair is a hubforge pair whose two sides each carry a remote branch to push to.
type pushPair struct {
	hub        *hubforge.Hub
	location   *lyxcwd.Location
	warpPath   string
	weftPath   string
	warpBranch string
	weftBranch string
}

// newPushPair adds a pair to a fresh hub and resolves its location from its warp worktree.
func newPushPair(t *testing.T) pushPair {
	t.Helper()

	h := hubforge.NewHub(t, ".")
	added := hubforge.AddPair(t, h, "pushpair")
	loc, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("pushpair"))
	if err != nil {
		t.Fatalf("ResolveWorktree: %v", err)
	}
	return pushPair{
		hub:        h,
		location:   loc,
		warpPath:   h.PairCodeWorktree("pushpair"),
		weftPath:   h.PairRecordsSibling("pushpair"),
		warpBranch: added.Branch,
		weftBranch: fabricengine.RecordsBranchName(added.Branch),
	}
}

// preReceiveHookPath returns the pre-receive hook file of bareDir.
func preReceiveHookPath(bareDir string) string {
	return filepath.Join(bareDir, "hooks", "pre-receive")
}

// installPreReceive writes script as the pre-receive hook of bareDir.
func installPreReceive(t *testing.T, bareDir, script string) {
	t.Helper()

	hookPath := preReceiveHookPath(bareDir)
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write pre-receive hook: %v", err)
	}
}

// declineOncePreReceive installs a pre-receive hook in bareDir that declines the first push it sees and accepts every later one.
func declineOncePreReceive(t *testing.T, bareDir string) {
	t.Helper()

	marker := filepath.ToSlash(filepath.Join(bareDir, "declined-once"))
	installPreReceive(t, bareDir, "#!/bin/sh\nif [ ! -e '"+marker+"' ]; then\n  : > '"+marker+"'\n  echo 'declined once' >&2\n  exit 1\nfi\nexit 0\n")
}

// countBranchPushed returns how many KindBranchPushed entries of res carry detailPrefix.
func countBranchPushed(res fabricengine.PushResult, detailPrefix string) int {
	count := 0
	for _, entry := range res.Mutated().Entries() {
		if entry.Kind == fabricengine.KindBranchPushed && strings.HasPrefix(entry.Detail, detailPrefix) {
			count++
		}
	}
	return count
}

// TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide walks one pair through the behaviors of PushPairAnchored in order:
// both sides pushed and recorded, a remote that declines once retried to success on both entries, a rejection whose fetch then fails returned without a retry, a remote that keeps declining reported as a bare rejection, an unborn code side skipped while the records side still pushes, a diverged code side reported while the records side still pushes, and both sides failing reported together.
// Two subtests then run on hubs of their own: the prime pushes its records side only and reports the skipped code push, and a prime that cannot be resolved fails the call with nothing pushed.
func TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide(t *testing.T) {
	t.Parallel()

	p := newPushPair(t)
	head := func(path string) string { return gitkit.RevParse(t, path, "HEAD") }

	// Both sides: a raw commit on each is pushed and recorded.
	warpSHA := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change", "warp change")
	weftSHA := gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change", "weft change")
	res, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if err != nil {
		t.Fatalf("PushPairAnchored() error = %v; want nil", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpSHA {
		t.Errorf("warp bare = %q; want local HEAD %q", got, warpSHA)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
		t.Errorf("weft bare = %q; want local HEAD %q", got, weftSHA)
	}
	if got := countBranchPushed(res, "side=code repo="); got != 1 {
		t.Errorf("PushPairAnchored() record = %+v; want exactly one warp KindBranchPushed entry, got %d", res.Mutated().Entries(), got)
	}
	if got := countBranchPushed(res, "side=records repo="); got != 1 {
		t.Errorf("PushPairAnchored() record = %+v; want exactly one weft KindBranchPushed entry, got %d", res.Mutated().Entries(), got)
	}

	// A remote that declines once: the stale-ref rejection is retried and succeeds, on both entries.
	declineOncePreReceive(t, p.hub.CodeBare)
	warpSHA = gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change 2", "warp change 2")
	if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded); err != nil {
		t.Fatalf("PushPairAnchored() against a remote declining once error = %v; want nil after the retry", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpSHA {
		t.Errorf("warp bare = %q; want local HEAD %q after the retry", got, warpSHA)
	}
	declineOncePreReceive(t, p.hub.RecordsBare)
	weftSHA = gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change 2", "weft change 2")
	if _, err := fabricengine.PushAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded); err != nil {
		t.Fatalf("PushAnchored() against a records remote declining once error = %v; want nil after the retry", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
		t.Errorf("weft bare = %q; want local HEAD %q after the retry", got, weftSHA)
	}

	// A rejection whose fetch then fails is not retried:
	// the code side pushes to its real bare but fetches from a missing path,
	// and a remote that declines once would accept a retry.
	warpBareBeforeFetchFailure := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch)
	if err := os.Remove(filepath.Join(p.hub.CodeBare, "declined-once")); err != nil {
		t.Fatalf("reset decline-once marker: %v", err)
	}
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change fetch failure", "warp change fetch failure")
	warpFetchURL := gitkit.Git(t, p.warpPath, "config", "--get", "remote.origin.url")
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.pushurl", warpFetchURL)
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.url", filepath.Join(t.TempDir(), "missing"))
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() with a rejected push and a failing fetch error = %v; want the bare rejection", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpBareBeforeFetchFailure {
		t.Errorf("warp bare = %q; want it unmoved at %q (no retry after a failed fetch)", got, warpBareBeforeFetchFailure)
	}
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.url", warpFetchURL)
	gitkit.MustRun(t, p.warpPath, "git", "config", "--unset", "remote.origin.pushurl")

	// A remote that keeps declining: the one retry is rejected too,
	// so the bare rejection surfaces and nothing local moves.
	installPreReceive(t, p.hub.CodeBare, "#!/bin/sh\necho 'declined always' >&2\nexit 1\n")
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change declined", "warp change declined")
	declinedWarpBare := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch)
	declinedWarpHead := head(p.warpPath)
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() against a remote that keeps declining error = %v; want IsPushRejected after the one retry", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != declinedWarpBare {
		t.Errorf("warp bare = %q; want it unmoved at %q", got, declinedWarpBare)
	}
	if got := head(p.warpPath); got != declinedWarpHead {
		t.Errorf("local warp HEAD = %q; want unchanged %q", got, declinedWarpHead)
	}

	if err := os.Remove(preReceiveHookPath(p.hub.CodeBare)); err != nil {
		t.Fatalf("remove always-declining hook: %v", err)
	}

	// An unborn code side is skipped,
	// and the records side still pushes.
	gitkit.MustRun(t, p.warpPath, "git", "checkout", "-q", "--orphan", "unborn")
	weftSHA = gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change unborn", "weft change unborn")
	if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded); err != nil {
		t.Fatalf("PushPairAnchored() with an unborn code side error = %v; want nil", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
		t.Errorf("weft bare = %q; want local HEAD %q with the code side unborn", got, weftSHA)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != declinedWarpBare {
		t.Errorf("warp bare = %q; want it unmoved at %q", got, declinedWarpBare)
	}
	gitkit.MustRun(t, p.warpPath, "git", "checkout", "-q", "-f", p.warpBranch)

	// A diverged code side: reported against the code side, the records side still pushes.
	warpClone := cloneBareForTest(t, p.hub.CodeBare, p.warpBranch)
	remoteSHA := gitkit.CommitFile(t, warpClone, "other.txt", "from second warp clone", "from second warp clone")
	gitkit.MustRun(t, warpClone, "git", "push")
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change 3", "warp change 3")
	weftSHA = gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change 3", "weft change 3")
	warpHead, weftHead := head(p.warpPath), head(p.weftPath)
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if err == nil {
		t.Fatal("PushPairAnchored() against a diverged code side error = nil; want gitrepo.ErrPushRejected")
	}
	if !errors.Is(err, gitrepo.ErrPushRejected) || !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPushRejected) and IsPushRejected", err)
	}
	if want := "push code side at " + p.warpPath; !strings.Contains(err.Error(), want) {
		t.Errorf("PushPairAnchored() error = %q; want it to contain %q", err, want)
	}
	if strings.Contains(err.Error(), "push records side") {
		t.Errorf("PushPairAnchored() error = %q; want the records side absent from it", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != remoteSHA {
		t.Errorf("warp bare = %q; want it left at the diverging commit %q", got, remoteSHA)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
		t.Errorf("weft bare = %q; want the records side pushed to %q despite the code side failing", got, weftSHA)
	}
	if head(p.warpPath) != warpHead || head(p.weftPath) != weftHead {
		t.Errorf("local HEADs moved: warp %q -> %q, weft %q -> %q; want both unchanged (no rebase, merge or force)", warpHead, head(p.warpPath), weftHead, head(p.weftPath))
	}

	// Both sides failing:
	// the records origin is gone,
	// and the code side is still diverged.
	gitkit.MustRun(t, p.weftPath, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
	gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change 4", "weft change 4")
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)
	if err == nil {
		t.Fatal("PushPairAnchored() with both sides failing error = nil; want both errors")
	}
	if !errors.Is(err, gitrepo.ErrPushRejected) {
		t.Errorf("PushPairAnchored() error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPushRejected) through the code side", err)
	}
	for _, want := range []string{"push code side at " + p.warpPath, "push records side at " + p.weftPath} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PushPairAnchored() error = %q; want it to contain %q", err, want)
		}
	}

	// The prime pushes the records side only and reports its unpushed code branch.
	t.Run("PrimePushesRecordsSideOnly", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")
		codeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		recordsBranch := gitkit.CurrentBranch(t, h.PrimeRecords())
		codeBefore := fabricengine.BareBranchSHAForTest(t, h.CodeBare, codeBranch)
		gitkit.CommitFile(t, h.PrimeWorktree(), "code-file.txt", "code change", "code change")
		recordsSHA := gitkit.CommitFile(t, h.PrimeRecords(), "records-file.txt", "records change", "records change")

		res, err := fabricengine.PushPairAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)

		if err != nil {
			t.Fatalf("PushPairAnchored() in the prime error = %v; want nil", err)
		}
		if got := fabricengine.BareBranchSHAForTest(t, h.CodeBare, codeBranch); got != codeBefore {
			t.Errorf("code bare = %q; want it unchanged at %q", got, codeBefore)
		}
		if got := fabricengine.BareBranchSHAForTest(t, h.RecordsBare, recordsBranch); got != recordsSHA {
			t.Errorf("records bare = %q; want local HEAD %q", got, recordsSHA)
		}
		if !strings.Contains(res.CodePushSkipped, codeBranch) {
			t.Errorf("CodePushSkipped = %q; want it to name branch %q", res.CodePushSkipped, codeBranch)
		}
	})

	// A prime that cannot be resolved fails the call with nothing pushed.
	t.Run("UnresolvablePrimePushesNothing", func(t *testing.T) {
		h := hubforge.NewHub(t, ".")
		recordsBranch := gitkit.CurrentBranch(t, h.PrimeRecords())
		gitkit.CommitFile(t, h.PrimeRecords(), "records-file.txt", "records change", "records change")
		unresolvable := *h.Location
		unresolvable.AnchorRel = "absent/subdir"

		_, err := fabricengine.PushPairAnchored(&unresolvable, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded)

		if err == nil || !strings.Contains(err.Error(), "resolve main worktree") {
			t.Fatalf("PushPairAnchored() error = %v; want one carrying %q", err, "resolve main worktree")
		}
		if gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
			t.Errorf("records bare holds %q; want nothing pushed", recordsBranch)
		}
	})
}

// TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks covers the push lock both entries take:
// with the lock held by the test, a bounded wait returns ErrPushLockBusy naming neither side and moves no remote;
// SkipPush and SkipGit return at once;
// and an unbounded wait returns only once the lock is released and then pushes.
func TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		push       func(l *lyxcwd.Location, opts fabricengine.SyncOptions, lockWait time.Duration) error
		pushesWarp bool
	}{
		{
			name: "PushPairAnchored",
			push: func(l *lyxcwd.Location, opts fabricengine.SyncOptions, lockWait time.Duration) error {
				_, err := fabricengine.PushPairAnchored(l, opts, lockWait)
				return err
			},
			pushesWarp: true,
		},
		{
			name: "PushAnchored",
			push: func(l *lyxcwd.Location, opts fabricengine.SyncOptions, lockWait time.Duration) error {
				_, err := fabricengine.PushAnchored(l, opts, lockWait)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newPushPair(t)
			warpBefore := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch)
			weftBefore := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch)
			warpSHA := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change", "warp change")
			weftSHA := gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change", "weft change")

			held, err := lock.AcquireWriteLock(fabricengine.PushLockPathForTest(t, p.weftPath))
			if err != nil {
				t.Fatalf("hold push lock: %v", err)
			}
			released := false
			defer func() {
				if !released {
					_ = held.Release()
				}
			}()

			err = tt.push(p.location, fabricengine.SyncOptions{}, 50*time.Millisecond)
			if !errors.Is(err, fabricengine.ErrPushLockBusy) {
				t.Fatalf("bounded push under a held lock error = %v; want it to satisfy errors.Is(err, ErrPushLockBusy)", err)
			}
			for _, side := range []string{"push code side", "push records side"} {
				if strings.Contains(err.Error(), side) {
					t.Errorf("bounded push error = %q; want it to name neither side, found %q", err, side)
				}
			}
			if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpBefore {
				t.Errorf("warp bare = %q; want it unmoved at %q", got, warpBefore)
			}
			if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftBefore {
				t.Errorf("weft bare = %q; want it unmoved at %q", got, weftBefore)
			}

			for _, opts := range []fabricengine.SyncOptions{{SkipPush: true}, {SkipGit: true}} {
				if err := tt.push(p.location, opts, fabricengine.LockWaitUnbounded); err != nil {
					t.Errorf("push with %+v under a held lock error = %v; want nil at once", opts, err)
				}
			}

			done := make(chan error, 1)
			go func() { done <- tt.push(p.location, fabricengine.SyncOptions{}, fabricengine.LockWaitUnbounded) }()
			select {
			case err := <-done:
				t.Fatalf("unbounded push returned %v while the lock was held; want it to block", err)
			case <-time.After(150 * time.Millisecond):
			}
			if err := held.Release(); err != nil {
				t.Fatalf("release push lock: %v", err)
			}
			released = true
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("unbounded push after release error = %v; want nil", err)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("unbounded push still blocked 30s after the lock was released")
			}

			if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
				t.Errorf("weft bare = %q; want it pushed to %q after the release", got, weftSHA)
			}
			wantWarp := warpBefore
			if tt.pushesWarp {
				wantWarp = warpSHA
			}
			if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != wantWarp {
				t.Errorf("warp bare = %q; want %q after the release", got, wantWarp)
			}
		})
	}
}
