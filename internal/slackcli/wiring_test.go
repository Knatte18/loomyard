// wiring_test.go covers the production seams of wiring.go that read a real hub on disk.

package slackcli

import (
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReceiverPaused(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	pairSlug := hubforge.TemplatePairSlug
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	prime, err := lymcwd.ResolveWorktree(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("ResolveWorktree(%q) = %v", hub.PrimeWorktree(), err)
	}
	geometry, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		t.Fatalf("PauseGeometry() = %v", err)
	}
	_, err = pauserecord.Update(geometry.RecordPath, time.Now(), func(record *pauserecord.Record) error {
		record.Live = []pauserecord.Entry{{Scope: pairSlug, PausedAt: time.Now().Add(-time.Minute)}}
		return nil
	})
	if err != nil {
		t.Fatalf("Update() = %v", err)
	}

	tests := []struct {
		name     string
		receiver string
		want     bool
	}{
		{"HubOrchReadsThePrimesPauseAndIsNotCoveredByAPairEntry", "ly:orch", false},
		{"BeamOrchReadsItsOwnWorktreesPauseAndIsCoveredByItsPairEntry", "ly:" + pairSlug + ":orch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := receiverPaused(hub.Path, prime, tt.receiver)
			if err != nil || got != tt.want {
				t.Errorf("receiverPaused(%q) = (%v, %v); want (%v, nil)", tt.receiver, got, err, tt.want)
			}
		})
	}
}
