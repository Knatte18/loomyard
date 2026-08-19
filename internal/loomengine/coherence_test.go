// coherence_test.go is the TDD driver for checkCoherence: table tests over in-memory
// shedengine.Status/Status pairs covering every rule check 4 enforces plus the fresh-start
// invariants. It is untagged (Tier 1): no spawn, no git, no filesystem I/O — checkCoherence is
// pure.

package loomengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// validFreshShed returns a valid fresh shedengine.Status baseline for testing: current_producer
// at "Preflight", running, no history.
func validFreshShed() shedengine.Status {
	return shedengine.Status{
		CurrentProducer: "Preflight",
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
		wantEmpty  bool      // when true, checkCoherence must return no failures
		wantChecks []CheckID // every CheckID that must appear in the result
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
			name:       "CurrentProducerNotPreflight",
			mutateShed: func(s shedengine.Status) shedengine.Status { s.CurrentProducer = "Discussion-Write"; return s },
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
				s.History = []shedengine.HistoryEntry{{Producer: "Preflight", Outcome: "bogus", At: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name: "NonRFC3339Timestamp",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Preflight", Outcome: shedengine.Stuck, At: "not-a-timestamp"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name: "NonUTCTimestamp",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{{Producer: "Preflight", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30+02:00"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			// The retry-deadlock regression: shedengine.Run appends a history entry before
			// persisting StateBlocked, including on the OnStuck: "" escalation path, so a Stuck
			// at row 1 (Preflight itself) leaves one Preflight-named entry behind. That entry
			// alone must not trip CheckHalfFinished, or a blocked row-1 run could never be
			// resumed.
			name: "HistoryOfOnlyPreflightPassesFreshStartCheck",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.State = shedengine.StateBlocked
				s.Error = "bounce budget exhausted"
				s.History = []shedengine.HistoryEntry{{Producer: "Preflight", Outcome: shedengine.Stuck, At: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantEmpty: true,
		},
		{
			// The other side of the same regression guard: a history entry naming a later
			// producer is the real half-finished signal.
			name: "HistoryNamingLaterProducerFailsFreshStartCheck",
			mutateShed: func(s shedengine.Status) shedengine.Status {
				s.History = []shedengine.HistoryEntry{
					{Producer: "Preflight", Outcome: shedengine.Done, At: "2026-07-17T10:01:30Z"},
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

			got := checkCoherence(shed, product)

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
