//go:build integration

// remove_localmode_integration_test.go covers RemoveLocal, the removal mode that runs no origin step and waits on no other writer, and SnapshotUncommitted with SnapshotTagged.
// TestRemoveLocal swaps package-global hooks, so it is not parallel.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// newLocalModePair returns a hub with one added pair named slug.
func newLocalModePair(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, slug, fabricengine.AddOptions{})
	return h
}

// holdLockForTest takes the write lock at path, creating its directory, and releases it when the test ends.
func holdLockForTest(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of the lock %s: %v", path, err)
	}
	held, err := lock.AcquireWriteLock(path)
	if err != nil {
		t.Fatalf("hold the lock %s: %v", path, err)
	}
	t.Cleanup(func() { _ = held.Release() })
}

// openPairFabric opens the fabric handle on slug's task worktree.
func openPairFabric(t *testing.T, h *hubforge.Hub, slug string) *fabricengine.Fabric {
	t.Helper()

	location, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", h.PairCodeWorktree(slug), err)
	}
	f, err := fabricengine.Open(location)
	if err != nil {
		t.Fatalf("Open(%s): %v", h.PairCodeWorktree(slug), err)
	}
	return f
}

// bareRefs returns the refs of the bare repository at dir, one line each.
func bareRefs(t *testing.T, dir string) string {
	t.Helper()

	return gitkit.Git(t, dir, "for-each-ref")
}

// requirePairIntact asserts both worktrees of slug's pair exist.
func requirePairIntact(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	for _, p := range []string{h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("worktree %s: %v; want the pair intact", p, err)
		}
	}
}

func TestRemoveLocal(t *testing.T) {
	const slug = "localmode"
	const statusRel = "_lyx/shed/" + slug + "/status.json"
	const reportRel = "_lyx/shed/" + slug + "/drive-reports/stop.md"

	t.Run("CommitsRecordsAndTagsTheWeftTipWithNothingPushed", func(t *testing.T) {
		h := newLocalModePair(t, slug)
		weft := h.PairRecordsSibling(slug)
		writeFile(t, filepath.Join(weft, filepath.FromSlash(statusRel)), "{}\n")
		if _, committed, err := fabricengine.CommitRecordsPaths(fabricengine.NewMutations(""), weft, h.Location.AnchorRel, []string{"_lyx"}, "local-only record", fabricengine.SyncOptions{}); err != nil || !committed {
			t.Fatalf("commit the local-only record: committed = %v, err = %v", committed, err)
		}
		writeFile(t, filepath.Join(weft, filepath.FromSlash(reportRel)), "stopped\n")
		scratchRoot, _ := scratchPaths(h.Location, slug)
		writeFile(t, filepath.Join(scratchRoot, "note.txt"), "scratch\n")
		recordsBefore, codeBefore := bareRefs(t, h.RecordsBare), bareRefs(t, h.CodeBare)

		// Not forced: a dirty gate that counted the linked scratch content would refuse here.
		res, err := h.Topology.RemoveLocal(h.Location, slug, false)
		if err != nil {
			t.Fatalf("RemoveLocal: %v", err)
		}
		unlinked := mutationIndexes(res.Mutations, fabricengine.KindLinkRemoved, scratchHubRelative(t, h.Location, scratchRoot))
		warpRemoved := mutationIndexes(res.Mutations, fabricengine.KindWorktreeRemoved, scratchHubRelative(t, h.Location, fabricengine.WorktreePath(h.Location, slug)))
		if len(unlinked) != 1 || len(warpRemoved) != 1 || unlinked[0] > warpRemoved[0] {
			t.Errorf("record %+v; want one link_removed at the root .scratch ahead of the warp worktree_removed", res.Mutations.Entries())
		}
		if !strings.HasPrefix(res.SetAsideTag, "set-aside/"+slug+"/") {
			t.Fatalf("SetAsideTag = %q; want a set-aside/%s/ tag", res.SetAsideTag, slug)
		}
		if got := showAtTag(t, h.PrimeRecords(), res.SetAsideTag, reportRel); got != "stopped\n" {
			t.Errorf("set-aside tag holds %s = %q; want the pending record committed", reportRel, got)
		}
		if res.ArchiveTag != "" {
			t.Errorf("ArchiveTag = %q; want none", res.ArchiveTag)
		}
		if got := bareRefs(t, h.RecordsBare); got != recordsBefore {
			t.Errorf("weft bare refs changed:\n%s\nwant\n%s", got, recordsBefore)
		}
		if got := bareRefs(t, h.CodeBare); got != codeBefore {
			t.Errorf("warp bare refs changed:\n%s\nwant\n%s", got, codeBefore)
		}
		requireNoPairWorktrees(t, h, slug)
	})

	t.Run("BusyLocksReturnErrPairBusyWithThePairIntact", func(t *testing.T) {
		tests := []struct {
			name     string
			lockPath func(t *testing.T, h *hubforge.Hub) string
		}{
			{"TeardownLock", func(t *testing.T, h *hubforge.Hub) string {
				return fabricengine.TeardownLockPath(h.Path, slug)
			}},
			{"WeftWriteLock", func(t *testing.T, h *hubforge.Hub) string {
				return fabricengine.WeftWriteLockPathForTest(t, openPairFabric(t, h, slug))
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := newLocalModePair(t, slug)
				writeFile(t, filepath.Join(h.PairRecordsSibling(slug), filepath.FromSlash(reportRel)), "stopped\n")
				holdLockForTest(t, tt.lockPath(t, h))

				_, err := h.Topology.RemoveLocal(h.Location, slug, false)
				if !errors.Is(err, fabricengine.ErrPairBusy) {
					t.Fatalf("RemoveLocal error = %v; want one wrapping ErrPairBusy", err)
				}
				requirePairIntact(t, h, slug)
				if tags := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "tag", "-l", "set-aside/*")); tags != "" {
					t.Errorf("set-aside tags = %q; want none after a busy removal", tags)
				}
			})
		}
	})

	t.Run("AFailureAfterTheTeardownLockLeavesBothWorktreesAndTheBareUnchanged", func(t *testing.T) {
		h := newLocalModePair(t, slug)
		recordsBefore := bareRefs(t, h.RecordsBare)
		boom := errors.New("injected failure after the lock")
		defer fabricengine.SetRemoveAfterLockHookForTest(func() error { return boom })()

		_, err := h.Topology.RemoveLocal(h.Location, slug, false)
		if !errors.Is(err, boom) {
			t.Fatalf("RemoveLocal error = %v; want the injected failure", err)
		}
		requirePairIntact(t, h, slug)
		if got := bareRefs(t, h.RecordsBare); got != recordsBefore {
			t.Errorf("weft bare refs changed:\n%s\nwant\n%s", got, recordsBefore)
		}
	})

	t.Run("AHeldHubWorkspaceLockSkipsTheWorkspaceAndTheHousekeeping", func(t *testing.T) {
		h := newLocalModePair(t, slug)
		holdLockForTest(t, fabricengine.HubWorkspaceLockPath(h.Path))
		housekept := 0
		defer fabricengine.SetHousekeepHookForTest(func(string) { housekept++ })()

		res, err := h.Topology.RemoveLocal(h.Location, slug, false)
		if err != nil {
			t.Fatalf("RemoveLocal: %v", err)
		}
		if !res.WorkspaceSkipped {
			t.Errorf("WorkspaceSkipped = false; want true under a held hub workspace lock")
		}
		if housekept != 0 {
			t.Errorf("housekeeping ran %d times; want none in local mode", housekept)
		}
		requireNoPairWorktrees(t, h, slug)
	})

	t.Run("APlainRemoveRunsTheHousekeeping", func(t *testing.T) {
		h := newLocalModePair(t, slug)
		var housekept []string
		defer fabricengine.SetHousekeepHookForTest(func(s string) { housekept = append(housekept, s) })()

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if !reflect.DeepEqual(housekept, []string{slug}) {
			t.Errorf("housekeeping calls = %v; want one for %q", housekept, slug)
		}
	})

	t.Run("BothModesClearTheFailedPushRecordsOfThePair", func(t *testing.T) {
		tests := []struct {
			name   string
			remove func(h *hubforge.Hub) error
		}{
			{"RemoveLocal", func(h *hubforge.Hub) error {
				_, err := h.Topology.RemoveLocal(h.Location, slug, false)
				return err
			}},
			{"Remove", func(h *hubforge.Hub) error {
				_, err := h.Topology.Remove(h.Location, slug, false, false)
				return err
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				h := newLocalModePair(t, slug)
				for repo, branch := range map[string]string{"code": slug, "records": fabricengine.RecordsBranchName(slug)} {
					failure := fabricengine.FailedPush{Repo: repo, RepoPath: h.Path, Branch: branch, Class: fabricengine.PushFailureTransport, Error: "refused"}
					if err := fabricengine.RecordPushFailure(h.Path, failure, time.Now()); err != nil {
						t.Fatalf("record the failed push of %s: %v", repo, err)
					}
				}

				if err := tt.remove(h); err != nil {
					t.Fatalf("remove: %v", err)
				}
				left, err := fabricengine.ListFailedPushes(h.Path)
				if err != nil || len(left) != 0 {
					t.Errorf("ListFailedPushes = %+v, %v; want none after the removal", left, err)
				}
			})
		}
	})

	t.Run("APairWithNoSiblingAsksNoOriginEvenWithADeadOne", func(t *testing.T) {
		h := newLocalModePair(t, slug)
		gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(slug))
		gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "-D", fabricengine.RecordsBranchName(slug))
		gitkit.MustRun(t, h.PrimeRecords(), "git", "remote", "set-url", "origin", "http://127.0.0.1:1/x")

		res, err := h.Topology.RemoveLocal(h.Location, slug, false)
		if err != nil {
			t.Fatalf("RemoveLocal: %v", err)
		}
		if res.RemoteBranchError != "" || res.SetAsideTag != "" {
			t.Errorf("RemoteBranchError = %q, SetAsideTag = %q; want neither for a pair with no sibling", res.RemoteBranchError, res.SetAsideTag)
		}
		requireNoPairWorktrees(t, h, slug)
	})
}

func TestSnapshotUncommitted(t *testing.T) {
	t.Parallel()

	const slug = "snapshot"

	t.Run("TagsOneCommitHoldingBothSidesAndMovesNothing", func(t *testing.T) {
		t.Parallel()

		h := newLocalModePair(t, slug)
		code, weft := h.PairCodeWorktree(slug), h.PairRecordsSibling(slug)
		if tagged, err := fabricengine.SnapshotTagged(h.Location, slug); err != nil || tagged {
			t.Fatalf("SnapshotTagged before = %v, %v; want false", tagged, err)
		}
		writeFile(t, filepath.Join(code, "dirty.txt"), "task\n")
		writeFile(t, filepath.Join(weft, "outside.txt"), "records\n")
		writeFile(t, filepath.Join(weft, "_lyx", "shed", slug, "drive-reports", "stop.md"), "inside the pathspec\n")
		scratchRoot, _ := scratchPaths(h.Location, slug)
		writeFile(t, filepath.Join(scratchRoot, "note.txt"), "scratch\n")
		codeStatus, weftStatus := gitkit.GitStatusPorcelain(t, code), gitkit.GitStatusPorcelain(t, weft)
		codeTip, weftTip := gitkit.RevParse(t, code, "HEAD"), gitkit.RevParse(t, weft, "HEAD")
		rec := fabricengine.NewMutations("")

		tag, paths, err := h.Topology.SnapshotUncommitted(rec, h.Location, slug)
		if err != nil {
			t.Fatalf("SnapshotUncommitted: %v", err)
		}
		if !strings.HasPrefix(tag, "set-aside/"+slug+"/") {
			t.Fatalf("tag = %q; want a set-aside/%s/ tag", tag, slug)
		}
		if want := []string{"warp/dirty.txt", "weft/outside.txt"}; !reflect.DeepEqual(paths, want) {
			t.Errorf("paths = %v; want %v", paths, want)
		}
		if kind := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "cat-file", "-t", "refs/tags/"+tag)); kind != "tag" {
			t.Errorf("tag object type = %q; want an annotated tag", kind)
		}
		held := strings.Fields(gitkit.Git(t, h.PrimeRecords(), "ls-tree", "-r", "--name-only", tag+"^{commit}"))
		if want := []string{"warp/dirty.txt", "weft/outside.txt"}; !reflect.DeepEqual(held, want) {
			t.Errorf("snapshot commit holds %v; want %v", held, want)
		}
		if got := gitkit.GitStatusPorcelain(t, code); got != codeStatus {
			t.Errorf("task worktree status changed:\n%s\nwant\n%s", got, codeStatus)
		}
		if got := gitkit.GitStatusPorcelain(t, weft); got != weftStatus {
			t.Errorf("weft worktree status changed:\n%s\nwant\n%s", got, weftStatus)
		}
		if got := gitkit.RevParse(t, code, "HEAD"); got != codeTip {
			t.Errorf("task branch tip = %s; want %s", got, codeTip)
		}
		if got := gitkit.RevParse(t, weft, "HEAD"); got != weftTip {
			t.Errorf("weft branch tip = %s; want %s", got, weftTip)
		}
		if got := countKind(rec, fabricengine.KindTagCreated); got != 1 {
			t.Errorf("tag_created mutations = %d; want 1", got)
		}
		if got := countKind(rec, fabricengine.KindCommitCreated); got != 1 {
			t.Errorf("commit_created mutations = %d; want 1", got)
		}
		if tagged, err := fabricengine.SnapshotTagged(h.Location, slug); err != nil || !tagged {
			t.Errorf("SnapshotTagged after = %v, %v; want true", tagged, err)
		}
	})

	t.Run("AHeldWeftWriteLockReturnsErrPairBusyAndWritesNoTag", func(t *testing.T) {
		t.Parallel()

		h := newLocalModePair(t, slug)
		writeFile(t, filepath.Join(h.PairCodeWorktree(slug), "dirty.txt"), "task\n")
		holdLockForTest(t, fabricengine.WeftWriteLockPathForTest(t, openPairFabric(t, h, slug)))

		tag, _, err := h.Topology.SnapshotUncommitted(fabricengine.NewMutations(""), h.Location, slug)
		if !errors.Is(err, fabricengine.ErrPairBusy) || tag != "" {
			t.Fatalf("SnapshotUncommitted = %q, %v; want no tag and an error wrapping ErrPairBusy", tag, err)
		}
		if tags := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "tag", "-l", "set-aside/*")); tags != "" {
			t.Errorf("set-aside tags = %q; want none", tags)
		}
	})

	t.Run("ACleanPairWritesNothing", func(t *testing.T) {
		t.Parallel()

		h := newLocalModePair(t, slug)
		rec := fabricengine.NewMutations("")

		tag, paths, err := h.Topology.SnapshotUncommitted(rec, h.Location, slug)
		if err != nil || tag != "" || len(paths) != 0 {
			t.Fatalf("SnapshotUncommitted = %q, %v, %v; want an empty tag", tag, paths, err)
		}
		if got := rec.Snapshot().Len(); got != 0 {
			t.Errorf("mutations = %d; want none", got)
		}
		if tags := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "tag", "-l", "set-aside/*")); tags != "" {
			t.Errorf("set-aside tags = %q; want none", tags)
		}
	})
}
