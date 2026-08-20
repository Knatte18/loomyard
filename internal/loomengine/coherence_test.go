// coherence_test.go is the TDD driver for checkCoherence: table tests over in-memory
// shedengine.Status/Status pairs covering every rule check 4 enforces plus the fresh-start
// invariants, across the two-row (generic then loom's own) world. It is untagged (Tier 1): no
// spawn, no git, no filesystem I/O -- checkCoherence is pure.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// validFreshShed returns a valid fresh shedengine.Status baseline for testing: current_producer
// at "Loom-Preflight" (the post-row-1 shape Shed itself persists once the generic row has run),
// running, no history.
func validFreshShed() shedengine.Status {
	return shedengine.Status{
		CurrentProducer: "Loom-Preflight",
		State:           shedengine.StateRunning,
	}
}

// validFreshProduct returns a valid fresh loom Status product baseline for testing.
func validFreshProduct() Status {
	return Status{
		Slug:   "loom-contracts",
		Parent: "main",
	}
}

// containsCheck reports whether failures includes at least one entry whose Check equals want.
func containsCheck(failures []Failure, want CheckID) bool {
	for _, f := range failures {
		if f.Check == want {
			return true
		}
	}
	return false
}

func TestCheckCoherence(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name       string
		mutateShed func(shedengine.Status) shedengine.Status
		mutateProd func(Status) Status
		// expectedProducer and toleratedProducers default to loom's own row when left zero:
		// "Loom-Preflight" and []string{"Preflight", "Loom-Preflight"}.
		expectedProducer   string
		toleratedProducers []string
		wantEmpty          bool      // when true, checkCoherence must return no failures
		wantChecks         []CheckID // every CheckID that must appear in the result
	}{
		{
			name:      "ValidFreshSeed",
			wantEmpty: true,
		},
		{
			name:       "EmptyMandatoryString_Slug",
			mutateProd: func(s Status) Status { s.Slug = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "EmptyMandatoryString_Parent",
			mutateProd: func(s Status) Status { s.Parent = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "CurrentProducerNotToldExpectedName",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.CurrentProducer = "Discussion-Write"; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			// current_producer equal to the told expected name passes.
			name:       "CurrentProducerEqualsToldExpectedName",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.CurrentProducer = "Loom-Preflight"; return s },
			wantEmpty:  true,
		},
		{
			// current_producer equal to the generic row's name is the previous row, not the
			// expected one, and must fail here.
			name:       "CurrentProducerEqualsPreviousRowName",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.CurrentProducer = "Preflight"; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "StateDoneIsRejected",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.State = shedengine.StateDone; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "StateInvalidIsRejected",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.State = "bogus"; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "StatePausedTolerated",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.State = shedengine.StatePaused; return s },
			wantEmpty:  true,
		},
		{
			name:       "StateBlockedTolerated",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.State = shedengine.StateBlocked; return s },
			wantEmpty:  true,
		},
		{
			name:       "StateFailedTolerated",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.State = shedengine.StateFailed; return s },
			wantEmpty:  true,
		},
		{
			name:       "NonEmptyErrorTolerated",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.Error = "bounce budget exhausted"; return s },
			wantEmpty:  true,
		},
		{
			name: "BadEnum_HistoryOutcome",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Loom-Preflight", Outcome: "bogus", At: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name: "NonRFC3339Timestamp",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Loom-Preflight", Outcome: shedengine.Stuck, At: "not-a-timestamp"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name: "NonUTCTimestamp",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Loom-Preflight", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30+02:00"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			// The retry-deadlock regression: shedengine.Run appends a history entry before
			// persisting StateBlocked, including on the OnStuck: "" escalation path, so a Stuck
			// at either the generic row or loom's own row leaves that row's own entry behind.
			// Both names are therefore in the tolerated set, and that entry alone must not trip
			// CheckHalfFinished, or a blocked run at either tolerated row could never be
			// resumed.
			name: "HistoryOfOnlyLoomPreflightPassesFreshStartCheck",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.State = shedengine.StateBlocked
				s.Error = "bounce budget exhausted"
				s.History = []shedengine.HistoryEntry{{Producer: "Loom-Preflight", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantEmpty: true,
		},
		{
			// A history containing only a "Preflight" Done entry -- the finished generic
			// row -- also passes.
			name: "HistoryOfOnlyPreflightPasses",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantEmpty: true,
		},
		{
			// A history mixing both tolerated rows passes.
			name: "HistoryMixingBothTolerated_Passes",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.State = shedengine.StateBlocked
				s.Error = "bounce budget exhausted"
				s.History = []shedengine.HistoryEntry{
					{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
					{Producer: "Loom-Preflight", Outcome: shedengine.Stuck, At: "2026-07-17T10:05:00Z"},
				}
				return s
			},
			wantEmpty: true,
		},
		{
			// The other side of the same regression guard: a history entry naming a producer
			// outside the tolerated set is the real half-finished signal.
			name: "HistoryNamingThirdProducerFailsFreshStartCheck",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{
					{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
					{Producer: "Loom-Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:03:00Z"},
					{Producer: "Discussion-Write", Outcome: shedengine.Stuck, At: "2026-07-17T10:05:00Z"},
				}
				return s
			},
			wantChecks: []CheckID{CheckHalfFinished},
		},
		{
			name:       "SetStartSha",
			mutateProd: func(s Status) Status { s.StartSha = strPtr("a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4"); return s },
			wantChecks: []CheckID{CheckHalfFinished},
		},
		{
			name:       "PauseRequestedTrue",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.PauseRequested = true; return s },
			wantChecks: []CheckID{CheckHalfFinished},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed := validFreshShed()
			if tt.mutateShed != nil {
				shed = tt.mutateShed(shed)
			}
			product := validFreshProduct()
			if tt.mutateProd != nil {
				product = tt.mutateProd(product)
			}

			expectedProducer := tt.expectedProducer
			if expectedProducer == "" {
				expectedProducer = "Loom-Preflight"
			}
			toleratedProducers := tt.toleratedProducers
			if toleratedProducers == nil {
				toleratedProducers = []string{"Preflight", "Loom-Preflight"}
			}

			got := checkCoherence(shed, product, expectedProducer, toleratedProducers)

			if tt.wantEmpty {
				if len(got) != 0 {
					t.Errorf("checkCoherence() = %+v; want empty", got)
				}
				return
			}

			for _, want := range tt.wantChecks {
				if !containsCheck(got, want) {
					t.Errorf("checkCoherence() = %+v; want to contain CheckID %q", got, want)
				}
			}
		})
	}
}
