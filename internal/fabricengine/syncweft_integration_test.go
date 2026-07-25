//go:build integration

// syncweft_integration_test.go — integration coverage for the parts of the
// weft-git surface that genuinely need git spawned: the Warp-SHA trailer
// landing in a real commit, RebuildIndex reconstructing what an incremental
// build produces, SyncWeft recording the post-push (not pre-push) SHA
// across a rebase-recovered push, the detached-commit path's self-healing
// lookup, staleness surviving a rebuild, and RevertWithWeft's end-to-end
// mutation behavior including its rollback-on-failure path. Package
// fabricengine (internal) because it needs the unexported index plumbing
// (corrIndexPath, loadCorrIndex, weftGitDir) alongside the exported Fabric
// surface. Reuses index_integration_test.go's fixture helpers
// (newPlainWarpRepo, commitWarp, commitWeftWithTrailer, currentSHA,
// newFabric) rather than redefining them, since both files share this
// package.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxtest"
)

// writeWeftConfigContent overwrites the tracked _lyx/config.yaml file
// CopyWeft fixtures ship with, without staging or committing — the standard
// way this file's tests dirty a weft worktree's pathspec-covered content
// before calling CommitWeft/SyncWeft.
func writeWeftConfigContent(t *testing.T, weftPath, content string) {
	t.Helper()

	configPath := filepath.Join(weftPath, "_lyx", "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// commitMessageAt returns rev's full raw commit message (subject + body +
// trailers) in repoPath, via `git log --format=%B`.
func commitMessageAt(t *testing.T, repoPath, rev string) string {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%B", rev)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log -1 --format=%%B %s in %s: %v", rev, repoPath, err)
	}
	return string(out)
}

// parentOf returns sha's sole first parent in repoPath, via `git rev-parse
// <sha>^`.
func parentOf(t *testing.T, repoPath, sha string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", sha+"^")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s^ in %s: %v", sha, repoPath, err)
	}
	return strings.TrimSpace(string(out))
}

// TestSyncWeft_WritesTrailer asserts that a SyncWeft commit's message
// carries the Warp-SHA trailer, verified both by reading the raw commit
// message and by round-tripping it through git's own trailer machinery
// (`git interpret-trailers --parse`), not just string containment.
func TestSyncWeft_WritesTrailer(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

	res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil {
		t.Fatalf("SyncWeft() error = %v", err)
	}
	if !res.Committed {
		t.Fatalf("SyncWeft() Committed = false; want true")
	}

	rawMessage := commitMessageAt(t, weftFixture.WeftPath, "HEAD")
	wantTrailer := WarpSHATrailerKey + ": " + warpSHA
	if !strings.Contains(rawMessage, wantTrailer) {
		t.Errorf("commit message = %q; want it to contain %q", rawMessage, wantTrailer)
	}

	// Cross-check via git's own trailer machinery rather than string
	// matching alone, per the discussion's "git interpret-trailers" scan
	// contract.
	interpretCmd := exec.Command("git", "interpret-trailers", "--parse")
	interpretCmd.Dir = weftFixture.WeftPath
	interpretCmd.Stdin = strings.NewReader(rawMessage)
	trailerOut, err := interpretCmd.Output()
	if err != nil {
		t.Fatalf("git interpret-trailers --parse: %v", err)
	}
	if !strings.Contains(string(trailerOut), wantTrailer) {
		t.Errorf("git interpret-trailers --parse output = %q; want it to contain %q", trailerOut, wantTrailer)
	}
}

// TestRebuildIndex_EqualsIncrementallyBuiltIndex asserts that after several
// SyncWeft rounds (each incrementally updating the index via
// RecordCorrespondence), a full RebuildIndex reconstructs an index whose
// entries are identical to the incrementally-built one.
func TestRebuildIndex_EqualsIncrementallyBuiltIndex(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	for i := 0; i < 3; i++ {
		commitWarp(t, warpPath, fmt.Sprintf("warp round %d", i))
		writeWeftConfigContent(t, weftFixture.WeftPath, fmt.Sprintf("weft round %d", i))
		res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
		if err != nil {
			t.Fatalf("SyncWeft() round %d error = %v", i, err)
		}
		if !res.Committed {
			t.Fatalf("SyncWeft() round %d Committed = false; want true", i)
		}
	}

	path, err := f.corrIndexPath()
	if err != nil {
		t.Fatalf("corrIndexPath() error = %v", err)
	}
	incremental, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() error = %v", err)
	}
	wantEntries := incremental.entries()
	if len(wantEntries) != 3 {
		t.Fatalf("incrementally-built index has %d entries; want 3", len(wantEntries))
	}

	if err := f.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuilt, err := loadCorrIndex(path)
	if err != nil {
		t.Fatalf("loadCorrIndex() (post-rebuild) error = %v", err)
	}
	gotEntries := rebuilt.entries()

	if len(gotEntries) != len(wantEntries) {
		t.Fatalf("RebuildIndex() entries = %d; want %d", len(gotEntries), len(wantEntries))
	}
	for i := range wantEntries {
		if gotEntries[i] != wantEntries[i] {
			t.Errorf("entries[%d]: RebuildIndex()=%+v incremental=%+v; want equal", i, gotEntries[i], wantEntries[i])
		}
	}
}

// TestSyncWeft_RecordsPostPushSHA proves SyncWeft records the post-push
// weft SHA, never the pre-push one, when the push recovers via gitrepo's
// rebase-retry. A second, independent clone of the same bare remote
// (push_test.go's cross-clone-rebase technique) pushes ahead first, so
// fabric's own push below is rejected as non-fast-forward and must recover.
// The recorded SHA's parent is asserted to be clone B's pushed commit — the
// rebase-retry's re-parenting — rather than the pre-rebase base, which is
// what "not the pre-push SHA" means concretely.
func TestSyncWeft_RecordsPostPushSHA(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	initialWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	cloneBPath := filepath.Join(t.TempDir(), "cloneB")
	lyxtest.MustRun(t, t.TempDir(), "git", "clone", "-q", weftFixture.Bare, cloneBPath)
	if err := os.WriteFile(filepath.Join(cloneBPath, "from-clone-b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	lyxtest.MustRun(t, cloneBPath, "git", "add", ".")
	lyxtest.MustRun(t, cloneBPath, "git", "commit", "-q", "-m", "from clone B")
	lyxtest.MustRun(t, cloneBPath, "git", "push", "-q")
	cloneBSHA := currentSHA(t, cloneBPath)

	warpSHA := commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

	res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil {
		t.Fatalf("SyncWeft() error = %v", err)
	}
	if !res.Committed || !res.Pushed {
		t.Fatalf("SyncWeft() = %+v; want Committed and Pushed both true", res)
	}

	// A pre-rebase commit's sole parent would have been initialWeftSHA; the
	// rebase-retry re-parents the replayed commit onto clone B's pushed
	// commit instead, which is the concrete proof the recorded SHA is the
	// post-push survivor, not the pre-push one.
	parentSHA := parentOf(t, weftFixture.WeftPath, res.WeftSHA)
	if parentSHA == initialWeftSHA {
		t.Fatalf("recorded weft SHA %s still has the pre-rebase parent %s; want the rebase-retry to have rewritten it", res.WeftSHA, initialWeftSHA)
	}
	if parentSHA != cloneBSHA {
		t.Errorf("recorded weft SHA %s has parent %s; want clone B's pushed commit %s", res.WeftSHA, parentSHA, cloneBSHA)
	}

	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() error = %v", err)
	}
	if got != res.WeftSHA {
		t.Errorf("WeftSHAForWarpSHA() = %q; want the recorded post-push SHA %q", got, res.WeftSHA)
	}
	if !f.Weft.SHAExists(res.WeftSHA) {
		t.Errorf("SHAExists(%q) = false; want true (it is HEAD)", res.WeftSHA)
	}
}

// TestSyncWeft_WarpSHAMatchesTrailer covers the R11 fix: the warp SHA
// SyncWeft reports (and records in the correspondence index) must be the one
// parsed out of the weft commit's own Warp-SHA trailer — never a separate
// re-read of warp HEAD, which could advance between CommitWeft's trailer
// write and the re-read and leave the index disagreeing with the trailer it
// sits beside. Asserted on both the pushed path and the SkipPush path.
func TestSyncWeft_WarpSHAMatchesTrailer(t *testing.T) {
	t.Run("PushedPath", func(t *testing.T) {
		warpPath := newPlainWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		warpSHA := commitWarp(t, warpPath, "warp change")
		writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

		res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
		if err != nil {
			t.Fatalf("SyncWeft() error = %v", err)
		}
		if !res.Committed || !res.Pushed {
			t.Fatalf("SyncWeft() = %+v; want Committed and Pushed both true", res)
		}

		// The result's warp SHA is the trailer's value by construction.
		trailerSHA, ok := parseWarpSHATrailer(commitMessageAt(t, weftFixture.WeftPath, res.WeftSHA))
		if !ok {
			t.Fatalf("pushed weft commit %s carries no Warp-SHA trailer", res.WeftSHA)
		}
		if res.WarpSHA != trailerSHA {
			t.Errorf("SyncWeft() WarpSHA = %q; want the trailer's %q", res.WarpSHA, trailerSHA)
		}
		if res.WarpSHA != warpSHA {
			t.Errorf("SyncWeft() WarpSHA = %q; want the warp HEAD at commit time %q", res.WarpSHA, warpSHA)
		}

		// The index entry recorded beside the trailer names the same pair.
		path, err := f.corrIndexPath()
		if err != nil {
			t.Fatalf("corrIndexPath() error = %v", err)
		}
		ix, err := loadCorrIndex(path)
		if err != nil {
			t.Fatalf("loadCorrIndex() error = %v", err)
		}
		entry, ok := ix.exact(trailerSHA)
		if !ok {
			t.Fatalf("index has no entry for the trailer's warp SHA %q", trailerSHA)
		}
		if entry.WeftSHA != res.WeftSHA {
			t.Errorf("index entry for %q names weft SHA %q; want the post-push %q", trailerSHA, entry.WeftSHA, res.WeftSHA)
		}
	})

	t.Run("SkipPushPath", func(t *testing.T) {
		warpPath := newPlainWarpRepo(t)
		weftFixture := lyxtest.CopyWeft(t)
		f := newFabric(t, warpPath, weftFixture.WeftPath)

		commitWarp(t, warpPath, "warp change")
		writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

		res, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{SkipPush: true})
		if err != nil {
			t.Fatalf("SyncWeft(SkipPush) error = %v", err)
		}
		if !res.Committed || res.Pushed {
			t.Fatalf("SyncWeft(SkipPush) = %+v; want Committed=true Pushed=false", res)
		}

		trailerSHA, ok := parseWarpSHATrailer(commitMessageAt(t, weftFixture.WeftPath, res.WeftSHA))
		if !ok {
			t.Fatalf("weft commit %s carries no Warp-SHA trailer", res.WeftSHA)
		}
		if res.WarpSHA != trailerSHA {
			t.Errorf("SyncWeft(SkipPush) WarpSHA = %q; want the trailer's %q", res.WarpSHA, trailerSHA)
		}
	})
}

// expireAndPruneUnreachable forces git to actually forget any commit
// objects that are no longer reachable from a ref, in repoPath. A plain
// amend/reset alone leaves the superseded object resolvable via the reflog
// for a while — gitrepo/doc.go's own documented SHAExists caveat — so tests
// that need SHAExists to genuinely report false on a rewritten-away SHA
// must call this first.
func expireAndPruneUnreachable(t *testing.T, repoPath string) {
	t.Helper()

	lyxtest.MustRun(t, repoPath, "git", "reflog", "expire", "--expire=now", "--all")
	lyxtest.MustRun(t, repoPath, "git", "gc", "--prune=now", "-q")
}

// TestWeftSHAForWarpSHA_DetachedPathSelfCorrection covers the CLI detached
// path's self-correction: CommitWeft (not SyncWeft) records a pre-push SHA;
// the commit is then amended in place (same trailer, new SHA) and the old
// SHA is forced to genuinely stop resolving via expireAndPruneUnreachable.
// WeftSHAForWarpSHA must heal via one RebuildIndex retry to the surviving
// (amended) trailer commit.
func TestWeftSHAForWarpSHA_DetachedPathSelfCorrection(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA := commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")

	preAmendSHA, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true")
	}

	got, err := f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() (pre-amend) error = %v", err)
	}
	if got != preAmendSHA {
		t.Fatalf("WeftSHAForWarpSHA() (pre-amend) = %q; want %q", got, preAmendSHA)
	}

	// Change the tree before amending — a same-second, same-tree amend can
	// legitimately land on the identical SHA (git's SHA covers tree,
	// parent, author, committer, and message; two amends within the same
	// second can otherwise tie on every field), so this guarantees a
	// genuinely new commit object while --no-edit preserves the trailer.
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change, amended")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", "-A")
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "--amend", "--no-edit")
	postAmendSHA := currentSHA(t, weftFixture.WeftPath)
	if postAmendSHA == preAmendSHA {
		t.Fatalf("amend did not change the weft SHA")
	}
	expireAndPruneUnreachable(t, weftFixture.WeftPath)

	got, err = f.WeftSHAForWarpSHA(warpSHA)
	if err != nil {
		t.Fatalf("WeftSHAForWarpSHA() (post-amend) error = %v", err)
	}
	if got != postAmendSHA {
		t.Errorf("WeftSHAForWarpSHA() (post-amend) = %q; want the surviving trailer commit %q", got, postAmendSHA)
	}
}

// staleCorrespondenceFixture builds a fresh warp/weft pair with exactly one
// recorded correspondence, then rewrites weft's history so that
// correspondence is stale beyond what RebuildIndex can recover: the trailer
// commit is reset away and the orphaned object is genuinely pruned (not
// just amended in place), so a full trailer rescan finds nothing naming
// warpSHA anywhere. Returns the Fabric and the now-stale warpSHA. Each
// caller gets its own fixture so one call's self-correcting side effect
// (WeftSHAForWarpSHA and RevertWithWeft both rebuild the index as part of
// their own staleness handling) cannot leak into the other's assertion.
func staleCorrespondenceFixture(t *testing.T) (f *Fabric, warpSHA string) {
	t.Helper()

	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f = newFabric(t, warpPath, weftFixture.WeftPath)

	baseWeftSHA := currentSHA(t, weftFixture.WeftPath)

	warpSHA = commitWarp(t, warpPath, "warp change")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change")
	weftSHA, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
	if err != nil {
		t.Fatalf("CommitWeft() error = %v", err)
	}
	if !committed {
		t.Fatalf("CommitWeft() committed = false; want true")
	}
	if got, err := f.WeftSHAForWarpSHA(warpSHA); err != nil || got != weftSHA {
		t.Fatalf("WeftSHAForWarpSHA() (pre-rewrite) = %q, %v; want %q, nil", got, err, weftSHA)
	}

	// Discard the trailer commit from history entirely, then force git to
	// genuinely forget the orphaned object — RebuildIndex's scan will find
	// no trailer naming warpSHA anywhere afterwards.
	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "reset", "--hard", baseWeftSHA)
	expireAndPruneUnreachable(t, weftFixture.WeftPath)

	return f, warpSHA
}

// TestWeftSHAForWarpSHA_StalenessSurvivesRebuild covers a history rewrite
// RebuildIndex cannot recover from: WeftSHAForWarpSHA must surface wrapped
// ErrStaleSHA (errors.Is).
func TestWeftSHAForWarpSHA_StalenessSurvivesRebuild(t *testing.T) {
	f, warpSHA := staleCorrespondenceFixture(t)

	if _, err := f.WeftSHAForWarpSHA(warpSHA); !errors.Is(err, ErrStaleSHA) {
		t.Errorf("WeftSHAForWarpSHA() error = %v; want errors.Is(err, ErrStaleSHA)", err)
	}
}

// TestRevertWithWeft_StalenessSurvivesRebuild_NoMutation covers the same
// unrecoverable history rewrite from RevertWithWeft's side: it must surface
// wrapped ErrStaleSHA (errors.Is) and must not have mutated either repo.
func TestRevertWithWeft_StalenessSurvivesRebuild_NoMutation(t *testing.T) {
	f, warpSHA := staleCorrespondenceFixture(t)

	preRevertWarpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatalf("Warp.CurrentSHA() error = %v", err)
	}
	preRevertWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}

	if _, err := f.RevertWithWeft(warpSHA); !errors.Is(err, ErrStaleSHA) {
		t.Errorf("RevertWithWeft() error = %v; want errors.Is(err, ErrStaleSHA)", err)
	}

	postRevertWarpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatalf("Warp.CurrentSHA() error = %v", err)
	}
	postRevertWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatalf("Weft.CurrentSHA() error = %v", err)
	}
	if postRevertWarpSHA != preRevertWarpSHA {
		t.Errorf("warp SHA changed after a failed (stale) RevertWithWeft: %q -> %q; want unchanged", preRevertWarpSHA, postRevertWarpSHA)
	}
	if postRevertWeftSHA != preRevertWeftSHA {
		t.Errorf("weft SHA changed after a failed (stale) RevertWithWeft: %q -> %q; want unchanged", preRevertWeftSHA, postRevertWeftSHA)
	}
}

// TestRevertWithWeft_ExactMatch_ResetsBothRepos covers the exact-match
// resolution path end to end: reverting to a warp SHA with its own
// recorded correspondence resets both repos to exactly that pair.
func TestRevertWithWeft_ExactMatch_ResetsBothRepos(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA1 := commitWarp(t, warpPath, "warp change 1")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change 1")
	res1, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil || !res1.Committed {
		t.Fatalf("first SyncWeft: %+v, err=%v", res1, err)
	}

	commitWarp(t, warpPath, "warp change 2")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change 2")
	res2, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil || !res2.Committed {
		t.Fatalf("second SyncWeft: %+v, err=%v", res2, err)
	}

	rr, err := f.RevertWithWeft(warpSHA1)
	if err != nil {
		t.Fatalf("RevertWithWeft() error = %v", err)
	}
	if !rr.Exact {
		t.Errorf("RevertWithWeft() Exact = false; want true")
	}
	if rr.WarpSHA != warpSHA1 || rr.WeftSHA != res1.WeftSHA {
		t.Errorf("RevertWithWeft() = %+v; want WarpSHA=%q WeftSHA=%q", rr, warpSHA1, res1.WeftSHA)
	}

	warpNow, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	weftNow, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	if warpNow != warpSHA1 || weftNow != res1.WeftSHA {
		t.Errorf("after revert: warp=%q weft=%q; want warp=%q weft=%q", warpNow, weftNow, warpSHA1, res1.WeftSHA)
	}
}

// TestRevertWithWeft_Gap_ResetsToNearestOlderAndReportsRange covers the
// gap-resolution path: reverting to a warp SHA with no correspondence of
// its own falls back to the nearest older synced pair and reports the
// warp-SHA range as a gap.
func TestRevertWithWeft_Gap_ResetsToNearestOlderAndReportsRange(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA1 := commitWarp(t, warpPath, "warp change 1")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change 1")
	res1, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil || !res1.Committed {
		t.Fatalf("SyncWeft: %+v, err=%v", res1, err)
	}

	// A further warp commit with no corresponding weft sync at all.
	warpSHA2 := commitWarp(t, warpPath, "warp change 2, never synced")

	rr, err := f.RevertWithWeft(warpSHA2)
	if err != nil {
		t.Fatalf("RevertWithWeft() error = %v", err)
	}
	if rr.Exact {
		t.Errorf("RevertWithWeft() Exact = true; want false (gap)")
	}
	if rr.WarpSHA != warpSHA2 {
		t.Errorf("RevertWithWeft() WarpSHA = %q; want the requested target %q", rr.WarpSHA, warpSHA2)
	}
	if rr.WeftSHA != res1.WeftSHA {
		t.Errorf("RevertWithWeft() WeftSHA = %q; want the nearest older synced weft SHA %q", rr.WeftSHA, res1.WeftSHA)
	}
	if rr.GapFrom != warpSHA1 || rr.GapTo != warpSHA2 {
		t.Errorf("RevertWithWeft() gap range = [%q, %q]; want [%q, %q]", rr.GapFrom, rr.GapTo, warpSHA1, warpSHA2)
	}

	warpNow, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	weftNow, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	if warpNow != warpSHA2 || weftNow != res1.WeftSHA {
		t.Errorf("after gap revert: warp=%q weft=%q; want warp=%q weft=%q", warpNow, weftNow, warpSHA2, res1.WeftSHA)
	}
}

// TestRevertWithWeft_WeftResetFailure_RollsWarpBack covers the
// partial-failure rollback path: resolution succeeds, warp is reset, but
// the weft ResetHard fails (forced here by pre-creating the weft gitdir's
// index.lock — SHAExists/rev-parse, which resolution depends on, are
// unaffected by a stale index.lock, but `git reset --hard` refuses). Warp
// must be rolled back to its pre-revert SHA, and the failure must not be
// reported as ErrRevertRollbackFailed (the rollback itself succeeds here).
func TestRevertWithWeft_WeftResetFailure_RollsWarpBack(t *testing.T) {
	warpPath := newPlainWarpRepo(t)
	weftFixture := lyxtest.CopyWeft(t)
	f := newFabric(t, warpPath, weftFixture.WeftPath)

	warpSHA1 := commitWarp(t, warpPath, "warp change 1")
	writeWeftConfigContent(t, weftFixture.WeftPath, "weft change 1")
	res1, err := f.SyncWeft(DefaultCommitMessage, []string{"_lyx"}, SyncOptions{})
	if err != nil || !res1.Committed {
		t.Fatalf("SyncWeft: %+v, err=%v", res1, err)
	}

	// Advance warp further so RevertWithWeft(warpSHA1) has real movement to
	// roll back from.
	commitWarp(t, warpPath, "warp change 2")

	preRevertWarpSHA, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	preRevertWeftSHA, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}

	gitDir, err := f.weftGitDir()
	if err != nil {
		t.Fatalf("weftGitDir() error = %v", err)
	}
	lockPath := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatalf("WriteFile(index.lock): %v", err)
	}
	t.Cleanup(func() { os.Remove(lockPath) })

	_, err = f.RevertWithWeft(warpSHA1)
	if err == nil {
		t.Fatal("RevertWithWeft() error = nil; want an error (weft reset should fail)")
	}
	if errors.Is(err, ErrRevertRollbackFailed) {
		t.Fatalf("RevertWithWeft() error = %v; want the warp rollback to succeed (not ErrRevertRollbackFailed)", err)
	}

	warpNow, err := f.Warp.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	weftNow, err := f.Weft.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}
	if warpNow != preRevertWarpSHA {
		t.Errorf("warp SHA after failed revert = %q; want rolled back to the pre-revert SHA %q", warpNow, preRevertWarpSHA)
	}
	if weftNow != preRevertWeftSHA {
		t.Errorf("weft SHA after failed revert = %q; want unchanged at %q", weftNow, preRevertWeftSHA)
	}
}
