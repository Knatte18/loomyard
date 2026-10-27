//go:build tier1

// remove_local_integration_test.go covers Remove of a local-only pair and the two origin-write checks that guard a marked branch.
// Every hub here has an origin URL pointing at a path that does not exist, so any origin step fails the call.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
)

// addLocalPairOnDeadOrigin returns a dead-origin hub holding one AddLocal pair named slug.
func addLocalPairOnDeadOrigin(t *testing.T, slug string) *hubforge.Hub {
	t.Helper()

	h := newDeadOriginHub(t)
	if _, err := h.Topology.AddLocal(h.Location, slug); err != nil {
		t.Fatalf("setup AddLocal(%q): %v", slug, err)
	}
	return h
}

// TestRemove_LocalPair removes AddLocal pairs without force and asserts no origin step runs, nothing is archived and the records go with the sibling branch.
func TestRemove_LocalPair(t *testing.T) {
	t.Parallel()

	const slug = "localpair"
	weftBranch := fabricengine.RecordsBranchName(slug)

	tests := []struct {
		name   string
		remote bool
		setup  func(t *testing.T, h *hubforge.Hub)
		// wantMarkBranch is the branch the result names as carrying the mark.
		wantMarkBranch string
		// wantTip reports whether the dropped records tip is expected.
		wantTip bool
		// wantWarpKept reports whether the unlanded warp branch is expected to survive.
		wantWarpKept bool
		// wantSkippedReason is a substring of the expected RemoteSkippedReason; empty means none.
		wantSkippedReason string
	}{
		{name: "FreshPair", wantMarkBranch: slug, wantTip: true},
		{
			name: "CommittedRunRecord",
			setup: func(t *testing.T, h *hubforge.Hub) {
				rel := "_lym/shed/" + slug + "/status.json"
				weft := fabricengine.RecordsWorktreePath(h.Location, slug)
				writeFile(t, filepath.Join(weft, filepath.FromSlash(rel)), "{}\n")
				if _, committed, err := fabricengine.CommitRecordsPaths(fabricengine.NewMutations(""), weft, h.Location.AnchorRel, []string{"_lym"}, "record", fabricengine.SyncOptions{}); err != nil || !committed {
					t.Fatalf("commit the run record: committed = %v, err = %v", committed, err)
				}
			},
			wantMarkBranch: slug,
			wantTip:        true,
		},
		{
			name: "UncommittedRecord",
			setup: func(t *testing.T, h *hubforge.Hub) {
				weft := fabricengine.RecordsWorktreePath(h.Location, slug)
				writeFile(t, filepath.Join(weft, "_lym", "shed", slug, "drive-reports", "stop.md"), "stopped\n")
			},
			wantMarkBranch: slug,
			wantTip:        true,
		},
		{
			name:              "RemoteFlag",
			remote:            true,
			wantMarkBranch:    slug,
			wantTip:           true,
			wantSkippedReason: "local-only",
		},
		{
			name: "UnlandedWarpCommit",
			setup: func(t *testing.T, h *hubforge.Hub) {
				warpBranchCommit(t, h.Location, slug, "work.txt")
			},
			wantMarkBranch: slug,
			wantTip:        true,
			wantWarpKept:   true,
		},
		{
			name: "WeftSideDeletedByHand",
			setup: func(t *testing.T, h *hubforge.Hub) {
				gitkit.MustRun(t, h.PrimeRecords(), "git", "worktree", "remove", "--force", h.PairRecordsSibling(slug))
				gitkit.MustRun(t, h.PrimeRecords(), "git", "branch", "-D", weftBranch)
			},
			wantMarkBranch: slug,
		},
		{
			name: "WarpSideDeletedByHand",
			setup: func(t *testing.T, h *hubforge.Hub) {
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "worktree", "remove", "--force", h.PairCodeWorktree(slug))
				gitkit.MustRun(t, h.PrimeWorktree(), "git", "branch", "-D", slug)
			},
			wantMarkBranch: weftBranch,
			wantTip:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := addLocalPairOnDeadOrigin(t, slug)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			weftTip := ""
			if gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
				weftTip = gitkit.RevParse(t, h.PrimeRecords(), weftBranch)
			}

			res, err := h.Topology.Remove(h.Location, slug, false, tt.remote)
			if err != nil {
				t.Fatalf("Remove: %v", err)
			}
			if !res.Local || res.LocalMarkBranch != tt.wantMarkBranch {
				t.Errorf("Local = %v, LocalMarkBranch = %q; want true, %q", res.Local, res.LocalMarkBranch, tt.wantMarkBranch)
			}
			wantTip := ""
			if tt.wantTip {
				wantTip = weftTip
			}
			if res.DroppedRecordsTip != wantTip {
				t.Errorf("DroppedRecordsTip = %q; want %q", res.DroppedRecordsTip, wantTip)
			}
			if res.ArchiveTag != "" {
				t.Errorf("ArchiveTag = %q; want none", res.ArchiveTag)
			}
			if tags := strings.TrimSpace(gitkit.Git(t, h.PrimeRecords(), "tag", "-l", "archive/*")); tags != "" {
				t.Errorf("archive tags in the weft repo = %q; want none", tags)
			}
			if gitkit.BranchExists(t, h.PrimeRecords(), weftBranch) {
				t.Errorf("weft branch %q still exists after Remove", weftBranch)
			}
			for _, step := range []string{fabricengine.RemoveStepArchive, fabricengine.RemoveStepRecordCommit, fabricengine.RemoveStepSiblingBranchOnOrigin, fabricengine.RemoveStepTaskBranchOnOrigin} {
				if slices.Contains(res.Steps, step) {
					t.Errorf("Steps = %v; want no %q", res.Steps, step)
				}
			}
			if tt.wantSkippedReason == "" {
				if res.RemoteSkippedReason != "" {
					t.Errorf("RemoteSkippedReason = %q; want none", res.RemoteSkippedReason)
				}
			} else if !strings.Contains(res.RemoteSkippedReason, tt.wantSkippedReason) {
				t.Errorf("RemoteSkippedReason = %q; want it to name %q", res.RemoteSkippedReason, tt.wantSkippedReason)
			}
			if res.RemoteBranchError != "" || res.RemoteCodeBranchKeptReason != "" {
				t.Errorf("RemoteBranchError = %q, RemoteCodeBranchKeptReason = %q; want no origin step", res.RemoteBranchError, res.RemoteCodeBranchKeptReason)
			}

			warpKept := gitkit.BranchExists(t, h.PrimeWorktree(), slug)
			if warpKept != tt.wantWarpKept {
				t.Errorf("warp branch exists = %v; want %v (kept reason %q)", warpKept, tt.wantWarpKept, res.CodeBranchKeptReason)
			}
			if tt.wantWarpKept {
				if res.CodeBranchKeptReason == "" {
					t.Errorf("CodeBranchKeptReason is empty; want the gate's reason")
				}
				requireMarked(t, h.PrimeWorktree(), slug, true)
			}
			if _, err := os.Lstat(h.PairRecordsSibling(slug)); !os.IsNotExist(err) {
				t.Errorf("sibling worktree still present (stat err = %v)", err)
			}
		})
	}
}

// TestOriginWrites_LocalMarkedBranch drives the two origin-write checks over a marked weft branch: the archive returns no tag and the remote-deletion gate refuses.
func TestOriginWrites_LocalMarkedBranch(t *testing.T) {
	t.Parallel()

	const slug = "markedpair"
	weftBranch := fabricengine.RecordsBranchName(slug)
	h := addLocalPairOnDeadOrigin(t, slug)
	weftRoot, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}

	t.Run("ArchiveReturnsNoTag", func(t *testing.T) {
		t.Parallel()

		tag, skipped, err := fabricengine.ArchiveWeftTipForTest(fabricengine.NewMutations(""), h.Location, slug, weftBranch)
		if err != nil || tag != "" || !strings.Contains(skipped, "local-only") {
			t.Errorf("archiveWeftTip = (%q, %q, %v); want no tag, a local-only reason and no error", tag, skipped, err)
		}
		if tags := strings.TrimSpace(gitkit.Git(t, weftRoot, "tag", "-l", "archive/*")); tags != "" {
			t.Errorf("archive tags in the weft repo = %q; want none", tags)
		}
	})

	t.Run("RemoteDeletionGateRefuses", func(t *testing.T) {
		t.Parallel()

		err := fabricengine.CheckRemoteBranchRequestForTest(h.Location, weftRoot, "origin", weftBranch, "")
		refusal, ok := fabricengine.RefusalOf(err)
		if !ok || refusal.Check != fabricengine.CheckOwnership {
			t.Fatalf("gate error = %v; want a %s refusal", err, fabricengine.CheckOwnership)
		}
		for _, want := range []string{"local-only", "lym fabric remove"} {
			if !strings.Contains(refusal.Reason, want) {
				t.Errorf("refusal reason %q does not name %q", refusal.Reason, want)
			}
		}
	})
}
