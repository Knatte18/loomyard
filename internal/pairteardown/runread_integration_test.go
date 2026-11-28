//go:build !windows

// runread_integration_test.go pins that ReadRun reads a run's state from tracked records alone, on a real hub pair.

package pairteardown_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

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
