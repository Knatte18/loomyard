// pushanchored_integration_test.go covers PushPairAnchored against real hubforge pairs.
// It also covers the push lock it takes, and its push of a prime's records side.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cloneBareForTest clones bareRepo into a fresh directory under t.TempDir() and returns its path.
// The clone gets a commit identity and is checked out on branch, tracking origin/branch.
// A test commits in this clone to advance the bare remote.
// The explicit checkout matters: a plain clone lands on the bare repository's HEAD branch;
// a commit made there would go to the wrong ref.
func cloneBareForTest(t *testing.T, bareRepo, branch string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clone")
	gitkit.MustRun(t, filepath.Dir(dir), "git", "clone", bareRepo, dir)
	gitkit.Git(t, dir, "config", "user.email", "test@test.com")
	gitkit.Git(t, dir, "config", "user.name", "Test")
	gitkit.MustRun(t, dir, "git", "checkout", "-B", branch, "origin/"+branch)
	return dir
}

// TestPushPairAnchored_PrimeDivergedWeftRemote_ReturnsErrPushRejected covers a prime push to a diverged weft remote.
// A second clone of the weft bare changes `status.json`, as the prime's weft does;
// so the prime's next PushPairAnchored push is rejected and its replay stops on a conflict.
// The returned error must match errors.Is(err, gitrepo.ErrPushRejected) through the failed-push error wrapping it.
// Callers that discriminate a rejection depend on that, and the failed-push record names the conflicting path.
//
//testtiming:keep a diverged weft remote returning gitrepo.ErrPushRejected through its failed-push error; coverage of its blocks by other tests does not show an assertion of this
func TestPushPairAnchored_PrimeDivergedWeftRemote_ReturnsErrPushRejected(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	// A priming push gives the weft bare content and upstream tracking;
	// the bare is empty at clone time, so this happens before the second clone below diverges from it.
	if _, err := fabricengine.PushPairAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
		t.Fatalf("PushPairAnchored() priming push error = %v; want nil", err)
	}

	weftClone2 := cloneBareForTest(t, h.RecordsBare, fabricengine.RecordsBranchName("main"))
	gitkit.CommitFile(t, weftClone2, "status.json", "from second weft clone", "from second weft clone")
	gitkit.MustRun(t, weftClone2, "git", "push")

	gitkit.CommitFile(t, h.PrimeRecords(), "status.json", "weft change that will be rejected", "weft change that will be rejected")

	_, err := fabricengine.PushPairAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	if err == nil {
		t.Fatal("PushPairAnchored() against a diverged weft remote error = nil; want gitrepo.ErrPushRejected")
	}
	if !errors.Is(err, gitrepo.ErrPushRejected) || !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() error = %v; want it to satisfy errors.Is(err, gitrepo.ErrPushRejected)", err)
	}
	var failed *fabricengine.PushFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("PushPairAnchored() error = %v; want a *PushFailedError", err)
	}
	if failed.Record.Class != fabricengine.PushFailureNotFastForward || failed.Record.Repo != "records" || !slices.Contains(failed.Record.Paths, "status.json") {
		t.Errorf("PushPairAnchored() record = %+v; want a not-fast-forward records record naming status.json", failed.Record)
	}
	records, err := fabricengine.ListFailedPushes(h.Location.HubPath)
	if err != nil || len(records) != 1 || records[0].Branch != failed.Record.Branch || records[0].Class != fabricengine.PushFailureNotFastForward {
		t.Errorf("ListFailedPushes() = %+v, %v; want the one not-fast-forward record of the push", records, err)
	}
}

// pushPair is a hubforge pair whose two sides each carry a remote branch to push to.
type pushPair struct {
	hub        *hubforge.Hub
	location   *lymcwd.Location
	warpPath   string
	weftPath   string
	warpBranch string
	weftBranch string
}

// newPushPair adds a pair to a fresh hub and resolves its location from its warp worktree.
func newPushPair(t *testing.T) pushPair {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	added := hubforge.AddPair(t, h, "pushpair")
	loc, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("pushpair"))
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

// declineOncePreReceive installs a pre-receive hook in bareDir that declines the first push it sees.
// Every later push is accepted.
func declineOncePreReceive(t *testing.T, bareDir string) {
	t.Helper()

	marker := filepath.ToSlash(filepath.Join(bareDir, "declined-once"))
	installPreReceive(t, bareDir, "#!/bin/sh\nif [ ! -e '"+marker+"' ]; then\n  : > '"+marker+"'\n  echo 'declined once' >&2\n  exit 1\nfi\nexit 0\n")
}

// countBranchPushed returns how many KindBranchPushed entries of res carry detailPrefix and a remote part.
func countBranchPushed(res fabricengine.PushResult, detailPrefix string) int {
	count := 0
	for _, entry := range res.Mutated().Entries() {
		if entry.Kind == fabricengine.KindBranchPushed && strings.HasPrefix(entry.Detail, detailPrefix) && strings.Contains(entry.Detail, " remote=") {
			count++
		}
	}
	return count
}

// branchPushedPosition returns the position in res of the first KindBranchPushed entry carrying detailPrefix.
// It returns -1 when none does.
func branchPushedPosition(res fabricengine.PushResult, detailPrefix string) int {
	for i, entry := range res.Mutated().Entries() {
		if entry.Kind == fabricengine.KindBranchPushed && strings.HasPrefix(entry.Detail, detailPrefix) && strings.Contains(entry.Detail, " remote=") {
			return i
		}
	}
	return -1
}

// TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide walks one pair through PushPairAnchored's behaviors.
// In order, it covers: both sides pushed and recorded, the records side first;
// a remote that declines once is retried to success on both entries;
// a rejection whose fetch keeps failing is never pushed again;
// a remote that keeps declining is reported as a bare rejection;
// an unborn code side is skipped while the records side still pushes;
// a diverged code side is reported while the records side still pushes;
// and a failing records side withholds the code side.
// Subtests run on hubs of their own.
// A refusing records remote fails the call and leaves origin's code branch at its old tip.
// So does a records side deferred by a held write lock, and so does one deferred over uncommitted changes.
// A prime pushes its records side only and reports the skipped code push;
// a prime pushes its records side only and reports the skipped code push;
// an unresolvable prime fails the call with nothing pushed.
func TestPushPairAnchored_PushesBothSidesRetriesAndReportsEachSide(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	p := newPushPair(t)
	head := func(path string) string { return gitkit.RevParse(t, path, "HEAD") }

	// Both sides: a raw commit on each is pushed and recorded.
	warpSHA := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change", "warp change")
	weftSHA := gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change", "weft change")
	res, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
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
	recordsPosition, codePosition := branchPushedPosition(res, "side=records repo="), branchPushedPosition(res, "side=code repo=")
	if recordsPosition < 0 || codePosition < 0 || recordsPosition > codePosition {
		t.Errorf("PushPairAnchored() record = %+v; want the records side's push listed before the code side's", res.Mutated().Entries())
	}

	// A remote that declines once: the stale-ref rejection is retried and succeeds, on both entries.
	declineOncePreReceive(t, p.hub.CodeBare)
	warpSHA = gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change 2", "warp change 2")
	if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
		t.Fatalf("PushPairAnchored() against a remote declining once error = %v; want nil after the retry", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpSHA {
		t.Errorf("warp bare = %q; want local HEAD %q after the retry", got, warpSHA)
	}
	declineOncePreReceive(t, p.hub.RecordsBare)
	weftSHA = gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change 2", "weft change 2")
	if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
		t.Fatalf("PushPairAnchored() against a records remote declining once error = %v; want nil after the retry", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftSHA {
		t.Errorf("weft bare = %q; want local HEAD %q after the retry", got, weftSHA)
	}

	// A rejection whose fetch then keeps failing is never pushed again:
	// the code side pushes to its real bare but fetches from a missing path,
	// and a remote that declines once would accept a retry.
	noSleep := fabricengine.SyncOptions{BackoffSleep: func(time.Duration) {}}
	warpBareBeforeFetchFailure := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch)
	if err := os.Remove(filepath.Join(p.hub.CodeBare, "declined-once")); err != nil {
		t.Fatalf("reset decline-once marker: %v", err)
	}
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change fetch failure", "warp change fetch failure")
	warpFetchURL := gitkit.Git(t, p.warpPath, "config", "--get", "remote.origin.url")
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.pushurl", warpFetchURL)
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.url", filepath.Join(t.TempDir(), "missing"))
	_, err = fabricengine.PushPairAnchored(p.location, noSleep, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	if !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() with a rejected push and a failing fetch error = %v; want the bare rejection", err)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpBareBeforeFetchFailure {
		t.Errorf("warp bare = %q; want it unmoved at %q (no push after a failed fetch)", got, warpBareBeforeFetchFailure)
	}
	gitkit.MustRun(t, p.warpPath, "git", "config", "remote.origin.url", warpFetchURL)
	gitkit.MustRun(t, p.warpPath, "git", "config", "--unset", "remote.origin.pushurl")

	// A remote that keeps declining: every retry is rejected too,
	// so the bare rejection surfaces and nothing local moves.
	installPreReceive(t, p.hub.CodeBare, "#!/bin/sh\necho 'declined always' >&2\nexit 1\n")
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp change declined", "warp change declined")
	declinedWarpBare := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch)
	declinedWarpHead := head(p.warpPath)
	sleeps := 0
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{BackoffSleep: func(time.Duration) { sleeps++ }}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	if !fabricengine.IsPushRejected(err) {
		t.Errorf("PushPairAnchored() against a remote that keeps declining error = %v; want IsPushRejected after the retries", err)
	}
	if want := len(gitrepo.PushBackoffDelays()); sleeps < want {
		t.Errorf("backoff sleeps = %d; want at least %d, the whole backoff waited out before the record", sleeps, want)
	}
	requireOneFailedPush(t, p.hub.Location.HubPath, "code", fabricengine.PushFailureOther)
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
	if _, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
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
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
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

	// A failing records side withholds the code side:
	// the records origin is gone, so the call fails on the records side and the code side is not tried.
	gitkit.MustRun(t, p.weftPath, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing"))
	gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft change 4", "weft change 4")
	_, err = fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	if err == nil {
		t.Fatal("PushPairAnchored() with a failing records side error = nil; want the records error")
	}
	for _, want := range []string{"push records side at " + p.weftPath, "the code side at " + p.warpPath + " was withheld"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PushPairAnchored() error = %q; want it to contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "push code side") {
		t.Errorf("PushPairAnchored() error = %q; want the code side untried", err)
	}

	// A records remote that refuses the push sends no code side.
	t.Run("RefusingRecordsRemoteWithholdsTheCodeSide", func(t *testing.T) {
		t.Parallel()
		p := newPairFixture(t, "refusedrecords")
		installPreReceive(t, p.hub.RecordsBare, authRefusingHook)
		codeBefore := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
		gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "warp change")
		gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft\n", "weft change")

		_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

		if err == nil {
			t.Fatal("PushPairAnchored() against a refusing records remote error = nil; want the records error")
		}
		for _, want := range []string{"push records side at " + p.weftPath, "the code side at " + p.warpPath + " was withheld"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("PushPairAnchored() error = %q; want it to contain %q", err, want)
			}
		}
		if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != codeBefore {
			t.Errorf("code bare = %q; want it unmoved at %q", got, codeBefore)
		}
		requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabricengine.PushFailureAuth)
	})

	// A records side deferred, over a held weft write lock or an uncommitted change, returns its deferred record.
	// No code side is sent.
	deferredRows := []struct {
		name string
		// prepare makes the push deferred after origin's records branch diverged.
		prepare func(t *testing.T, p *pairFixture)
		// wantError is the text the deferred record's error holds.
		wantError string
	}{
		{
			"HeldWeftWriteLock",
			func(t *testing.T, p *pairFixture) {
				held := holdWeftWriteLock(t, p.weftPath)
				t.Cleanup(func() { _ = held.Release() })
			},
			"deferred",
		},
		{
			"UncommittedChangeOnAReplayedPath",
			func(t *testing.T, p *pairFixture) {
				if err := os.WriteFile(filepath.Join(p.weftPath, "weft-file.txt"), []byte("uncommitted\n"), 0o644); err != nil {
					t.Fatalf("write the uncommitted change: %v", err)
				}
			},
			"uncommitted changes",
		},
	}
	for _, tt := range deferredRows {
		t.Run("Deferred_"+tt.name, func(t *testing.T) {
			t.Parallel()
			p := newPairFixture(t, "deferred"+strings.ToLower(tt.name))
			codeBefore := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
			gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "warp change")
			gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft\n", "weft change")
			originTip := p.originCommit(p.weftBranch, "other.txt", "other\n", "other machine")
			tt.prepare(t, p)

			_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

			var failed *fabricengine.PushFailedError
			if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureDeferred {
				t.Fatalf("PushPairAnchored() error = %v; want a deferred *PushFailedError", err)
			}
			if !strings.Contains(failed.Record.Error, tt.wantError) {
				t.Errorf("deferred record error = %q; want it to hold %q", failed.Record.Error, tt.wantError)
			}
			if tt.wantError != "deferred" && !strings.Contains(failed.Record.Error, p.weftPath) {
				t.Errorf("deferred record error = %q; want it to name the worktree %s", failed.Record.Error, p.weftPath)
			}
			if got := bareBranchTip(t, p.hub.RecordsBare, p.weftBranch); got != originTip {
				t.Errorf("records bare = %q; want it left at the other machine's %q", got, originTip)
			}
			if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != codeBefore {
				t.Errorf("code bare = %q; want it unmoved at %q", got, codeBefore)
			}
			requireOneFailedPush(t, p.hub.Location.HubPath, "records", fabricengine.PushFailureDeferred)
		})
	}

	// The owner's guard: a code side rejected by an origin tip past the tip the last transition names resets that tip.
	// A tip equal to the named one, and a push sent with no records side before it, keep the rejection.
	guardRows := []struct {
		name string
		// namedIsOriginTip makes the last transition name the commit another machine pushed, not the pair's earlier tip.
		namedIsOriginTip bool
		// push runs the code-side push under test and returns its error.
		push      func(p *pairFixture) error
		wantReset bool
	}{
		{
			"ResetsATipPastTheNamedOneAndLandsItsOwnCodeSide",
			false,
			func(p *pairFixture) error {
				_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
				return err
			},
			true,
		},
		{
			"OriginTipEqualToTheNamedTipKeepsTheRejection",
			true,
			func(p *pairFixture) error {
				_, err := fabricengine.PushPairAnchored(p.location, p.fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
				return err
			},
			false,
		},
		{
			"APushWithNoRecordsSideBeforeItKeepsTheRejection",
			false,
			func(p *pairFixture) error {
				_, err := fabricengine.PushWarpRebaseFreeAt(p.warpPath, p.fake.options(nil))
				return err
			},
			false,
		},
	}
	for _, tt := range guardRows {
		t.Run("OwnerGuard_"+tt.name, func(t *testing.T) {
			t.Parallel()
			p := newPairFixture(t, "ownerguard")
			earlier := bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
			warpClone := cloneBareForTest(t, p.hub.CodeBare, p.warpBranch)
			otherMachinesTip := gitkit.CommitFile(t, warpClone, "other.txt", "other\n", "pushed by another machine")
			gitkit.MustRun(t, warpClone, "git", "push")
			named := earlier
			if tt.namedIsOriginTip {
				named = otherMachinesTip
			}
			plantRunTransition(t, p, "ownerguard", named, "oslo")
			ownTip := gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "this machine's code")

			err := tt.push(p)

			if tt.wantReset {
				if err != nil {
					t.Fatalf("push error = %v; want none after the reset", err)
				}
				if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != ownTip {
					t.Errorf("code bare = %q; want this machine's tip %q", got, ownTip)
				}
				return
			}
			if !fabricengine.IsPushRejected(err) {
				t.Errorf("push error = %v; want the rejection kept", err)
			}
			if got := bareBranchTip(t, p.hub.CodeBare, p.warpBranch); got != otherMachinesTip {
				t.Errorf("code bare = %q; want it left at the other machine's tip %q", got, otherMachinesTip)
			}
		})
	}

	// The prime pushes the records side only and reports its unpushed code branch.
	t.Run("PrimePushesRecordsSideOnly", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		codeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		recordsBranch := gitkit.CurrentBranch(t, h.PrimeRecords())
		codeBefore := fabricengine.BareBranchSHAForTest(t, h.CodeBare, codeBranch)
		gitkit.CommitFile(t, h.PrimeWorktree(), "code-file.txt", "code change", "code change")
		recordsSHA := gitkit.CommitFile(t, h.PrimeRecords(), "records-file.txt", "records change", "records change")

		res, err := fabricengine.PushPairAnchored(h.Location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

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
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		recordsBranch := gitkit.CurrentBranch(t, h.PrimeRecords())
		gitkit.CommitFile(t, h.PrimeRecords(), "records-file.txt", "records change", "records change")
		unresolvable := *h.Location
		unresolvable.AnchorRel = "absent/subdir"

		_, err := fabricengine.PushPairAnchored(&unresolvable, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})

		if err == nil || !strings.Contains(err.Error(), "resolve main worktree") {
			t.Fatalf("PushPairAnchored() error = %v; want one carrying %q", err, "resolve main worktree")
		}
		if gitkit.BranchExists(t, h.RecordsBare, recordsBranch) {
			t.Errorf("records bare holds %q; want nothing pushed", recordsBranch)
		}
	})
}

// TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks covers the push lock PushPairAnchored takes:
// with the lock held by the test, a bounded wait returns ErrPushLockBusy naming neither side and moves no remote;
// the detached child's push returns nil at once with nothing pushed and a pending.records flag left on disk;
// SkipPush and SkipGit return at once;
// and an unbounded wait returns only once the lock is released and then pushes.
func TestPushLock_BoundedWaitGivesUpAndUnboundedWaitBlocks(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	push := func(l *lymcwd.Location, opts fabricengine.SyncOptions, pairPush fabricengine.PairPush) error {
		_, err := fabricengine.PushPairAnchored(l, opts, pairPush)
		return err
	}

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

	err = push(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: 50 * time.Millisecond})
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

	childErr := func() error {
		_, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WarpPath: p.warpPath, WeftPath: p.weftPath}, fabricengine.SyncOptions{}, fabricengine.MachineNames{})
		return err
	}()
	if childErr != nil {
		t.Errorf("CoalescePushRequest under a held lock error = %v; want nil at once", childErr)
	}
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.RecordsBare, p.weftBranch); got != weftBefore {
		t.Errorf("records bare after the child = %q; want it unmoved at %q", got, weftBefore)
	}
	recordsFlag := filepath.Join(filepath.Dir(fabricengine.PushLockPathForTest(t, p.weftPath)), "pending.records")
	if _, err := os.Stat(recordsFlag); err != nil {
		t.Errorf("pending.records after the child found the lock held: %v; want it left on disk", err)
	}

	for _, opts := range []fabricengine.SyncOptions{{SkipPush: true}, {SkipGit: true}} {
		if err := push(p.location, opts, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
			t.Errorf("push with %+v under a held lock error = %v; want nil at once", opts, err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- push(p.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded})
	}()
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
	if got := fabricengine.BareBranchSHAForTest(t, p.hub.CodeBare, p.warpBranch); got != warpSHA {
		t.Errorf("warp bare = %q; want %q after the release", got, warpSHA)
	}

	// A pair push whose lock wait gave up leaves pending.code.
	// A records-only push of the same worktree takes and releases the lock.
	// It leaves that flag and origin's code branch alone.
	// A pair push of the worktree then lands its records side first and the code after.
	t.Run("ACodeFlagLeftByAGivenUpPairPushStaysUntilAPairPush", func(t *testing.T) {
		t.Parallel()
		q := newPushPair(t)
		codeBefore := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch)
		warpSHA := gitkit.CommitFile(t, q.warpPath, "warp-file.txt", "warp change", "warp change")
		weftSHA := gitkit.CommitFile(t, q.weftPath, "weft-file.txt", "weft change", "weft change")
		pushLockPath := fabricengine.PushLockPathForTest(t, q.weftPath)
		codeFlag := filepath.Join(filepath.Dir(pushLockPath), "pending.code")

		held, err := lock.AcquireWriteLock(pushLockPath)
		if err != nil {
			t.Fatalf("hold push lock: %v", err)
		}
		_, err = fabricengine.PushPairAnchored(q.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: 50 * time.Millisecond})
		if !errors.Is(err, fabricengine.ErrPushLockBusy) {
			t.Fatalf("bounded push under a held lock error = %v; want it to satisfy errors.Is(err, ErrPushLockBusy)", err)
		}
		if err := held.Release(); err != nil {
			t.Fatalf("release push lock: %v", err)
		}
		if _, err := os.Stat(codeFlag); err != nil {
			t.Fatalf("pending.code after the given-up pair push: %v; want it left on disk", err)
		}

		if _, err := fabricengine.CoalescePushRequest(fabricengine.PushSpawn{WeftPath: q.weftPath}, fabricengine.SyncOptions{}, fabricengine.MachineNames{}); err != nil {
			t.Fatalf("records-only CoalescePushRequest error = %v; want nil", err)
		}
		relock, ok, err := lock.TryAcquireWriteLock(pushLockPath)
		if err != nil || !ok {
			t.Fatalf("push lock after the records-only push = (%v, %v); want it released", ok, err)
		}
		_ = relock.Release()
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.RecordsBare, q.weftBranch); got != weftSHA {
			t.Errorf("weft bare = %q; want the records-only push to land %q", got, weftSHA)
		}
		if _, err := os.Stat(codeFlag); err != nil {
			t.Errorf("pending.code after the records-only push: %v; want it left for the worktree's next pair push", err)
		}
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch); got != codeBefore {
			t.Errorf("warp bare = %q; want it unmoved at %q by the records-only push", got, codeBefore)
		}

		if _, err := fabricengine.PushPairAnchored(q.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded}); err != nil {
			t.Fatalf("pair push after the records-only push error = %v; want nil", err)
		}
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch); got != warpSHA {
			t.Errorf("warp bare = %q; want the pair push to land %q", got, warpSHA)
		}
		if _, err := os.Stat(codeFlag); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("pending.code after the pair push: %v; want it taken", err)
		}
	})

	// The pair's teardown lock held exclusive past a short TeardownWait owes the push.
	t.Run("TeardownLockHeldPastTheWaitOwesTheRecordsPush", func(t *testing.T) {
		t.Parallel()
		q := newPushPair(t)
		warpBefore := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch)
		weftBefore := fabricengine.BareBranchSHAForTest(t, q.hub.RecordsBare, q.weftBranch)
		gitkit.CommitFile(t, q.warpPath, "warp-file.txt", "warp change", "warp change")
		gitkit.CommitFile(t, q.weftPath, "weft-file.txt", "weft change", "weft change")
		holdTeardownLock(t, q.hub, "pushpair")

		_, err := fabricengine.PushPairAnchored(q.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded, TeardownWait: teardownTestWait})

		var failed *fabricengine.PushFailedError
		if !errors.As(err, &failed) || failed.Record.Class != fabricengine.PushFailureTeardownLock {
			t.Fatalf("PushPairAnchored() error = %v; want a teardown_lock *PushFailedError", err)
		}
		if !errors.Is(err, fabricengine.ErrPairPushRunning) {
			t.Errorf("PushPairAnchored() error = %v; want it to satisfy errors.Is(err, ErrPairPushRunning)", err)
		}
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch); got != warpBefore {
			t.Errorf("warp bare = %q; want it unmoved at %q", got, warpBefore)
		}
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.RecordsBare, q.weftBranch); got != weftBefore {
			t.Errorf("weft bare = %q; want it unmoved at %q", got, weftBefore)
		}
		record := requireOneFailedPush(t, q.hub.Location.HubPath, "records", fabricengine.PushFailureTeardownLock)
		if !slices.Equal(record.Runs, []string{"pushpair"}) {
			t.Errorf("record Runs = %q; want the pair's slug", record.Runs)
		}
	})

	// The lock released inside TeardownWait lets the push wait, land both sides and leave no record.
	t.Run("TeardownLockReleasedInsideTheWaitPushesBothSides", func(t *testing.T) {
		t.Parallel()
		q := newPushPair(t)
		warpSHA := gitkit.CommitFile(t, q.warpPath, "warp-file.txt", "warp change", "warp change")
		weftSHA := gitkit.CommitFile(t, q.weftPath, "weft-file.txt", "weft change", "weft change")
		release := holdTeardownLock(t, q.hub, "pushpair")

		done := make(chan error, 1)
		go func() {
			_, err := fabricengine.PushPairAnchored(q.location, fabricengine.SyncOptions{}, fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded, TeardownWait: time.Minute})
			done <- err
		}()
		select {
		case err := <-done:
			t.Fatalf("push returned %v while the teardown lock was held; want it to wait", err)
		case <-time.After(150 * time.Millisecond):
		}
		release()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("push after the release error = %v; want nil", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("push still waiting 30s after the teardown lock was released")
		}

		if got := fabricengine.BareBranchSHAForTest(t, q.hub.RecordsBare, q.weftBranch); got != weftSHA {
			t.Errorf("weft bare = %q; want %q after the release", got, weftSHA)
		}
		if got := fabricengine.BareBranchSHAForTest(t, q.hub.CodeBare, q.warpBranch); got != warpSHA {
			t.Errorf("warp bare = %q; want %q after the release", got, warpSHA)
		}
		requireNoFailedPush(t, q.hub.Location.HubPath)
	})
}
