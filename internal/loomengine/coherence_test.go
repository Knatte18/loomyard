// coherence_test.go is the TDD driver for checkCoherence: table tests over
// in-memory Status values covering every rule in status-schema.md's
// validation checklist plus the fresh-start invariants. It is untagged
// (Tier 1): no spawn, no git, no filesystem I/O — checkCoherence is pure.

package loomengine

import "testing"

// validFreshStatus returns a Status that satisfies every coherence rule: a
// fresh t=0 seed with all mandatory strings set, valid enum values, and every
// fresh-start invariant at its zero value. Each test case mutates a copy of
// this baseline to isolate exactly one rule violation (or none, for the
// valid-seed case).
func validFreshStatus() Status {
	return Status{
		Slug:      "loom-contracts",
		Parent:    "main",
		Phase:     "discussion",
		Stage:     "produce",
		Narration: "now: awaiting discussion input / last: — / wait: operator to run `lyx run`",
	}
}

// containsCheck reports whether failures includes at least one entry whose
// Check equals want. Tests use this rather than exact-slice comparison
// because a single mutation can legitimately trip more than one rule (e.g.
// adding a bad history entry also makes History non-empty, tripping the
// fresh-start invariant too) — the test only needs to confirm the rule under
// test fired, not enumerate every incidental side effect.
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
		mutate     func(Status) Status
		wantEmpty  bool      // when true, checkCoherence must return no failures
		wantChecks []CheckID // every CheckID that must appear in the result
	}{
		{
			name:      "ValidFreshSeed",
			mutate:    func(s Status) Status { return s },
			wantEmpty: true,
		},
		{
			name:       "EmptyMandatoryString_Slug",
			mutate:     func(s Status) Status { s.Slug = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "EmptyMandatoryString_Parent",
			mutate:     func(s Status) Status { s.Parent = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "EmptyMandatoryString_Phase",
			mutate:     func(s Status) Status { s.Phase = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "EmptyMandatoryString_Stage",
			mutate:     func(s Status) Status { s.Stage = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "EmptyMandatoryString_Narration",
			mutate:     func(s Status) Status { s.Narration = ""; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "BadEnum_Phase",
			mutate:     func(s Status) Status { s.Phase = "bogus"; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name:       "BadEnum_Stage",
			mutate:     func(s Status) Status { s.Stage = "bogus"; return s },
			wantChecks: []CheckID{CheckSeedIncoherent},
		},
		{
			name: "BadEnum_HistoryOutcome",
			mutate: func(s Status) Status {
				s.History = []HistoryEntry{{Phase: "discussion", Outcome: "bogus", Ts: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent, CheckHalfFinished},
		},
		{
			name: "BouncedToWithoutStuck",
			mutate: func(s Status) Status {
				s.History = []HistoryEntry{{Phase: "plan", Outcome: "approved", BouncedTo: strPtr("discussion"), Ts: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent, CheckHalfFinished},
		},
		{
			name: "NonRFC3339Timestamp",
			mutate: func(s Status) Status {
				s.History = []HistoryEntry{{Phase: "discussion", Outcome: "approved", Ts: "not-a-timestamp"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent, CheckHalfFinished},
		},
		{
			name: "NonUTCTimestamp",
			mutate: func(s Status) Status {
				s.History = []HistoryEntry{{Phase: "discussion", Outcome: "approved", Ts: "2026-07-17T10:01:30+02:00"}}
				return s
			},
			wantChecks: []CheckID{CheckSeedIncoherent, CheckHalfFinished},
		},
		{
			name: "NonEmptyHistory",
			mutate: func(s Status) Status {
				s.History = []HistoryEntry{{Phase: "discussion", Outcome: "approved", Ts: "2026-07-17T10:01:30Z"}}
				return s
			},
			wantChecks: []CheckID{CheckHalfFinished},
		},
		{
			name: "SetStartSha",
			mutate: func(s Status) Status {
				s.StartSha = strPtr("a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4")
				return s
			},
			wantChecks: []CheckID{CheckHalfFinished},
		},
		{
			name: "SetNextAction",
			mutate: func(s Status) Status {
				s.NextAction = strPtr("operator: review plan")
				return s
			},
			wantChecks: []CheckID{CheckHalfFinished},
		},
		{
			name: "PauseRequestedTrue",
			mutate: func(s Status) Status {
				s.PauseRequested = true
				return s
			},
			wantChecks: []CheckID{CheckHalfFinished},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkCoherence(tt.mutate(validFreshStatus()))

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
