// buildidentity.go decides whether the running lyx build differs from the one a step's record names.

package shedverbs

import "github.com/Knatte18/loomyard/internal/buildvcs"

// binaryChanged reports whether running is a different known build than recorded: both revisions must be non-empty, and the pairs must differ in either field.
func binaryChanged(running, recorded buildvcs.Identity) bool {
	if running.Revision == "" || recorded.Revision == "" {
		return false
	}
	return running != recorded
}
