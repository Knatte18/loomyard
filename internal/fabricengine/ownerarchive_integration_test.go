// ownerarchive_integration_test.go covers ArchiveOwnerTips over the prime pair of a hubforge hub.
// The hub's CodeBare and RecordsBare are the code and records origins.
// Each row builds its own hub, since a row commits to the prime or breaks its origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestArchiveOwnerTips(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// commitCode and commitRecords add a local commit origin lacks on that side.
		commitCode, commitRecords bool
		// breakOrigin points the code repository's origin at a path that does not exist.
		breakOrigin bool
		// setupCode shapes the code repository and returns the tip the archive must tag, empty for none.
		setupCode func(t *testing.T, codeDir string) string
		wantErr   bool
	}{
		{name: "TipsOriginHoldsArchiveNothing"},
		{
			name: "ATipOriginIsAheadOfArchivesNothing",
			setupCode: func(t *testing.T, codeDir string) string {
				gitkit.CommitFile(t, codeDir, "ahead.txt", "origin", "origin ahead")
				gitkit.Git(t, codeDir, "push", "origin", "HEAD")
				gitkit.Git(t, codeDir, "reset", "--hard", "HEAD~1")
				return ""
			},
		},
		{
			name: "ATipOriginDivergedFromIsTagged",
			setupCode: func(t *testing.T, codeDir string) string {
				gitkit.CommitFile(t, codeDir, "diverged.txt", "origin", "origin side")
				gitkit.Git(t, codeDir, "push", "origin", "HEAD")
				gitkit.Git(t, codeDir, "reset", "--hard", "HEAD~1")
				return gitkit.CommitFile(t, codeDir, "diverged.txt", "local", "local side")
			},
		},
		{
			name: "ABranchAbsentOnOriginIsTagged",
			setupCode: func(t *testing.T, codeDir string) string {
				gitkit.Git(t, codeDir, "checkout", "-b", "unpublished")
				return gitkit.Git(t, codeDir, "rev-parse", "HEAD")
			},
		},
		{name: "AnUnpushedCodeTipIsTaggedOnTheCodeOrigin", commitCode: true},
		{name: "AnUnpushedRecordsTipIsTaggedOnTheRecordsOrigin", commitRecords: true},
		{name: "AnUnreachableOriginFailsTheArchive", commitCode: true, breakOrigin: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			codeDir, recordsDir := l.WorktreePath(), fabricengine.RecordsWorktree(l)
			// The fixture leaves the records prime's tip unpushed, so push it to start from tips origin holds.
			gitkit.MustRun(t, recordsDir, "git", "push", "origin", "HEAD")
			var want []string
			var codeTip, recordsTip string
			if tt.commitCode {
				codeTip = gitkit.CommitFile(t, codeDir, "owner-archive.txt", "local", "local code commit")
				want = append(want, "archive-owner/root/"+codeTip[:12])
			}
			if tt.setupCode != nil {
				if codeTip = tt.setupCode(t, codeDir); codeTip != "" {
					want = append(want, "archive-owner/root/"+codeTip[:12])
				}
			}
			if tt.commitRecords {
				recordsTip = gitkit.CommitFile(t, recordsDir, "owner-archive.txt", "local", "local records commit")
				want = append(want, "archive-owner/root/"+recordsTip[:12])
			}
			if tt.breakOrigin {
				mustBreakOrigin(t, codeDir)
			}
			pairs := []fabricengine.OwnerArchivePair{{Slug: "root", Pair: l}}
			originBranchBefore := gitkit.Git(t, h.CodeBare, "rev-parse", "HEAD")

			tags, err := fabricengine.ArchiveOwnerTips(l.HubPath, pairs)

			if tt.wantErr {
				if !errors.Is(err, fabricengine.ErrArchiveFailed) {
					t.Fatalf("ArchiveOwnerTips() error = %v; want one wrapping ErrArchiveFailed", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ArchiveOwnerTips() error = %v", err)
			}
			if !slices.Equal(tags, want) {
				t.Errorf("ArchiveOwnerTips() = %v; want %v", tags, want)
			}
			if codeTip != "" {
				if got := tagTargetAt(t, h.CodeBare, want[0]); got != codeTip {
					t.Errorf("code origin tag %s points at %q; want the tip %s", want[0], got, codeTip)
				}
			}
			if recordsTip != "" {
				if got := tagTargetAt(t, h.RecordsBare, want[len(want)-1]); got != recordsTip {
					t.Errorf("records origin tag %s points at %q; want the tip %s", want[len(want)-1], got, recordsTip)
				}
			}
			if got := gitkit.Git(t, h.CodeBare, "rev-parse", "HEAD"); got != originBranchBefore {
				t.Errorf("code origin branch tip = %s after the archive; want it unchanged at %s, no branch pushed", got, originBranchBefore)
			}
			again, err := fabricengine.ArchiveOwnerTips(l.HubPath, pairs)
			if err != nil || !slices.Equal(again, want) {
				t.Errorf("re-run ArchiveOwnerTips() = %v, %v; want %v, nil", again, err, want)
			}
		})
	}
}

func TestArchiveOwnerTips_ATornPairDoesNotStopTheOthers(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
	l := h.Location
	gitkit.MustRun(t, fabricengine.RecordsWorktree(l), "git", "push", "origin", "HEAD")
	tip := gitkit.CommitFile(t, l.WorktreePath(), "owner-archive.txt", "local", "local code commit")
	// A detached checkout has no branch to archive, so the pair fails.
	gitkit.Git(t, h.PairCodeWorktree("b"), "checkout", "--detach")
	torn, err := lymcwd.NamedWorktree(h.Path, "b")
	if err != nil {
		t.Fatalf("NamedWorktree() = %v; want nil", err)
	}
	pairs := []fabricengine.OwnerArchivePair{{Slug: "b", Pair: torn}, {Slug: "root", Pair: l}}

	tags, err := fabricengine.ArchiveOwnerTips(l.HubPath, pairs)

	if !errors.Is(err, fabricengine.ErrArchiveFailed) || !strings.Contains(err.Error(), "pair b") {
		t.Errorf("ArchiveOwnerTips() error = %v; want one wrapping ErrArchiveFailed and naming pair b", err)
	}
	if want := []string{"archive-owner/root/" + tip[:12]}; !slices.Equal(tags, want) {
		t.Errorf("ArchiveOwnerTips() = %v; want %v, the other pair still tagged", tags, want)
	}
}
