// runid.go declares the run-id vocabulary shedrun owns: the default literal "self", single-segment
// validation for any run-id joined onto a path, and the reservation check consulted only where a
// run-id derives from a Board slug.

package shedrun

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SelfRunID is the literal default run-id, meaning "this worktree's own primary run".
// It is always a legal address: ValidateRunID accepts it and IsReserved is the only check that
// treats it specially, and only at slug-derivation sites.
const SelfRunID = "self"

// ValidateRunID reports whether runID is a legal single path segment to join onto a run-directory
// anchor.
// It refuses an empty string, a string containing a forward or backward slash, the exact strings "."
// and "..", and anything filepath.Base does not return unchanged -- the general test for "this string
// is not itself a single clean path segment".
// It accepts SelfRunID: the default run-id must stay addressable by this check, since every path
// constructor in this package is meant to be called only after ValidateRunID has passed.
func ValidateRunID(runID string) error {
	if runID == "" {
		return fmt.Errorf("shedrun: run-id must not be empty")
	}
	if strings.ContainsAny(runID, "/\\") {
		return fmt.Errorf("shedrun: run-id %q must not contain a path separator", runID)
	}
	if runID == "." || runID == ".." {
		return fmt.Errorf("shedrun: run-id %q is not a valid single path segment", runID)
	}
	if filepath.Base(runID) != runID {
		return fmt.Errorf("shedrun: run-id %q is not a valid single path segment", runID)
	}
	return nil
}

// IsReserved reports whether runID collides with a meaning shedrun itself already assigns, rather
// than being a legal choice for a Board slug to seed a run under.
// It is consulted only where a run-id derives from a Board slug -- a task slugged "self" would
// collide with SelfRunID's default meaning -- and never on an addressing path, where SelfRunID is
// always legal: use ValidateRunID there instead.
func IsReserved(runID string) bool {
	return runID == SelfRunID
}
