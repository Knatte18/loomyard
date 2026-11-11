// pull_integration_test.go is the end-to-end integration matrix for Fabric.Pull.
// It covers a clean fast-forward and warp history rewrite detection.
// Reconcile is tested with single-back, multi-back, no-surviving-anchor and empty-index cases.
// It also covers idempotency after a reconcile, the double-conflict abort, and the weft-first partial-failure contract.
// The tests reuse the fixture shims in export_test.go and addWarpBareRemote in coalesce_integration_test.go.
// Both files share package fabricengine_test.
// They also use hubforge.NewHub for the weft side.
// Its upstream tracking lets PullWeft's ff-pull no-op cleanly in every test that does not deliberately diverge weft.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// buildReconcileFixture builds a warp+weft pair with a bare warp remote and n
// synced warp<->weft correspondences, returning the Fabric handle, both worktree
// paths, the bare remote's path, the initial warp SHA (the root for no-surviving-anchor),
// and the recorded warp/weft SHA pairs in commit order.
func buildReconcileFixture(t *testing.T, fixturesDir string, n int) (f *fabricengine.Fabric, warpPath, bareDir string, weftFixture *hubforge.Hub, initWarpSHA string, warpSHAs, weftSHAs []string) {
	t.Helper()

	warpPath = fabricengine.NewPlainWarpRepoForTest(t)
	bareDir = addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA = fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture = hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f = fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	for i := 0; i < n; i++ {
		warpSHA := fabricengine.CommitWarpForTest(t, warpPath, fmt.Sprintf("warp change %d", i))
		fabricengine.WriteWeftConfigContentForTest(t, weftFixture.PrimeRecords(), fmt.Sprintf("weft change %d", i))
		weftSHA, committed, err := fabricengine.CommitWeftForTest(f, []string{"_lym"}, fabricengine.DefaultCommitMessage, fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("commitWeft() round %d error = %v", i, err)
		}
		if !committed {
			t.Fatalf("commitWeft() round %d committed = false; want true", i)
		}
		warpSHAs = append(warpSHAs, warpSHA)
		weftSHAs = append(weftSHAs, weftSHA)
	}

	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")
	return f, warpPath, bareDir, weftFixture, initWarpSHA, warpSHAs, weftSHAs
}

// rewriteWarpRemoteHistory simulates an upstream rebase or force-push.
// It clones bareDir fresh, resets that clone to resetToSHA, commits one new distinct commit, and force-pushes.
// The bare remote's main branch then holds a history that diverges from resetToSHA.
// It does not descend from the tip it had before.
// Returns the new remote tip SHA.
func rewriteWarpRemoteHistory(t *testing.T, fixturesDir, bareDir, resetToSHA string) string {
	t.Helper()

	clone := filepath.Join(fixturesDir, "warp-clone-rewrite")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	gitkit.MustRun(t, clone, "git", "reset", "--hard", resetToSHA)
	gitkit.CommitFile(t, clone, "rewritten.txt", "rewritten history", "rewritten history")
	gitkit.MustRun(t, clone, "git", "push", "--force", "origin", "main")
	return fabricengine.CurrentSHAForTest(t, clone)
}

// TestPull_AbortsOnUnpushedPlusDiverged covers the double-conflict abort.
// Local warp holds a commit that no value of the tracking ref's reflog reaches,
// and the remote has diverged, judged after the fetch.
// Fabric.Pull must return fabricengine.ErrWarpDivergedUnpushed and mutate neither repo.
func TestPull_AbortsOnUnpushedPlusDiverged(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CommitWarpForTest(t, warpPath, "local unpushed change")
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrWarpDivergedUnpushed) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrWarpDivergedUnpushed)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_NoSurvivingAnchorAborts covers a rewrite so thorough that no recorded correspondence entry survives.
// Fabric.Pull must return fabricengine.ErrNoSurvivingAnchor and mutate neither repo.
func TestPull_NoSurvivingAnchorAborts(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, initWarpSHA, _, _ := buildReconcileFixture(t, fixturesDir, 2)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)
	preWeftHEAD := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords())

	rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	_, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrNoSurvivingAnchor) {
		t.Fatalf("Pull() error = %v; want errors.Is(err, fabricengine.ErrNoSurvivingAnchor)", err)
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after aborted Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != preWeftHEAD {
		t.Errorf("weft HEAD after aborted Pull() = %q; want unchanged %q", got, preWeftHEAD)
	}
}

// TestPull_DirtyWarpRefusesBeforeMovingWarp guards against data loss on a routine fast-forward.
// With a modified tracked file in the warp worktree and an advanced remote, Pull must return fabricengine.ErrWarpDirty.
// Warp HEAD must stay unmoved, and the modification must stay intact on disk.
//
//testtiming:keep the user's uncommitted edit to a tracked warp file surviving a refused Pull byte for byte; coverage of its blocks by other tests does not show an assertion of this
func TestPull_DirtyWarpRefusesBeforeMovingWarp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, _, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)

	preWarpHEAD := fabricengine.CurrentSHAForTest(t, warpPath)

	clone := filepath.Join(fixturesDir, "warp-clone-dirty-ff")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	// Dirty a tracked warp file, the state a fast-forward must not overwrite.
	dirtyFile := filepath.Join(warpPath, "README")
	const dirtyContent = "uncommitted local edit that must survive pull"
	if err := os.WriteFile(dirtyFile, []byte(dirtyContent), 0o644); err != nil {
		t.Fatalf("dirty tracked file: %v", err)
	}

	result, err := f.Pull(fabricengine.SyncOptions{})
	if !errors.Is(err, fabricengine.ErrWarpDirty) {
		t.Fatalf("Pull() error = %v; want fabricengine.ErrWarpDirty", err)
	}
	if result.WarpAdvanced {
		t.Errorf("Pull() WarpAdvanced = true; want false (refused before moving warp)")
	}

	if got := fabricengine.CurrentSHAForTest(t, warpPath); got != preWarpHEAD {
		t.Errorf("warp HEAD after refused Pull() = %q; want unchanged %q", got, preWarpHEAD)
	}
	data, readErr := os.ReadFile(dirtyFile)
	if readErr != nil {
		t.Fatalf("read dirtied file after Pull(): %v", readErr)
	}
	if string(data) != dirtyContent {
		t.Errorf("uncommitted warp edit was destroyed by Pull(): got %q; want %q", data, dirtyContent)
	}
}

// TestPull_EmptyIndexNoDrift covers a non-fast-forward remote with an empty correspondence index.
// The warp commits were never synced to weft, so warp must still advance, with no reconcile commit written.
func TestPull_EmptyIndexNoDrift(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	bareDir := addWarpBareRemote(t, fixturesDir, warpPath)
	initWarpSHA := fabricengine.CurrentSHAForTest(t, warpPath)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// Warp commits happen, but nothing is ever synced to weft, so the correspondence index stays empty.
	fabricengine.CommitWarpForTest(t, warpPath, "warp change never synced 1")
	fabricengine.CommitWarpForTest(t, warpPath, "warp change never synced 2")
	gitkit.MustRun(t, warpPath, "git", "push", "origin", "main")

	newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, initWarpSHA)

	result, err := f.Pull(fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if !result.RewriteDetected {
		t.Errorf("Pull() RewriteDetected = false; want true")
	}
	if result.Reconciled {
		t.Errorf("Pull() Reconciled = true; want false (empty index)")
	}
	if !result.WarpAdvanced {
		t.Errorf("Pull() WarpAdvanced = false; want true")
	}
	if result.NewWarpHEAD != newTip {
		t.Errorf("Pull() NewWarpHEAD = %q; want %q", result.NewWarpHEAD, newTip)
	}
}

// TestPull_WeftDivergedAndWarpFetchFails_PartialError forces a weft replay stop combined with a warp-side failure.
// The stop comes from a local weft commit and an upstream commit that change the same path.
// The warp failure is warpPath carrying no configured remote, so f.code.Fetch() itself fails.
// The weft stop never stalls the warp side.
// The combined failure must surface as a *fabricengine.WeftStopError.
// It must also surface as a *PartialPullError whose WeftPulled is false.
// Its Error() names both failures rather than claiming the weft pull succeeded.
func TestPull_WeftDivergedAndWarpFetchFails_PartialError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	warpPath := fabricengine.NewPlainWarpRepoForTest(t)
	weftFixture := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f := fabricengine.NewFabricForTest(t, warpPath, weftFixture.PrimeRecords())

	// A real hub's weft primary checks out the suffixed branch (fabricengine.RecordsBranchName), never bare "main".
	// That branch carries no upstream until something pushes it.
	// A fresh hub's primary stays local-only until the first real push.
	// Establish that upstream explicitly here.
	// Without it, weftHasUpstream() reports false, so the weft pull is skipped as a vacuous success.
	// The genuine divergence this test needs never happens.
	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftFixture.PrimeRecords(), "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	gitkit.CommitFile(t, cloneB, "status.json", "from clone b", "b")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, on the same path, so the replay of the local commit conflicts.
	gitkit.CommitFile(t, weftFixture.PrimeRecords(), "status.json", "local weft change", "local weft change")

	// warpPath has no configured remote, so f.code.Fetch() fails and gives this test its warp-side failure.
	result, err := f.Pull(fabricengine.SyncOptions{})
	var stopErr *fabricengine.WeftStopError
	if !errors.As(err, &stopErr) {
		t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError among the joined errors", err, err)
	}
	var partialErr *fabricengine.PartialPullError
	if !errors.As(err, &partialErr) {
		t.Fatalf("Pull() error = %v (%T); want a *fabricengine.PartialPullError among the joined errors", err, err)
	}
	if partialErr.WeftPulled {
		t.Errorf("PartialPullError.WeftPulled = true; want false (the weft pull failed to fast-forward)")
	}
	if result.WeftPulled {
		t.Errorf("Pull() result.WeftPulled = true; want false")
	}
	if strings.Contains(partialErr.Error(), "weft pull succeeded") {
		t.Errorf("PartialPullError.Error() = %q; must not claim the weft pull succeeded", partialErr.Error())
	}
}

// divergedWeftFixture is a reconcile fixture whose weft branch moved both locally and on origin.
// Its warp remote advanced by one commit.
type divergedWeftFixture struct {
	f           *fabricengine.Fabric
	warpPath    string
	weftPath    string
	weftBranch  string
	originSHA   string
	localSHA    string
	warpFFSHA   string
	weftHeadWas string
}

// removeMarker is a marker value that asks for the weft's anchor marker to be removed.
const removeMarker = "<remove the marker>"

// commitWeftSide commits rel with content, then writes the anchor marker in the weft checkout at dir.
// An empty rel commits no file, and an empty marker leaves the marker alone.
// removeMarker deletes the marker, and any other value is written.
// It returns the checkout's HEAD.
func commitWeftSide(t *testing.T, dir, rel, content, marker, message string) string {
	t.Helper()

	if rel != "" {
		gitkit.CommitFile(t, dir, rel, content, message)
	}
	switch marker {
	case "":
	case removeMarker:
		gitkit.Git(t, dir, "rm", "--quiet", lymcwd.AnchorFileName)
		gitkit.Git(t, dir, "commit", "--quiet", "-m", message+": drop the marker")
	default:
		gitkit.CommitFile(t, dir, lymcwd.AnchorFileName, marker+"\n", message+": set the marker")
	}
	return gitkit.RevParse(t, dir, "HEAD")
}

// newDivergedWeftFixture commits originRel on the weft's origin, from a second clone.
// It commits localRel on this machine's weft; originContent and localContent are the files' contents.
// originMarker and localMarker, when set, change the anchor marker in a later commit on their side;
// removeMarker deletes it.
// An empty originRel and originMarker mean origin gains no commit,
// and an empty localRel and localMarker mean the weft gains none.
// It then advances warp's remote with a clean commit, so a pull runs both of its arms.
func newDivergedWeftFixture(t *testing.T, originRel, originContent, originMarker, localRel, localContent, localMarker string) divergedWeftFixture {
	t.Helper()
	boardkit.InstallReplay(t)

	fixturesDir := t.TempDir()
	f, warpPath, bareDir, weftFixture, _, _, _ := buildReconcileFixture(t, fixturesDir, 1)
	weftPath := weftFixture.PrimeRecords()

	weftBranch := fabricengine.RecordsBranchName("main")
	gitkit.MustRun(t, weftPath, "git", "push", "-u", "origin", weftBranch)

	cloneB := filepath.Join(fixturesDir, "weft-diverge-cloneB")
	gitkit.MustRun(t, fixturesDir, "git", "clone", "-q", "-b", weftBranch, weftFixture.RecordsBare, cloneB)
	gitkit.Git(t, cloneB, "config", "user.email", "test@test.com")
	gitkit.Git(t, cloneB, "config", "user.name", "Test")
	originSHA := commitWeftSide(t, cloneB, originRel, originContent, originMarker, "origin weft change")
	gitkit.MustRun(t, cloneB, "git", "push", "-q")

	// Diverge local weft too, so `git pull --ff-only` cannot fast-forward.
	localSHA := commitWeftSide(t, weftPath, localRel, localContent, localMarker, "local weft change")

	// Advance warp's remote with a clean, non-rewritten commit.
	clone := filepath.Join(fixturesDir, "warp-clone-weft-diverge")
	gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
	gitkit.Git(t, clone, "config", "user.email", "test@test.com")
	gitkit.Git(t, clone, "config", "user.name", "Test")
	warpFFSHA := gitkit.CommitFile(t, clone, "ff-file.txt", "ff change", "ff change")
	gitkit.MustRun(t, clone, "git", "push")

	return divergedWeftFixture{
		f: f, warpPath: warpPath, weftPath: weftPath, weftBranch: gitkit.CurrentBranch(t, weftPath),
		originSHA: originSHA, localSHA: localSHA, warpFFSHA: warpFFSHA, weftHeadWas: localSHA,
	}
}

// TestPull_WeftDivergedWarpAdvancesCleanly covers a weft that this machine and origin both moved.
// The weft's local commit replays onto origin's and the pull succeeds.
// A replay conflict and an uncommitted change on a replayed path each return a *WeftStopError.
// The warp arm runs before the stop, and the weft stays where it was.
func TestPull_WeftDivergedWarpAdvancesCleanly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("A local weft commit replays onto origin's and the pull succeeds", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (the local weft commit replays)", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want true (the weft was brought level)")
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != fx.warpFFSHA {
			t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q", result.WarpAdvanced, result.NewWarpHEAD, fx.warpFFSHA)
		}
		if got := gitkit.RevParse(t, fx.weftPath, "HEAD~1"); got != fx.originSHA {
			t.Errorf("weft HEAD~1 after Pull() = %q; want origin's commit %q under the replayed one", got, fx.originSHA)
		}
		for _, rel := range []string{"from-clone-b.txt", "local-only.txt"} {
			if _, err := os.Stat(filepath.Join(fx.weftPath, rel)); err != nil {
				t.Errorf("weft after Pull() lacks %s: %v", rel, err)
			}
		}
	})

	t.Run("A both-sides change returns a weft stop after warp advanced", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "status.json", "from origin", "", "status.json", "from this machine", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError", err, err)
		}
		if !errors.Is(err, fabricengine.ErrReplayConflict) {
			t.Errorf("Pull() error = %v; want errors.Is(err, ErrReplayConflict)", err)
		}
		if !result.WarpAdvanced || result.NewWarpHEAD != fx.warpFFSHA {
			t.Errorf("Pull() WarpAdvanced=%v NewWarpHEAD=%q; want warp advanced to %q before the weft stop is returned", result.WarpAdvanced, result.NewWarpHEAD, fx.warpFFSHA)
		}
		if result.WeftPulled {
			t.Errorf("Pull() WeftPulled = true; want false beside a weft stop")
		}
		slug, ok := fabricengine.WeftWarpSlug(filepath.Base(fx.weftPath))
		if !ok {
			t.Fatalf("WeftWarpSlug(%q) ok = false; want the weft sibling's warp slug", filepath.Base(fx.weftPath))
		}
		loc, err := lymcwd.ResolveWorktree(filepath.Join(filepath.Dir(fx.weftPath), slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%s): %v", slug, err)
		}
		pushVerb := "`lym fabric push` in " + loc.AnchorPath()
		wantLine := fabricengine.RecordsConflictWayForwardForTest(fx.weftPath, fx.weftBranch, []string{"status.json"}, []string{fx.localSHA}, pushVerb)
		if !strings.Contains(err.Error(), wantLine) {
			t.Errorf("Pull() error = %q; want it to carry the way forward %q", err.Error(), wantLine)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
	})

	t.Run("An uncommitted change on a replayed path returns a weft stop naming it", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		dirtyFile := filepath.Join(fx.weftPath, "local-only.txt")
		if err := os.WriteFile(dirtyFile, []byte("uncommitted edit"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", dirtyFile, err)
		}

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError", err, err)
		}
		if !errors.Is(err, fabricengine.ErrReplayDirty) {
			t.Errorf("Pull() error = %v; want errors.Is(err, ErrReplayDirty)", err)
		}
		if len(stop.Paths) != 1 || stop.Paths[0] != "local-only.txt" || !strings.Contains(err.Error(), "local-only.txt") {
			t.Errorf("WeftStopError paths = %v, message %q; want local-only.txt named", stop.Paths, err.Error())
		}
		if !result.WarpAdvanced {
			t.Errorf("Pull() WarpAdvanced = false; want the warp arm to run beside the weft stop")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
		data, readErr := os.ReadFile(dirtyFile)
		if readErr != nil || string(data) != "uncommitted edit" {
			t.Errorf("uncommitted weft edit after the stop = %q, %v; want it kept", data, readErr)
		}
	})

	t.Run("A marker change on either side stops the pull before warp or the weft moves", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			// fixture builds the diverged weft for this case, with the marker changes it sets.
			fixture func(t *testing.T) divergedWeftFixture
			// wantSet is the anchor the way forward must name.
			wantSet string
		}{
			{
				"OriginChangesTheMarkerAndTheWeftHasNoLocalCommit",
				func(t *testing.T) divergedWeftFixture {
					return newDivergedWeftFixture(t, "from-clone-b.txt", "b", "sub", "", "", "")
				},
				"sub",
			},
			{
				"BothSidesChangeItDifferently",
				func(t *testing.T) divergedWeftFixture {
					return newDivergedWeftFixture(t, "from-clone-b.txt", "b", "sub", "local-only.txt", "local", "other")
				},
				"other",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx := tt.fixture(t)
				warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)

				result, err := fx.f.Pull(fabricengine.SyncOptions{})

				var stop *fabricengine.WeftStopError
				if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
					t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
				}
				for _, want := range []string{"lym fabric anchor", "--set " + tt.wantSet} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Pull() error = %q; want it to name %q", err.Error(), want)
					}
				}
				if result.WarpAdvanced || result.WeftPulled {
					t.Errorf("Pull() WarpAdvanced=%v WeftPulled=%v; want neither side moved", result.WarpAdvanced, result.WeftPulled)
				}
				if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
					t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
				}
				if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
					t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
				}
			})
		}
	})

	t.Run("Markerless commits whose content sits at the local marker replay with no stop", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", removeMarker, "local-only.txt", "local weft change", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil, the markerless tip compares equal by its content's anchor", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want the weft brought level")
		}
	})

	t.Run("A weft ahead of origin with a re-anchor not yet pushed is no stop", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "", "", "", "", "", "sub")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil, origin has nothing the weft lacks", err)
		}
		if !result.WeftPulled || !result.WarpAdvanced {
			t.Errorf("Pull() WeftPulled=%v WarpAdvanced=%v; want both arms run", result.WeftPulled, result.WarpAdvanced)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after Pull() = %q; want the unpushed re-anchor %q kept", got, fx.weftHeadWas)
		}
	})

	t.Run("An empty own marker stops the pull naming the marker file", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)
		if err := os.WriteFile(filepath.Join(fx.weftPath, lymcwd.AnchorFileName), []byte("\n"), 0o644); err != nil {
			t.Fatalf("truncate the marker: %v", err)
		}

		result, err := fx.f.Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
		}
		if !strings.Contains(err.Error(), lymcwd.AnchorFileName) {
			t.Errorf("Pull() error = %q; want it to name %s", err.Error(), lymcwd.AnchorFileName)
		}
		if result.WarpAdvanced {
			t.Errorf("Pull() WarpAdvanced = true; want warp left alone")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
			t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
		}
	})

	t.Run("A move record on the weft stops the pull before warp or the weft moves", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
		warpHeadWas := fabricengine.CurrentSHAForTest(t, fx.warpPath)
		writeMoveRecord(t, fx.weftPath, fabricengine.MoveRecord{A: ".", B: "x", Kind: fabricengine.MoveKindMove, PID: 4242})

		result, err := fx.f.Pull(fabricengine.SyncOptions{})

		var stop *fabricengine.WeftStopError
		if !errors.As(err, &stop) || !errors.Is(err, fabricengine.ErrAnchorChange) {
			t.Fatalf("Pull() error = %v (%T); want a *fabricengine.WeftStopError wrapping ErrAnchorChange", err, err)
		}
		if !strings.Contains(stop.WayForward, "lym fabric anchor") || !strings.Contains(stop.WayForward, "--set x") {
			t.Errorf("WeftStopError way forward = %q; want the move record's `lym fabric anchor` re-run", stop.WayForward)
		}
		if result.WarpAdvanced || result.WeftPulled {
			t.Errorf("Pull() WarpAdvanced=%v WeftPulled=%v; want neither side moved", result.WarpAdvanced, result.WeftPulled)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.warpPath); got != warpHeadWas {
			t.Errorf("warp HEAD after the stop = %q; want unchanged %q", got, warpHeadWas)
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.weftHeadWas {
			t.Errorf("weft HEAD after the stop = %q; want unchanged %q", got, fx.weftHeadWas)
		}
	})
}

// TestPull_WeftUpstreamUnreadableIsAnError covers a weft whose pull fails because origin is unreachable.
// Its tracking ref names an object the repository lacks, so its commits against head cannot be listed.
// A failed local read is an infrastructure error: Pull returns it rather than a nil error with WeftPulled false.
func TestPull_WeftUpstreamUnreadableIsAnError(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	fx := newDivergedWeftFixture(t, "from-clone-b.txt", "b", "", "local-only.txt", "local weft change", "")
	gitkit.Git(t, fx.weftPath, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "no-such-origin"))
	trackingRef := strings.TrimSpace(gitkit.Git(t, fx.weftPath, "rev-parse", "--path-format=absolute", "--git-path", "refs/remotes/origin/"+fx.weftBranch))
	if err := os.MkdirAll(filepath.Dir(trackingRef), 0o755); err != nil {
		t.Fatalf("make the tracking ref's directory: %v", err)
	}
	if err := os.WriteFile(trackingRef, []byte(strings.Repeat("a", 40)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", trackingRef, err)
	}

	result, err := fx.f.Pull(fabricengine.SyncOptions{})
	if err == nil || !strings.Contains(err.Error(), "against its upstream") {
		t.Fatalf("Pull() = %+v, %v; want an error that the weft's state against its upstream could not be read", result, err)
	}
	if result.WeftPulled {
		t.Errorf("Pull() WeftPulled = true; want false beside the error")
	}
}

// TestPull_ReflogJudgesUnpushedAndRewinds covers the pull decisions that rest on the reflog.
// A branch fetched but never pushed from this clone is not unpushed and takes the anchor walk.
// A task branch origin rewound is refused with ErrWarpRewound before and after its tracking ref was fetched.
// A weft whose only commits replay empty pulls clean.
func TestPull_ReflogJudgesUnpushedAndRewinds(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	t.Run("A fetched, never-pushed branch with a rewritten head takes the anchor walk", func(t *testing.T) {
		t.Parallel()
		fixturesDir := t.TempDir()
		_, _, bareDir, weftFixture, _, warpSHAs, _ := buildReconcileFixture(t, fixturesDir, 2)

		clone := filepath.Join(fixturesDir, "warp-clone-never-pushed")
		gitkit.MustRun(t, fixturesDir, "git", "clone", bareDir, clone)
		f := fabricengine.NewFabricForTest(t, clone, weftFixture.PrimeRecords())
		// A clone records no reflog entry for its tracking ref, so the ref is recreated by a fetch.
		// That matches a checkout that fetched its branch and never pushed it.
		gitkit.MustRun(t, clone, "git", "update-ref", "-d", "refs/remotes/origin/main")
		gitkit.MustRun(t, clone, "git", "fetch")
		newTip := rewriteWarpRemoteHistory(t, fixturesDir, bareDir, warpSHAs[0])
		gitkit.MustRun(t, clone, "git", "fetch")

		result, err := f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (no local commit is unpushed)", err)
		}
		if !result.Reconciled || !result.WarpAdvanced || result.NewWarpHEAD != newTip {
			t.Errorf("Pull() Reconciled=%v WarpAdvanced=%v NewWarpHEAD=%q; want a reconcile to %q", result.Reconciled, result.WarpAdvanced, result.NewWarpHEAD, newTip)
		}
		if got := fabricengine.CurrentSHAForTest(t, clone); got != newTip {
			t.Errorf("warp HEAD after Pull() = %q; want origin's head %q", got, newTip)
		}
		if got := fabricengine.CurrentSHAForTest(t, weftFixture.PrimeRecords()); got != result.ReanchorWeftSHA {
			t.Errorf("weft HEAD after Pull() = %q; want the re-anchor commit %q", got, result.ReanchorWeftSHA)
		}
	})

	for _, fetchFirst := range []bool{false, true} {
		name := "A rewound task branch is refused before its tracking ref was fetched"
		if fetchFirst {
			name = "A rewound task branch is refused after its tracking ref was fetched"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPair(t, hub, "task")
			pairCode := hub.PairCodeWorktree("task")
			branch := gitkit.CurrentBranch(t, pairCode)
			gitkit.Git(t, pairCode, "branch", "--set-upstream-to=origin/"+branch)
			base := gitkit.RevParse(t, pairCode, "HEAD")
			first := gitkit.CommitFile(t, pairCode, "one.txt", "one", "one")
			second := gitkit.CommitFile(t, pairCode, "two.txt", "two", "two")
			gitkit.Git(t, pairCode, "push", "origin", branch)

			other := filepath.Join(t.TempDir(), "other")
			gitkit.Git(t, filepath.Dir(other), "clone", hub.CodeBare, other)
			gitkit.Git(t, other, "push", "--force-with-lease=refs/heads/"+branch+":"+second, "origin", base+":refs/heads/"+branch)
			if fetchFirst {
				gitkit.Git(t, pairCode, "fetch")
			}

			loc, err := lymcwd.ResolveWorktree(pairCode)
			if err != nil {
				t.Fatalf("ResolveWorktree(%s): %v", pairCode, err)
			}
			f, err := fabricengine.Open(loc)
			if err != nil {
				t.Fatalf("Open(%s): %v", pairCode, err)
			}
			weftHeadBefore := fabricengine.CurrentSHAForTest(t, hub.PairRecordsSibling("task"))

			_, err = f.Pull(fabricengine.SyncOptions{})
			if !errors.Is(err, fabricengine.ErrWarpRewound) {
				t.Fatalf("Pull() error = %v; want errors.Is(err, ErrWarpRewound)", err)
			}
			wantLine := fabricengine.RewoundWayForwardForTest(pairCode, pairCode, branch, base, []string{second, first})
			if !strings.Contains(err.Error(), wantLine) {
				t.Errorf("Pull() error = %q; want it to carry the way forward %q", err.Error(), wantLine)
			}
			if got := fabricengine.CurrentSHAForTest(t, pairCode); got != second {
				t.Errorf("task branch HEAD after the refusal = %q; want unchanged %q", got, second)
			}
			if got := fabricengine.CurrentSHAForTest(t, hub.PairRecordsSibling("task")); got != weftHeadBefore {
				t.Errorf("weft HEAD after the refusal = %q; want unchanged %q", got, weftHeadBefore)
			}
		})
	}

	t.Run("A weft with only commits that replay empty pulls clean", func(t *testing.T) {
		t.Parallel()
		fx := newDivergedWeftFixture(t, "same.txt", "same", "", "same.txt", "same", "")

		result, err := fx.f.Pull(fabricengine.SyncOptions{})
		if err != nil {
			t.Fatalf("Pull() error = %v; want nil (the local commit replays empty)", err)
		}
		if !result.WeftPulled {
			t.Errorf("Pull() WeftPulled = false; want true")
		}
		if got := fabricengine.CurrentSHAForTest(t, fx.weftPath); got != fx.originSHA {
			t.Errorf("weft HEAD after Pull() = %q; want origin's head %q", got, fx.originSHA)
		}
	})
}
