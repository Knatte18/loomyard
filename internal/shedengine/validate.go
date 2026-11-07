// validate.go implements (*Shed).validate, the check that Run makes before it touches a lock, a file, or a producer.

package shedengine

import "fmt"

// validate checks every field Run depends on before it touches a lock, a file, or a producer.
// Each rule returns its own distinct "shedengine: "-prefixed message.
//
// An empty producer Name is rejected because OnStuck uses the empty string as its escalate-to-human sentinel.
// A producer named "" would make OnStuck ambiguous.
// A malformed or partial seed can also leave the zero value in current_producer.
// The loop would then resolve that name and run the producer, turning a corrupt status file into silent execution.
//
// A nil Producer is rejected because it panics at the call step instead of failing loud.
// A panic inside a long unattended run is worse than a validation error at second zero.
//
// The equal-lock-paths rule converts a deadlock into an error.
// internal/state acquires its lock in blocking mode,
// so a Shed whose two lock paths name one file would hang on its first persist rather than fail.
//
// OnDone naming its own producer is rejected, while OnStuck naming its own producer stays legal.
// Done routing consumes no bounce budget, so a self-referencing OnDone is a statically certain infinite loop.
// A self-referencing OnStuck is budgeted, and therefore bounded.
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
	// segmentByName maps each producer's Name to its Segment.
	// seen stays a set of bools.
	// An empty Segment is a legitimate value, so carrying Segment in seen would collide with its presence check.
	segmentByName := make(map[string]string, len(s.Producers))
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
		if p.MaxBounces < 0 {
			return fmt.Errorf("shedengine: producer %q at index %d has a negative MaxBounces", p.Name, i)
		}
		seen[p.Name] = true
		segmentByName[p.Name] = p.Segment
	}

	// OnStuck and OnDone are checked after the whole name set is collected, so a forward
	// reference to a later producer is legal for both.
	for i, p := range s.Producers {
		if p.OnDone != "" && !seen[p.OnDone] {
			return fmt.Errorf("shedengine: producer %q at index %d has OnDone %q naming no producer in the list", p.Name, i, p.OnDone)
		}
		if p.OnDone != "" && p.OnDone == p.Name {
			return fmt.Errorf("shedengine: producer %q at index %d has OnDone naming itself", p.Name, i)
		}
		if p.SkipTo != "" && !seen[p.SkipTo] {
			return fmt.Errorf("shedengine: producer %q at index %d has SkipTo %q naming no producer in the list", p.Name, i, p.SkipTo)
		}
		if p.OnReroute != "" && !seen[p.OnReroute] {
			return fmt.Errorf("shedengine: producer %q at index %d has OnReroute %q naming no producer in the list", p.Name, i, p.OnReroute)
		}
		if p.OnReroute != "" && p.OnReroute == p.Name {
			return fmt.Errorf("shedengine: producer %q at index %d has OnReroute naming itself", p.Name, i)
		}
		if p.OnStuck != "" && !seen[p.OnStuck] {
			return fmt.Errorf("shedengine: producer %q at index %d has OnStuck %q naming no producer in the list", p.Name, i, p.OnStuck)
		}
		if p.OnStuck != "" && segmentByName[p.OnStuck] != p.Segment {
			return fmt.Errorf("shedengine: producer %q at index %d has OnStuck %q naming a producer in a different Segment", p.Name, i, p.OnStuck)
		}
	}

	return nil
}
