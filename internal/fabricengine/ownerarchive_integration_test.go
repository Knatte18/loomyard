// ownerarchive_integration_test.go covers ArchiveOwnerTips and ReadOwnerArchives over the prime pair of a hubforge hub.
// The hub's CodeBare and RecordsBare are the code and records origins.
// Each row builds its own hub, since a row commits to the prime or breaks its origin.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"reflect"
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

func TestReadOwnerArchives(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// archiveRecords archives a records tip instead of a code tip.
		archiveRecords bool
		// pushTip pushes the archived tip to its branch on origin after the archive.
		pushTip bool
		// keepNone admits no slug to the read.
		keepNone bool
		// unpublishedBranch commits on a new branch origin lacks.
		unpublishedBranch bool
		// teardownTag also pushes a teardown archive tag, archive/root/<tip>, naming the same tip.
		teardownTag bool
		wantListed  bool
	}{
		{name: "ACodeTagTheBranchOnOriginLacksIsListedAndFetched", wantListed: true},
		{name: "ARecordsTagIsJudgedAgainstTheRecordsBranch", archiveRecords: true, wantListed: true},
		{name: "ARecordsTagTheRecordsBranchHoldsIsNotListed", archiveRecords: true, pushTip: true},
		{name: "ATagTheBranchOnOriginHoldsIsNotListed", pushTip: true},
		{name: "ATagOfABranchAbsentOnOriginIsListed", unpublishedBranch: true, wantListed: true},
		{name: "ATeardownArchiveTagIsNeitherListedNorFetched", teardownTag: true, wantListed: true},
		{name: "ATagOfASlugKeepRefusesIsNotListed", keepNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			l := h.Location
			recordsDir := fabricengine.RecordsWorktree(l)
			// The fixture leaves the records prime's tip unpushed, so push it to archive one side alone.
			gitkit.MustRun(t, recordsDir, "git", "push", "origin", "HEAD")
			dir, side := l.WorktreePath(), fabricengine.OwnerArchiveSideCode
			if tt.archiveRecords {
				dir, side = recordsDir, fabricengine.OwnerArchiveSideRecords
			}
			if tt.unpublishedBranch {
				gitkit.Git(t, dir, "checkout", "-b", "unpublished")
			}
			tip := gitkit.CommitFile(t, dir, "owner-archive.txt", "local", "local commit")
			tags, err := fabricengine.ArchiveOwnerTips(l.HubPath, []fabricengine.OwnerArchivePair{{Slug: "root", Pair: l}})
			if err != nil || len(tags) != 1 {
				t.Fatalf("ArchiveOwnerTips() = %v, %v; want one tag", tags, err)
			}
			if tt.pushTip {
				gitkit.Git(t, dir, "push", "origin", "HEAD")
			}
			// The new owner holds no local tag, so the read must fetch it.
			gitkit.Git(t, dir, "tag", "-d", tags[0])
			if tt.teardownTag {
				teardown := fabricengine.ArchiveTagPrefix + "root/" + tip[:fabricengine.ArchiveTagSHALen]
				gitkit.Git(t, dir, "tag", teardown, tip)
				gitkit.Git(t, dir, "push", "origin", "refs/tags/"+teardown)
				gitkit.Git(t, dir, "tag", "-d", teardown)
			}
			keep := func(slug string) bool { return !tt.keepNone && slug == "root" }

			got, err := fabricengine.NewTopology(fabricengine.Config{}).ReadOwnerArchives(l, "root", keep)

			if err != nil {
				t.Fatalf("ReadOwnerArchives() error = %v", err)
			}
			var want []fabricengine.OwnerArchive
			if tt.wantListed {
				branch := gitkit.Git(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
				want = []fabricengine.OwnerArchive{{Slug: "root", Side: side, Tag: tags[0], Commit: tip, Branch: branch, Dir: dir}}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ReadOwnerArchives() = %+v; want %+v", got, want)
			}
			if local := gitkit.Git(t, dir, "tag", "--list", tags[0]); (local != "") != tt.wantListed {
				t.Errorf("local tag %s after the read = %q; want it present only when listed", tags[0], local)
			}
			if local := gitkit.Git(t, dir, "tag", "--list", fabricengine.ArchiveTagPrefix+"*"); local != "" {
				t.Errorf("local teardown archive tags after the read = %q; want none fetched", local)
			}
		})
	}
}

func TestReadOwnerArchives_ListsEveryPairSortedBySlugSideAndTag(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{Beam: true})
	l := h.Location
	codeDir, recordsDir, pairDir := l.WorktreePath(), fabricengine.RecordsWorktree(l), h.PairCodeWorktree("b")
	// The fixture leaves the records prime's tip unpushed, so push it to archive only the commits made here.
	gitkit.MustRun(t, recordsDir, "git", "push", "origin", "HEAD")
	pairTip := gitkit.CommitFile(t, pairDir, "owner-archive.txt", "local", "local pair commit")
	codeTip := gitkit.CommitFile(t, codeDir, "owner-archive.txt", "local", "local code commit")
	recordsTip := gitkit.CommitFile(t, recordsDir, "owner-archive.txt", "local", "local records commit")
	pair, err := lymcwd.NamedWorktree(h.Path, "b")
	if err != nil {
		t.Fatalf("NamedWorktree() = %v; want nil", err)
	}
	pairs := []fabricengine.OwnerArchivePair{{Slug: "root", Pair: l}, {Slug: "b", Pair: pair}}
	if tags, err := fabricengine.ArchiveOwnerTips(l.HubPath, pairs); err != nil || len(tags) != 3 {
		t.Fatalf("ArchiveOwnerTips() = %v, %v; want three tags", tags, err)
	}
	pairTag := fabricengine.OwnerArchiveTagName("b", pairTip)
	codeTag := fabricengine.OwnerArchiveTagName("root", codeTip)
	recordsTag := fabricengine.OwnerArchiveTagName("root", recordsTip)
	// The new owner holds no local tag, so the read must fetch each.
	gitkit.Git(t, codeDir, "tag", "-d", pairTag, codeTag)
	gitkit.Git(t, recordsDir, "tag", "-d", recordsTag)

	got, err := fabricengine.NewTopology(fabricengine.Config{}).ReadOwnerArchives(l, "root", func(string) bool { return true })

	if err != nil {
		t.Fatalf("ReadOwnerArchives() error = %v", err)
	}
	want := []fabricengine.OwnerArchive{
		{Slug: "b", Side: fabricengine.OwnerArchiveSideCode, Tag: pairTag, Commit: pairTip, Branch: gitkit.CurrentBranch(t, pairDir), Dir: codeDir},
		{Slug: "root", Side: fabricengine.OwnerArchiveSideCode, Tag: codeTag, Commit: codeTip, Branch: gitkit.CurrentBranch(t, codeDir), Dir: codeDir},
		{Slug: "root", Side: fabricengine.OwnerArchiveSideRecords, Tag: recordsTag, Commit: recordsTip, Branch: gitkit.CurrentBranch(t, recordsDir), Dir: recordsDir},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadOwnerArchives() = %+v; want %+v", got, want)
	}
}
