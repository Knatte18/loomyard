// holdcheck_integration_test.go drives the hub-filled seams of the hold check over a hubforge hub.
// A missed assignment in the wiring fails there, and the fake-seam rows cannot see it.

package adoptcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubHoldCheckDeps_DropBoltHoldLeavesNoBoltHoldEntry(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	now := time.Now()
	entry := func(scope, caller string) pauserecord.Entry {
		return pauserecord.Entry{Scope: scope, PausedAt: now, GraceEnd: now, Caller: caller, Sessions: []pauserecord.Session{}, Machine: "boxA"}
	}
	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, entry("a", pauserecord.BoltHoldCaller), entry("b", pauserecord.BoltHoldCaller), entry("c", "EXTERNAL"))
		return nil
	}); err != nil {
		t.Fatalf("plant the pause entries: %v", err)
	}
	deps, err := hubHoldCheckDeps(h.Location, nil, false)
	if err != nil {
		t.Fatalf("hubHoldCheckDeps: %v", err)
	}

	if err := deps.DropBoltHold(); err != nil {
		t.Fatalf("DropBoltHold() error = %v; want nil", err)
	}

	record, err := pauserecord.Read(geometry.RecordPath)
	if err != nil {
		t.Fatalf("read the pause record: %v", err)
	}
	if len(record.Live) != 1 || record.Live[0].Scope != "c" || record.Live[0].Caller != "EXTERNAL" {
		t.Errorf("live entries = %+v; want the operator's entry of c alone", record.Live)
	}
}

func TestHubHoldCheckDeps_FilledSeamsNameTheRerun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	const rerun = "lym adopt b root"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{Beam: true})
	board, err := boardengine.OpenHub(h.Path)
	if err != nil {
		t.Fatalf("open the hub's board: %v", err)
	}
	if _, err := board.UpsertTask(map[string]any{"slug": "b", "title": "b", "kind": "project", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the beam project: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: "otherbox", Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("record another machine as the beam's owner: %v", err)
	}
	deps, err := hubHoldCheckDeps(h.Location, nil, false)
	if err != nil {
		t.Fatalf("hubHoldCheckDeps: %v", err)
	}

	t.Run("TheCheckOfASubjectOwnedElsewhereNamesTheRerun", func(t *testing.T) {
		record, ran, err := deps.Check(context.Background(), "b", rerun)

		if ran || err == nil || !strings.Contains(err.Error(), "owned by otherbox") || !strings.Contains(err.Error(), "run "+rerun) {
			t.Errorf("Check() = %+v, ran %v, error %v; want it not to run, with the owned-elsewhere line naming %q", record, ran, err, rerun)
		}
		if _, err := os.Stat(filepath.Join(subjectDir(h.Path, "b"), checkRecordName)); !os.IsNotExist(err) {
			t.Errorf("Stat(check.json) error = %v; want no record written, so no gate ran", err)
		}
	})

	t.Run("TheFixAtTheLimitNamesTheRerun", func(t *testing.T) {
		const beyondAnyLimit = 1000

		task, lines, lost := deps.Fix("root", beyondAnyLimit, failedRecord(), rerun)

		if task != "" || lost != nil {
			t.Errorf("Fix() task = %q, lost = %v; want no task filed and no refusal", task, lost)
		}
		if len(lines) != 1 || !strings.HasPrefix(lines[0], fixLimitLineOpening) || !strings.Contains(lines[0], "run "+rerun) {
			t.Errorf("Fix() lines = %q; want the limit line naming %q", lines, rerun)
		}
	})
}
