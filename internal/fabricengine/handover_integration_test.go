// handover_integration_test.go covers HandOverTips over pair fixtures.
// The records origin of each holds another machine's takeover entry.
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// plantTransition commits a transition of the run slug on origin's records branch from a second clone, naming codeTip.
// It then fetches the records branch into the pair's records worktree, whose upstream the hand-over reads.
func plantTransition(t *testing.T, d *divergedPair, codeTip string) {
	t.Helper()

	message := "transition\n\n" + fabricengine.TransitionTrailerKey + ": " + d.slug + "\n" + fabricengine.WarpSHATrailerKey + ": " + codeTip
	d.originCommit(d.weftBranch, "transition-"+codeTip[:8]+".txt", codeTip, message)
	gitkit.Git(t, d.weftPath, "fetch")
}

// requireHandOverMark fails the test unless d's hub holds this machine's hand-over mark exactly when want says so.
func requireHandOverMark(t *testing.T, d *divergedPair, entry shedengine.Takeover, want bool) {
	t.Helper()

	mark, found, err := fabricengine.ReadHandOverMark(d.hub.Location.HubPath, d.slug)
	if err != nil {
		t.Fatalf("ReadHandOverMark() error = %v", err)
	}
	if found != want {
		t.Fatalf("hand-over mark found = %v; want %v", found, want)
	}
	if want && (mark.Machine != entry.Machine || mark.At != entry.At) {
		t.Errorf("hand-over mark = %+v; want machine %q and time %q", mark, entry.Machine, entry.At)
	}
}

// TestHandOverTips drives the git steps of a hand-over over a pair whose run another machine took over.
// The rows install the process's push notifier and set package-level hooks, so none of them runs in parallel.
func TestHandOverTips(t *testing.T) {
	tiergate.Require(t, 1)

	const slug = "hand-over"
	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}
	request := func(d *divergedPair) fabricengine.HandOverRequest {
		return fabricengine.HandOverRequest{Slug: slug, Pair: d.location, Entry: entry}
	}

	t.Run("LocalCommitsAndAnUncommittedFileEndUnderPushedTagsAndOriginsBranchesStay", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		localCode := gitkit.RevParse(t, d.warpPath, "HEAD")
		if err := os.WriteFile(filepath.Join(d.warpPath, "uncommitted.txt"), []byte("work in progress\n"), 0o644); err != nil {
			t.Fatalf("write the uncommitted file: %v", err)
		}
		hookCalls := 0
		fabricengine.SetArchiveTagPushHookForTest(t, func(string) error {
			hookCalls++
			requireHandOverMark(t, d, entry, false)
			return nil
		})

		res, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		if res.TipPushed || len(res.Tags) != 2 || hookCalls != 2 {
			t.Fatalf("HandOverTips() = %+v with %d tag pushes; want two tags, no tip pushed and two tag pushes", res, hookCalls)
		}
		codeTag, recordsTag := res.Tags[0], res.Tags[1]
		codeCommit := tagTargetAt(t, d.hub.CodeBare, codeTag)
		if got := gitkit.Git(t, d.warpPath, "show", codeCommit+":uncommitted.txt"); got != "work in progress" {
			t.Errorf("uncommitted.txt at the code tag = %q; want the file's content", got)
		}
		if got := gitkit.RevParse(t, d.warpPath, codeCommit+"^"); got != localCode {
			t.Errorf("parent of the code tag's commit = %s; want the local tip %s", got, localCode)
		}
		if got, want := tagTargetAt(t, d.hub.RecordsBare, recordsTag), d.localRecords; got != want {
			t.Errorf("records tag %s points at %s; want the local records tip %s", recordsTag, got, want)
		}
		if got := gitkit.RevParse(t, d.warpPath, "HEAD"); got != localCode {
			t.Errorf("local code branch = %s; want it left at %s", got, localCode)
		}
		d.requireNothingPushed(t)
		requireHandOverMark(t, d, entry, true)
	})

	t.Run("AParentBranchAheadByTheRunsMergeAloneIsTaggedAndResetLevelWithItsUpstream", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		primeDir := d.hub.Location.WorktreePath()
		primeBranch := gitkit.CurrentBranch(t, primeDir)
		gitkit.Git(t, primeDir, "push", "--set-upstream", "origin", primeBranch)
		upstreamBefore := gitkit.RevParse(t, primeDir, "origin/"+primeBranch)
		gitkit.Git(t, primeDir, "merge", "--no-ff", "-m", "merge the run", d.warpBranch)
		merge := gitkit.RevParse(t, primeDir, "HEAD")

		req := request(d)
		req.Parent = d.hub.Location
		res, err := fabricengine.HandOverTips(d.hub.Location, req)

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		mergeTag := fabricengine.OwnerArchiveTagName(slug, merge)
		if !slices.Contains(res.Tags, mergeTag) {
			t.Errorf("tags = %v; want the merge's tag %s among them", res.Tags, mergeTag)
		}
		if got := tagTargetAt(t, d.hub.CodeBare, mergeTag); got != merge {
			t.Errorf("origin tag %s points at %s; want the merge %s", mergeTag, got, merge)
		}
		if got := gitkit.RevParse(t, primeDir, "HEAD"); got != upstreamBefore {
			t.Errorf("parent branch = %s; want it reset level with its upstream %s", got, upstreamBefore)
		}
	})

	t.Run("AParentBranchWithACommitBeyondTheRunsMergeIsLeftAlone", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		primeDir := d.hub.Location.WorktreePath()
		primeBranch := gitkit.CurrentBranch(t, primeDir)
		gitkit.Git(t, primeDir, "push", "--set-upstream", "origin", primeBranch)
		gitkit.Git(t, primeDir, "merge", "--no-ff", "-m", "merge the run", d.warpBranch)
		operators := gitkit.CommitFile(t, primeDir, "operator.txt", "operator\n", "the operator's own commit")

		req := request(d)
		req.Parent = d.hub.Location
		if _, err := fabricengine.HandOverTips(d.hub.Location, req); err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}

		if got := gitkit.RevParse(t, primeDir, "HEAD"); got != operators {
			t.Errorf("parent branch = %s; want it left at the operator's commit %s", got, operators)
		}
	})

	t.Run("ATipTheLastTransitionNamesIsPushedUnderTheLease", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		localCode := gitkit.RevParse(t, d.warpPath, "HEAD")
		plantTransition(t, d, localCode)

		res, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		if !res.TipPushed {
			t.Errorf("HandOverTips() = %+v; want TipPushed", res)
		}
		if got := bareBranchTip(t, d.hub.CodeBare, d.warpBranch); got != localCode {
			t.Errorf("origin code tip = %s; want the owed tip %s", got, localCode)
		}
	})

	t.Run("ATipWhoseTransitionIsGivenUpOnOriginIsResetOffOrigin", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		localCode := gitkit.RevParse(t, d.warpPath, "HEAD")
		plantTransition(t, d, localCode)
		fabricengine.SetOwedTipPushedHookForTest(t, func() { plantTransition(t, d, d.codeOrigin) })

		res, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		if res.TipPushed {
			t.Errorf("HandOverTips() = %+v; want no tip left pushed", res)
		}
		if got := bareBranchTip(t, d.hub.CodeBare, d.warpBranch); got != d.codeOrigin {
			t.Errorf("origin code tip = %s; want it reset to %s", got, d.codeOrigin)
		}
	})

	t.Run("ATipIsPushedToCreateAnOriginCodeBranchThatIsAbsent", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		localCode := gitkit.RevParse(t, d.warpPath, "HEAD")
		plantTransition(t, d, localCode)
		gitkit.Git(t, d.hub.CodeBare, "branch", "-D", d.warpBranch)

		res, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		if !res.TipPushed {
			t.Errorf("HandOverTips() = %+v; want TipPushed", res)
		}
		if got := bareBranchTip(t, d.hub.CodeBare, d.warpBranch); got != localCode {
			t.Errorf("origin code tip = %s; want the owed tip %s", got, localCode)
		}
	})

	t.Run("ATipWhoseTransitionIsGivenUpDeletesTheOriginCodeBranchItCreated", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		localCode := gitkit.RevParse(t, d.warpPath, "HEAD")
		plantTransition(t, d, localCode)
		gitkit.Git(t, d.hub.CodeBare, "branch", "-D", d.warpBranch)
		fabricengine.SetOwedTipPushedHookForTest(t, func() { plantTransition(t, d, d.codeOrigin) })

		res, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}
		if res.TipPushed {
			t.Errorf("HandOverTips() = %+v; want no tip left pushed", res)
		}
		if got := gitkit.Git(t, d.hub.CodeBare, "for-each-ref", "--format=%(refname)", "refs/heads/"+d.warpBranch); got != "" {
			t.Errorf("origin code branch refs = %q; want the branch the push created deleted", got)
		}
		tag := fabricengine.OwnerArchiveTagName(slug, localCode)
		if got := tagTargetAt(t, d.hub.CodeBare, tag); got != localCode {
			t.Errorf("origin tag %s points at %q; want the given-up tip %s", tag, got, localCode)
		}
	})

	t.Run("ATagPushOriginRefusesLeavesNoMarkAndReturnsTheFailure", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		fabricengine.SetArchiveTagPushHookForTest(t, func(string) error { return errors.New("origin refuses the tag push") })

		_, err := fabricengine.HandOverTips(d.hub.Location, request(d))

		if !errors.Is(err, fabricengine.ErrArchiveFailed) || !strings.Contains(err.Error(), "origin refuses the tag push") {
			t.Fatalf("HandOverTips() error = %v; want one wrapping ErrArchiveFailed and naming the refusal", err)
		}
		requireHandOverMark(t, d, entry, false)
	})

	t.Run("ADroppedPauseIntentPostsOneNoticeNamingTheMachineAndTheIntent", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		t.Cleanup(fabricengine.InstallPushNotifier(d.fake.notify))
		req := request(d)
		req.DroppedIntents = []string{"pause intent"}

		if _, err := fabricengine.HandOverTips(d.hub.Location, req); err != nil {
			t.Fatalf("HandOverTips() error = %v", err)
		}

		if len(d.fake.notices) != 1 {
			t.Fatalf("notices = %+v; want one", d.fake.notices)
		}
		notice := d.fake.notices[0]
		if !strings.Contains(notice.Line, entry.Machine) || !strings.Contains(notice.Line, "pause intent") || notice.Topic != "hand-over/"+slug {
			t.Errorf("notice = %+v; want the machine %q, the pause intent and the topic hand-over/%s", notice, entry.Machine, slug)
		}
	})
}
