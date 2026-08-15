// validate.go implements (*Shed).validate, the pre-loop check batch 2's Run calls before it
// acquires any lock, reads anything, or calls any producer.

package shedengine

import "fmt"

// validate checks every field Run depends on before it touches a lock, a file, or a producer.
// Every rule below returns its own distinct, "shedengine: "-prefixed message; no rule shares
// wording with another.
//
// Two of these rules look like defensive noise without an explanation, so both are documented
// here. An empty Name is rejected because the empty string is already load-bearing twice over --
// it is OnStuck's escalate-to-human sentinel, so a producer literally named "" would make an
// OnStuck: "" ambiguous, and it is the zero value a malformed or partial seed leaves in
// current_producer, which the loop's lookup would then resolve successfully and run, turning a
// corrupt status file into silent execution. A nil Producer is rejected because it panics at the
// call step rather than failing loud, and a panic inside a long unattended run is strictly worse
// than a validation error at second zero.
//
// The equal-lock-paths rule exists to convert a deadlock into an error: internal/state acquires
// its lock with the blocking form, so a Shed whose two lock paths name one file would hang on its
// first persist rather than failing.
func (s *Shed) validate() error {
	if s.StatusPath == "" {
		return fmt.Errorf("shedengine: StatusPath must not be empty")
	}
	if s.LockPath == "" {
		return fmt.Errorf("shedengine: LockPath must not be empty")
	}
	if s.StatusLockPath == "" {
		return fmt.Errorf("shedengine: StatusLockPath must not be empty")
	}
	if s.LockPath == s.StatusLockPath {
		return fmt.Errorf("shedengine: LockPath and StatusLockPath must not name the same file")
	}
	if s.MaxBounces < 0 {
		return fmt.Errorf("shedengine: MaxBounces must not be negative")
	}
	if len(s.Producers) == 0 {
		return fmt.Errorf("shedengine: Producers must not be empty")
	}

	seen := make(map[string]bool, len(s.Producers))
	for i, p := range s.Producers {
		if p.Name == "" {
			return fmt.Errorf("shedengine: producer at index %d has an empty name", i)
		}
		if p.Producer == nil {
			return fmt.Errorf("shedengine: producer %q at index %d has a nil Producer", p.Name, i)
		}
		if seen[p.Name] {
			return fmt.Errorf("shedengine: producer %q at index %d is a duplicate of an earlier entry", p.Name, i)
		}
		seen[p.Name] = true
	}

	// OnStuck is checked after the whole name set is collected, so a forward reference to a
	// later producer is legal.
	for i, p := range s.Producers {
		if p.OnStuck != "" && !seen[p.OnStuck] {
			return fmt.Errorf("shedengine: producer %q at index %d has OnStuck %q naming no producer in the list", p.Name, i, p.OnStuck)
		}
	}

	return nil
}
