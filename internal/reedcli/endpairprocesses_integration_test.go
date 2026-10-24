//go:build tier1

// endpairprocesses_integration_test.go pins endPairProcesses over a hub fixture: a pair reads the pause and ends what is registered with no network call, and the prime ends nothing and reads no pair.

package reedcli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// TestEndPairProcesses installs a pause gate over the hub's record holding a live hub entry, whose Sleep fails the test, so any network call would be held and fail it.
// It installs a process-global gate, so it is not parallel.
func TestEndPairProcesses(t *testing.T) {
	gate := func(t *testing.T, recordPath string) {
		t.Helper()
		now := time.Now()
		if _, err := pauserecord.Update(recordPath, now, func(r *pauserecord.Record) error {
			r.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: now}}
			return nil
		}); err != nil {
			t.Fatalf("write the hub pause: %v", err)
		}
		t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
			Geometry: pauserecord.Geometry{RecordPath: recordPath},
			Notify:   func(string) {},
			Sleep: func(context.Context, time.Duration) error {
				t.Error("endPairProcesses made a network call, which the pause held")
				return errors.New("held")
			},
		}))
	}

	t.Run("a pair reads the pause, ends nothing registered and makes no network call", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, "pair")
		pair, err := lyxcwd.ResolveWorktree(h.PairCodeWorktree("pair"))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		geometry, err := hubgeom.PauseGeometry(pair)
		if err != nil {
			t.Fatalf("PauseGeometry: %v", err)
		}
		gate(t, geometry.RecordPath)

		c := &reedCLI{anchorPath: pair.AnchorPath()}
		ended, err := c.endPairProcesses(context.Background())
		if err != nil {
			t.Fatalf("endPairProcesses() error = %v, want none", err)
		}
		if len(ended) != 0 {
			t.Errorf("endPairProcesses() = %v, want an empty list", ended)
		}
	})

	t.Run("the prime ends nothing and reads no pair", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		prime, err := lyxcwd.ResolveWorktree(h.PrimeWorktree())
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		geometry, err := hubgeom.PauseGeometry(prime)
		if err != nil {
			t.Fatalf("PauseGeometry: %v", err)
		}
		gate(t, geometry.RecordPath)

		c := &reedCLI{anchorPath: prime.AnchorPath()}
		ended, err := c.endPairProcesses(context.Background())
		if err != nil {
			t.Fatalf("endPairProcesses() error = %v, want none", err)
		}
		if len(ended) != 0 {
			t.Errorf("endPairProcesses() = %v, want an empty list", ended)
		}
	})
}
