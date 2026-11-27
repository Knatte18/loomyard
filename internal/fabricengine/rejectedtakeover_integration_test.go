// rejectedtakeover_integration_test.go covers the takeover read of a rejected records push:
// a run origin records as taken over by another machine is neither replayed nor pushed.
// Any other rejection replays as before.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/takeoverkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// plantTakeover commits the status file of the run slug on branch from a second clone of the records origin.
// The file carries entry as its takeover entry, and the commit is pushed.
// It returns the new origin tip.
func plantTakeover(t *testing.T, f *recordsFixture, branch, slug string, entry shedengine.Takeover) string {
	t.Helper()

	_, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
	}
	base, err := json.Marshal(shedengine.Status{State: shedengine.StateRunning})
	if err != nil {
		t.Fatalf("marshal the status: %v", err)
	}
	statusPath := filepath.Join(t.TempDir(), "status.json")
	if err := os.WriteFile(statusPath, base, 0o644); err != nil {
		t.Fatalf("write the status: %v", err)
	}
	if err := takeoverkit.WriteTakeover(statusPath, entry); err != nil {
		t.Fatalf("WriteTakeover: %v", err)
	}
	content, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read the status: %v", err)
	}
	return f.originCommit(branch, statusRel, string(content), "takeover of "+slug)
}

// writeHandOverMark writes this machine's hand-over mark of slug as the JSON the mark's reader decodes.
func writeHandOverMark(t *testing.T, hubPath, slug string, mark fabriclayout.HandOverMark) {
	t.Helper()

	path := fabriclayout.HandOverMarkPath(hubPath, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir the hand-over directory: %v", err)
	}
	data, err := json.Marshal(mark)
	if err != nil {
		t.Fatalf("marshal the hand-over mark: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write the hand-over mark: %v", err)
	}
}

// divergedPair is a pushed pair with a local transition on each side.
// Another machine moved its records branch on origin with a takeover entry in the run's record.
type divergedPair struct {
	*pairFixture
	slug string
	// recordsOrigin is origin's records tip, the takeover commit.
	recordsOrigin string
	// codeOrigin is origin's code tip.
	codeOrigin string
	// localRecords is the local records tip before any push.
	localRecords string
}

// newDivergedPair builds the pair slug with a takeover entry naming entry planted on origin's records branch.
func newDivergedPair(t *testing.T, slug string, entry shedengine.Takeover) *divergedPair {
	t.Helper()

	p := newPairFixture(t, slug)
	d := &divergedPair{pairFixture: p, slug: slug}
	d.codeOrigin = bareBranchTip(t, p.hub.CodeBare, p.warpBranch)
	gitkit.CommitFile(t, p.warpPath, "warp-file.txt", "warp\n", "code transition")
	d.localRecords = gitkit.CommitFile(t, p.weftPath, "weft-file.txt", "weft\n", "records transition")
	d.recordsOrigin = plantTakeover(t, p.recordsFixture, p.weftBranch, slug, entry)
	return d
}

// push runs the pair's synchronous push as the machine names.
func (d *divergedPair) push(names fabriclayout.MachineNames) error {
	_, err := fabricengine.PushPairAnchored(d.location, d.fake.options(nil), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded, Here: names})
	return err
}

// requireTakeoverError fails the test unless err is a *TakeoverError for d's run naming entry and d's records worktree.
func (d *divergedPair) requireTakeoverError(t *testing.T, err error, entry shedengine.Takeover) {
	t.Helper()

	var takeover *fabricengine.TakeoverError
	if !errors.As(err, &takeover) {
		t.Fatalf("push error = %v; want a *TakeoverError", err)
	}
	if takeover.Slug != d.slug || takeover.Entry != entry || takeover.RecordsWorktree != d.weftPath {
		t.Errorf("TakeoverError = %+v; want slug %q, entry %+v and worktree %s", takeover, d.slug, entry, d.weftPath)
	}
	if errors.Is(err, fabricengine.ErrHandedOver) {
		t.Errorf("push error = %v; want no ErrHandedOver", err)
	}
}

// requireNothingPushed fails the test unless both origin tips and the local records tip are as before the push.
// It also fails when a failed-push record or a mail stands.
func (d *divergedPair) requireNothingPushed(t *testing.T) {
	t.Helper()

	if got := bareBranchTip(t, d.hub.RecordsBare, d.weftBranch); got != d.recordsOrigin {
		t.Errorf("origin records tip = %s; want it unchanged at %s", got, d.recordsOrigin)
	}
	if got := bareBranchTip(t, d.hub.CodeBare, d.warpBranch); got != d.codeOrigin {
		t.Errorf("origin code tip = %s; want it unchanged at %s", got, d.codeOrigin)
	}
	if got := gitkit.RevParse(t, d.weftPath, "HEAD"); got != d.localRecords {
		t.Errorf("local records tip = %s; want no replay, so %s", got, d.localRecords)
	}
	requireNoFailedPush(t, d.hub.Location.HubPath)
	if len(d.fake.lines) != 0 {
		t.Errorf("mails = %q; want none", d.fake.lines)
	}
}

// requireBothSidesLanded fails the test unless origin holds the local tip of each side.
func (d *divergedPair) requireBothSidesLanded(t *testing.T) {
	t.Helper()

	if got, want := bareBranchTip(t, d.hub.RecordsBare, d.weftBranch), gitkit.RevParse(t, d.weftPath, "HEAD"); got != want || got == d.recordsOrigin {
		t.Errorf("origin records tip = %s; want the replayed local tip %s, past the takeover commit %s", got, want, d.recordsOrigin)
	}
	if got, want := bareBranchTip(t, d.hub.CodeBare, d.warpBranch), gitkit.RevParse(t, d.warpPath, "HEAD"); got != want {
		t.Errorf("origin code tip = %s; want the local tip %s", got, want)
	}
}

// newPrimeRunsAhead builds a hub whose prime records branch holds one unpushed commit per slug.
// Each commit stages a file under that run's record directory and carries its Run-Transition trailer.
// Origin's prime records branch has moved past the local tip by another clone's commit,
// so the prime's next push is rejected.
// It returns the fixture of the hub that holds that prime.
func newPrimeRunsAhead(t *testing.T, slugs []string) *recordsFixture {
	t.Helper()

	f, _ := newFailureFixture(t)
	weft, branch := f.primeWeft()
	for _, slug := range slugs {
		_, statusRel, err := shedrun.TreeRunFileRels(slug)
		if err != nil {
			t.Fatalf("TreeRunFileRels(%q): %v", slug, err)
		}
		message := "transition of " + slug + "\n\n" + fabriclayout.TransitionTrailerKey + ": " + slug
		gitkit.CommitFile(t, weft, path.Join(path.Dir(statusRel), "transition.txt"), slug+"\n", message)
	}
	f.originCommit(branch, "other.txt", "other\n", "another machine's commit")
	return f
}

// primeRunsAhead is the prime of newPrimeRunsAhead with the paths and handle a prime row reads and moves origin through.
type primeRunsAhead struct {
	location *lymcwd.Location
	weft     string
	branch   string
	bare     string
	origin   *recordsFixture
}

// newPrimeRunsAheadWithOrigin builds newPrimeRunsAhead's hub.
// It also builds the handle that commits on its origin from another clone.
func newPrimeRunsAheadWithOrigin(t *testing.T, slugs []string) *primeRunsAhead {
	t.Helper()

	f := newPrimeRunsAhead(t, slugs)
	location := f.hub.Location
	weft := fabricengine.RecordsWorktree(location)
	bare := gitkit.Git(t, weft, "remote", "get-url", "origin")
	return &primeRunsAhead{
		location: location,
		weft:     weft,
		branch:   gitkit.CurrentBranch(t, weft),
		bare:     bare,
		origin:   f,
	}
}

// push runs the prime's synchronous push as the machine names, naming the run runSlug when it is not empty.
func (p *primeRunsAhead) push(names fabriclayout.MachineNames, runSlug string) error {
	_, err := fabricengine.PushPairAnchored(p.location, fabricengine.SyncOptions{}, fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded, RunSlug: runSlug, Here: names})
	return err
}

// originSubjects returns the subjects of the commits on origin's prime records branch, oldest first.
func (p *primeRunsAhead) originSubjects(t *testing.T) []string {
	t.Helper()
	return strings.Split(gitkit.Git(t, p.bare, "log", "--reverse", "--format=%s", p.branch), "\n")
}

// requireTakeoversError fails the test unless err is a *TakeoversError naming exactly the one run slug with entry.
func (p *primeRunsAhead) requireTakeoversError(t *testing.T, err error, slug string, entry shedengine.Takeover) {
	t.Helper()

	var takeovers *fabricengine.TakeoversError
	if !errors.As(err, &takeovers) {
		t.Fatalf("push error = %v; want a *TakeoversError", err)
	}
	if len(takeovers.Takeovers) != 1 || takeovers.Takeovers[0].Slug != slug || takeovers.Takeovers[0].Entry != entry || takeovers.Takeovers[0].RecordsWorktree != p.weft {
		t.Errorf("TakeoversError = %+v; want the one run %q with entry %+v at %s", takeovers.Takeovers, slug, entry, p.weft)
	}
}

// TestRejectedRecordsPush_ReadsTheTakeover drives a rejected records push against origin's record of the run.
func TestRejectedRecordsPush_ReadsTheTakeover(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "takeover-read"
	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}
	here := fabriclayout.MachineNames{Current: "neve"}

	t.Run("EntryNamingAnotherMachineReturnsATakeoverErrorAndPushesNothing", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(here)

		d.requireTakeoverError(t, err, entry)
		d.requireNothingPushed(t)
	})

	t.Run("EntryNamingAFormerNameOfThisMachineReplaysAndLandsBothSides", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(fabriclayout.MachineNames{Current: "oslo-2", Former: []string{"oslo"}})

		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}
		d.requireBothSidesLanded(t)
	})

	t.Run("MarkNamingTheEntrysMachineReturnsErrHandedOverAndPushesNothing", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		writeHandOverMark(t, d.hub.Location.HubPath, slug, fabriclayout.HandOverMark{Machine: entry.Machine, At: entry.At, Written: time.Now()})

		err := d.push(here)

		if !errors.Is(err, fabricengine.ErrHandedOver) {
			t.Fatalf("push error = %v; want an error wrapping ErrHandedOver", err)
		}
		d.requireNothingPushed(t)
	})

	t.Run("MarkNamingAnotherMachineThanTheEntrysDoesNotMatch", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		writeHandOverMark(t, d.hub.Location.HubPath, slug, fabriclayout.HandOverMark{Machine: "paris", At: entry.At, Written: time.Now()})

		err := d.push(here)

		d.requireTakeoverError(t, err, entry)
		d.requireNothingPushed(t)
	})

	t.Run("ZeroMachineNamesLeavesTheTakeoverUnreadAndReplays", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(fabriclayout.MachineNames{})

		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}
		d.requireBothSidesLanded(t)
	})

	t.Run("ARecordsWorktreeWithNoUpstreamReadsNoTakeover", func(t *testing.T) {
		p := newPairFixture(t, slug)

		got, found, err := fabricengine.ReadUpstreamTakeover(p.location, p.hub.PrimeRecords(), slug)

		if err != nil || found {
			t.Errorf("ReadUpstreamTakeover() = (%+v, %v, %v); want (zero, false, nil)", got, found, err)
		}
	})

	t.Run("APrimePushRejectedByAnotherRunsCommitWithNoEntryReplaysAndLands", func(t *testing.T) {
		f, fake := newFailureFixture(t)
		weft, branch := f.primeWeft()
		f.originCommit(branch, "other.txt", "other\n", "another run's batten commit")
		gitkit.CommitFile(t, weft, "local.txt", "local\n", "local")

		_, err := fabricengine.PushPairAnchored(f.hub.Location, fake.options(nil), fabriclayout.PairPush{LockWait: fabriclayout.LockWaitUnbounded, RunSlug: "other-run", Here: here})

		if err != nil {
			t.Fatalf("prime push error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin records tip = %s; want the replayed local tip %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})

	// A rejected push of the prime's records carries every batten run's commits, so it reads each run's takeover.
	const takenOver, other = "run-taken", "run-other"
	t.Run("PrimeRunTakenOverByAnotherMachineReturnsATakeoversErrorAndPushesNothing", func(t *testing.T) {
		p := newPrimeRunsAheadWithOrigin(t, []string{takenOver, other})
		originTip := plantTakeover(t, p.origin, p.branch, takenOver, entry)
		localTip := gitkit.RevParse(t, p.weft, "HEAD")

		err := p.push(here, "")

		p.requireTakeoversError(t, err, takenOver, entry)
		if got := bareBranchTip(t, p.bare, p.branch); got != originTip {
			t.Errorf("origin records tip = %s; want it unchanged at %s", got, originTip)
		}
		if got := gitkit.RevParse(t, p.weft, "HEAD"); got != localTip {
			t.Errorf("local records tip = %s; want no replay, so %s", got, localTip)
		}
		requireNoFailedPush(t, p.location.HubPath)
	})

	t.Run("APushNamingTheRunNotTakenOverStillReadsTheTakenOverRun", func(t *testing.T) {
		p := newPrimeRunsAheadWithOrigin(t, []string{takenOver, other})
		originTip := plantTakeover(t, p.origin, p.branch, takenOver, entry)

		err := p.push(here, other)

		p.requireTakeoversError(t, err, takenOver, entry)
		if got := bareBranchTip(t, p.bare, p.branch); got != originTip {
			t.Errorf("origin records tip = %s; want it unchanged at %s", got, originTip)
		}
		requireNoFailedPush(t, p.location.HubPath)
	})

	handedOverRows := []struct {
		name string
		// prepare runs after the hand-over mark is written.
		prepare func(t *testing.T, p *primeRunsAhead)
	}{
		{"HandedOverRunsCommitsAreSetAsideAndTheOthersLand", func(t *testing.T, p *primeRunsAhead) {}},
		{"HandedOverRunAtItsLandRowIsStillSetAside", func(t *testing.T, p *primeRunsAhead) {
			_, statusRel, err := shedrun.TreeRunFileRels(takenOver)
			if err != nil {
				t.Fatalf("TreeRunFileRels(%q): %v", takenOver, err)
			}
			message := "land status of " + takenOver + "\n\n" + fabriclayout.TransitionTrailerKey + ": " + takenOver
			gitkit.CommitFile(t, p.weft, statusRel, statusJSON("Land"), message)
		}},
	}
	for _, tt := range handedOverRows {
		t.Run(tt.name, func(t *testing.T) {
			p := newPrimeRunsAheadWithOrigin(t, []string{takenOver, other})
			plantTakeover(t, p.origin, p.branch, takenOver, entry)
			writeHandOverMark(t, p.location.HubPath, takenOver, fabriclayout.HandOverMark{Machine: entry.Machine, At: entry.At, Written: time.Now()})
			tt.prepare(t, p)

			err := p.push(here, "")

			if err != nil {
				t.Fatalf("push error = %v; want nil", err)
			}
			subjects := p.originSubjects(t)
			if !slices.Contains(subjects, "transition of "+other) || slices.Contains(subjects, "transition of "+takenOver) || slices.Contains(subjects, "land status of "+takenOver) {
				t.Errorf("origin records history = %q; want the other run's commit and not the handed-over run's", subjects)
			}
			if got, want := bareBranchTip(t, p.bare, p.branch), gitkit.RevParse(t, p.weft, "HEAD"); got != want {
				t.Errorf("origin records tip = %s; want the replayed local tip %s", got, want)
			}
			requireNoFailedPush(t, p.location.HubPath)
		})
	}

	t.Run("APushNamingATakenOverRunWithNoCommitOfItsOwnStillReadsItsTakeover", func(t *testing.T) {
		p := newPrimeRunsAheadWithOrigin(t, []string{other})
		originTip := plantTakeover(t, p.origin, p.branch, takenOver, entry)
		localTip := gitkit.RevParse(t, p.weft, "HEAD")

		err := p.push(here, takenOver)

		p.requireTakeoversError(t, err, takenOver, entry)
		if got := bareBranchTip(t, p.bare, p.branch); got != originTip {
			t.Errorf("origin records tip = %s; want it unchanged at %s", got, originTip)
		}
		if got := gitkit.RevParse(t, p.weft, "HEAD"); got != localTip {
			t.Errorf("local records tip = %s; want no replay, so %s", got, localTip)
		}
		requireNoFailedPush(t, p.location.HubPath)
	})

	t.Run("ARunOriginEndedAndARunHandedOverAreBothSetAsideAndTheOthersLand", func(t *testing.T) {
		const ended = "run-ended"
		p := newPrimeRunsAheadWithOrigin(t, []string{takenOver, ended, other})
		plantTakeover(t, p.origin, p.branch, takenOver, entry)
		p.origin.endEntryOnOrigin(ended)
		writeHandOverMark(t, p.location.HubPath, takenOver, fabriclayout.HandOverMark{Machine: entry.Machine, At: entry.At, Written: time.Now()})

		err := p.push(here, "")

		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}
		subjects := p.originSubjects(t)
		if !slices.Contains(subjects, "transition of "+other) || slices.Contains(subjects, "transition of "+takenOver) || slices.Contains(subjects, "transition of "+ended) {
			t.Errorf("origin records history = %q; want the other run's commit and neither the handed-over run's nor the ended run's", subjects)
		}
		if got, want := bareBranchTip(t, p.bare, p.branch), gitkit.RevParse(t, p.weft, "HEAD"); got != want {
			t.Errorf("origin records tip = %s; want the replayed local tip %s", got, want)
		}
		requireNoFailedPush(t, p.location.HubPath)
	})

	t.Run("AFailedTakeoverReadOwesThePushWithNothingPushedOrRecorded", func(t *testing.T) {
		p := newPrimeRunsAheadWithOrigin(t, []string{takenOver, other})
		_, statusRel, err := shedrun.TreeRunFileRels(takenOver)
		if err != nil {
			t.Fatalf("TreeRunFileRels(%q): %v", takenOver, err)
		}
		originTip := p.origin.originCommit(p.branch, statusRel, "{not json", "unreadable status of "+takenOver)

		err = p.push(here, "")

		if !errors.Is(err, fabricengine.ErrTakeoverPassOwed) || !fabricengine.IsPushOwed(err) {
			t.Fatalf("push error = %v; want an owed error wrapping ErrTakeoverPassOwed", err)
		}
		if got := bareBranchTip(t, p.bare, p.branch); got != originTip {
			t.Errorf("origin records tip = %s; want it unchanged at %s", got, originTip)
		}
		requireNoFailedPush(t, p.location.HubPath)
	})
}
