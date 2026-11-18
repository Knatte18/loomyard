//go:build !windows

// runread_integration_test.go pins that ReadRun reads a run's state from tracked records alone, on a real hub pair.

package pairteardown_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadRunPulled(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	boardkit.InstallReplay(t)

	const slug = "pulled"
	newHub := func(t *testing.T) *hubforge.Hub {
		t.Helper()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		writeStatus(t, h.Location, slug, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Run-Shed"})
		return h
	}
	// pushHoldFromSecondClone writes an adopt hold of machine oslo to origin through a second clone of h's hub.
	pushHoldFromSecondClone := func(t *testing.T, h *hubforge.Hub) {
		t.Helper()
		second := hubforge.CloneSecond(t, h)
		hold := boardengine.Hold{
			Kind: boardengine.HoldKindAdopt, Machine: "oslo", At: time.Now().UTC(),
			Subjects: []boardengine.HoldSubject{{Subject: "root"}},
		}
		if written, err := boardengine.WriteHold(second.BoardDir(), hold); err != nil || !written.Wrote {
			t.Fatalf("WriteHold(second clone) = %+v, %v; want the hold written", written, err)
		}
	}

	t.Run("AHoldAnotherCloneWroteAfterTheLastPullReadsHeldWithThatMachine", func(t *testing.T) {
		t.Parallel()
		h := newHub(t)
		pushHoldFromSecondClone(t, h)

		if _, before, err := pairteardown.ReadRun(h.Location, slug); err != nil || before.Held {
			t.Fatalf("ReadRun before the pull = Held %v, %v; want not held, as this hub has not pulled", before.Held, err)
		}
		_, inputs, err := pairteardown.ReadRunPulled(h.Location, slug)
		if err != nil || !inputs.Held || inputs.HoldMachine != "oslo" {
			t.Errorf("ReadRunPulled() = Held %v, HoldMachine %q, %v; want held by oslo", inputs.Held, inputs.HoldMachine, err)
		}
	})

	t.Run("AnUnreachableOriginReadsTheLocalBolt", func(t *testing.T) {
		t.Parallel()
		h := newHub(t)
		gitkit.Git(t, h.BoardDir(), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "nowhere"))

		records, _, err := pairteardown.ReadRunPulled(h.Location, slug)
		if err != nil || runfsm.Derive(records).State != runfsm.Running {
			t.Errorf("ReadRunPulled() = state %q, %v; want the run read as running over the local Bolt", runfsm.Derive(records).State, err)
		}
	})

	t.Run("ADirtyBoltReturnsTheWayForwardAndReadsNothing", func(t *testing.T) {
		t.Parallel()
		h := newHub(t)
		pushHoldFromSecondClone(t, h)
		// The pull only skips as dirty when origin moved a file the local change touches.
		boardFile := filepath.Join(h.BoardDir(), boardengine.BoardFileRel())
		if err := os.WriteFile(boardFile, []byte("uncommitted\n"), 0o644); err != nil {
			t.Fatalf("dirty %s: %v", boardFile, err)
		}

		records, inputs, err := pairteardown.ReadRunPulled(h.Location, slug)
		if err == nil || !strings.Contains(err.Error(), "way forward: bring the board level") {
			t.Fatalf("ReadRunPulled() error = %v; want one naming the way forward", err)
		}
		if !reflect.DeepEqual(records, runfsm.Records{}) || inputs != (runfsm.Inputs{}) {
			t.Errorf("ReadRunPulled() = %+v, %+v; want nothing read", records, inputs)
		}
	})
}

func TestReadRun_ReadsTheSameStateWithoutTheCache(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "cached"
	hubforge.AddPair(t, h, slug)
	pair, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(pair): %v", err)
	}
	writeStatus(t, h.Location, slug, shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Run-Shed"})

	geometry, err := hubgeom.PauseGeometry(h.Location)
	if err != nil {
		t.Fatalf("PauseGeometry: %v", err)
	}
	now := time.Now()
	if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: slug, PausedAt: now, GraceEnd: now, Caller: "EXTERNAL"})
		return nil
	}); err != nil {
		t.Fatalf("seed the pause entry: %v", err)
	}

	derived := func() runfsm.State {
		records, _, err := pairteardown.ReadRun(h.Location, slug)
		if err != nil {
			t.Fatalf("ReadRun(%s) error = %v; want none", slug, err)
		}
		return runfsm.Derive(records).State
	}
	if got := derived(); got != runfsm.Running {
		t.Fatalf("state with the caches present = %q; want %q", got, runfsm.Running)
	}

	for _, location := range []*lymcwd.Location{h.Location, pair} {
		if err := os.RemoveAll(filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName)); err != nil {
			t.Fatalf("remove the never-tracked directory of %s: %v", location.WorktreePath(), err)
		}
	}
	if got := derived(); got != runfsm.Running {
		t.Errorf("state with the never-tracked directories removed = %q; want %q", got, runfsm.Running)
	}
}
