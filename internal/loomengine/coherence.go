// coherence.go implements the pure, in-memory validator that checks a decoded
// Status against docs/reference/status-schema.md's validation checklist plus
// the fresh-start invariants Preflight enforces before a task is fit to run.
// It performs no I/O and spawns nothing, so it is exhaustively table-tested
// in Tier 1 (coherence_test.go).

package loomengine

import (
	"fmt"
	"time"
)

// validPhases is the closed set of legal Status.Phase values, per
// status-schema.md.
var validPhases = map[string]bool{
	"preflight":  true,
	"discussion": true,
	"plan":       true,
	"builder":    true,
	"raddle":     true,
	"finalize":   true,
	"done":       true,
}

// validStages is the closed set of legal Status.Stage values, per
// status-schema.md.
var validStages = map[string]bool{
	"produce": true,
	"gate":    true,
}

// validOutcomes is the closed set of legal HistoryEntry.Outcome values, per
// status-schema.md.
var validOutcomes = map[string]bool{
	"approved": true,
	"stuck":    true,
}

// checkCoherence validates a decoded Status against status-schema.md's
// validation checklist plus Preflight's fresh-start invariants. It is pure
// (no I/O, no git) so it can be exhaustively table-tested in isolation.
//
// It never short-circuits: every violated rule is collected into the
// returned slice, so a caller sees every reason a seed is invalid in one
// pass rather than fixing them one at a time. Returns nil when s is a valid
// fresh seed.
func checkCoherence(s Status) []Failure {
	var failures []Failure

	// Mandatory strings: strict decode zero-fills an absent JSON field to "",
	// so an empty string here is the only signal a strict decode gives us that
	// the field was missing or explicitly blank in the source — either way,
	// status-schema.md's "required fields present" rule is violated.
	mandatory := []struct {
		name  string
		value string
	}{
		{"slug", s.Slug},
		{"parent", s.Parent},
		{"phase", s.Phase},
		{"stage", s.Stage},
		{"narration", s.Narration},
	}
	for _, m := range mandatory {
		if m.value == "" {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("mandatory field %q is empty or absent", m.name),
			})
		}
	}

	// Enum validity. An empty Phase/Stage was already reported above by the
	// mandatory-string check; skip it here so the same root cause is not
	// reported twice as both "missing" and "not a valid enum member".
	if s.Phase != "" && !validPhases[s.Phase] {
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("phase %q is not a valid phase", s.Phase),
		})
	}
	if s.Stage != "" && !validStages[s.Stage] {
		failures = append(failures, Failure{
			Check:  CheckSeedIncoherent,
			Reason: fmt.Sprintf("stage %q is not a valid stage", s.Stage),
		})
	}

	// Per-history-entry checks: outcome enum, bounced_to gating, and timestamp
	// format. Collected per entry so a multi-entry history with several bad
	// entries surfaces every one, not just the first.
	for i, h := range s.History {
		if !validOutcomes[h.Outcome] {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("history[%d].outcome %q is not a valid outcome", i, h.Outcome),
			})
		}
		if h.BouncedTo != nil && h.Outcome != "stuck" {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("history[%d].bounced_to is set but outcome is %q, not \"stuck\"", i, h.Outcome),
			})
		}
		if !isRFC3339UTC(h.Ts) {
			failures = append(failures, Failure{
				Check:  CheckSeedIncoherent,
				Reason: fmt.Sprintf("history[%d].ts %q is not RFC3339 UTC", i, h.Ts),
			})
		}
	}

	// Fresh-start invariants: Preflight is a stateless validator for a task at
	// its t=0 seed, not a general-purpose status validator at any point in a
	// task's life — any sign the task has already advanced (a non-empty
	// history, a stamped start_sha, a set next_action, or a pending pause)
	// means Preflight was invoked too late, and that is reported as
	// half-finished rather than silently accepted.
	if len(s.History) != 0 || s.StartSha != nil || s.NextAction != nil || s.PauseRequested {
		failures = append(failures, Failure{
			Check:  CheckHalfFinished,
			Reason: "status.json is not a fresh seed: history, start_sha, next_action, or pause_requested is already set",
		})
	}

	return failures
}

// isRFC3339UTC reports whether ts parses as RFC3339 with a zero UTC offset.
// time.Parse(time.RFC3339, ts) alone accepts any numeric offset (e.g.
// "+02:00"); status-schema.md pins timestamps to UTC specifically, so this
// additionally requires the parsed offset to be exactly zero seconds — which
// accepts both the "Z" literal and an explicit "+00:00"/"-00:00" offset,
// while rejecting any non-zero offset.
func isRFC3339UTC(ts string) bool {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	_, offset := t.Zone()
	return offset == 0
}
