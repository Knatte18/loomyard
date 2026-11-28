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
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
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
func writeHandOverMark(t *testing.T, hubPath, slug string, mark fabricengine.HandOverMark) {
	t.Helper()

	path := fabricengine.HandOverMarkPath(hubPath, slug)
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
func (d *divergedPair) push(names fabricengine.MachineNames) error {
	_, err := fabricengine.PushPairAnchored(d.location, d.fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded, Here: names})
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

// TestRejectedRecordsPush_ReadsTheTakeover drives a rejected records push against origin's record of the run.
func TestRejectedRecordsPush_ReadsTheTakeover(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "takeover-read"
	entry := shedengine.Takeover{Machine: "oslo", Subject: "sub", At: "2026-11-16T10:00:00Z"}
	here := fabricengine.MachineNames{Current: "neve"}

	t.Run("EntryNamingAnotherMachineReturnsATakeoverErrorAndPushesNothing", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(here)

		d.requireTakeoverError(t, err, entry)
		d.requireNothingPushed(t)
	})

	t.Run("TheAnchorChangesPushPushesNoCodeSideOverATakeover", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		spawned, err := fabricengine.SpawnStatusPush(d.location, "")
		if !errors.Is(err, fabricengine.ErrSpawnUnderTest) {
			t.Fatalf("SpawnStatusPush() error = %v; want ErrSpawnUnderTest, which still reports the request", err)
		}

		_, err = fabricengine.CoalescePushRequest(spawned.Request, d.fake.options(nil), here)

		d.requireTakeoverError(t, err, entry)
		d.requireNothingPushed(t)
	})

	t.Run("EntryNamingAFormerNameOfThisMachineReplaysAndLandsBothSides", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(fabricengine.MachineNames{Current: "oslo-2", Former: []string{"oslo"}})

		if err != nil {
			t.Fatalf("push error = %v; want nil", err)
		}
		d.requireBothSidesLanded(t)
	})

	t.Run("MarkNamingTheEntrysMachineReturnsErrHandedOverAndPushesNothing", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		writeHandOverMark(t, d.hub.Location.HubPath, slug, fabricengine.HandOverMark{Machine: entry.Machine, At: entry.At, Written: time.Now()})

		err := d.push(here)

		if !errors.Is(err, fabricengine.ErrHandedOver) {
			t.Fatalf("push error = %v; want an error wrapping ErrHandedOver", err)
		}
		d.requireNothingPushed(t)
	})

	t.Run("MarkNamingAnotherMachineThanTheEntrysDoesNotMatch", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)
		writeHandOverMark(t, d.hub.Location.HubPath, slug, fabricengine.HandOverMark{Machine: "paris", At: entry.At, Written: time.Now()})

		err := d.push(here)

		d.requireTakeoverError(t, err, entry)
		d.requireNothingPushed(t)
	})

	t.Run("ZeroMachineNamesLeavesTheTakeoverUnreadAndReplays", func(t *testing.T) {
		d := newDivergedPair(t, slug, entry)

		err := d.push(fabricengine.MachineNames{})

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

		_, err := fabricengine.PushPairAnchored(f.hub.Location, fake.options(nil), fabricengine.PairPush{LockWait: fabricengine.LockWaitUnbounded, RunSlug: "other-run", Here: here})

		if err != nil {
			t.Fatalf("prime push error = %v; want nil", err)
		}
		if got, want := bareBranchTip(t, f.hub.RecordsBare, branch), gitkit.RevParse(t, weft, "HEAD"); got != want {
			t.Errorf("origin records tip = %s; want the replayed local tip %s", got, want)
		}
		requireNoFailedPush(t, f.hub.Location.HubPath)
	})
}
